package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/pterm/pterm"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

type ColumnInfo struct {
	Name     string
	DataType string
	Nullable string
	Default  string
}

type TableInfo struct {
	Name    string
	Columns []ColumnInfo
}

type DiffResult struct {
	NewTables      []TableInfo
	DroppedTables  []TableInfo
	ModifiedTables []TableDiff
	Identical      bool
}

type ColumnDiff struct {
	ColumnName string
	OldType    string
	NewType    string
	Status     string
}

type TableDiff struct {
	TableName       string
	NewColumns      []ColumnInfo
	DroppedColumns  []ColumnInfo
	ModifiedColumns []ColumnDiff
}

type tableReader struct {
	listTablesQuery  string
	listColumnsQuery string
	scanColumn       func(*sql.Rows) (ColumnInfo, error)
	filterTable      func(string) bool
}

func Diff(ctx context.Context, currentCfg, tempCfg *Config) error {
	var (
		tempDB *TempDB
		err    error
	)

	if tempCfg != nil {
		tempDB, err = CreateTempDB(ctx, tempCfg)
		if err != nil {
			return errors.Newf("failed to create temp database: %v", err)
		}
		defer func() {
			if !tempCfg.KeepTemp {
				tempDB.Close(ctx)
			}
		}()

		if migrator, err := NewMigratorFromDB(tempCfg, tempDB.db); err != nil {
			return errors.Newf("failed to create migrator for temp db: %v", err)
		} else if err = migrator.Up(ctx); err != nil {
			return errors.Newf("migration on temp db failed: %v", err)
		}
	}

	currentMigrator, err := NewMigrator(currentCfg)
	if err != nil {
		return errors.Newf("failed to connect to current database: %v", err)
	}
	defer currentMigrator.Close()

	currentTables, err := getTables(ctx, currentMigrator.db, currentCfg.Driver)
	if err != nil {
		return errors.Newf("failed to get current tables: %v", err)
	}

	var targetTables map[string]TableInfo
	if tempDB != nil {
		targetTables, err = getTables(ctx, tempDB.db, tempCfg.Driver)
	} else {
		targetTables, err = getTables(ctx, currentMigrator.db, currentCfg.Driver)
	}
	if err != nil {
		return errors.Newf("failed to get target tables: %v", err)
	}

	result := compareTableSets(currentTables, targetTables)
	if result.Identical {
		pterm.Success.Println("Database schema is up to date, no changes pending")
		return nil
	}

	data := pterm.TableData{
		{"Change", "Table", "Column", "Detail"},
	}

	for _, t := range result.NewTables {
		for _, col := range t.Columns {
			data = append(data, []string{"New Table", t.Name, col.Name, fmt.Sprintf("%s %s", col.DataType, col.Nullable)})
		}
	}
	for _, t := range result.DroppedTables {
		for _, col := range t.Columns {
			data = append(data, []string{"Dropped Table", t.Name, col.Name, col.DataType})
		}
	}
	for _, t := range result.ModifiedTables {
		for _, col := range t.NewColumns {
			data = append(data, []string{"New Column", t.TableName, col.Name, fmt.Sprintf("%s %s", col.DataType, col.Nullable)})
		}
		for _, col := range t.DroppedColumns {
			data = append(data, []string{"Dropped Column", t.TableName, col.Name, col.DataType})
		}
		for _, col := range t.ModifiedColumns {
			detail := fmt.Sprintf("%s → %s", col.OldType, col.NewType)
			data = append(data, []string{"Modified", t.TableName, col.ColumnName, detail})
		}
	}

	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getTables(ctx context.Context, db *sql.DB, driver string) (map[string]TableInfo, error) {
	switch driver {
	case "postgres":
		return readTables(ctx, db, tableReader{
			listTablesQuery: `SELECT table_name
FROM information_schema.tables
WHERE table_schema = 'public'
AND table_type = 'BASE TABLE'
ORDER BY table_name`,
			listColumnsQuery: `SELECT column_name, data_type, is_nullable,
       COALESCE(column_default, '')
FROM information_schema.columns
WHERE table_schema = 'public' AND table_name = $1
ORDER BY ordinal_position`,
			scanColumn: func(rows *sql.Rows) (ColumnInfo, error) {
				var col ColumnInfo
				err := rows.Scan(&col.Name, &col.DataType, &col.Nullable, &col.Default)
				return col, err
			},
		})
	case "clickhouse":
		return readTables(ctx, db, tableReader{
			listTablesQuery: `SELECT name
FROM system.tables
WHERE database = currentDatabase()
ORDER BY name`,
			listColumnsQuery: `SELECT name, type, ''
FROM system.columns
WHERE database = currentDatabase() AND table = ?
ORDER BY position`,
			scanColumn: func(rows *sql.Rows) (ColumnInfo, error) {
				var col ColumnInfo
				err := rows.Scan(&col.Name, &col.DataType, &col.Nullable)
				return col, err
			},
			filterTable: func(name string) bool {
				if strings.HasPrefix(name, ".inner") {
					return false
				}
				if strings.HasSuffix(name, "_mv") {
					return false
				}
				return true
			},
		})
	default:
		return nil, errors.Newf("unsupported driver for diff: %s", driver)
	}
}

func readTables(ctx context.Context, db *sql.DB, r tableReader) (map[string]TableInfo, error) {
	rows, err := db.QueryContext(ctx, r.listTablesQuery)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tableNames []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if r.filterTable != nil && !r.filterTable(name) {
			continue
		}
		tableNames = append(tableNames, name)
	}

	tables := make(map[string]TableInfo)
	for _, name := range tableNames {
		colRows, err := db.QueryContext(ctx, r.listColumnsQuery, name)
		if err != nil {
			return nil, err
		}

		var cols []ColumnInfo
		for colRows.Next() {
			col, err := r.scanColumn(colRows)
			if err != nil {
				colRows.Close()
				return nil, err
			}
			cols = append(cols, col)
		}
		colRows.Close()

		tables[name] = TableInfo{Name: name, Columns: cols}
	}

	return tables, nil
}

