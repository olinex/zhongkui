package periodicreport

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	// 第三方
	"github.com/samber/do/v2"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/ai/agentkit"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	skillpkg "zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	managerconfig "zhongkuisec.com/zhongkui/internal/manager/config"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
	"zhongkuisec.com/zhongkui/internal/provider"
)

const (
	// periodicReportTitleMaxBytes 是运行记录标题的最大字节数，与 periodic_report_runs.title 列宽保持一致。
	periodicReportTitleMaxBytes = 256

	// interruptedReason 是报告生成因需要人工确认而中断时的失败原因。
	interruptedReason = "报告生成因需要人工确认而中断"

	// preemptedReason 是本轮生成被同会话新一轮触发抢占、未产出终态回答时的失败原因。
	preemptedReason = "报告生成被新一轮触发抢占，未产出结果"

	// reportAttempts 是报告输出不符合统一 JSON 信封时允许的最大生成次数（含首次）。
	reportAttempts = 3
)

// PeriodicReportRunner 执行一次 Agent 生成，生产实现包装 AI 运行时；测试注入假实现。
type PeriodicReportRunner interface {
	// Run 执行一次报告生成并返回最终正文或中断状态。
	Run(ctx context.Context, in RunInput) (RunOutput, error)
}

// RunInput 是一次报告生成的输入。
type RunInput struct {
	// Plan 待执行的报告计划。
	Plan *modelsai.PeriodicReportPlan
	// Skill 计划使用的技能（含解密后的正文）。
	Skill *modelsai.AgentSkill
	// Query 送入 Agent 的中文提示词（重试时为纠错提示）。
	Query string
	// TriggerAt 本次运行的触发时间点，用于为独立会话生成可辨识的标题。
	TriggerAt time.Time
	// SessionID 本次运行已创建的会话 ID，0 表示尚未创建（每次运行独立建会话，不复用其它运行）。
	SessionID uint
	// SystemHint 本轮注入的系统级输出契约，追加在系统提示词末尾。
	SystemHint string
	// Automation 是无人值守运行上下文：周期报告重试沿用同一个调用器，凭据只在内存中。
	Automation *agentkit.Automation
}

// RunOutput 是一次报告生成的结果。
type RunOutput struct {
	// SessionID 本次运行使用的会话 ID，供调用方回写运行记录。
	SessionID uint
	// Content 生成的 Markdown 报告正文。
	Content string
	// Interrupted 表示生成因需要人工确认而中断。
	Interrupted bool
	// Reason 中断或失败原因。
	Reason string
}

// componentsRunner 是基于 AI 运行时快照的 PeriodicReportRunner 生产实现。
type componentsRunner struct {
	// comps 惰性返回当前 AI 运行时组件快照；运行时会在配置变更时重建，不能缓存其指针。
	comps func() (*ai.Components, error)
}

