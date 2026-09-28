// Package migrate（本文件）实现模型定义与数据库实际结构的比对校验，
// 在临时库上执行迁移后逐表逐列检查差异。
package migrate

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"sync"

	"github.com/pterm/pterm"
	"gorm.io/gorm/schema"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

var (
	postgresAliases = map[string]string{
		"character varying":           "varchar",
		"character":                   "char",
		"integer":                     "int",
		"int4":                        "int",
		"serial":                      "int",
		"bigint":                      "bigint",
		"int8":                        "bigint",
		"bigserial":                   "bigint",
		"smallint":                    "smallint",
		"int2":                        "smallint",
		"double precision":            "float",
		"float8":                      "float",
		"real":                        "float",
		"float4":                      "float",
		"timestamp with time zone":    "timestamptz",
		"timestamp without time zone": "timestamp",
		"time with time zone":         "timetz",
		"time without time zone":      "time",
		"boolean":                     "bool",
		"bytea":                       "bytes",
		"text":                        "text",
		"uuid":                        "uuid",
		"numeric":                     "numeric",
		"decimal":                     "numeric",
		"json":                        "json",
		"jsonb":                       "jsonb",
		"array":                       "array",
		"user-defined":                "text",
	}
	clickhouseAliases = map[string]string{
		"int8":    "int8",
		"int16":   "int16",
		"int32":   "int32",
		"int64":   "int64",
		"int128":  "int128",
		"int256":  "int256",
		"uint8":   "uint8",
		"uint16":  "uint16",
		"uint32":  "uint32",
		"uint64":  "uint64",
		"uint128": "uint128",
		"uint256": "uint256",
		"float32": "float",
		"float64": "double",
		"string":  "string",
		"bool":    "bool",
		"boolean": "bool",
	}
)

// IssueKind 表示结构比对问题的种类。
type IssueKind int

// 结构比对问题种类：类型不匹配、缺失表、多余表、缺失列、多余列。
const (
	// IssueKindMismatch 列类型与模型声明不匹配。
	IssueKindMismatch IssueKind = iota
	// IssueKindMissingTable 模型声明了但数据库缺失的表。
	IssueKindMissingTable
	// IssueKindExtraTable 数据库存在但模型未声明的表。
	IssueKindExtraTable
	// IssueKindMissingColumn 模型声明了但数据库缺失的列。
	IssueKindMissingColumn
	// IssueKindExtraColumn 数据库存在但模型未声明的列。
	IssueKindExtraColumn
)

// Label 返回问题种类对应的本地化名称。
func (k IssueKind) Label() string {
	switch k {
	case IssueKindMissingTable:
		return translate.T("cli.migrate.issue-missing-table", nil)
	case IssueKindExtraTable:
		return translate.T("cli.migrate.issue-extra-table", nil)
	case IssueKindMissingColumn:
		return translate.T("cli.migrate.issue-missing-column", nil)
	case IssueKindExtraColumn:
		return translate.T("cli.migrate.issue-extra-column", nil)
	default:
		return translate.T("cli.migrate.issue-mismatch", nil)
	}
}

// CheckIssue 描述一条结构和模型比对发现的问题。
type CheckIssue struct {
	// Table 问题所属表名。
	Table string
	// Column 问题所属列名，整表级问题为空。
	Column string
	// Kind 问题种类。
	Kind IssueKind
	// ExpectedType 模型声明的期望类型。
	ExpectedType string
	// ActualType 数据库实际类型。
	ActualType string
}

// Check 在临时数据库上执行全部迁移，然后比对模型声明与实际结构。
func Check(ctx context.Context, cfg *Config, models []any) error {
	pterm.Info.Println(translate.T("cli.migrate.check-creating", nil))
	tempDB, err := CreateTempDB(ctx, cfg)
	if err != nil {
		return errors.Newf("failed to create temp database: %v", err)
	}

	tempCfg := &Config{
		Driver:        cfg.Driver,
		DSN:           tempDB.dsn,
		MigrationsFS:  cfg.MigrationsFS,
		MigrationsDir: cfg.MigrationsDir,
		Timeout:       cfg.Timeout,
		Verbose:       cfg.Verbose,
		KeepTemp:      cfg.KeepTemp,
	}

	migrator, err := NewMigratorFromDB(tempCfg, tempDB.db)
	if err != nil {
		tempDB.Close(ctx)
		return errors.Newf("failed to create migrator for temp db: %v", err)
	}

	pterm.Info.Println(translate.T("cli.migrate.check-running", map[string]any{"DB": tempDB.name}))
	if err = migrator.Up(ctx); err != nil {
		if !cfg.KeepTemp {
			tempDB.Close(ctx)
		}
		return errors.Newf("migration failed: %v", err)
	}

	pterm.Info.Println(translate.T("cli.migrate.check-comparing", nil))
	err = CheckModels(ctx, tempDB.db, cfg.Driver, models)

	if !cfg.KeepTemp {
		pterm.Info.Println(translate.T("cli.migrate.check-cleanup", map[string]any{"DB": tempDB.name}))
		if closeErr := tempDB.Close(ctx); closeErr != nil {
			pterm.Warning.Println(translate.T("cli.migrate.check-drop-failed", map[string]any{"DB": tempDB.name, "Err": closeErr}))
		}
	} else {
		pterm.Info.Println(translate.T("cli.migrate.check-preserved", map[string]any{"DB": tempDB.name}))
	}

	return err
}

