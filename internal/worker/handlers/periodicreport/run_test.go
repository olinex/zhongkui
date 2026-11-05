package periodicreport

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/ai/assistant"
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/ai/turns"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/etl"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// 报告处理器测试专用命名空间，与迁移种子的 default 隔离。
const (
	// periodicReportHandlerTestNamespace 报告处理器测试的主命名空间。
	periodicReportHandlerTestNamespace = "reportworker"
	// periodicReportHandlerOtherNamespace 跨命名空间隔离用例的另一个命名空间。
	periodicReportHandlerOtherNamespace = "reportworker2"

	// periodicReportTestInputFields 是可周期执行技能的输入字段 JSON：含运行时注入的 nowTime、
	// 一个必填文本字段与一个可选文本字段。
	periodicReportTestInputFields = `[{"name":"nowTime","type":"date","required":true,"description":"运行时注入"},{"name":"region","type":"string","required":true,"description":"区域"},{"name":"note","type":"string","required":false,"description":"补充说明"}]`
	// periodicReportTestFixedFields 是可周期执行技能的固定参数 JSON：含合法时间窗口与混合类型固定值。
	periodicReportTestFixedFields = `{"timeOffset":"-24h","timeWindow":"24h","threshold":0.5,"onlyHigh":true}`
)

// periodicReportTaskStub 实现 etl.Task 的最小桩，用于向处理器注入指定载荷。
type periodicReportTaskStub struct {
	// typ 任务类型
	typ string
	// payload 任务载荷
	payload []byte
}

// 编译期断言：确保 periodicReportTaskStub 实现了 etl.Task 接口。
var _ etl.Task = (*periodicReportTaskStub)(nil)

// Type 返回任务类型标识。
func (t *periodicReportTaskStub) Type() string { return t.typ }

// Payload 返回任务载荷。
func (t *periodicReportTaskStub) Payload() []byte { return t.payload }

// fakePeriodicReportRunner 记录 Run 调用并返回预设结果的测试替身。
type fakePeriodicReportRunner struct {
	// calls 调用次数
	calls int
	// input 最近一次输入
	input RunInput
	// inputs 按调用次序记录的输入，用于验证每次重试都携带无人值守上下文
	inputs []RunInput
	// output 预设输出（outputs 用尽后返回）
	output RunOutput
	// outputs 按调用次序返回的预设输出，用于覆盖「首次不合规、纠错后合规」的重试路径
	outputs []RunOutput
	// onRun 在返回结果前调用，用于模拟生成期间的外部变更（如删除运行记录）
	onRun func()
	// err 预设错误
	err error
}

// Run 记录一次调用并返回预设结果：优先按调用次序取 outputs，用尽后回退 output。
func (f *fakePeriodicReportRunner) Run(_ context.Context, in RunInput) (RunOutput, error) {
	index := f.calls
	f.calls++
	f.input = in
	f.inputs = append(f.inputs, in)
	if f.onRun != nil {
		f.onRun()
	}
	if f.err != nil {
		return RunOutput{}, f.err
	}
	if index < len(f.outputs) {
		return f.outputs[index], nil
	}
	return f.output, nil
}

// fakeReportObjects 记录对象写入与删除的测试替身。
type fakeReportObjects struct {
	// calls 写入次数
	calls int
	// keys 写入的对象键
	keys []string
	// contents 写入的正文
	contents []string
	// removed 删除的对象键
	removed []string
	// err 预设写入错误
	err error
	// removeErr 预设删除错误
	removeErr error
}

// Store 记录一次对象写入并返回预设错误（可为 nil）。
func (f *fakeReportObjects) Store(_ context.Context, objectKey string, r io.Reader) error {
	f.calls++
	if f.err != nil {
		return f.err
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	f.keys = append(f.keys, objectKey)
	f.contents = append(f.contents, string(data))
	return nil
}

// Remove 记录一次对象删除并返回预设错误（可为 nil）。
func (f *fakeReportObjects) Remove(_ context.Context, objectKey string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, objectKey)
	return nil
}

// fakeAccessKeySource 是可用访问密钥查询的测试替身，返回预设密钥或错误。
type fakeAccessKeySource struct {
	// key 预设返回的访问密钥，nil 表示无可用密钥
	key *core.AccessKey
	// err 预设错误
	err error
}