// Run 使用当前 AI 运行时执行一次 Agent 生成：为本次运行创建独立会话（重试复用同一会话），
// 调用 turns.Manager.Chat，排空回放与事件流并记录本轮终态，检查会话是否挂起，最后按终态
// 事件加载本轮落库的助手消息作为报告正文。独立会话使同一计划的多份报告可以并发生成，
// 不会因共享会话而被新一轮触发抢占。
func (r *componentsRunner) Run(ctx context.Context, in RunInput) (RunOutput, error) {
	comps, err := r.comps()
	if err != nil {
		return RunOutput{}, errors.WithStack(err)
	}
	if comps == nil || comps.Turns == nil || comps.Sessions == nil || comps.Skills == nil {
		return RunOutput{}, errors.New("periodic report run: AI runtime is unavailable")
	}
	if in.Plan == nil || in.Skill == nil {
		return RunOutput{}, errors.New("periodic report run: plan or skill is nil")
	}
	sessionID, err := ensureRunSession(ctx, comps.Sessions, in.Plan, in.TriggerAt, in.SessionID)
	if err != nil {
		return RunOutput{}, err
	}
	stream, err := comps.Turns.Chat(ctx, turns.ChatInput{
		SessionID:        sessionID,
		UserID:           in.Plan.UserID,
		NamespaceName:    in.Plan.NamespaceName,
		Query:            in.Query,
		SkillID:          in.Plan.SkillID,
		SkillName:        in.Skill.Name,
		SkillDescription: in.Skill.Description,
		SkillContent:     string(in.Skill.Content.Value()),
		SystemHint:       in.SystemHint,
		Automation:       in.Automation,
	})
	if err != nil {
		return RunOutput{SessionID: sessionID}, errors.WithStack(err)
	}
	// Replay 是订阅前已缓冲的条目，必须与实时通道一起消费：生成被抢占时终态
	// turn.preempted 只会落在缓冲里，且本轮不会产生 turn.completed。
	terminal := consumeTurnTerminal(stream.Replay, stream.Events)
	if stream.Close != nil {
		stream.Close()
	}
	state, err := comps.Turns.State(ctx, sessionID, in.Plan.UserID, in.Plan.NamespaceName)
	if err != nil {
		return RunOutput{SessionID: sessionID}, errors.WithStack(err)
	}
	if state.Interrupted {
		// 后台无人应答：best-effort 作废挂起现场，避免本次独立会话长期停留在等待输入状态。
		_ = comps.Turns.Abort(ctx, sessionID, in.Plan.UserID, in.Plan.NamespaceName)
		return RunOutput{SessionID: sessionID, Interrupted: true, Reason: interruptedReason}, nil
	}
	// 只接受本轮终态事件指向的助手消息：同一运行的重试会复用会话，此前尝试的回答仍然存在，
	// 直接取会话最近一条助手消息可能把上一条回答当成本轮产出。
	if terminal == nil {
		return RunOutput{SessionID: sessionID}, errors.New("periodic report run: " + preemptedReason)
	}
	if terminal.Phase != modelsai.AgentRunPhaseDone || terminal.MessageID == 0 {
		return RunOutput{SessionID: sessionID}, errors.Newf(
			"periodic report run: the turn ended with phase %q without a final answer", string(terminal.Phase))
	}
	msg, err := comps.Sessions.GetMessage(ctx, terminal.MessageID, in.Plan.NamespaceName, in.Plan.UserID)
	if err != nil {
		return RunOutput{SessionID: sessionID}, errors.WithStack(err)
	}
	// 空回答不在此判失败：由调用方按统一信封契约校验并回传纠错，避免空洞回答直接终结运行。
	content := strings.TrimSpace(string(msg.Content.Value()))
	return RunOutput{SessionID: sessionID, Content: content}, nil
}

// consumeTurnTerminal 排空本轮的回放与实时条目，返回最后一条 turn.completed 事件的载荷。
// 生成被抢占或挂起时本轮不会下发 turn.completed，此时返回 nil，由调用方按未产出处理。
func consumeTurnTerminal(replay []turns.StreamItem, events <-chan turns.StreamItem) *assistant.TurnCompletedPayload {
	var terminal *assistant.TurnCompletedPayload
	consume := func(item turns.StreamItem) {
		if item.Event == nil || item.Event.Type != assistant.EventTypeTurnCompleted {
			return
		}
		switch payload := item.Event.Payload.(type) {
		case assistant.TurnCompletedPayload:
			value := payload
			terminal = &value
		case *assistant.TurnCompletedPayload:
			if payload != nil {
				terminal = payload
			}
		}
	}
	for _, item := range replay {
		consume(item)
	}
	for item := range events {
		consume(item)
	}
	return terminal
}

// ensureRunSession 确保本次运行的独立会话可用：未关联会话时新建一个以触发时间命名的会话，
// 关联的会话已不存在时重新创建，返回实际使用的会话 ID。会话不跨运行复用，使同一计划的多份
// 报告可以并发生成。
func ensureRunSession(ctx context.Context, sessions *session.Store, plan *modelsai.PeriodicReportPlan, triggerAt time.Time, sessionID uint) (uint, error) {
	if sessionID != 0 {
		if _, err := sessions.GetSession(ctx, sessionID, plan.NamespaceName, plan.UserID); err == nil {
			return sessionID, nil
		} else if !errors.Is(err, session.ErrSessionNotFound) {
			return 0, errors.WithStack(err)
		}
	}
	sess, err := sessions.CreateSessionWithOrigin(ctx, plan.NamespaceName, runSessionTitle(plan, triggerAt), "",
		modelsai.AgentKindKnowledge, modelsai.SessionOriginPeriodicReport, plan.UserID)
	if err != nil {
		return 0, errors.WithStack(err)
	}
	return sess.ID, nil
}

