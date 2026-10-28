package cmd

import (
	"testing"
	"time"
	"zhongkuisec.com/zhongkui/internal/ai/contract"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// TestFormatDuration 验证耗时的三种量级格式化口径：毫秒、秒（1 位小数）、分秒。
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"毫秒", 350 * time.Millisecond, "350ms"},
		{"秒", 1200 * time.Millisecond, "1.2s"},
		{"分秒", 65000 * time.Millisecond, "1m05s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatDuration(tc.in); got != tc.want {
				t.Errorf("formatDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatElapsedMs 验证毫秒输入的格式化委托 formatDuration。
func TestFormatElapsedMs(t *testing.T) {
	cases := []struct {
		name string
		in   int64
		want string
	}{
		{"毫秒", 350, "350ms"},
		{"秒", 1200, "1.2s"},
		{"分秒", 65000, "1m05s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatElapsedMs(tc.in); got != tc.want {
				t.Errorf("formatElapsedMs(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatTokens 验证 token 数格式化：非正数显示 "-"，正数千分位分组。
func TestFormatTokens(t *testing.T) {
	cases := []struct {
		name string
		in   int
		want string
	}{
		{"零", 0, "-"},
		{"负数", -1, "-"},
		{"四位数", 1234, "1,234"},
		{"三位数", 999, "999"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatTokens(tc.in); got != tc.want {
				t.Errorf("formatTokens(%d) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestFormatCountGrouped 验证整数三位分组格式化。
func TestFormatCountGrouped(t *testing.T) {
	if got := formatCountGrouped(1234567); got != "1,234,567" {
		t.Errorf("formatCountGrouped(1234567) = %q, want %q", got, "1,234,567")
	}
}

// TestSyncRate 验证处理速度计算：正常值与零耗时边界。
func TestSyncRate(t *testing.T) {
	if got := syncRate(2, time.Second); got != "2.0" {
		t.Errorf("syncRate(2, 1s) = %q, want %q", got, "2.0")
	}
	if got := syncRate(2, 0); got != "0.0" {
		t.Errorf("syncRate(2, 0) = %q, want %q", got, "0.0")
	}
}

// TestSyncETA 验证预计剩余时间估算：未开始、已完成全部均返回 "-"，进行中按速度外推。
func TestSyncETA(t *testing.T) {
	if got := syncETA(0, 10, 10*time.Second); got != "-" {
		t.Errorf("syncETA(done=0) = %q, want %q", got, "-")
	}
	if got := syncETA(10, 10, 10*time.Second); got != "-" {
		t.Errorf("syncETA(done=total) = %q, want %q", got, "-")
	}
	if got := syncETA(5, 10, 10*time.Second); got != "10.0s" {
		t.Errorf("syncETA(done=5,total=10) = %q, want %q", got, "10.0s")
	}
}

// TestSyncItemTitle 验证明细条目标题：Title 非空原样返回，为空时回退记忆批本地化标签。
func TestSyncItemTitle(t *testing.T) {
	if got := syncItemTitle(contract.SyncProgressItem{Title: "pkg/foo"}); got != "pkg/foo" {
		t.Errorf("syncItemTitle(有标题) = %q, want %q", got, "pkg/foo")
	}
	got := syncItemTitle(contract.SyncProgressItem{Count: 3})
	key := "cli.sync.progress.memory-item"
	if got == key || got == "" {
		t.Errorf("syncItemTitle(空标题) = %q, want 本地化标签（非键名）", got)
	}
}

// TestSyncActionLabel 验证动作标识本地化：非空动作返回本地化标签，空串返回 "-"。
func TestSyncActionLabel(t *testing.T) {
	got := syncActionLabel(contract.SyncActionCreate)
	if got == "cli.sync.progress.action."+contract.SyncActionCreate {
		t.Errorf("syncActionLabel(%q) 未命中本地化，返回键名 %q", contract.SyncActionCreate, got)
	}
	if got == "" {
		t.Errorf("syncActionLabel(%q) 返回空串", contract.SyncActionCreate)
	}
	if blank := syncActionLabel(""); blank != "-" {
		t.Errorf("syncActionLabel(\"\") = %q, want %q", blank, "-")
	}
}

// TestBuildSyncTable 验证汇总表组装：表头 + 逐条明细 + 合计行，合计行取总数标签与 token 之和，
// 零用量行 token 列显示 "-"。
func TestBuildSyncTable(t *testing.T) {
	rows := []contract.SyncProgressItem{
		{
			Title:     "pkg/foo",
			Action:    contract.SyncActionCreate,
			Count:     1,
			ElapsedMs: 1200,
			Usage:     contract.EmbedUsage{PromptTokens: 100},
		},
		{
			Title:     "",
			Action:    contract.SyncActionMemory,
			Count:     5,
			ElapsedMs: 500,
			Usage:     contract.EmbedUsage{},
		},
	}
	data := buildSyncTable(rows)
	if len(data) != 4 {
		t.Fatalf("buildSyncTable 行数 = %d, want 4（表头+2 明细+合计）", len(data))
	}

	total := data[len(data)-1]
	if want := translate.T("cli.sync.progress.total-row", nil); total[0] != want {
		t.Errorf("合计行条目列 = %q, want %q", total[0], want)
	}
	if want := translate.T("cli.sync.progress.count", map[string]any{"Count": 6}); total[1] != want {
		t.Errorf("合计行动作列 = %q, want %q", total[1], want)
	}
	if total[3] != formatTokens(100) {
		t.Errorf("合计行输入 Token = %q, want %q", total[3], formatTokens(100))
	}
	if total[4] != "-" {
		t.Errorf("合计行输出 Token = %q, want %q", total[4], "-")
	}

	if data[2][3] != "-" || data[2][4] != "-" {
		t.Errorf("零用量行 token 列 = (%q, %q), want (\"-\", \"-\")", data[2][3], data[2][4])
	}
}