// FirstUsableByUser 返回预设的访问密钥或错误。
func (f *fakeAccessKeySource) FirstUsableByUser(_ context.Context, _ uint) (*core.AccessKey, error) {
	return f.key, f.err
}

// testAccessKey 构造测试用的伪造访问密钥。
func testAccessKey() *core.AccessKey {
	return &core.AccessKey{
		AccessKeyID:     "test-ak",
		AccessKeySecret: serializer.NewCrypto([]byte("test-secret")),
	}
}

// periodicReportRunTestDB 返回共享测试库事务并确保测试命名空间存在。
func periodicReportRunTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := testdb.Core(t)
	for _, name := range []string{periodicReportHandlerTestNamespace, periodicReportHandlerOtherNamespace} {
		if err := db.Exec(
			`INSERT INTO namespaces (name, description) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`,
			name, "report worker test namespace",
		).Error; err != nil {
			t.Fatalf("seed namespace %q: %v", name, err)
		}
	}
	// periodic_report_plans.user_id 为 users(id) 外键，测试计划统一使用属主 1，先确保该用户存在。
	if err := db.Exec(
		`INSERT INTO users (id, union_id, nickle_name, username, password, email)
		 VALUES (1, '00000000-0000-0000-0000-000000000102', 'report-worker-test', 'report-worker-test-user', '\x00'::bytea, 'report-worker-test@example.com')
		 ON CONFLICT DO NOTHING`,
	).Error; err != nil {
		t.Fatalf("seed report worker test user: %v", err)
	}
	return db
}

// insertPeriodicReportSkill 插入一条技能行并返回 ID；正文留空，测试无需初始化加密密钥。
func insertPeriodicReportSkill(t *testing.T, db *database.DB, namespace, name string, enabled, periodicCapable bool) uint {
	t.Helper()
	return insertPeriodicReportSkillWithFields(t, db, namespace, name, enabled, periodicCapable,
		periodicReportTestInputFields, periodicReportTestFixedFields)
}

// insertPeriodicReportSkillWithFields 插入一条指定输入字段与固定参数的技能行并返回 ID。
func insertPeriodicReportSkillWithFields(t *testing.T, db *database.DB, namespace, name string, enabled, periodicCapable bool, inputFields, fixedFields string) uint {
	t.Helper()
	var id uint
	if err := db.Raw(
		`INSERT INTO agent_skills (namespace_name, source, name, description, content, enabled, periodic_capable, input_fields, fixed_fields)
		 VALUES (?, 'custom', ?, '', NULL, ?, ?, ?::jsonb, ?::jsonb) RETURNING id`,
		namespace, name, enabled, periodicCapable, inputFields, fixedFields,
	).Scan(&id).Error; err != nil {
		t.Fatalf("insert skill %q: %v", name, err)
	}
	return id
}

// insertPeriodicReportPlan 插入一条报告计划并返回 ID；expiresAt 为 nil 表示长期有效。
func insertPeriodicReportPlan(t *testing.T, db *database.DB, namespace string, skillID uint, skillName, name string, enabled bool, expiresAt *time.Time) uint {
	t.Helper()
	return insertPeriodicReportPlanWithValues(t, db, namespace, skillID, name, enabled, expiresAt, `{"region":"cn"}`)
}

// insertPeriodicReportPlanWithValues 插入一条带指定 input_values 的报告计划并返回 ID。
func insertPeriodicReportPlanWithValues(t *testing.T, db *database.DB, namespace string, skillID uint, name string, enabled bool, expiresAt *time.Time, inputValues string) uint {
	t.Helper()
	var expires any
	if expiresAt != nil {
		expires = *expiresAt
	}
	var id uint
	if err := db.Raw(
		`INSERT INTO periodic_report_plans
		 (namespace_name, name, description, skill_id, cron_spec, input_values, enabled, user_id, retention_days, expires_at)
		 VALUES (?, ?, '', ?, '0 * * * *', ?::jsonb, ?, 1, 1, ?) RETURNING id`,
		namespace, name, skillID, inputValues, enabled, expires,
	).Scan(&id).Error; err != nil {
		t.Fatalf("insert report plan %q: %v", name, err)
	}
	return id
}

