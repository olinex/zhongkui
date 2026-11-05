package cmd

import (
	"zhongkuisec.com/zhongkui/internal/ai/contract"
	// 标准库
	"strconv"
	"strings"
	"sync"
	"time"

	// 第三方
	"github.com/pterm/pterm"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// syncProgress 是 contract.SyncReporter 的 CLI 渲染实现：以 pterm 进度条实时展示处理速度与预计
// 剩余时间，并在结束后打印逐条明细汇总表。它供 sync docs / sync apis / sync embeddings
// 三条链路复用；同一实例只服务于一次 Start→Step...→Finish 生命周期。
type syncProgress struct {
	// namespace 本次同步的目标命名空间，用于头部信息与汇总表。
	namespace string
	// model 本次使用的 embedding 模型名，未知时为 "-"。
	model string
	// bar 进度条实例；total<=0 或创建失败时为 nil（降级为仅上报不渲染进度条）。
	bar *pterm.ProgressbarPrinter
	// rows 逐条进度明细，Finish 时汇总为表格。
	rows []contract.SyncProgressItem
	// total 本次待处理总条数。
	total int
	// done 已完成条数累计。
	done int
	// started 本次开始时间，用于速度与预计剩余时间估算。
	started time.Time
	// mu 保护 bar/rows/total/done/started，Step 与后台刷新 goroutine 并发访问。
	mu sync.Mutex
	// stop 关闭信号：Finish 时关闭以终止后台刷新 goroutine。
	stop chan struct{}
	// stopOnce 保证 stop 只被关闭一次，使 Finish 幂等可重复调用。
	stopOnce sync.Once
	// wg 等待后台刷新 goroutine 退出。
	wg sync.WaitGroup
}

// newSyncProgress 构造一个进度上报实例；model 未知时传 "-"。
func newSyncProgress(namespace, model string) *syncProgress {
	return &syncProgress{
		namespace: namespace,
		model:     model,
		stop:      make(chan struct{}),
	}
}

// rateTitle 拼装进度条标题：速度、预计剩余、已完成/总数。调用方负责持有 mu（构造期除外）。
func (p *syncProgress) rateTitle() string {
	elapsed := time.Since(p.started)
	return translate.T("cli.sync.progress.rate-title", map[string]any{
		"Rate":  syncRate(p.done, elapsed),
		"Eta":   syncETA(p.done, p.total, elapsed),
		"Done":  p.done,
		"Total": p.total,
	})
}

// Start 上报本次待处理总条数：总是先打印头部信息；total<=0 时仅提示「无需处理」并返回，
// 不创建进度条；否则创建进度条并在成功后启动后台刷新 goroutine。进度条创建失败时打印
// 错误并降级为「无进度条但继续上报」，不中断任务。
func (p *syncProgress) Start(total int) {
	p.mu.Lock()
	p.total = total
	p.done = 0
	p.started = time.Now()
	title := p.rateTitle()
	p.mu.Unlock()

	pterm.Info.Println(translate.T("cli.sync.progress.header", map[string]any{
		"Namespace": p.namespace,
		"Model":     p.model,
		"Total":     total,
	}))
	if total <= 0 {
		pterm.Info.Println(translate.T("cli.sync.progress.empty", nil))
		return
	}

	bar, err := pterm.DefaultProgressbar.
		WithTotal(total).
		WithTitle(title).
		WithRemoveWhenDone(true).
		WithShowCount(true).
		WithShowPercentage(true).
		WithShowElapsedTime(true).
		WithShowTitle(true).
		Start()
	if err != nil {
		pterm.Error.Println(err.Error())
		return
	}

	p.mu.Lock()
	p.bar = bar
	p.mu.Unlock()

	p.wg.Add(1)
	go p.refresh()
}

// refresh 每 500ms 刷新一次进度条标题（速度与预计剩余时间）；收到 stop 关闭信号或 ticker
// 触发时循环，退出前停止 ticker。
func (p *syncProgress) refresh() {
	defer p.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.mu.Lock()
			if p.bar != nil {
				p.bar.UpdateTitle(p.rateTitle())
			}
			p.mu.Unlock()
		}
	}
}

// Step 上报一条进度明细：记录明细、累加已完成条数并推进进度条。
func (p *syncProgress) Step(item contract.SyncProgressItem) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.rows = append(p.rows, item)
	p.done += item.Count
	if p.bar != nil {
		p.bar.Add(item.Count)
	}
}

// Finish 结束上报：幂等地关闭刷新 goroutine、停止进度条，并在有明细时打印汇总表。
// 可安全重复调用。
func (p *syncProgress) Finish() {
	p.stopOnce.Do(func() { close(p.stop) })
	p.wg.Wait()

	p.mu.Lock()
	bar := p.bar
	rows := p.rows
	p.mu.Unlock()

	if bar != nil {
		_, _ = bar.Stop()
	}
	if len(rows) > 0 {
		_ = pterm.DefaultTable.WithHasHeader().WithData(buildSyncTable(rows)).Render()
	}
}

