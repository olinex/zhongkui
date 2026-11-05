package contract

// ReportStart 在 r 非空时转发 Start。
func ReportStart(r SyncReporter, total int) {
	if r == nil {
		return
	}
	r.Start(total)
}

// ReportStep 在 r 非空时转发 Step。
func ReportStep(r SyncReporter, item SyncProgressItem) {
	if r == nil {
		return
	}
	r.Step(item)
}

// ReportFinish 在 r 非空时转发 Finish。
func ReportFinish(r SyncReporter) {
	if r == nil {
		return
	}
	r.Finish()
}