// periodicReportRunTaskPayload 编码一条携带触发时间与计划 ID 的游标任务载荷。
func periodicReportRunTaskPayload(t *testing.T, planID uint, triggerAt time.Time) []byte {
	t.Helper()
	raw, err := json.Marshal(pipeline.CursorTaskPayload{
		TriggerAt: triggerAt,
		Payload:   map[string]any{"planId": planID},
	})
	if err != nil {
		t.Fatalf("marshal task payload: %v", err)
	}
	return raw
}

// newRunHandler 构造注入假依赖的报告生成处理器；comps 返回真实测试库上的技能存储。
func newRunHandler(t *testing.T, db *database.DB, runner PeriodicReportRunner, objects objectStore, now time.Time) *RunHandler {
	t.Helper()
	store, err := periodic.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	skills, err := skill.NewStore(db)
	if err != nil {
		t.Fatalf("skill.NewStore error = %v", err)
	}
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("NewDefaultLogger error = %v", err)
	}
	return &RunHandler{
		store:   store,
		runner:  runner,
		objects: objects,
		logger:  logger,
		comps: func() (*ai.Components, error) {
			return &ai.Components{Skills: skills}, nil
		},
		keys:       &fakeAccessKeySource{key: testAccessKey()},
		apiBaseURL: "http://127.0.0.1:8080",
		now:        func() time.Time { return now },
	}
}

// listTestRuns 读取指定计划的全部运行记录。
func listTestRuns(t *testing.T, db *database.DB, namespace string, planID uint) []*modelsai.PeriodicReportRun {
	t.Helper()
	runs := make([]*modelsai.PeriodicReportRun, 0)
	if err := db.Where("namespace_name = ? AND plan_id = ?", namespace, planID).
		Order("created_at ASC").
		Find(&runs).Error; err != nil {
		t.Fatalf("list runs error = %v", err)
	}
	return runs
}

// TestRunHandler_Success 验证成功路径：运行记录收敛为成功、对象键/标题/大小正确且对象只写一次。
func TestRunHandler_Success(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "weekly-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "weekly-skill", "周报计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	markdown := "# 安全周报\n\n本周共拦截 12 起事件。\n"
	content := fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)
	var sessionID uint
	if err := db.Raw(
		`INSERT INTO agent_sessions (namespace_name, user_id, title, target_model, status, agent_kind, review_valve)
		 VALUES (?, 1, 'report-run-test', '', 'active', 'knowledge', 'each') RETURNING id`,
		periodicReportHandlerTestNamespace,
	).Scan(&sessionID).Error; err != nil {
		t.Fatalf("seed agent session: %v", err)
	}
	runner := &fakePeriodicReportRunner{output: RunOutput{SessionID: sessionID, Content: content}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != 1 || objects.calls != 1 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 1 and 1", runner.calls, objects.calls)
	}
	if runner.input.Query == "" || runner.input.Skill == nil {
		t.Fatalf("runner input = %+v, want query and skill", runner.input)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	run := runs[0]
	wantObjectKey := fmt.Sprintf("periodic-report/%s/%d/%s.md", periodicReportHandlerTestNamespace, planID, triggerAt.Format("20060102T150405Z"))
	if run.Status != modelsai.PeriodicReportRunStatusSuccess || run.ObjectKey != wantObjectKey ||
		run.Title != "安全周报" || run.SizeBytes != int64(len(content)) {
		t.Fatalf("run = %+v, want success with objectKey=%q title=%q size=%d",
			run, wantObjectKey, "安全周报", len(content))
	}
	if run.SessionID == nil || *run.SessionID != sessionID {
		t.Fatalf("run.SessionID = %v, want %d", run.SessionID, sessionID)
	}
	if objects.keys[0] != wantObjectKey || objects.contents[0] != content {
		t.Fatalf("stored object = %q / %q, want %q / %q", objects.keys[0], objects.contents[0], wantObjectKey, content)
	}
}

