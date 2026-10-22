package docs

import (
	// 标准库
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	// 第三方
	"gorm.io/gorm/schema"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// migratedTablePattern 从迁移脚本中提取 CREATE TABLE 的表名。
var migratedTablePattern = regexp.MustCompile(`(?im)^\s*CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+[` + "`" + `"]?([A-Za-z0-9_]+)[` + "`" + `"]?\s*\(`)

// schemaField 是数据表页面中的一行字段。
type schemaField struct {
	// Column 是数据库列名。
	Column string
	// GoType 是 Go 类型或显式数据库类型。
	GoType string
	// Nullable 是可空性说明。
	Nullable string
	// Default 是默认值。
	Default string
	// Comment 是字段说明。
	Comment string
	// Primary 表示主键。
	Primary bool
	// Unique 表示唯一索引。
	Unique bool
	// Index 表示普通索引。
	Index bool
	// Foreign 是外键声明。
	Foreign string
}

// schemaModel 是一张已迁移数据表的文档模型。
type schemaModel struct {
	// Domain 是模型所属域（目录名）。
	Domain string
	// driver 是该表所属数据库驱动（postgres / clickhouse），用于判定可空性。
	driver string
	// Name 是 Go 结构体名。
	Name string
	// Table 是数据库表名。
	Table string
	// Doc 是结构体用途说明。
	Doc string
	// Fields 是展开后的字段列表。
	Fields []schemaField
}

// schemaReferenceProjections 生成数据表参考投影：按域分组的索引页，以及每张表一页。
// 只导出在迁移脚本中出现过的表，避免把值对象或内嵌结构体误当成表。
func schemaReferenceProjections(projectRoot string) ([]Projection, error) {
	tables, err := migratedTables(projectRoot)
	if err != nil {
		return nil, err
	}
	models, err := loadSchemaModels(projectRoot, tables)
	if err != nil {
		return nil, err
	}

	projections := make([]Projection, 0, len(models)+1)
	for _, model := range models {
		output := path.Join(GeneratedRoot, "reference", "schema", model.Domain, model.Table+".md")
		projections = append(projections, Projection{
			Title:       "数据表：" + model.Table,
			Summary:     schemaSummary(model.Doc),
			Roles:       []Role{RoleDev, RoleOps},
			Scene:       "需要理解数据存储结构、字段含义或排查数据问题时阅读。",
			Output:      output,
			SourceLabel: "internal/models/" + model.Domain,
			Content:     renderSchemaPage(model),
		})
	}

	indexOutput := path.Join(GeneratedRoot, "reference", "schema", "index.md")
	projections = append(projections, Projection{
		Title:       "数据表参考索引",
		Summary:     "按域列出全部已迁移数据表，每张表一页。",
		Roles:       []Role{RoleDev, RoleOps},
		Scene:       "需要查找某张表、确认表归属域或浏览数据结构时阅读。",
		Output:      indexOutput,
		SourceLabel: "internal/models",
		Content:     renderSchemaIndex(models),
	})
	return projections, nil
}

// migratedTables 读取 core 与 raw 迁移脚本，返回「表名 → 驱动」映射。
func migratedTables(projectRoot string) (map[string]string, error) {
	tables := make(map[string]string)
	sources := []struct {
		dir    string
		driver string
	}{
		{dir: filepath.Join(projectRoot, "internal", "migrations", "core"), driver: "postgres"},
		{dir: filepath.Join(projectRoot, "internal", "migrations", "raw"), driver: "clickhouse"},
	}
	for _, source := range sources {
		files, err := filepath.Glob(filepath.Join(source.dir, "*.sql"))
		if err != nil {
			return nil, errors.WithStack(err)
		}
		sort.Strings(files)
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, errors.WithStack(err)
			}
			for _, match := range migratedTablePattern.FindAllStringSubmatch(string(data), -1) {
				tables[match[1]] = source.driver
			}
		}
	}
	return tables, nil
}