// CheckModels 比对给定数据库结构与模型声明，输出差异表格。
func CheckModels(ctx context.Context, db *sql.DB, driver string, models []any) error {
	actualTables, err := getTables(ctx, db, driver)
	if err != nil {
		return errors.Newf("failed to read actual database schema: %v", err)
	}

	expectedTables, err := getExpectedSchema(models)
	if err != nil {
		return errors.Newf("failed to parse model schema: %v", err)
	}

	var issues []CheckIssue
	for tableName, expectedCols := range expectedTables {
		actualTable, exists := actualTables[tableName]
		if !exists {
			issues = append(issues, CheckIssue{
				Table:  tableName,
				Column: "(entire table)",
				Kind:   IssueKindMissingTable,
			})
			continue
		}
		issues = append(issues, checkTableColumns(driver, tableName, expectedCols, actualTable)...)
	}

	issues = append(issues, checkExtraTables(expectedTables, actualTables)...)

	if len(issues) == 0 {
		pterm.Success.Println(translate.T("cli.migrate.check-ok", nil))
		return nil
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Table != issues[j].Table {
			return issues[i].Table < issues[j].Table
		}
		return issues[i].Column < issues[j].Column
	})

	renderIssuesTable(issues)
	return nil
}

func checkTableColumns(driver, tableName string, expectedCols map[string]string, actualTable TableInfo) []CheckIssue {
	var issues []CheckIssue

	actualCols := make(map[string]ColumnInfo)
	for _, c := range actualTable.Columns {
		actualCols[strings.ToLower(c.Name)] = c
	}

	for colName, expectedType := range expectedCols {
		actualCol, exists := actualCols[strings.ToLower(colName)]
		if !exists {
			issues = append(issues, CheckIssue{
				Table:        tableName,
				Column:       colName,
				Kind:         IssueKindMissingColumn,
				ExpectedType: expectedType,
			})
			continue
		}

		if !typeMatches(driver, expectedType, actualCol.DataType) {
			issues = append(issues, CheckIssue{
				Table:        tableName,
				Column:       colName,
				Kind:         IssueKindMismatch,
				ExpectedType: expectedType,
				ActualType:   actualCol.DataType,
			})
		}
	}

	expectedColNames := make(map[string]bool)
	for colName := range expectedCols {
		expectedColNames[strings.ToLower(colName)] = true
	}
	for _, col := range actualTable.Columns {
		if !expectedColNames[strings.ToLower(col.Name)] {
			if isAutoColumn(col.Name) {
				continue
			}
			issues = append(issues, CheckIssue{
				Table:      tableName,
				Column:     col.Name,
				Kind:       IssueKindExtraColumn,
				ActualType: col.DataType,
			})
		}
	}

	return issues
}

func checkExtraTables(expectedTables map[string]map[string]string, actualTables map[string]TableInfo) []CheckIssue {
	var issues []CheckIssue

	modelTableSet := make(map[string]bool)
	for t := range expectedTables {
		modelTableSet[strings.ToLower(t)] = true
	}

	for tableName := range actualTables {
		if strings.HasPrefix(tableName, "goose_db_version") {
			continue
		}
		if !modelTableSet[strings.ToLower(tableName)] {
			issues = append(issues, CheckIssue{
				Table:  tableName,
				Column: "(entire table)",
				Kind:   IssueKindExtraTable,
			})
		}
	}

	return issues
}

func isAutoColumn(colName string) bool {
	return colName == "id" || colName == "created_at" || colName == "updated_at" || colName == "deleted_at"
}

func (issue CheckIssue) expectedDisplay() string {
	switch issue.Kind {
	case IssueKindMismatch, IssueKindMissingColumn:
		return issue.ExpectedType
	default:
		return "-"
	}
}