// TestRunHandler_RetriesInvalidEnvelope 验证首次输出不符合统一信封时，把纠错原因回传模型重试，
// 并在同一会话中拿到合规信封后落库。
func TestRunHandler_RetriesInvalidEnvelope(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "retry-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "retry-skill", "纠错计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	var sessionID uint
	if err := db.Raw(
		`INSERT INTO agent_sessions (namespace_name, user_id, title, target_model, status, agent_kind, review_valve)
		 VALUES (?, 1, 'report-retry-test', '', 'active', 'knowledge', 'each') RETURNING id`,
		periodicReportHandlerTestNamespace,
	).Scan(&sessionID).Error; err != nil {
		t.Fatalf("seed agent session: %v", err)
	}
	markdown := "# 纠错后的报告\n\n正文。\n"
	valid := fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)
	runner := &fakePeriodicReportRunner{outputs: []RunOutput{
		{SessionID: sessionID, Content: "# 直接输出 Markdown，不合规"},
		{SessionID: sessionID, Content: valid},
	}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != 2 || objects.calls != 1 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 2 and 1", runner.calls, objects.calls)
	}
	if runner.input.Query == "" || !strings.Contains(runner.input.Query, "上一条回答不符合报告输出契约") {
		t.Fatalf("retry query = %q, want a correction prompt with the failure cause", runner.input.Query)
	}
	if runner.input.SystemHint != periodic.ReportSystemHint {
		t.Fatalf("retry SystemHint = %q, want the report contract hint", runner.input.SystemHint)
	}
	if runner.input.SessionID != sessionID {
		t.Fatalf("retry SessionID = %d, want the same session %d", runner.input.SessionID, sessionID)
	}
	if objects.contents[0] != valid {
		t.Fatalf("stored object = %q, want the validated envelope %q", objects.contents[0], valid)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
	if runs[0].Status != modelsai.PeriodicReportRunStatusSuccess ||
		runs[0].Title != "纠错后的报告" || runs[0].SizeBytes != int64(len(valid)) {
		t.Fatalf("run = %+v, want success with title=%q size=%d", runs[0], "纠错后的报告", len(valid))
	}
}

// TestRunHandler_EnvelopeRetriesExhausted 验证连续不合规时运行标记失败、不写对象，且重试次数有上限。
func TestRunHandler_EnvelopeRetriesExhausted(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "invalid-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "invalid-skill", "不合规计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: "# 纯 Markdown，不合规"}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != reportAttempts {
		t.Fatalf("runner.calls = %d, want %d", runner.calls, reportAttempts)
	}
	if objects.calls != 0 {
		t.Fatalf("objects.calls = %d, want 0", objects.calls)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if !strings.Contains(runs[0].Error, "report output violates the output contract") {
		t.Fatalf("run error = %q, want the contract violation reason", runs[0].Error)
	}
}

// TestRunHandler_RemovesObjectWhenRunDeletedDuringGeneration 验证生成期间运行记录被用户删除时，
// 以 MarkRunSuccess 是否命中为准回滚刚写入的对象，不留下无主报告。
func TestRunHandler_RemovesObjectWhenRunDeletedDuringGeneration(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "deleted-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "deleted-skill", "删除竞态计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	markdown := "# 竞态报告\n\n正文。\n"
	valid := fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)
	objects := &fakeReportObjects{}
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: valid}}
	runner.onRun = func() {
		// 模拟生成期间用户删除运行记录
		if err := db.Where("plan_id = ? AND trigger_at = ?", planID, triggerAt).
			Delete(&modelsai.PeriodicReportRun{}).Error; err != nil {
			t.Fatalf("delete run during generation: %v", err)
		}
	}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if objects.calls != 1 {
		t.Fatalf("objects.calls = %d, want 1", objects.calls)
	}
	if len(objects.removed) != 1 || objects.removed[0] != objects.keys[0] {
		t.Fatalf("removed objects = %v, want the stored object %v", objects.removed, objects.keys)
	}
	if runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID); len(runs) != 0 {
		t.Fatalf("runs = %+v, want the run deleted", runs)
	}
}

// TestRunHandler_DuplicateDelivery 验证同一计划同一触发点重复投递时不重复执行与上传。
func TestRunHandler_DuplicateDelivery(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dup-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dup-skill", "重复投递计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)
	markdown := "# 报告\n正文\n"
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)
	task := &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}

	if err := handler.Do(ctx, task); err != nil {
		t.Fatalf("first Do error = %v", err)
	}
	if err := handler.Do(ctx, task); err != nil {
		t.Fatalf("second Do error = %v", err)
	}
	if runner.calls != 1 || objects.calls != 1 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 1 and 1 after duplicate delivery", runner.calls, objects.calls)
	}
	if runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID); len(runs) != 1 {
		t.Fatalf("run count = %d, want 1", len(runs))
	}
}