// formatDuration 将耗时格式化为人读文本：<1s 显示 "350ms"，<1min 显示 "1.2s"，否则 "1m05s"。
func formatDuration(d time.Duration) string {
	switch {
	case d < time.Second:
		return strconv.FormatInt(d.Milliseconds(), 10) + "ms"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
	default:
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return strconv.Itoa(minutes) + "m" + strconv.Itoa(seconds/10) + strconv.Itoa(seconds%10) + "s"
	}
}

// formatElapsedMs 将毫秒耗时格式化为人读文本（委托 formatDuration）。
func formatElapsedMs(ms int64) string {
	return formatDuration(time.Duration(ms) * time.Millisecond)
}

// formatCountGrouped 三位分组格式化整数（1234567 → "1,234,567"）。
func formatCountGrouped(n int) string {
	s := strconv.Itoa(n)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign = "-"
		s = s[1:]
	}
	if len(s) <= 3 {
		return sign + s
	}
	var b strings.Builder
	b.WriteString(sign)
	pre := len(s) % 3
	if pre > 0 {
		b.WriteString(s[:pre])
		b.WriteString(",")
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteString(",")
		}
	}
	return b.String()
}

// formatTokens 格式化 token 数：非正数（供应商未返回用量）显示 "-"，否则千分位分组。
func formatTokens(n int) string {
	if n <= 0 {
		return "-"
	}
	return formatCountGrouped(n)
}

// syncRate 计算处理速度（条/秒，1 位小数）；elapsed 非正时返回 "0.0"。
func syncRate(count int, elapsed time.Duration) string {
	if elapsed <= 0 {
		return "0.0"
	}
	return strconv.FormatFloat(float64(count)/elapsed.Seconds(), 'f', 1, 64)
}

// syncETA 依据已完成条数与已耗时估算剩余时间；done<=0 或 done>=total 时返回 "-"。
func syncETA(done, total int, elapsed time.Duration) string {
	if done <= 0 || done >= total {
		return "-"
	}
	remaining := time.Duration(float64(elapsed) / float64(done) * float64(total-done))
	return formatDuration(remaining)
}

// syncItemTitle 取明细条目标题：Title 非空时用 Title，为空（记忆批）时取
// cli.sync.progress.memory-item（占位 Count）。
func syncItemTitle(row contract.SyncProgressItem) string {
	if row.Title != "" {
		return row.Title
	}
	return translate.T("cli.sync.progress.memory-item", map[string]any{"Count": row.Count})
}

// syncActionLabel 把动作标识翻译为本地化标签（cli.sync.progress.action.<action>）；
// action 为空时返回 "-"。
func syncActionLabel(action string) string {
	if action == "" {
		return "-"
	}
	return translate.T("cli.sync.progress.action."+action, nil)
}

// buildSyncTable 组装汇总表数据：表头（条目/动作/耗时/输入 Token/输出 Token）+ 逐条明细 +
// 合计行。合计行的「条目」列取 cli.sync.progress.total-row，「动作」列取 cli.sync.progress.count
// （占位 Count = 各行 Count 之和），耗时与 token 列取各行之和。
func buildSyncTable(rows []contract.SyncProgressItem) [][]string {
	data := [][]string{{
		translate.T("cli.sync.progress.col.item", nil),
		translate.T("cli.sync.progress.col.action", nil),
		translate.T("cli.sync.progress.col.elapsed", nil),
		translate.T("cli.sync.progress.col.prompt", nil),
		translate.T("cli.sync.progress.col.completion", nil),
	}}
	var totalCount int
	var totalElapsed int64
	var totalPrompt, totalCompletion int
	for _, row := range rows {
		data = append(data, []string{
			syncItemTitle(row),
			syncActionLabel(row.Action),
			formatElapsedMs(row.ElapsedMs),
			formatTokens(row.Usage.PromptTokens),
			formatTokens(row.Usage.CompletionTokens),
		})
		totalCount += row.Count
		totalElapsed += row.ElapsedMs
		totalPrompt += row.Usage.PromptTokens
		totalCompletion += row.Usage.CompletionTokens
	}
	data = append(data, []string{
		translate.T("cli.sync.progress.total-row", nil),
		translate.T("cli.sync.progress.count", map[string]any{"Count": totalCount}),
		formatElapsedMs(totalElapsed),
		formatTokens(totalPrompt),
		formatTokens(totalCompletion),
	})
	return data
}

// embeddingModelName 取当前 embedding 模型名用于进度头部展示：rt.Embed() 失败或不支持
// 模型名能力时返回 "-"。
func embeddingModelName(rt *ai.Runtime) string {
	emb, err := rt.Embed()
	if err != nil {
		return "-"
	}
	if namer, ok := emb.(interface{ ModelName() string }); ok {
		return namer.ModelName()
	}
	return "-"
}