// runSessionTitle 生成运行会话标题：计划名 + 触发时间，便于在会话列表中区分同一计划的不同运行。
func runSessionTitle(plan *modelsai.PeriodicReportPlan, triggerAt time.Time) string {
	if triggerAt.IsZero() {
		return "报告计划：" + plan.Name
	}
	return "报告计划：" + plan.Name + " " + triggerAt.Format("2006-01-02 15:04")
}

// objectStore 抽象报告对象的写入与删除能力，生产实现为 *s3.ObjectCache，测试注入假实现。
type objectStore interface {
	// Store 以流式方式写入指定对象键对应的报告正文。
	Store(ctx context.Context, objectKey string, r io.Reader) error
	// Remove 删除指定对象键对应的报告对象。
	Remove(ctx context.Context, objectKey string) error
}

// accessKeySource 抽象属主可用访问密钥的查询能力，生产实现为 *core.AccessKeyPool，测试注入假实现。
type accessKeySource interface {
	// FirstUsableByUser 返回指定用户第一条启用且未过期的访问密钥，不存在时返回 (nil, nil)。
	FirstUsableByUser(ctx context.Context, userID uint) (*core.AccessKey, error)
}

// RunHandler 消费 periodic-report:plan:run 任务：按触发时间点加载计划与技能，调用 Agent 生成报告并
// 上传对象存储，最终把运行记录收敛为成功或失败。
type RunHandler struct {
	// store 报告领域存储（计划与运行记录读写）。
	store *periodic.Store
	// runner 执行一次 Agent 生成。
	runner PeriodicReportRunner
	// objects 报告对象写入。
	objects objectStore
	// logger 日志记录器。
	logger *logging.Logger
	// comps 惰性返回当前 AI 运行时组件快照，用于按 ID 加载技能；运行时重建后仍取最新快照。
	comps func() (*ai.Components, error)
	// keys 查询计划属主当前可用的访问密钥，用于本次无人值守运行的签名身份。
	keys accessKeySource
	// apiBaseURL 是 Web API 的内部基地址，自动调用据此拼接路径。
	apiBaseURL string
	// now 当前时间来源，测试可替换。
	now func() time.Time
}

// 编译期断言：确保 RunHandler 实现了 etl.Handler 接口。
var _ etl.Handler = (*RunHandler)(nil)

// NewRunHandler 从 do.Injector 解析依赖并创建报告生成处理器。AI 运行时以回调形式惰性持有，
// 每次任务执行时取当前快照，不固化启动瞬间的组件指针。
func NewRunHandler(i do.Injector) *RunHandler {
	db := provider.MustInvokeCoreDatabase(i)
	objects := provider.MustInvokeObjectCache(i)
	logger := provider.MustInvokeDatabaseLogger(i)
	v := provider.MustInvokeViper(i)
	cfg := managerconfig.MustExtractConfig(v, validator.Validate)
	// provider 装配发生在启动阶段，此处无请求上下文，用 Background 作为一次性的装配上下文。
	rt := provider.MustInvokeAi(context.Background(), i)
	comps := func() (*ai.Components, error) {
		current := rt.Current()
		if current == nil {
			return nil, errors.New("periodic report run: AI runtime is unavailable")
		}
		return current, nil
	}
	store, err := periodic.NewStore(db)
	if err != nil {
		panic(err)
	}
	return &RunHandler{
		store:      store,
		runner:     &componentsRunner{comps: comps},
		objects:    objects,
		logger:     logger,
		comps:      comps,
		keys:       provider.MustInvokeAccessKeyPool(i),
		apiBaseURL: cfg.InternalBaseURL,
		now:        time.Now,
	}
}