// TestRunHandler_Interrupted 验证生成中断时记录失败且不上传对象。
func TestRunHandler_Interrupted(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "interrupt-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "interrupt-skill", "中断计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	var sessionID uint
	if err := db.Raw(
		`INSERT INTO agent_sessions (namespace_name, user_id, title, target_model, status, agent_kind, review_valve)
		 VALUES (?, 1, 'report-interrupt-test', '', 'active', 'knowledge', 'each') RETURNING id`,
		periodicReportHandlerTestNamespace,
	).Scan(&sessionID).Error; err != nil {
		t.Fatalf("seed agent session: %v", err)
	}
	runner := &fakePeriodicReportRunner{output: RunOutput{SessionID: sessionID, Interrupted: true, Reason: interruptedReason}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if objects.calls != 0 {
		t.Fatalf("objects.calls = %d, want 0", objects.calls)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if runs[0].Error != interruptedReason {
		t.Fatalf("run error = %q, want %q", runs[0].Error, interruptedReason)
	}
	if runs[0].SessionID == nil || *runs[0].SessionID != sessionID {
		t.Fatalf("run.SessionID = %v, want %d", runs[0].SessionID, sessionID)
	}
}

// TestConsumeTurnTerminal 验证终态事件只认 turn.completed，且以最后一条为准；被抢占（没有
// turn.completed）时返回 nil，调用方据此判定本轮未产出，而不是回落到上一条旧回答。
func TestConsumeTurnTerminal(t *testing.T) {
	completed := func(phase modelsai.AgentRunPhase, messageID uint) turns.StreamItem {
		return turns.StreamItem{Event: &assistant.Event{
			Type:    assistant.EventTypeTurnCompleted,
			Payload: assistant.TurnCompletedPayload{Phase: phase, MessageID: messageID},
		}}
	}
	t.Run("取最后一条终态", func(t *testing.T) {
		events := make(chan turns.StreamItem, 1)
		close(events)
		got := consumeTurnTerminal(
			[]turns.StreamItem{completed(modelsai.AgentRunPhaseDone, 11)},
			events,
		)
		if got == nil || got.MessageID != 11 || got.Phase != modelsai.AgentRunPhaseDone {
			t.Fatalf("consumeTurnTerminal() = %+v, want the buffered done payload of message 11", got)
		}
	})
	t.Run("被抢占时无终态", func(t *testing.T) {
		events := make(chan turns.StreamItem, 1)
		events <- turns.StreamItem{Event: &assistant.Event{
			Type:    assistant.EventTypeTurnPreempted,
			Payload: assistant.TurnPreemptedPayload{ByRunID: "next-run"},
		}}
		close(events)
		if got := consumeTurnTerminal(nil, events); got != nil {
			t.Fatalf("consumeTurnTerminal() = %+v, want nil when the turn was preempted", got)
		}
	})
}

// TestRunHandler_SkipsIneligiblePlan 验证停用与到期计划记录失败且不调用生成器。
func TestRunHandler_SkipsIneligiblePlan(t *testing.T) {
	cases := []struct {
		// name 用例名称
		name string
		// enabled 计划启停状态
		enabled bool
		// expired 计划是否已到期
		expired bool
	}{
		{"计划停用", false, false},
		{"计划到期", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := periodicReportRunTestDB(t)
			ctx := context.Background()
			skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "skip-skill", true, true)
			nowTime := time.Date(2026, 10, 30, 11, 0, 0, 0, time.UTC)
			var expiresAt *time.Time
			if tc.expired {
				expiredAt := nowTime.Add(-time.Hour)
				expiresAt = &expiredAt
			}
			planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "skip-skill", "跳过计划", tc.enabled, expiresAt)
			runner := &fakePeriodicReportRunner{output: RunOutput{Content: "# 不应生成\n"}}
			objects := &fakeReportObjects{}
			handler := newRunHandler(t, db, runner, objects, nowTime)

			if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, nowTime)}); err != nil {
				t.Fatalf("Do error = %v", err)
			}
			if runner.calls != 0 || objects.calls != 0 {
				t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
			}
			runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
			if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
				t.Fatalf("runs = %+v, want one failed run", runs)
			}
			wantReason := "报告计划已停用"
			if tc.expired {
				wantReason = "报告计划已到期"
			}
			if runs[0].Error != wantReason {
				t.Fatalf("run error = %q, want %q", runs[0].Error, wantReason)
			}
		})
	}
}

