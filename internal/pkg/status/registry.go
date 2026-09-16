package status

import (
	"context"
	"fmt"
	"time"

	"github.com/creasty/defaults"
	"github.com/pterm/pterm"

	"zhongkui.com/zhongkui/internal/pkg/errgroupx"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// Registry 依赖自省注册表。
// 持有全部已注册探测项与统一的单探测超时，最后一次 Run 的结果被缓存，
// 可供 Render 直接渲染，并负责并行执行与表格渲染。
// Registry is the dependency introspection registry. It holds all registered
// probes and a unified per-probe timeout, caches the latest Run results for
// Render, and is responsible for running them concurrently and rendering the
// results as a table.
type Registry struct {
	probes          []Spec
	PerProbeTimeout time.Duration `default:"3s"` // 单探测独立超时（默认 3s） / per-probe timeout (default 3s)
	results         []Status
}

// New 创建一个新的依赖自省注册表。
// 默认的单探测超时为 3s，经 creasty/defaults 依据 perProbeTimeout 的 default tag 注入。
// New creates a new dependency introspection registry; the default per-probe
// timeout is 3s, injected by creasty/defaults from the perProbeTimeout default tag.
func New() *Registry {
	r := &Registry{}
	// 默认值来自编译期常量（perProbeTimeout 的 default:"3s"）；
	// 解析失败仅可能由 tag 书写错误导致，视为程序员错误，直接 panic。
	if err := defaults.Set(r); err != nil {
		panic(err)
	}
	return r
}

// Register 追加一个探测项。
// 注册顺序即 Run 返回结果的顺序。
// Register appends a probe item. The registration order is the order of the
// results returned by Run.
func (r *Registry) Register(spec Spec) {
	r.probes = append(r.probes, spec)
}

// SetPerProbeTimeout 设置每个探测项的独立超时。
// 默认值为 perProbeTimeout 的 default tag 设定（3s）。
// SetPerProbeTimeout sets the per-probe timeout, defaulting to the
// perProbeTimeout default tag value (3s).
func (r *Registry) SetPerProbeTimeout(d time.Duration) {
	r.PerProbeTimeout = d
}

// Run 并行执行全部探测，返回与注册顺序一致的 []Status。
//
// 要求：
//   - 每个探测带独立超时 ctx（超时为 SetPerProbeTimeout 设定的值）；
//   - 单个探测 panic 必须被 recover 且不中断其他探测，recover 后该探测状态
//     置为 Reachable=false、Err="panic: <reason>"；
//   - 整体不因单个依赖宕机而失败。
//
// 内部使用 errgroupx 并行调度，各协程回调恒返回 nil（不抛错误）。
// Latency 在此处计时并填充为毫秒时长。
//
// Run executes all probes concurrently and returns a []Status aligned with the
// registration order. Each probe runs under its own timeout context; a panicking
// probe is recovered without interrupting others (its status becomes
// Reachable=false with Err="panic: <reason>"), and the overall run never fails
// because of a single dependency. errgroupx is used for scheduling, each goroutine
// always returns nil; Latency is measured here and filled as milliseconds.
func (r *Registry) Run(ctx context.Context) []Status {
	results := make([]Status, len(r.probes))
	if len(r.probes) > 0 {
		g, ctx := errgroupx.WithContext(ctx)
		for i, spec := range r.probes {
			result := &results[i]
			g.Go(func() error {
				*result = r.probe(ctx, spec)
				return nil
			})
		}
		_ = g.Wait()
	}
	r.results = results
	return results
}

// probe 对单个探测项执行超时受限的探测并计时。
// 负责创建独立超时 ctx、recover panic、填充 Latency（毫秒）。
// probe runs a single probe under a per-probe timeout context, recovers panics,
// and measures latency filled into Latency (milliseconds).
func (r *Registry) probe(parent context.Context, spec Spec) (st Status) {
	st.Name = spec.Name
	st.Category = spec.Category
	ctx, cancel := context.WithTimeout(parent, r.PerProbeTimeout)
	defer cancel()

	start := time.Now()
	defer func() {
		if rec := recover(); rec != nil {
			// 被探测过即视为已配置，探测失败（panic）则标记为不可达，
			// 使 State() 收敛为 unreachable。
			st.Configured = true
			st.Reachable = false
			st.Err = fmt.Sprintf("panic: %v", rec)
		}
		st.Latency = serializer.DurationMS(time.Since(start).Milliseconds())
	}()
	pst := spec.Probe(ctx)
	st.Configured = pst.Configured
	st.Reachable = pst.Reachable
	st.Err = pst.Err
	return st
}

// Render 将最近一次 Run 的结果以 pterm 表格输出到 stdout。
// 列：名称/类别/已配置/状态/耗时/说明。
//
// localize 用于本地化表头与状态值；本地化缺失时以 key 本身作为回退
// （与项目 translate.T 行为一致），故实现内不硬编码任何语言字面量。
// 传 nil 时直接展示 key 本身。
//   - 表头键：cli.deps.col-name col-category col-configured col-state col-latency col-message
//   - 状态值键：cli.deps.value.ok|unreachable|disabled
//
// 返回值：表格渲染错误（包装为 errors.WithStack）。
//
// Render prints the latest Run results as a pterm table to stdout with columns
// name/category/configured/state/latency/message. localize localizes headers and
// state values, falling back to the key itself when a translation is missing
// (consistent with translate.T), so no language literals are hardcoded here;
// nil falls back to the key itself.
// It returns errors.WithStack-wrapped rendering errors.
func (r *Registry) Render(localize func(key string) string) error {
	// loc 以 key 本身作为兜底：localize 为 nil 或未命中时直接展示 key。
	// loc falls back to the key itself when localize is nil or is a no-op.
	loc := func(key string) string {
		if localize == nil {
			return key
		}
		if v := localize(key); v != "" {
			return v
		}
		return key
	}
	data := pterm.TableData{
		{
			loc("cli.deps.col-name"),
			loc("cli.deps.col-category"),
			loc("cli.deps.col-configured"),
			loc("cli.deps.col-state"),
			loc("cli.deps.col-latency"),
			loc("cli.deps.col-message"),
		},
	}
	for _, st := range r.results {
		data = append(data, []string{
			st.Name,
			st.Category,
			fmt.Sprintf("%t", st.Configured),
			loc("cli.deps.value." + st.State()),
			st.Latency.String(),
			st.Err,
		})
	}
	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}