// Do 消费一条 periodic-report:plan:run 任务。
// 流程：解析触发时间与计划 ID -> 加载计划 -> 幂等创建运行记录 -> 校验计划与技能资格 ->
// 调用 Agent 生成 -> 上传对象 -> 收敛运行记录。计划不存在、重复投递与资格校验失败均返回 nil
// （任务无需重试）；生成、上传或落库失败返回带堆栈的错误。
func (h *RunHandler) Do(ctx context.Context, task etl.Task) error {
	triggerAt, payload, err := pipeline.ParseCursorTaskPayload(task.Payload())
	if err != nil {
		return errors.WithStack(err)
	}
	if triggerAt.IsZero() {
		return errors.New("periodic report run: the cursor task payload is missing triggerAt")
	}
	planID, err := parsePlanID(payload)
	if err != nil {
		return err
	}
	plan, err := h.store.GetPlanByID(ctx, planID)
	if err != nil {
		if errors.Is(err, periodic.ErrPlanNotFound) {
			h.logger.Ctx(ctx).Warnf("periodic report run: plan %d not found, skip", planID)
			return nil
		}
		return errors.WithStack(err)
	}
	nowTime := h.now()
	run := &modelsai.PeriodicReportRun{
		NamespaceName: plan.NamespaceName,
		PlanID:        plan.ID,
		TriggerAt:     triggerAt,
		Status:        modelsai.PeriodicReportRunStatusRunning,
		StartedAt:     &nowTime,
	}
	created, err := h.store.CreateRunIfAbsent(ctx, run)
	if err != nil {
		return errors.WithStack(err)
	}
	if !created {
		h.logger.Ctx(ctx).Infof("periodic report run: plan %d trigger %s already dispatched, skip",
			plan.ID, triggerAt.Format(time.RFC3339))
		return nil
	}
	if reason, skip := eligibilityReason(plan, nowTime); skip {
		return h.fail(ctx, run.ID, reason, nowTime)
	}
	// 无人值守运行以计划属主的访问密钥作为身份自动调用 Web API；密钥每次运行实时查询。
	key, err := h.keys.FirstUsableByUser(ctx, plan.UserID)
	if err != nil {
		return errors.WithStack(err)
	}
	if key == nil {
		return h.fail(ctx, run.ID,
			"no usable access key for the plan owner; create or enable an access key in account settings and retry", nowTime)
	}
	// 密钥只在内存中流转，不写日志、不落库；同一运行内所有重试复用同一个签名调用器。
	invoker := newAPIInvoker(h.apiBaseURL, key.AccessKeyID, key.Secret())
	automation := &agentkit.Automation{InvokeAPI: invoker.Invoke}
	skill, reason, skip := h.loadSkill(ctx, plan)
	if skip {
		return h.fail(ctx, run.ID, reason, nowTime)
	}
	inputs, err := periodic.DecodeInputValues(plan.InputValues)
	if err != nil {
		return h.fail(ctx, run.ID, "plan input values are invalid: "+err.Error(), nowTime)
	}
	if err := periodic.ValidatePlanBias(skill, inputs); err != nil {
		return h.fail(ctx, run.ID, "plan bias parameters are invalid: "+err.Error(), nowTime)
	}
	fixedFields, err := modelsai.ParseSkillFixedFields(skill.FixedFields)
	if err != nil {
		return h.fail(ctx, run.ID, "skill fixed fields cannot be parsed: "+err.Error(), nowTime)
	}
	effective, err := buildEffectiveParameters(fixedFields, inputs, triggerAt)
	if err != nil {
		return h.fail(ctx, run.ID, "skill time window is invalid: "+err.Error(), nowTime)
	}
	objectKey := fmt.Sprintf("periodic-report/%s/%d/%s.md", plan.NamespaceName, plan.ID, triggerAt.UTC().Format("20060102T150405Z"))
	// 报告输出必须符合统一 JSON 信封；校验失败时把原因回传模型重试，最多 reportAttempts 次。
	// 每次运行独立创建会话，重试沿用本次运行刚创建的会话，不跨运行复用。
	sessionID := uint(0)
	query := buildPeriodicReportQuery(plan, skill, triggerAt, effective)
	var (
		output   RunOutput
		envelope periodic.ReportEnvelope
		parseErr error
	)
	for attempt := 1; attempt <= reportAttempts; attempt++ {
		output, err = h.runner.Run(ctx, RunInput{
			Plan:       plan,
			Skill:      skill,
			Query:      query,
			TriggerAt:  triggerAt,
			SessionID:  sessionID,
			SystemHint: periodic.ReportSystemHint,
			Automation: automation,
		})
		if output.SessionID != 0 && output.SessionID != sessionID {
			sessionID = output.SessionID
			if setErr := h.store.SetRunSession(ctx, run.ID, sessionID); setErr != nil {
				h.logger.Ctx(ctx).Warnf("periodic report run: persist session %d for run %d failed: %v",
					sessionID, run.ID, setErr)
			}
		}
		if err != nil {
			h.logRunFailure(ctx, run.ID, err)
			return errors.WithStack(err)
		}
		if output.Interrupted {
			if err := h.fail(ctx, run.ID, output.Reason, nowTime); err != nil {
				return err
			}
			return nil
		}
		envelope, parseErr = periodic.ParseReportEnvelope(output.Content)
		if parseErr == nil {
			break
		}
		h.logger.Ctx(ctx).Warnf("periodic report run: attempt %d/%d output violates the envelope contract: %v",
			attempt, reportAttempts, parseErr)
		if attempt == reportAttempts {
			return h.fail(ctx, run.ID, "report output violates the output contract: "+parseErr.Error(), nowTime)
		}
		query = buildPeriodicReportCorrectionQuery(parseErr.Error())
	}
	content := strings.TrimSpace(output.Content)
	if err := h.objects.Store(ctx, objectKey, strings.NewReader(content)); err != nil {
		if markErr := h.store.MarkRunFailed(ctx, run.ID, err.Error(), nowTime); markErr != nil {
			h.logger.Ctx(ctx).Warnf("periodic report run: mark run %d failed: %v", run.ID, markErr)
		}
		return errors.WithStack(err)
	}
	// 标题取自信封内的 Markdown 正文，落库内容保留完整信封供下游消费。
	title := periodicReportTitle(envelope.Content, plan, triggerAt)
	updated, err := h.store.MarkRunSuccess(ctx, run.ID, objectKey, title, int64(len(content)), nowTime)
	if err != nil {
		return errors.WithStack(err)
	}
	// 是否仍存在以数据库更新结果为准：运行记录可能在生成期间被用户删除，此时更新不会命中，
	// 需要删除刚写入的对象。不能在写入前做存在性检查——检查与写入之间同样可能被删除。
	if !updated {
		if removeErr := h.objects.Remove(ctx, objectKey); removeErr != nil {
			h.logger.Ctx(ctx).Warnf("periodic report run: remove orphan object %s for deleted run %d failed: %v",
				objectKey, run.ID, removeErr)
		}
		h.logger.Ctx(ctx).Infof("periodic report run: run %d was deleted during generation, removed object %s", run.ID, objectKey)
		return nil
	}
	h.logger.Ctx(ctx).Infof("periodic report run: plan %d generated report %s (%d bytes)", plan.ID, objectKey, len(content))
	return nil
}