// TestRunHandler_DisabledSkill 验证技能不满足资格时记录失败且不调用生成器。
func TestRunHandler_DisabledSkill(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "disabled-skill", false, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "disabled-skill", "技能停用计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: "# 不应生成\n"}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != 0 || objects.calls != 0 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
}

// TestRunHandler_PlanNotFound 验证计划不存在时只记录告警并正常结束任务。
func TestRunHandler_PlanNotFound(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	runner := &fakePeriodicReportRunner{}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, time.Now())

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, 999999, time.Now())}); err != nil {
		t.Fatalf("Do error = %v, want nil for a missing plan", err)
	}
	if runner.calls != 0 || objects.calls != 0 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
	}
}

// queryLineValue 从提示词中提取以 prefix 开头的那一行，返回去掉前缀后的值；未命中返回空串。
func queryLineValue(query, prefix string) string {
	for _, line := range strings.Split(query, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

// TestRunHandler_BiasOverridesFixedFields 验证计划偏置覆盖同名固定值，提示词只包含覆盖后的值。
func TestRunHandler_BiasOverridesFixedFields(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "bias-override-skill", true, true)
	planID := insertPeriodicReportPlanWithValues(t, db, periodicReportHandlerTestNamespace, skillID, "偏置覆盖计划",
		true, nil, `{"region":"cn","threshold":0.75,"onlyHigh":false,"note":"重点看高危事件"}`)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	markdown := "# 偏置报告\n\n正文。\n"
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	query := runner.input.Query
	for _, want := range []string{`"threshold":0.75`, `"onlyHigh":false`, `"note":"重点看高危事件"`, `"region":"cn"`} {
		if !strings.Contains(query, want) {
			t.Fatalf("query = %q, want it to contain the biased value %q", query, want)
		}
	}
	if strings.Contains(query, `"threshold":0.5`) {
		t.Fatalf("query = %q, want the fixed threshold to be overridden", query)
	}
	if objects.calls != 1 {
		t.Fatalf("objects.calls = %d, want 1", objects.calls)
	}
	if runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID); len(runs) != 1 ||
		runs[0].Status != modelsai.PeriodicReportRunStatusSuccess {
		t.Fatalf("runs = %+v, want one successful run", runs)
	}
}

// TestRunHandler_WindowComputation 验证窗口公式 nowTime=triggerAt、startTime=nowTime+timeOffset、
// endTime=startTime+timeWindow，含负偏移与偏置修改窗口。
func TestRunHandler_WindowComputation(t *testing.T) {
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		// name 用例名称
		name string
		// values 计划偏置 JSON
		values string
		// wantStart 期望的开始时间（RFC3339）
		wantStart string
		// wantEnd 期望的结束时间（RFC3339）
		wantEnd string
	}{
		{"负偏移使用固定窗口", `{"region":"cn"}`, "2026-10-29T08:00:00Z", "2026-10-30T08:00:00Z"},
		{"偏置修改窗口", `{"region":"cn","timeWindow":"48h"}`, "2026-10-29T08:00:00Z", "2026-10-31T08:00:00Z"},
		{"偏置修改偏移与窗口", `{"region":"cn","timeOffset":"0s","timeWindow":"2h"}`, "2026-10-30T08:00:00Z", "2026-10-30T10:00:00Z"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := periodicReportRunTestDB(t)
			ctx := context.Background()
			skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "window-skill-"+tc.name, true, true)
			planID := insertPeriodicReportPlanWithValues(t, db, periodicReportHandlerTestNamespace, skillID, "窗口计划-"+tc.name,
				true, nil, tc.values)
			markdown := "# 窗口报告\n\n正文。\n"
			runner := &fakePeriodicReportRunner{output: RunOutput{Content: fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)}}
			objects := &fakeReportObjects{}
			handler := newRunHandler(t, db, runner, objects, triggerAt)

			if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
				t.Fatalf("Do error = %v", err)
			}
			query := runner.input.Query
			if got := queryLineValue(query, "- nowTime："); got != triggerAt.Format(time.RFC3339) {
				t.Fatalf("prompt nowTime = %q, want %q", got, triggerAt.Format(time.RFC3339))
			}
			if got := queryLineValue(query, "- startTime："); got != tc.wantStart {
				t.Fatalf("prompt startTime = %q, want %q", got, tc.wantStart)
			}
			if got := queryLineValue(query, "- endTime："); got != tc.wantEnd {
				t.Fatalf("prompt endTime = %q, want %q", got, tc.wantEnd)
			}
		})
	}
}

