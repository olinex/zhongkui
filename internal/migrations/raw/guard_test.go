package raw

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// createTablePattern 匹配 001_init.sql 中单条 CREATE TABLE 语句的表名与完整语句
var createTablePattern = regexp.MustCompile(`(?s)CREATE TABLE IF NOT EXISTS (\w+) \(.*?\n\).*?;`)

// upperPacketTables 是链路无关的上层逐包表，不允许保留 eth_* 列
var upperPacketTables = []string{
	"packet_ipv4",
	"packet_ipv6",
	"packet_ipv4_tcps",
	"packet_ipv6_tcps",
	"packet_ipv4_udps",
	"packet_ipv6_udps",
}

// tcpPacketTables 是 TCP 列表依赖 bloom 索引过滤的表
var tcpPacketTables = []string{"packet_ipv4_tcps", "packet_ipv6_tcps"}

// udpPacketTables 是 UDP 逐包表，ORDER BY 契约与 TCP 表一致
var udpPacketTables = []string{"packet_ipv4_udps", "packet_ipv6_udps"}

// parseCreateTables 读取内嵌迁移脚本并按表名返回完整建表语句
func parseCreateTables(t *testing.T) map[string]string {
	t.Helper()

	script, err := Migrations.ReadFile("001_init.sql")
	if err != nil {
		t.Fatalf("读取 001_init.sql 失败: %v", err)
	}

	tables := make(map[string]string)
	for _, match := range createTablePattern.FindAllStringSubmatch(string(script), -1) {
		tables[match[1]] = match[0]
	}
	if len(tables) == 0 {
		t.Fatal("未从 001_init.sql 解析到任何 CREATE TABLE 语句")
	}
	return tables
}

// hasColumn 判断建表语句中是否声明了指定列（忽略大小写与前导空白）
func hasColumn(body, column string) bool {
	pattern := regexp.MustCompile(`(?mi)^\s*` + regexp.QuoteMeta(column) + `\s+\w+`)
	return pattern.MatchString(body)
}

// hasBloomIndex 判断建表语句中是否存在指定列的 bloom_filter 索引
func hasBloomIndex(body, column string) bool {
	pattern := regexp.MustCompile(`(?mi)^\s*INDEX\s+\S+\s+` + regexp.QuoteMeta(column) + `\s+TYPE\s+bloom_filter`)
	return pattern.MatchString(body)
}

// hasAnyIndex 判断建表语句中是否存在引用指定列的索引
func hasAnyIndex(body, column string) bool {
	pattern := regexp.MustCompile(`(?mi)^\s*INDEX\s+\S+\s+` + regexp.QuoteMeta(column) + `\s+TYPE\s+\w+`)
	return pattern.MatchString(body)
}

