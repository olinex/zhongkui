package views

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"zhongkuisec.com/zhongkui/internal/migrations/raw"
)

// searchFilterField 描述 search.toml 中一个 filterByFields 声明
type searchFilterField struct {
	name    string
	column  string
	actions []string
}

// createTableStatementPattern 匹配 001_init.sql 中单条 CREATE TABLE 语句的表名与完整语句
var createTableStatementPattern = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS (\w+) \(.*?\n\).*?;`)

// bloomIndexPattern 匹配 bloom_filter 索引及其列名
var bloomIndexPattern = regexp.MustCompile(`(?mi)^\s*INDEX\s+\S+\s+(\w+)\s+TYPE\s+bloom_filter`)

// TestTcpListFilterFieldsIndexed 校验 TCP 列表过滤列命中 ORDER BY 前缀或 bloom_filter 索引，
// 且链路槽位过滤契约符合规格：linkType 支持 eq/neq，link1～link6 仅 eq，link7～link16 不开放筛选。
func TestTcpListFilterFieldsIndexed(t *testing.T) {
	searchConfig := readSearchConfig(t)
	sqlTables := parseCreateTableStatements(t)

	cases := []struct {
		section string
		table   string
	}{
		{section: "Ipv4TcpListGet", table: "packet_ipv4_tcps"},
		{section: "Ipv6TcpListGet", table: "packet_ipv6_tcps"},
	}

	for _, tc := range cases {
		statement, ok := sqlTables[tc.table]
		if !ok {
			t.Errorf("缺少建表语句: %s", tc.table)
			continue
		}
		orderColumns := sqlStringSet(sqlOrderByColumns(statement))
		bloomColumns := sqlStringSet(sqlBloomIndexColumns(statement))

		filters := parseFilterFields(t, searchConfig, tc.section)
		if len(filters) == 0 {
			t.Errorf("%s 未解析到任何 filterByFields", tc.section)
			continue
		}

		byColumn := make(map[string]searchFilterField, len(filters))
		for _, filter := range filters {
			byColumn[filter.column] = filter
			// created_at 是分区键且为必填范围条件，分区裁剪不依赖 ORDER BY 或跳数索引
			if filter.column == "created_at" {
				continue
			}
			if _, ok := orderColumns[filter.column]; ok {
				continue
			}
			if _, ok := bloomColumns[filter.column]; ok {
				continue
			}
			t.Errorf("%s 的过滤列 %s（%s）既不在 ORDER BY 中，也没有 bloom_filter 索引",
				tc.section, filter.column, filter.name)
		}

		linkType, ok := byColumn["link_type"]
		if !ok {
			t.Errorf("%s 缺少 linkType 过滤字段", tc.section)
		} else if !reflect.DeepEqual(linkType.actions, []string{"eq", "neq"}) {
			t.Errorf("%s 的 linkType actions = %v，期望 [eq neq]", tc.section, linkType.actions)
		}

		for slot := 1; slot <= 6; slot++ {
			column := "link_" + strconv.Itoa(slot)
			filter, ok := byColumn[column]
			if !ok {
				t.Errorf("%s 缺少 link%d 过滤字段", tc.section, slot)
				continue
			}
			if !reflect.DeepEqual(filter.actions, []string{"eq"}) {
				t.Errorf("%s 的 link%d actions = %v，仅允许 eq", tc.section, slot, filter.actions)
			}
		}

		for slot := 7; slot <= 16; slot++ {
			column := "link_" + strconv.Itoa(slot)
			if _, ok := byColumn[column]; ok {
				t.Errorf("%s 不应开放未建索引的 %s 过滤", tc.section, column)
			}
			fieldName := "link" + strconv.Itoa(slot)
			for _, filter := range filters {
				if filter.name == fieldName {
					t.Errorf("%s 不应开放未建索引的 %s 过滤字段", tc.section, fieldName)
				}
			}
		}
	}
}

// readSearchConfig 读取与测试文件同目录下的 search.toml
func readSearchConfig(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) 失败，无法定位 search.toml")
	}
	path := filepath.Join(filepath.Dir(thisFile), "search.toml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	return string(content)
}

// parseCreateTableStatements 从内嵌迁移脚本解析表名到完整建表语句的映射
func parseCreateTableStatements(t *testing.T) map[string]string {
	t.Helper()

	script, err := raw.Migrations.ReadFile("001_init.sql")
	if err != nil {
		t.Fatalf("读取 001_init.sql 失败: %v", err)
	}
	statements := make(map[string]string)
	for _, match := range createTableStatementPattern.FindAllStringSubmatch(string(script), -1) {
		statements[match[1]] = match[0]
	}
	if len(statements) == 0 {
		t.Fatal("未从 001_init.sql 解析到任何 CREATE TABLE 语句")
	}
	return statements
}

// parseFilterFields 解析指定列表接口的 filterByFields 声明
func parseFilterFields(t *testing.T, content, section string) []searchFilterField {
	t.Helper()

	fieldNamePattern := regexp.MustCompile(`(?m)^\s*fieldName\s*=\s*"([^"]*)"`)
	columnPattern := regexp.MustCompile(`(?m)^\s*column\s*=\s*"([^"]*)"`)
	actionsPattern := regexp.MustCompile(`(?s)\bactions\s*=\s*\[(.*?)\]`)
	actionPattern := regexp.MustCompile(`"([^"]*)"`)

	target := "[[" + section + ".filterByFields]]"
	var (
		fields  []searchFilterField
		current bool
		block   []string
	)

	flush := func() {
		if !current || len(block) == 0 {
			block = nil
			return
		}
		body := strings.Join(block, "\n")
		block = nil

		nameMatch := fieldNamePattern.FindStringSubmatch(body)
		columnMatch := columnPattern.FindStringSubmatch(body)
		actionsMatch := actionsPattern.FindStringSubmatch(body)
		if nameMatch == nil || columnMatch == nil || actionsMatch == nil {
			t.Errorf("%s 存在缺少 fieldName/column/actions 的 filterByFields", section)
			return
		}

		field := searchFilterField{name: nameMatch[1], column: columnMatch[1]}
		for _, action := range actionPattern.FindAllStringSubmatch(actionsMatch[1], -1) {
			field.actions = append(field.actions, action[1])
		}
		fields = append(fields, field)
	}

	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") {
			flush()
			current = trimmed == target
			continue
		}
		if current {
			block = append(block, line)
		}
	}
	flush()

	return fields
}

// sqlOrderByColumns 提取建表语句 ORDER BY 括号内的排序列
func sqlOrderByColumns(statement string) []string {
	const marker = "ORDER BY ("
	idx := strings.Index(statement, marker)
	if idx < 0 {
		return nil
	}
	rest := statement[idx+len(marker):]
	end := strings.Index(rest, ")")
	if end < 0 {
		return nil
	}

	var columns []string
	for _, line := range strings.Split(rest[:end], "\n") {
		column := strings.TrimSuffix(strings.TrimSpace(line), ",")
		if column != "" {
			columns = append(columns, column)
		}
	}
	return columns
}

// sqlBloomIndexColumns 提取建表语句中所有 bloom_filter 索引列
func sqlBloomIndexColumns(statement string) []string {
	var columns []string
	for _, match := range bloomIndexPattern.FindAllStringSubmatch(statement, -1) {
		columns = append(columns, match[1])
	}
	return columns
}

// sqlStringSet 将字符串切片转为集合
func sqlStringSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}