// TestRunHandler_RuntimeTimesNotOverridable 验证 nowTime/startTime/endTime 不能通过计划偏置或
// 技能固定参数覆盖，违规时运行失败且不调用生成器、不写对象。
func TestRunHandler_RuntimeTimesNotOverridable(t *testing.T) {
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	for _, key := range []string{"nowTime", "startTime", "endTime"} {
		t.Run("偏置覆盖 "+key, func(t *testing.T) {
			db := periodicReportRunTestDB(t)
			ctx := context.Background()
			skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "reserved-bias-"+key, true, true)
			planID := insertPeriodicReportPlanWithValues(t, db, periodicReportHandlerTestNamespace, skillID,
				"保留键偏置计划-"+key, true, nil, fmt.Sprintf(`{"region":"cn",%q:"2026-11-02T00:00:00Z"}`, key))
			runner := &fakePeriodicReportRunner{output: RunOutput{Content: `{"content":"# 不应生成\n","draft":{}}`}}
			objects := &fakeReportObjects{}
			handler := newRunHandler(t, db, runner, objects, triggerAt)

			if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
				t.Fatalf("Do error = %v", err)
			}
			if runner.calls != 0 || objects.calls != 0 {
				t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
			}
			runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
			if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
				t.Fatalf("runs = %+v, want one failed run", runs)
			}
			if !strings.Contains(runs[0].Error, "plan bias parameters are invalid") {
				t.Fatalf("run error = %q, want the invalid bias reason", runs[0].Error)
			}
		})
	}
	t.Run("固定参数含 startTime", func(t *testing.T) {
		db := periodicReportRunTestDB(t)
		ctx := context.Background()
		skillID := insertPeriodicReportSkillWithFields(t, db, periodicReportHandlerTestNamespace, "reserved-fixed-skill",
			true, true, periodicReportTestInputFields,
			`{"timeOffset":"-24h","timeWindow":"24h","startTime":"2026-11-02T00:00:00Z"}`)
		planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "reserved-fixed-skill", "保留键固定计划", true, nil)
		runner := &fakePeriodicReportRunner{output: RunOutput{Content: `{"content":"# 不应生成\n","draft":{}}`}}
		objects := &fakeReportObjects{}
		handler := newRunHandler(t, db, runner, objects, triggerAt)

		if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
			t.Fatalf("Do error = %v", err)
		}
		if runner.calls != 0 || objects.calls != 0 {
			t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
		}
		runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
		if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
			t.Fatalf("runs = %+v, want one failed run", runs)
		}
		if !strings.Contains(runs[0].Error, "skill fixed fields are invalid") {
			t.Fatalf("run error = %q, want the invalid fixed fields reason", runs[0].Error)
		}
	})
}

// TestRunHandler_InvalidBiasFailsWithoutObject 验证偏置类型不匹配时运行失败、不调用生成器且不写对象。
func TestRunHandler_InvalidBiasFailsWithoutObject(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "invalid-bias-skill", true, true)
	planID := insertPeriodicReportPlanWithValues(t, db, periodicReportHandlerTestNamespace, skillID, "非法偏置计划",
		true, nil, `{"region":"cn","threshold":"0.75"}`)
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: `{"content":"# 不应生成\n","draft":{}}`}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != 0 || objects.calls != 0 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if !strings.Contains(runs[0].Error, "plan bias parameters are invalid") {
		t.Fatalf("run error = %q, want the invalid bias reason", runs[0].Error)
	}
}