// eligibilityReason 判断计划本身是否仍可执行；返回原因与是否需要跳过。
func eligibilityReason(plan *modelsai.PeriodicReportPlan, now time.Time) (string, bool) {
	if !plan.Enabled {
		return "报告计划已停用", true
	}
	if plan.ExpiresAt != nil && now.After(*plan.ExpiresAt) {
		return "报告计划已到期", true
	}
	return "", false
}

// loadSkill 加载并校验计划使用的技能：技能必须存在、属于计划命名空间、为自定义且启用、可周期
// 执行；固定参数须通过周期契约校验（timeOffset/timeWindow 合法、不含运行时保留键），输入字段
// 须恰好含一个 name=nowTime、type=date 的字段。返回加载到的技能、失败原因与是否需要跳过。
func (h *RunHandler) loadSkill(ctx context.Context, plan *modelsai.PeriodicReportPlan) (*modelsai.AgentSkill, string, bool) {
	comps, err := h.comps()
	if err != nil {
		return nil, "AI 运行时不可用: " + err.Error(), true
	}
	if comps == nil || comps.Skills == nil {
		return nil, "AI 运行时不可用", true
	}
	skill, err := comps.Skills.GetByID(ctx, plan.SkillID)
	if err != nil {
		return nil, "加载技能失败: " + err.Error(), true
	}
	if skill.NamespaceName != plan.NamespaceName {
		return nil, "技能不属于计划所在命名空间", true
	}
	if skill.Source != modelsai.SkillSourceCustom {
		return nil, "技能不是自定义技能", true
	}
	if !skill.Enabled {
		return nil, "技能已停用", true
	}
	if !skill.PeriodicCapable {
		return nil, "技能不支持周期执行", true
	}
	fields, err := modelsai.ParseSkillInputFields(skill.InputFields)
	if err != nil {
		return nil, "skill input fields cannot be parsed: " + err.Error(), true
	}
	fixedFields, err := modelsai.ParseSkillFixedFields(skill.FixedFields)
	if err != nil {
		return nil, "skill fixed fields cannot be parsed: " + err.Error(), true
	}
	if err := skillpkg.ValidateInputFields(fields); err != nil {
		return nil, "skill input fields are invalid: " + err.Error(), true
	}
	if err := modelsai.ValidateSkillFixedFields(fixedFields, true, fields); err != nil {
		return nil, "skill fixed fields are invalid: " + err.Error(), true
	}
	if _, err := modelsai.ParsePeriodicWindowFromFields(fixedFields); err != nil {
		return nil, "skill fixed fields have no valid time window: " + err.Error(), true
	}
	if !hasUniqueNowTimeField(fields) {
		return nil, "skill input fields must contain exactly one nowTime field", true
	}
	return skill, "", false
}

