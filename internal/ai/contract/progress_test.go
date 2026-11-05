package contract

import "testing"

// TestReportHelpers_NilSafe 校验三个 nil 安全辅助函数在 report 为 nil 时不 panic。
func TestReportHelpers_NilSafe(t *testing.T) {
	ReportStart(nil, 3)
	ReportStep(nil, SyncProgressItem{Action: SyncActionReindex, Count: 1})
	ReportFinish(nil)
}