// orderByColumns 提取建表语句 ORDER BY 括号内的排序列
func orderByColumns(body string) []string {
	const marker = "ORDER BY ("
	idx := strings.Index(body, marker)
	if idx < 0 {
		return nil
	}
	rest := body[idx+len(marker):]
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

// TestUpperPacketTablesLinkColumns 校验上层 6 张表已移除 eth_*，并完整声明 link_type 与 link_1～link_16
func TestUpperPacketTablesLinkColumns(t *testing.T) {
	tables := parseCreateTables(t)

	for _, table := range upperPacketTables {
		body, ok := tables[table]
		if !ok {
			t.Errorf("缺少建表语句: %s", table)
			continue
		}
		if strings.Contains(body, "eth_") {
			t.Errorf("%s 仍包含 eth_* 列或表达式", table)
		}
		if !hasColumn(body, "link_type") {
			t.Errorf("%s 缺少 link_type 列", table)
		}
		for slot := 1; slot <= 16; slot++ {
			column := "link_" + strconv.Itoa(slot)
			if !hasColumn(body, column) {
				t.Errorf("%s 缺少 %s 列", table, column)
			}
		}
	}
}

// TestUpperPacketTablesSlotIndexes 校验 link_1～link_6 建 bloom 索引、link_7～link_16 不建索引
func TestUpperPacketTablesSlotIndexes(t *testing.T) {
	tables := parseCreateTables(t)

	for _, table := range upperPacketTables {
		body, ok := tables[table]
		if !ok {
			t.Errorf("缺少建表语句: %s", table)
			continue
		}
		for slot := 1; slot <= 6; slot++ {
			column := "link_" + strconv.Itoa(slot)
			if !hasBloomIndex(body, column) {
				t.Errorf("%s 的 %s 缺少 bloom_filter 索引", table, column)
			}
		}
		for slot := 7; slot <= 16; slot++ {
			column := "link_" + strconv.Itoa(slot)
			if hasAnyIndex(body, column) {
				t.Errorf("%s 的 %s 不应建索引", table, column)
			}
		}
	}
}

// TestTcpPacketTablesKeepBloomIndexes 校验 TCP 表保留 session_id、IP 源/目的与 TCP 端口的 bloom 索引
func TestTcpPacketTablesKeepBloomIndexes(t *testing.T) {
	tables := parseCreateTables(t)

	expected := map[string][]string{
		"packet_ipv4_tcps": {"session_id", "ipv4_src", "ipv4_dst", "tcp_src_port", "tcp_dst_port"},
		"packet_ipv6_tcps": {"session_id", "ipv6_src", "ipv6_dst", "tcp_src_port", "tcp_dst_port"},
	}
	for _, table := range tcpPacketTables {
		body, ok := tables[table]
		if !ok {
			t.Errorf("缺少建表语句: %s", table)
			continue
		}
		for _, column := range expected[table] {
			if !hasBloomIndex(body, column) {
				t.Errorf("%s 的 %s 缺少既有 bloom_filter 索引", table, column)
			}
		}
	}
}

// TestPacketListTablesOrderBy 校验 TCP/UDP 表 ORDER BY 以 timestamp, namespace 开头并包含 link_type
func TestPacketListTablesOrderBy(t *testing.T) {
	tables := parseCreateTables(t)

	tablesToCheck := append(append([]string{}, tcpPacketTables...), udpPacketTables...)
	for _, table := range tablesToCheck {
		body, ok := tables[table]
		if !ok {
			t.Errorf("缺少建表语句: %s", table)
			continue
		}

		columns := orderByColumns(body)
		if len(columns) < 3 {
			t.Errorf("%s 的 ORDER BY 列不足: %v", table, columns)
			continue
		}
		if columns[0] != "timestamp" || columns[1] != "namespace" {
			t.Errorf("%s 的 ORDER BY 应以 timestamp, namespace 开头，实际为 %v", table, columns)
		}
		containsLinkType := false
		for _, column := range columns {
			if column == "link_type" {
				containsLinkType = true
			}
			if strings.Contains(column, "eth_") || strings.Contains(column, "LEAST") || strings.Contains(column, "GREATEST") {
				t.Errorf("%s 的 ORDER BY 不应包含链路以太网表达式: %s", table, column)
			}
		}
		if !containsLinkType {
			t.Errorf("%s 的 ORDER BY 缺少 link_type: %v", table, columns)
		}
	}
}

// TestEthernetTableLinkColumns 校验 packet_ethernets 保留 eth_* 列且新增 link_type
func TestEthernetTableLinkColumns(t *testing.T) {
	tables := parseCreateTables(t)

	body, ok := tables["packet_ethernets"]
	if !ok {
		t.Fatal("缺少建表语句: packet_ethernets")
	}
	for _, column := range []string{"eth_dst_mac", "eth_src_mac", "eth_type", "eth_length"} {
		if !hasColumn(body, column) {
			t.Errorf("packet_ethernets 缺少 %s 列", column)
		}
	}
	if !hasColumn(body, "link_type") {
		t.Error("packet_ethernets 缺少 link_type 列")
	}
}

// TestLinkLayerPacketTables 校验三张链路层单包表存在且包含 link_type
func TestLinkLayerPacketTables(t *testing.T) {
	tables := parseCreateTables(t)

	for _, table := range []string{"packet_linux_slls", "packet_linux_sll2s", "packet_ppps", "packet_dot11s"} {
		body, ok := tables[table]
		if !ok {
			t.Errorf("缺少建表语句: %s", table)
			continue
		}
		if !hasColumn(body, "link_type") {
			t.Errorf("%s 缺少 link_type 列", table)
		}
	}
}