// fail 把运行记录标记为失败并记录告警；返回标记失败时的错误（正常情况下为 nil）。
func (h *RunHandler) fail(ctx context.Context, runID uint, reason string, at time.Time) error {
	if err := h.store.MarkRunFailed(ctx, runID, reason, at); err != nil {
		return errors.WithStack(err)
	}
	h.logger.Ctx(ctx).Warnf("periodic report run: run %d failed: %s", runID, reason)
	return nil
}

// logRunFailure 在报告生成失败时把原因写入运行记录；标记失败本身出错只记录告警，
// 保留生成错误的原始堆栈返回给调用方。
func (h *RunHandler) logRunFailure(ctx context.Context, runID uint, err error) {
	if markErr := h.store.MarkRunFailed(ctx, runID, err.Error(), h.now()); markErr != nil {
		h.logger.Ctx(ctx).Warnf("periodic report run: mark run %d failed: %v", runID, markErr)
	}
	h.logger.Ctx(ctx).Warnf("periodic report run: run %d failed: %v", runID, err)
}

// hasUniqueNowTimeField 判断输入字段列表中是否恰好有一个 name=nowTime、type=date 的字段。
func hasUniqueNowTimeField(fields []modelsai.SkillInputField) bool {
	count := 0
	for _, field := range fields {
		if field.Name != modelsai.SkillInputFieldNameNowTime {
			continue
		}
		if field.Type != modelsai.SkillInputFieldTypeDate {
			return false
		}
		count++
	}
	return count == 1
}

// buildEffectiveParameters 合并技能固定参数与计划偏置，得到基础参数，再按合并后的有效
// timeOffset/timeWindow 计算窗口：nowTime = triggerAt，startTime = nowTime + timeOffset，
// endTime = startTime + timeWindow。运行时时间参数最后合并，覆盖同名固定参数与偏置。
func buildEffectiveParameters(fixedFields, bias map[string]json.RawMessage, triggerAt time.Time) (map[string]json.RawMessage, error) {
	base := modelsai.MergeParameters(fixedFields, bias)
	window, err := modelsai.ParsePeriodicWindowFromFields(base)
	if err != nil {
		return nil, err
	}
	startTime := triggerAt.Add(window.TimeOffset)
	endTime := startTime.Add(window.TimeWindow)
	runtime := map[string]json.RawMessage{
		modelsai.SkillInputFieldNameNowTime: jsonStringRaw(triggerAt.Format(time.RFC3339)),
		"startTime":                         jsonStringRaw(startTime.Format(time.RFC3339)),
		"endTime":                           jsonStringRaw(endTime.Format(time.RFC3339)),
	}
	return modelsai.MergeParameters(base, runtime), nil
}