// loadSchemaModels 解析 internal/models 下全部域包，返回已迁移表的模型列表。
func loadSchemaModels(projectRoot string, tables map[string]string) ([]schemaModel, error) {
	modelsRoot := filepath.Join(projectRoot, "internal", "models")
	entries, err := os.ReadDir(modelsRoot)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	naming := schema.NamingStrategy{}
	var models []schemaModel
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		domain := entry.Name()
		dir := filepath.Join(modelsRoot, domain)
		parsed, err := parseSchemaPackage(dir, tables)
		if err != nil {
			return nil, err
		}
		for _, item := range parsed {
			table := naming.TableName(item.Name)
			driver, ok := tables[table]
			if !ok {
				continue
			}
			models = append(models, schemaModel{
				Domain: domain,
				Name:   item.Name,
				Table:  table,
				Doc:    item.Doc,
				Fields: item.Fields,
				driver: driver,
			})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Domain != models[j].Domain {
			return models[i].Domain < models[j].Domain
		}
		return models[i].Table < models[j].Table
	})
	return models, nil
}

// schemaType 是包内解析出的结构体中间结果。
type schemaType struct {
	// Name 是结构体名。
	Name string
	// Doc 是结构体说明。
	Doc string
	// Fields 是展开后的字段。
	Fields []schemaField
}

// structInfo 是包内用于展开嵌入字段的结构体信息。
type structInfo struct {
	// Doc 是结构体说明。
	Doc string
	// Struct 是结构体 AST。
	Struct *ast.StructType
}

// parseSchemaPackage 解析单个模型包，返回需要导出的表结构体。
func parseSchemaPackage(dir string, tables map[string]string) ([]schemaType, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.ParseComments)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	naming := schema.NamingStrategy{}
	structs := make(map[string]structInfo)
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gen, ok := decl.(*ast.GenDecl)
				if !ok || gen.Tok != token.TYPE {
					continue
				}
				for _, spec := range gen.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					doc := strings.TrimSpace(ts.Doc.Text())
					if doc == "" {
						doc = strings.TrimSpace(gen.Doc.Text())
					}
					structs[ts.Name.Name] = structInfo{Doc: doc, Struct: st}
				}
			}
		}
	}

	names := make([]string, 0, len(structs))
	for name := range structs {
		names = append(names, name)
	}
	sort.Strings(names)

	var out []schemaType
	for _, name := range names {
		info := structs[name]
		table := naming.TableName(name)
		seen := make(map[string]bool)
		fields := expandSchemaFields(info.Struct, structs, seen, naming, table, tables[table])
		out = append(out, schemaType{Name: name, Doc: info.Doc, Fields: fields})
	}
	return out, nil
}

// expandSchemaFields 展开结构体字段与同包嵌入结构体，保持声明顺序。
func expandSchemaFields(st *ast.StructType, structs map[string]structInfo, seen map[string]bool, naming schema.NamingStrategy, table, driver string) []schemaField {
	var fields []schemaField
	for _, field := range st.Fields.List {
		tag := structTag(field)
		if tag.Get("gorm") == "-" {
			continue
		}
		if len(field.Names) == 0 {
			name := exprText(field.Type)
			switch name {
			case "gorm.Model":
				fields = append(fields, gormModelFields()...)
				continue
			case "database.BaseModel":
				fields = append(fields, baseModelFields()...)
				continue
			}
			if info, ok := structs[name]; ok && !seen[name] {
				seen[name] = true
				fields = append(fields, expandSchemaFields(info.Struct, structs, seen, naming, table, driver)...)
				continue
			}
			fields = append(fields, schemaField{
				Column: "（嵌入）", GoType: name, Nullable: "-", Default: "-",
				Comment: "嵌入字段，字段列见对应类型定义",
			})
			continue
		}
		for _, ident := range field.Names {
			fields = append(fields, buildSchemaField(ident.Name, field, tag, naming, table, driver))
		}
	}
	return fields
}

// buildSchemaField 构造单个字段的展示信息。
func buildSchemaField(name string, field *ast.Field, tag structTagInfo, naming schema.NamingStrategy, table, driver string) schemaField {
	gorm := parseGormTag(tag.Get("gorm"))
	column := gorm["column"]
	if column == "" {
		column = naming.ColumnName(table, name)
	}
	goType := gorm["type"]
	if goType == "" {
		goType = exprText(field.Type)
	}
	nullable := "是"
	if driver == "clickhouse" {
		// ClickHouse 默认非空，只有显式 Nullable(...) 列才能存 NULL。
		if !strings.Contains(goType, "Nullable(") {
			nullable = "否"
		}
	} else if _, ok := gorm["not null"]; ok {
		nullable = "否"
	}
	defaultValue := gorm["default"]
	if defaultValue == "" {
		defaultValue = "-"
	}
	comment := strings.TrimSpace(field.Doc.Text())
	if comment == "" {
		comment = strings.TrimSpace(field.Comment.Text())
	}
	comment = strings.ReplaceAll(comment, "\n", " ")

	foreign := ""
	if fk := gorm["foreignkey"]; fk != "" {
		foreign = "foreignKey:" + fk
		if ref := gorm["references"]; ref != "" {
			foreign += "；references:" + ref
		}
	}
	return schemaField{
		Column:   column,
		GoType:   goType,
		Nullable: nullable,
		Default:  defaultValue,
		Comment:  comment,
		Primary:  hasGormKey(gorm, "primaryKey"),
		Unique:   hasGormKey(gorm, "uniqueIndex"),
		Index:    hasGormKey(gorm, "index"),
		Foreign:  foreign,
	}
}