func (issue CheckIssue) actualDisplay() string {
	switch issue.Kind {
	case IssueKindMismatch, IssueKindExtraColumn:
		return issue.ActualType
	default:
		return "-"
	}
}

func renderIssuesTable(issues []CheckIssue) {
	data := pterm.TableData{
		{
			translate.T("cli.migrate.col-issue", nil),
			translate.T("cli.migrate.col-table", nil),
			translate.T("cli.migrate.col-column", nil),
			translate.T("cli.migrate.col-expected-type", nil),
			translate.T("cli.migrate.col-actual-type", nil),
		},
	}

	for _, issue := range issues {
		data = append(data, []string{
			issue.Kind.Label(), issue.Table, issue.Column, issue.expectedDisplay(), issue.actualDisplay(),
		})
	}

	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		pterm.Error.Println(translate.T("cli.migrate.issues-table-error", map[string]any{"Err": err}))
	}
}

func getExpectedSchema(models []any) (map[string]map[string]string, error) {
	result := make(map[string]map[string]string)
	cache := &sync.Map{}
	ns := schema.NamingStrategy{}

	for _, model := range models {
		s, err := schema.Parse(model, cache, ns)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		cols := make(map[string]string)
		for _, f := range s.Fields {
			if f.DBName == "" {
				continue
			}
			if f.PrimaryKey && (f.DBName == "id" || strings.EqualFold(f.DBName, "id")) {
				continue
			}
			if f.DBName == "created_at" || f.DBName == "updated_at" || f.DBName == "deleted_at" {
				continue
			}
			dataType := string(f.DataType)
			if tagType, ok := f.TagSettings["TYPE"]; ok && tagType != "" {
				dataType = tagType
			}
			cols[f.DBName] = dataType
		}

		if len(cols) > 0 {
			result[s.Table] = cols
		}
	}

	return result, nil
}

func typeMatches(driver, expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)

	if strings.EqualFold(expected, actual) {
		return true
	}

	en := normalizeDBType(driver, expected)
	an := normalizeDBType(driver, actual)

	if en == an {
		return true
	}

	return compatibleDBTypes(driver, en, an)
}

func normalizeDBType(driver, typ string) string {
	typ = strings.TrimSpace(typ)
	lower := strings.ToLower(typ)

	switch driver {
	case "postgres":
		lower = stripSize(lower)
		if normalized, ok := postgresAliases[lower]; ok {
			return normalized
		}

	case "clickhouse":
		lower = strings.ReplaceAll(lower, " ", "")
		lower = stripCODEC(lower)

		if strings.HasPrefix(lower, "nullable(") {
			lower = lower[len("nullable(") : len(lower)-1]
		}
		if normalized, ok := clickhouseAliases[lower]; ok {
			return normalized
		}

		if strings.HasPrefix(lower, "fixedstring") {
			return "fixedstring"
		}
		if strings.HasPrefix(lower, "lowcardinality") {
			return normalizeLowCardinality(lower)
		}
		if strings.HasPrefix(lower, "datetime64") {
			return lower
		}
		if strings.HasPrefix(lower, "datetime") {
			return "datetime"
		}
		if strings.HasPrefix(lower, "array") {
			return lower
		}
	}

	return lower
}

func stripSize(typ string) string {
	if idx := strings.Index(typ, "("); idx >= 0 {
		closeIdx := strings.Index(typ[idx:], ")")
		if closeIdx >= 0 {
			return typ[:idx] + typ[idx+closeIdx+1:]
		}
		return typ[:idx]
	}
	return typ
}

func stripCODEC(typ string) string {
	if idx := strings.Index(strings.ToLower(typ), "codec"); idx >= 0 {
		return strings.TrimSpace(typ[:idx])
	}
	return typ
}

func normalizeLowCardinality(typ string) string {
	content := strings.TrimPrefix(strings.ToLower(typ), "lowcardinality(")
	content = strings.TrimSuffix(content, ")")
	return "lowcardinality(" + normalizeDBType("clickhouse", content) + ")"
}

func compatibleDBTypes(driver, expected, actual string) bool {
	switch driver {
	case "postgres":
		if expected == "varchar" && actual == "text" {
			return true
		}
		if expected == "text" && actual == "varchar" {
			return true
		}
		if (expected == "int" || expected == "bigint" || expected == "smallint") &&
			(actual == "int" || actual == "bigint" || actual == "smallint") {
			return true
		}
	}
	return false
}