func compareTableSets(current, target map[string]TableInfo) *DiffResult {
	result := &DiffResult{Identical: true}

	for name, info := range target {
		if _, ok := current[name]; !ok {
			result.NewTables = append(result.NewTables, info)
			result.Identical = false
		}
	}

	for name, info := range current {
		if _, ok := target[name]; !ok {
			result.DroppedTables = append(result.DroppedTables, info)
			result.Identical = false
		}
	}

	for name, targetInfo := range target {
		currentInfo, ok := current[name]
		if !ok {
			continue
		}

		diff := compareTableColumns(currentInfo, targetInfo)
		if diff != nil {
			result.ModifiedTables = append(result.ModifiedTables, *diff)
			result.Identical = false
		}
	}

	sort.Slice(result.NewTables, func(i, j int) bool { return result.NewTables[i].Name < result.NewTables[j].Name })
	sort.Slice(result.DroppedTables, func(i, j int) bool { return result.DroppedTables[i].Name < result.DroppedTables[j].Name })
	sort.Slice(result.ModifiedTables, func(i, j int) bool { return result.ModifiedTables[i].TableName < result.ModifiedTables[j].TableName })

	return result
}

func compareTableColumns(current, target TableInfo) *TableDiff {
	diff := &TableDiff{TableName: current.Name}

	currentCols := make(map[string]ColumnInfo)
	for _, c := range current.Columns {
		currentCols[c.Name] = c
	}
	targetCols := make(map[string]ColumnInfo)
	for _, c := range target.Columns {
		targetCols[c.Name] = c
	}

	for name, info := range targetCols {
		if _, ok := currentCols[name]; !ok {
			diff.NewColumns = append(diff.NewColumns, info)
		}
	}

	for name, info := range currentCols {
		if _, ok := targetCols[name]; !ok {
			diff.DroppedColumns = append(diff.DroppedColumns, info)
		}
	}

	for name, targetInfo := range targetCols {
		currentInfo, ok := currentCols[name]
		if !ok {
			continue
		}
		if targetInfo.DataType != currentInfo.DataType {
			diff.ModifiedColumns = append(diff.ModifiedColumns, ColumnDiff{
				ColumnName: name,
				OldType:    currentInfo.DataType,
				NewType:    targetInfo.DataType,
				Status:     "modified",
			})
		}
	}

	if len(diff.NewColumns) == 0 && len(diff.DroppedColumns) == 0 && len(diff.ModifiedColumns) == 0 {
		return nil
	}
	return diff
}