// structTagInfo 是字段标签的最小读取接口，避免直接依赖 reflect.StructTag 的取值形式。
type structTagInfo struct {
	// raw 是标签原文（不含反引号）。
	raw string
}

// Get 返回指定标签键的值。
func (t structTagInfo) Get(key string) string {
	prefix := key + `:"`
	idx := strings.Index(t.raw, prefix)
	if idx < 0 {
		return ""
	}
	rest := t.raw[idx+len(prefix):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// structTag 读取字段标签原文。
func structTag(field *ast.Field) structTagInfo {
	if field.Tag == nil {
		return structTagInfo{}
	}
	return structTagInfo{raw: strings.Trim(field.Tag.Value, "`")}
}

// parseGormTag 把 gorm 标签解析为键值对；无值选项的值为空串。
func parseGormTag(tag string) map[string]string {
	out := make(map[string]string)
	for _, part := range strings.Split(tag, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if idx := strings.Index(part, ":"); idx >= 0 {
			out[strings.ToLower(part[:idx])] = part[idx+1:]
			continue
		}
		out[strings.ToLower(part)] = ""
	}
	return out
}

// hasGormKey 报告 gorm 选项是否存在（包括带索引名的写法）。
func hasGormKey(options map[string]string, key string) bool {
	_, ok := options[strings.ToLower(key)]
	return ok
}

// exprText 把 AST 表达式渲染为源码文本。
func exprText(expr ast.Expr) string {
	var buf bytes.Buffer
	if err := format.Node(&buf, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return buf.String()
}

// gormModelFields 返回 gorm.Model 的标准字段。
func gormModelFields() []schemaField {
	return []schemaField{
		{Column: "id", GoType: "uint", Nullable: "否", Default: "-", Comment: "主键", Primary: true},
		{Column: "created_at", GoType: "time.Time", Nullable: "是", Default: "-", Comment: "创建时间"},
		{Column: "updated_at", GoType: "time.Time", Nullable: "是", Default: "-", Comment: "更新时间"},
		{Column: "deleted_at", GoType: "*time.Time", Nullable: "是", Default: "-", Comment: "软删除时间"},
	}
}

// baseModelFields 返回 database.BaseModel 的标准字段。
func baseModelFields() []schemaField {
	return []schemaField{
		{Column: "id", GoType: "uint", Nullable: "否", Default: "-", Comment: "主键", Primary: true},
		{Column: "created_at", GoType: "time.Time", Nullable: "是", Default: "-", Comment: "创建时间"},
		{Column: "updated_at", GoType: "time.Time", Nullable: "是", Default: "-", Comment: "更新时间"},
	}
}

// schemaSummary 取结构体说明的首句作为摘要。
func schemaSummary(doc string) string {
	doc = strings.ReplaceAll(strings.TrimSpace(doc), "\n", " ")
	if idx := strings.Index(doc, "。"); idx >= 0 {
		return doc[:idx+len("。")]
	}
	if doc == "" {
		return "已迁移数据表。"
	}
	return doc
}

// KnowledgeDocument 是一篇可入 RAG 的生成知识文档：Title 为同步与去重的稳定键，Content 为正文。
type KnowledgeDocument struct {
	// Key 生成条目的稳定键。
	Key string
	// Title 文档标题，同时作为入库与同步判定的稳定键。
	Title string
	// Content Markdown 正文。
	Content string
}

// SchemaKnowledgeDocuments 生成数据表分区的知识文档集：每张已迁移表一篇，正文与数据表参考页
// 同源，仅去掉指向仓库文件的相对链接（RAG 正文中的相对链接不可达）。返回顺序按域与表名稳定
// 排序，保证 zki sync schema 幂等。
func SchemaKnowledgeDocuments(projectRoot string) ([]KnowledgeDocument, error) {
	tables, err := migratedTables(projectRoot)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	models, err := loadSchemaModels(projectRoot, tables)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	docs := make([]KnowledgeDocument, 0, len(models))
	for _, model := range models {
		docs = append(docs, KnowledgeDocument{
			Key:     model.Table,
			Title:   "数据表：" + model.Table,
			Content: renderSchemaKnowledgeDoc(model),
		})
	}
	return docs, nil
}

// renderSchemaKnowledgeDoc 生成单张数据表的 RAG 正文：标题 + 参考页正文（去掉仓库相对链接小节）。
func renderSchemaKnowledgeDoc(model schemaModel) string {
	page := renderSchemaPage(model)
	if idx := strings.Index(page, "\n## 相关参考"); idx >= 0 {
		page = page[:idx]
	}
	return "# 数据表：" + model.Table + "\n\n" + strings.TrimRight(page, "\n") + "\n"
}

// renderSchemaPage 生成单张数据表页面的正文。
func renderSchemaPage(model schemaModel) string {
	var b strings.Builder
	b.WriteString("## 表用途\n\n")
	if model.Doc == "" {
		b.WriteString("（结构体未提供注释，字段以代码声明为准。）\n")
	} else {
		b.WriteString(model.Doc)
		b.WriteString("\n")
	}
	b.WriteString("\n## 归属域\n\n`")
	b.WriteString(model.Domain)
	b.WriteString("`\n")
	b.WriteString("\n## 字段\n\n")
	b.WriteString("| 列 | 类型 | 可空 | 默认值 | 说明 |\n| --- | --- | --- | --- | --- |\n")
	for _, field := range model.Fields {
		b.WriteString("| ")
		b.WriteString(mdCell(field.Column))
		b.WriteString(" | ")
		b.WriteString(mdCell(field.GoType))
		b.WriteString(" | ")
		b.WriteString(field.Nullable)
		b.WriteString(" | ")
		b.WriteString(mdCell(field.Default))
		b.WriteString(" | ")
		b.WriteString(mdCell(field.Comment))
		b.WriteString(" |\n")
	}
	b.WriteString("\n## 索引与外键\n\n")
	writeSchemaKeys(&b, model.Fields)
	b.WriteString("\n## 相关参考\n\n")
	b.WriteString("- [")
	b.WriteString(model.Domain)
	b.WriteString(" 包参考](../../internal/models/")
	b.WriteString(model.Domain)
	b.WriteString(".md)\n")
	return b.String()
}

// writeSchemaKeys 写出主键、索引与外键清单；没有声明时给出明确说明。
func writeSchemaKeys(b *strings.Builder, fields []schemaField) {
	wrote := false
	for _, field := range fields {
		if field.Primary {
			fmt.Fprintf(b, "- 主键：`%s`\n", field.Column)
			wrote = true
		}
	}
	for _, field := range fields {
		if field.Unique {
			fmt.Fprintf(b, "- 唯一索引：`%s`\n", field.Column)
			wrote = true
		}
	}
	for _, field := range fields {
		if field.Index {
			fmt.Fprintf(b, "- 普通索引：`%s`\n", field.Column)
			wrote = true
		}
	}
	for _, field := range fields {
		if field.Foreign != "" {
			fmt.Fprintf(b, "- 外键：`%s`（%s）\n", field.Column, field.Foreign)
			wrote = true
		}
	}
	if !wrote {
		b.WriteString("未声明主键、索引或外键。\n")
	}
}

// renderSchemaIndex 生成按域分组的数据表索引正文。
func renderSchemaIndex(models []schemaModel) string {
	var b strings.Builder
	b.WriteString("## 按域\n\n")
	current := ""
	for _, model := range models {
		if model.Domain != current {
			current = model.Domain
			b.WriteString("### ")
			b.WriteString(current)
			b.WriteString("\n\n")
		}
		b.WriteString("- [`")
		b.WriteString(model.Table)
		b.WriteString("`](./")
		b.WriteString(model.Domain)
		b.WriteString("/")
		b.WriteString(model.Table)
		b.WriteString(".md) — ")
		b.WriteString(schemaSummary(model.Doc))
		b.WriteString("\n")
	}
	return b.String()
}