// TestRunHandler_NoUsableAccessKey 验证属主没有可用访问密钥时运行直接标记失败，
// 原因明确提示创建或启用访问密钥，且不调用生成器、不写对象。
func TestRunHandler_NoUsableAccessKey(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "no-key-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "no-key-skill", "无密钥计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 13, 0, 0, 0, time.UTC)
	runner := &fakePeriodicReportRunner{output: RunOutput{Content: `{"content":"# 不应生成\n","draft":{}}`}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)
	handler.keys = &fakeAccessKeySource{}

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v, want nil when no access key is available", err)
	}
	if runner.calls != 0 || objects.calls != 0 {
		t.Fatalf("runner.calls=%d objects.calls=%d, want 0 and 0", runner.calls, objects.calls)
	}
	runs := listTestRuns(t, db, periodicReportHandlerTestNamespace, planID)
	if len(runs) != 1 || runs[0].Status != modelsai.PeriodicReportRunStatusFailed {
		t.Fatalf("runs = %+v, want one failed run", runs)
	}
	if !strings.Contains(runs[0].Error, "create or enable an access key") {
		t.Fatalf("run error = %q, want the access key hint", runs[0].Error)
	}
}

// TestRunHandler_InjectsAutomationOnEveryAttempt 验证有可用密钥时每次生成尝试都携带无人值守上下文，
// 包括信封不合规后的纠错重试。
func TestRunHandler_InjectsAutomationOnEveryAttempt(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "automation-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "automation-skill", "无人值守计划", true, nil)
	triggerAt := time.Date(2026, 10, 30, 14, 0, 0, 0, time.UTC)
	markdown := "# 无人值守报告\n\n正文。\n"
	valid := fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)
	runner := &fakePeriodicReportRunner{outputs: []RunOutput{
		{Content: "# 直接输出 Markdown，不合规"},
		{Content: valid},
	}}
	objects := &fakeReportObjects{}
	handler := newRunHandler(t, db, runner, objects, triggerAt)

	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planID, triggerAt)}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if runner.calls != 2 {
		t.Fatalf("runner.calls = %d, want 2", runner.calls)
	}
	for i, in := range runner.inputs {
		if in.Automation == nil || in.Automation.InvokeAPI == nil {
			t.Fatalf("attempt %d Automation = %+v, want a configured invoker", i+1, in.Automation)
		}
	}
	if objects.calls != 1 {
		t.Fatalf("objects.calls = %d, want 1", objects.calls)
	}
}

// TestRunHandler_WindowStableForSameTrigger 验证同一触发点在不同 worker 启动时间下注入的窗口一致，
// 不随实际执行时间漂移。
func TestRunHandler_WindowStableForSameTrigger(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	triggerAt := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "stable-window-skill", true, true)
	planA := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "stable-window-skill", "稳定窗口计划A", true, nil)
	planB := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "stable-window-skill", "稳定窗口计划B", true, nil)
	markdown := "# 稳定窗口报告\n\n正文。\n"
	content := fmt.Sprintf(`{"content":%q,"draft":{}}`, markdown)

	objectsA := &fakeReportObjects{}
	runnerA := &fakePeriodicReportRunner{output: RunOutput{Content: content}}
	handlerA := newRunHandler(t, db, runnerA, objectsA, triggerAt.Add(time.Hour))
	if err := handlerA.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planA, triggerAt)}); err != nil {
		t.Fatalf("Do(planA) error = %v", err)
	}
	objectsB := &fakeReportObjects{}
	runnerB := &fakePeriodicReportRunner{output: RunOutput{Content: content}}
	handlerB := newRunHandler(t, db, runnerB, objectsB, triggerAt.Add(2*time.Hour))
	if err := handlerB.Do(ctx, &periodicReportTaskStub{typ: TaskPlanRun, payload: periodicReportRunTaskPayload(t, planB, triggerAt)}); err != nil {
		t.Fatalf("Do(planB) error = %v", err)
	}

	for _, prefix := range []string{"- nowTime：", "- startTime：", "- endTime："} {
		gotA := queryLineValue(runnerA.input.Query, prefix)
		gotB := queryLineValue(runnerB.input.Query, prefix)
		if gotA == "" || gotA != gotB {
			t.Fatalf("prompt %q differs across worker start times: %q vs %q", prefix, gotA, gotB)
		}
	}
}