// jsonStringRaw 把字符串编码为 JSON 字符串原始值；编码失败时回退为空字符串。
func jsonStringRaw(value string) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return raw
}

// buildPeriodicReportQuery 构造送入 Agent 的中文提示词：包含计划名称、技能名称、触发时间、
// 合并后的有效参数（紧凑 JSON，保留原始类型）与 nowTime/startTime/endTime，并要求按系统输出
// 契约只回复统一 JSON 信封。
func buildPeriodicReportQuery(plan *modelsai.PeriodicReportPlan, skill *modelsai.AgentSkill, triggerAt time.Time, params map[string]json.RawMessage) string {
	skillName := ""
	if skill != nil {
		skillName = skill.Name
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		paramsJSON = []byte("{}")
	}
	return fmt.Sprintf(
		"请依据以下报告计划生成一份报告：\n"+
			"- 计划名称：%s\n"+
			"- 技能名称：%s\n"+
			"- 触发时间：%s\n"+
			"- nowTime：%s\n"+
			"- startTime：%s\n"+
			"- endTime：%s\n"+
			"- 有效参数：%s\n\n"+
			"请严格遵循技能指引完成取数与撰写，并按系统输出契约只回复一个 JSON 对象："+
			"content 放完整的 Markdown 报告正文，draft 放结构化载荷（没有则输出 {}），不要输出其他文字。",
		plan.Name, skillName, triggerAt.Format(time.RFC3339),
		effectiveStringValue(params, modelsai.SkillInputFieldNameNowTime),
		effectiveStringValue(params, "startTime"),
		effectiveStringValue(params, "endTime"),
		string(paramsJSON),
	)
}

// effectiveStringValue 读取有效参数中的字符串值；缺失或非字符串时返回空串。
func effectiveStringValue(params map[string]json.RawMessage, key string) string {
	raw, ok := params[key]
	if !ok {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return value
}

// buildPeriodicReportCorrectionQuery 构造信封校验失败后的纠错提示：把具体原因回传模型，
// 要求重新输出完整且合法的统一 JSON 信封，不截断、不改写。
func buildPeriodicReportCorrectionQuery(cause string) string {
	return fmt.Sprintf(
		"上一条回答不符合报告输出契约：%s\n"+
			"请重新输出，且只输出一个 JSON 对象：{\"content\":\"<完整的 Markdown 报告正文>\",\"draft\":{<结构化载荷>}}。\n"+
			"要求：content 必须是非空的完整 Markdown 报告正文；draft 没有内容时输出 {}；"+
			"不要输出任何解释、前后缀文字或 ```json 代码块围栏。",
		cause,
	)
}

// periodicReportTitle 从报告正文提取标题：取首个以「# 」开头的行，去掉井号与空白并截断到 256 字节；
// 没有 Markdown 一级标题时回退为「计划名称 + 触发时间」。
func periodicReportTitle(content string, plan *modelsai.PeriodicReportPlan, triggerAt time.Time) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "# ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if title != "" {
			return truncateUTF8Bytes(title, periodicReportTitleMaxBytes)
		}
	}
	return truncateUTF8Bytes(plan.Name+" "+triggerAt.Format("2006-01-02 15:04"), periodicReportTitleMaxBytes)
}

// truncateUTF8Bytes 把字符串截断到最多 limit 字节，并回退到最近的合法 UTF-8 边界。
func truncateUTF8Bytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	truncated := []byte(value)[:limit]
	for len(truncated) > 0 && !utf8.Valid(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return string(truncated)
}
