package periodic

import (
	// 标准库
	"context"
	"encoding/json"
	"testing"
	"time"

	// 项目内部
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 测试专用命名空间，与迁移种子的 default 隔离。
const (
	// periodicReportTestNamespace 报告领域层测试的主命名空间。
	periodicReportTestNamespace = "periodicreporttest"
	// periodicReportOtherNamespace 跨命名空间隔离用例的另一个命名空间。
	periodicReportOtherNamespace = "periodicreporttest2"
)

// periodicReportTestInputFields 是可周期执行技能的输入字段 JSON：含运行时注入的 nowTime、一个必填
// 文本字段与一个可选文本字段。
const periodicReportTestInputFields = `[{"name":"nowTime","type":"date","required":true,"description":"运行时注入"},{"name":"region","type":"string","required":true,"description":"区域"},{"name":"note","type":"string","required":false,"description":"补充说明"}]`

// periodicReportTestFixedFields 是可周期执行技能的固定参数 JSON：含时间窗口与混合类型的固定值。
const periodicReportTestFixedFields = `{"timeOffset":"-24h","timeWindow":"24h","threshold":0.5,"onlyHigh":true}`

// planRaw 把 JSON 文本转换为 json.RawMessage，便于构造带类型的计划偏置值。
func planRaw(text string) json.RawMessage {
	return json.RawMessage(text)
}

// fakeRemover 是内存版对象删除替身：记录成功删除的对象键，并可指定一组必定失败的对象键。
type fakeRemover struct {
	// removed 成功删除的对象键序列
	removed []string
	// fail 指定删除失败的对象键
	fail map[string]bool
}

// Remove 删除对象：命中失败集合时返回错误，否则记录到已删除序列。
func (f *fakeRemover) Remove(_ context.Context, objectKey string) error {
	if f.fail[objectKey] {
		return errors.New("fake object removal failure")
	}
	f.removed = append(f.removed, objectKey)
	return nil
}

// newReportTestStore 返回共享 PostgreSQL 测试库上的报告存储，并在测试事务内确保测试专用
// 命名空间与可选择技能存在（满足外键约束并支撑资格校验）。
func newReportTestStore(t *testing.T) (*Store, *database.DB) {
	t.Helper()
	db := testdb.Core(t)
	for _, name := range []string{periodicReportTestNamespace, periodicReportOtherNamespace} {
		if err := db.Exec(
			`INSERT INTO namespaces (name, description) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`,
			name, "report domain test namespace",
		).Error; err != nil {
			t.Fatalf("seed namespace %q: %v", name, err)
		}
	}
	seedEligibleSkill(t, db, periodicReportTestNamespace, "periodic-report-skill")
	// periodic_report_plans.user_id 为 users(id) 外键，测试计划统一使用属主 1，先确保该用户存在。
	if err := db.Exec(
		`INSERT INTO users (id, union_id, nickle_name, username, password, email)
		 VALUES (1, '00000000-0000-0000-0000-000000000101', 'report-test', 'report-test-user', '\x00'::bytea, 'report-test@example.com')
		 ON CONFLICT DO NOTHING`,
	).Error; err != nil {
		t.Fatalf("seed report test user: %v", err)
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	return store, db
}

// seedEligibleSkill 在给定命名空间下插入一条启用且可周期执行的自定义技能，返回其 ID。
func seedEligibleSkill(t *testing.T, db *database.DB, namespace, name string) uint {
	t.Helper()
	return seedSkillWithFields(t, db, namespace, name, true, true, periodicReportTestInputFields, periodicReportTestFixedFields)
}

// seedSkillWithFields 在给定命名空间下插入一条指定启停、周期能力、输入字段与固定参数的技能，
// 返回其 ID。
func seedSkillWithFields(t *testing.T, db *database.DB, namespace, name string, enabled, periodicCapable bool, inputFields, fixedFields string) uint {
	t.Helper()
	var id uint
	if err := db.Raw(
		`INSERT INTO agent_skills (namespace_name, source, name, description, content, enabled, periodic_capable, input_fields, fixed_fields)
		 VALUES (?, 'custom', ?, '', NULL, ?, ?, ?::jsonb, ?::jsonb) RETURNING id`,
		namespace, name, enabled, periodicCapable, inputFields, fixedFields,
	).Scan(&id).Error; err != nil {
		t.Fatalf("seed skill %q: %v", name, err)
	}
	return id
}

// defaultPlanInput 返回一条可通过校验的计划输入。
func defaultPlanInput(skillID uint) PlanInput {
	return PlanInput{
		Name:          "周报计划",
		Description:   "每周生成安全周报",
		SkillID:       skillID,
		CronSpec:      "0 * * * *",
		InputValues:   map[string]json.RawMessage{"region": planRaw(`"cn"`)},
		RetentionDays: 90,
	}
}

// TestCreatePlan_Success 测试合法输入可创建启用的计划并保存技能名称快照与保留天数。
func TestCreatePlan_Success(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-2")
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	if plan.ID == 0 || !plan.Enabled {
		t.Fatalf("CreatePlan() = %+v, want non-zero ID and enabled plan", plan)
	}
	names, err := store.SkillNamesByIDs(context.Background(), []uint{plan.SkillID})
	if err != nil {
		t.Fatalf("SkillNamesByIDs error = %v", err)
	}
	if names[plan.SkillID] != "periodic-report-skill-2" || plan.RetentionDays != 90 {
		t.Errorf("CreatePlan() skillName=%q retention=%d, want %q and 90",
			names[plan.SkillID], plan.RetentionDays, "periodic-report-skill-2")
	}
	if plan.NamespaceName != periodicReportTestNamespace || plan.UserID != 1 {
		t.Errorf("CreatePlan() namespace=%q userID=%d, want %q and 1", plan.NamespaceName, plan.UserID, periodicReportTestNamespace)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(plan.InputValues, &values); err != nil {
		t.Fatalf("unmarshal InputValues: %v", err)
	}
	if string(values["region"]) != `"cn"` {
		t.Errorf("CreatePlan().InputValues = %v, want the encoded input values", values)
	}
}

// TestCreatePlan_RejectsInvalidInput 测试非法 cron、必填输入缺失与偏置键/类型非法都被拒绝。
func TestCreatePlan_RejectsInvalidInput(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-3")
	cases := []struct {
		// name 用例名称
		name string
		// mutate 修改计划输入
		mutate func(in *PlanInput)
		// want 期望的错误哨兵
		want error
	}{
		{"invalid cron", func(in *PlanInput) { in.CronSpec = "not-a-cron" }, ErrScheduleInvalid},
		{"missing required input", func(in *PlanInput) { in.InputValues = nil }, ErrInputMissing},
		{"missing required string", func(in *PlanInput) { in.InputValues = map[string]json.RawMessage{"region": planRaw(`""`)} }, ErrInputMissing},
		{"nowTime bias", func(in *PlanInput) {
			in.InputValues["nowTime"] = planRaw(`"2026-11-02T00:00:00Z"`)
		}, ErrInputInvalid},
		{"unknown key", func(in *PlanInput) { in.InputValues["unknown"] = planRaw(`"x"`) }, ErrInputInvalid},
		{"number field as string", func(in *PlanInput) { in.InputValues["threshold"] = planRaw(`"0.75"`) }, ErrInputInvalid},
		{"boolean field as number", func(in *PlanInput) { in.InputValues["onlyHigh"] = planRaw(`1`) }, ErrInputInvalid},
		{"invalid timeOffset", func(in *PlanInput) { in.InputValues["timeOffset"] = planRaw(`"1d"`) }, ErrInputInvalid},
		{"zero timeWindow", func(in *PlanInput) { in.InputValues["timeWindow"] = planRaw(`"0s"`) }, ErrInputInvalid},
	}
	for _, tc := range cases {
		in := defaultPlanInput(skillID)
		tc.mutate(&in)
		if _, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, in); !errors.Is(err, tc.want) {
			t.Errorf("%s: CreatePlan error = %v, want %v", tc.name, err, tc.want)
		}
	}
}

// TestCreatePlan_MixedTypeBias 测试数字、布尔、字符串混合偏置成功落库且保留原始 JSON 类型。
func TestCreatePlan_MixedTypeBias(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-mixed")
	in := defaultPlanInput(skillID)
	in.InputValues = map[string]json.RawMessage{
		"region":     planRaw(`"cn"`),
		"threshold":  planRaw(`0.75`),
		"onlyHigh":   planRaw(`false`),
		"note":       planRaw(`"重点看高危事件"`),
		"timeWindow": planRaw(`"48h"`),
	}
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, in)
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	values, err := DecodeInputValues(plan.InputValues)
	if err != nil {
		t.Fatalf("DecodeInputValues error = %v", err)
	}
	for key, want := range map[string]string{
		"threshold":  "number",
		"onlyHigh":   "boolean",
		"region":     "string",
		"note":       "string",
		"timeWindow": "string",
	} {
		kind, err := validator.JSONValueKind(values[key])
		if err != nil || string(kind) != want {
			t.Errorf("decoded[%q] kind = (%q, %v), want %q", key, kind, err, want)
		}
	}
}

// TestEligibleSkills_FiltersIneligible 测试只有固定参数含合法时间窗口且输入字段含唯一 nowTime 的
// 技能才会进入计划选项。
func TestEligibleSkills_FiltersIneligible(t *testing.T) {
	store, _ := newReportTestStore(t)
	valid := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "eligible-valid")
	missingFixed := seedSkillWithFields(t, store.db, periodicReportTestNamespace, "eligible-missing-fixed",
		true, true, periodicReportTestInputFields, `{}`)
	missingNowTime := seedSkillWithFields(t, store.db, periodicReportTestNamespace, "eligible-missing-nowtime",
		true, true, `[{"name":"region","type":"string","required":true,"description":"区域"}]`, periodicReportTestFixedFields)
	invalidWindow := seedSkillWithFields(t, store.db, periodicReportTestNamespace, "eligible-invalid-window",
		true, true, periodicReportTestInputFields, `{"timeOffset":"0s","timeWindow":"0s"}`)

	rows, err := store.EligibleSkills(context.Background(), periodicReportTestNamespace)
	if err != nil {
		t.Fatalf("EligibleSkills error = %v", err)
	}
	ids := make(map[uint]bool, len(rows))
	for _, row := range rows {
		ids[row.ID] = true
	}
	if !ids[valid] {
		t.Errorf("EligibleSkills missing valid skill %d: %v", valid, ids)
	}
	for name, id := range map[string]uint{
		"缺少固定参数":     missingFixed,
		"缺少 nowTime": missingNowTime,
		"时间窗口非法":     invalidWindow,
	} {
		if ids[id] {
			t.Errorf("EligibleSkills unexpectedly included skill %d (%s)", id, name)
		}
	}
}

// TestGetPlan_CrossNamespace 测试跨命名空间读取计划按不存在处理。
func TestGetPlan_CrossNamespace(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-4")
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	if _, err := store.GetPlan(context.Background(), periodicReportOtherNamespace, plan.ID); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("GetPlan cross namespace error = %v, want ErrPlanNotFound", err)
	}
}

// TestSetPlanEnabled 测试启停切换后读回的状态与最新行一致。
func TestSetPlanEnabled(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-5")
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	disabled, err := store.SetPlanEnabled(context.Background(), periodicReportTestNamespace, plan.ID, false)
	if err != nil {
		t.Fatalf("SetPlanEnabled(false) error = %v", err)
	}
	if disabled.Enabled {
		t.Error("SetPlanEnabled(false).Enabled = true, want false")
	}
	enabled, err := store.SetPlanEnabled(context.Background(), periodicReportTestNamespace, plan.ID, true)
	if err != nil {
		t.Fatalf("SetPlanEnabled(true) error = %v", err)
	}
	if !enabled.Enabled {
		t.Error("SetPlanEnabled(true).Enabled = false, want true")
	}
}

// TestCreateRunIfAbsent_Idempotent 测试同一计划同一触发点的重复投递只落一条运行记录。
func TestCreateRunIfAbsent_Idempotent(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-6")
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	triggerAt := time.Now().Truncate(time.Second)
	first := &modelsai.PeriodicReportRun{
		NamespaceName: periodicReportTestNamespace,
		PlanID:        plan.ID,
		TriggerAt:     triggerAt,
		Status:        modelsai.PeriodicReportRunStatusRunning,
	}
	created, err := store.CreateRunIfAbsent(context.Background(), first)
	if err != nil || !created {
		t.Fatalf("CreateRunIfAbsent(first) = (%v, %v), want (true, nil)", created, err)
	}
	second := &modelsai.PeriodicReportRun{
		NamespaceName: periodicReportTestNamespace,
		PlanID:        plan.ID,
		TriggerAt:     triggerAt,
		Status:        modelsai.PeriodicReportRunStatusRunning,
	}
	created, err = store.CreateRunIfAbsent(context.Background(), second)
	if err != nil {
		t.Fatalf("CreateRunIfAbsent(second) error = %v", err)
	}
	if created {
		t.Error("CreateRunIfAbsent(second) = true, want false for a duplicate trigger")
	}
	var total int64
	if err := store.db.Model(&modelsai.PeriodicReportRun{}).Where("plan_id = ?", plan.ID).Count(&total).Error; err != nil || total != 1 {
		t.Errorf("run count = %d, err = %v, want 1 and nil", total, err)
	}
}

// TestSetWatch 测试关注关系幂等生效与取消。
func TestSetWatch(t *testing.T) {
	store, _ := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-7")
	plan, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	if err := store.SetWatch(ctx, periodicReportTestNamespace, 1, plan.ID, true); err != nil {
		t.Fatalf("SetWatch(true) error = %v", err)
	}
	if err := store.SetWatch(ctx, periodicReportTestNamespace, 1, plan.ID, true); err != nil {
		t.Fatalf("SetWatch(true) twice error = %v", err)
	}
	ids, err := store.ListWatchedPlanIDs(ctx, periodicReportTestNamespace, 1)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ListWatchedPlanIDs = %v, err = %v, want one plan", ids, err)
	}
	if err := store.SetWatch(ctx, periodicReportTestNamespace, 1, plan.ID, false); err != nil {
		t.Fatalf("SetWatch(false) error = %v", err)
	}
	if ids, err := store.ListWatchedPlanIDs(ctx, periodicReportTestNamespace, 1); err != nil || len(ids) != 0 {
		t.Errorf("ListWatchedPlanIDs after cancel = %v, err = %v, want empty", ids, err)
	}
}

// TestSetWatch_CrossNamespace 测试关注不属于该命名空间的计划被拒绝。
func TestSetWatch_CrossNamespace(t *testing.T) {
	store, _ := newReportTestStore(t)
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-8")
	plan, err := store.CreatePlan(context.Background(), periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	if err := store.SetWatch(context.Background(), periodicReportOtherNamespace, 1, plan.ID, true); !errors.Is(err, ErrPlanNotFound) {
		t.Errorf("SetWatch cross namespace error = %v, want ErrPlanNotFound", err)
	}
}

// TestCleanup 测试到期计划自动停用、超期对象与运行记录被清理，对象删除失败的记录保留。
func TestCleanup(t *testing.T) {
	store, db := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-9")
	now := time.Now()

	in := defaultPlanInput(skillID)
	in.RetentionDays = 1
	planA, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, in)
	if err != nil {
		t.Fatalf("CreatePlan(planA) error = %v", err)
	}
	expiresAt := now.Add(-time.Hour)
	expiredInput := defaultPlanInput(skillID)
	expiredInput.ExpiresAt = &expiresAt
	planB, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, expiredInput)
	if err != nil {
		t.Fatalf("CreatePlan(planB) error = %v", err)
	}

	statuses := []struct {
		// status 运行状态
		status modelsai.PeriodicReportRunStatus
		// objectKey 报告对象键
		objectKey string
		// age 相对当前时间的创建偏移
		age time.Duration
	}{
		{modelsai.PeriodicReportRunStatusSuccess, "obj-old-success", -48 * time.Hour},
		{modelsai.PeriodicReportRunStatusFailed, "obj-old-fail", -48 * time.Hour},
		{modelsai.PeriodicReportRunStatusRunning, "obj-old-running", -48 * time.Hour},
		{modelsai.PeriodicReportRunStatusSuccess, "obj-new", 0},
	}
	runs := make([]*modelsai.PeriodicReportRun, 0, len(statuses))
	for i, tc := range statuses {
		run := &modelsai.PeriodicReportRun{
			NamespaceName: periodicReportTestNamespace,
			PlanID:        planA.ID,
			TriggerAt:     now.Add(time.Duration(i) * time.Minute),
			Status:        tc.status,
			ObjectKey:     tc.objectKey,
		}
		if _, err := store.CreateRunIfAbsent(ctx, run); err != nil {
			t.Fatalf("CreateRunIfAbsent(%s) error = %v", tc.objectKey, err)
		}
		if tc.age != 0 {
			if err := db.Exec(`UPDATE periodic_report_runs SET created_at = ? WHERE id = ?`, now.Add(tc.age), run.ID).Error; err != nil {
				t.Fatalf("backdate run %s: %v", tc.objectKey, err)
			}
		}
		runs = append(runs, run)
	}

	remover := &fakeRemover{fail: map[string]bool{"obj-old-fail": true}}
	result, err := store.Cleanup(ctx, now, remover, nil, 0)
	if err != nil {
		t.Fatalf("Cleanup error = %v", err)
	}
	if result.ExpiredPlans != 1 || result.RemovedObjects != 1 || result.RemovedRuns != 1 {
		t.Errorf("Cleanup result = %+v, want ExpiredPlans=1 RemovedObjects=1 RemovedRuns=1", result)
	}
	disabled, err := store.GetPlan(ctx, periodicReportTestNamespace, planB.ID)
	if err != nil {
		t.Fatalf("GetPlan(planB) error = %v", err)
	}
	if disabled.Enabled {
		t.Error("Cleanup left the expired plan enabled, want disabled")
	}
	if _, err := store.GetRun(ctx, periodicReportTestNamespace, runs[0].ID); !errors.Is(err, ErrRunNotFound) {
		t.Errorf("cleaned success run error = %v, want ErrRunNotFound", err)
	}
	for _, idx := range []int{1, 2, 3} {
		if _, err := store.GetRun(ctx, periodicReportTestNamespace, runs[idx].ID); err != nil {
			t.Errorf("run %s should be retained, got error %v", statuses[idx].objectKey, err)
		}
	}
}

// TestUpdatePlan_ResetsNextRunAt 测试更新计划会清空下次执行时间，交由分发器按新 cron 重算。
func TestUpdatePlan_ResetsNextRunAt(t *testing.T) {
	store, db := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-10")
	plan, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	next := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := db.Exec(`UPDATE periodic_report_plans SET next_run_at = ? WHERE id = ?`, next, plan.ID).Error; err != nil {
		t.Fatalf("seed next_run_at: %v", err)
	}
	updated, err := store.UpdatePlan(ctx, periodicReportTestNamespace, plan.ID, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("UpdatePlan error = %v", err)
	}
	if updated.NextRunAt != nil {
		t.Fatalf("UpdatePlan().NextRunAt = %v, want nil", updated.NextRunAt)
	}
	reloaded, err := store.GetPlan(ctx, periodicReportTestNamespace, plan.ID)
	if err != nil {
		t.Fatalf("GetPlan error = %v", err)
	}
	if reloaded.NextRunAt != nil {
		t.Fatalf("persisted next_run_at = %v, want NULL", reloaded.NextRunAt)
	}
}

// TestSetPlanEnabled_ResetsNextRunAt 测试启用计划清空下次执行时间，停用保留原值。
func TestSetPlanEnabled_ResetsNextRunAt(t *testing.T) {
	store, db := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-11")
	plan, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	next := time.Now().Add(time.Hour).Truncate(time.Second)
	if err := db.Exec(`UPDATE periodic_report_plans SET next_run_at = ? WHERE id = ?`, next, plan.ID).Error; err != nil {
		t.Fatalf("seed next_run_at: %v", err)
	}
	disabled, err := store.SetPlanEnabled(ctx, periodicReportTestNamespace, plan.ID, false)
	if err != nil {
		t.Fatalf("SetPlanEnabled(false) error = %v", err)
	}
	if disabled.NextRunAt == nil {
		t.Fatalf("SetPlanEnabled(false).NextRunAt = nil, want retained %v", next)
	}
	enabled, err := store.SetPlanEnabled(ctx, periodicReportTestNamespace, plan.ID, true)
	if err != nil {
		t.Fatalf("SetPlanEnabled(true) error = %v", err)
	}
	if enabled.NextRunAt != nil {
		t.Fatalf("SetPlanEnabled(true).NextRunAt = %v, want nil", enabled.NextRunAt)
	}
}

// TestListDispatchablePlans 测试分发器只选取已启用、未到期且待计算或已到期的计划。
func TestListDispatchablePlans(t *testing.T) {
	store, db := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-12")
	now := time.Now().Truncate(time.Second)

	pending, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan(pending) error = %v", err)
	}
	due, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan(due) error = %v", err)
	}
	future, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan(future) error = %v", err)
	}
	disabled, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan(disabled) error = %v", err)
	}
	expiresAt := now.Add(-time.Hour)
	expiredInput := defaultPlanInput(skillID)
	expiredInput.ExpiresAt = &expiresAt
	expired, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, expiredInput)
	if err != nil {
		t.Fatalf("CreatePlan(expired) error = %v", err)
	}

	values := map[uint]*time.Time{
		due.ID:      ptr(now.Add(-time.Minute)),
		future.ID:   ptr(now.Add(time.Hour)),
		disabled.ID: ptr(now.Add(-time.Minute)),
		expired.ID:  ptr(now.Add(-time.Minute)),
	}
	for id, next := range values {
		if err := db.Exec(`UPDATE periodic_report_plans SET next_run_at = ? WHERE id = ?`, next, id).Error; err != nil {
			t.Fatalf("seed next_run_at for plan %d: %v", id, err)
		}
	}
	if _, err := store.SetPlanEnabled(ctx, periodicReportTestNamespace, disabled.ID, false); err != nil {
		t.Fatalf("SetPlanEnabled(false) error = %v", err)
	}

	plans, err := store.ListDispatchablePlans(ctx, now, 0)
	if err != nil {
		t.Fatalf("ListDispatchablePlans error = %v", err)
	}
	ids := make(map[uint]bool, len(plans))
	for _, plan := range plans {
		ids[plan.ID] = true
	}
	if !ids[pending.ID] || !ids[due.ID] {
		t.Fatalf("ListDispatchablePlans ids = %v, want pending %d and due %d", ids, pending.ID, due.ID)
	}
	for _, unwanted := range []uint{future.ID, disabled.ID, expired.ID} {
		if ids[unwanted] {
			t.Fatalf("ListDispatchablePlans included plan %d, want excluded", unwanted)
		}
	}
}

// TestSetNextRunAtIfUnchanged 测试 CAS 推进命中与未命中：未命中时保留已被推进的值。
func TestSetNextRunAtIfUnchanged(t *testing.T) {
	store, db := newReportTestStore(t)
	ctx := context.Background()
	skillID := seedEligibleSkill(t, store.db, periodicReportTestNamespace, "periodic-report-skill-13")
	plan, err := store.CreatePlan(ctx, periodicReportTestNamespace, 1, defaultPlanInput(skillID))
	if err != nil {
		t.Fatalf("CreatePlan error = %v", err)
	}
	old := time.Now().Truncate(time.Second)
	next := old.Add(time.Hour)
	if err := db.Exec(`UPDATE periodic_report_plans SET next_run_at = ? WHERE id = ?`, old, plan.ID).Error; err != nil {
		t.Fatalf("seed next_run_at: %v", err)
	}
	ok, err := store.SetNextRunAtIfUnchanged(ctx, plan.ID, old, next)
	if err != nil || !ok {
		t.Fatalf("SetNextRunAtIfUnchanged(hit) = (%v, %v), want (true, nil)", ok, err)
	}
	ok, err = store.SetNextRunAtIfUnchanged(ctx, plan.ID, old, next.Add(time.Hour))
	if err != nil {
		t.Fatalf("SetNextRunAtIfUnchanged(miss) error = %v", err)
	}
	if ok {
		t.Fatal("SetNextRunAtIfUnchanged(miss) = true, want false")
	}
	reloaded, err := store.GetPlan(ctx, periodicReportTestNamespace, plan.ID)
	if err != nil {
		t.Fatalf("GetPlan error = %v", err)
	}
	if reloaded.NextRunAt == nil || !reloaded.NextRunAt.Equal(next) {
		t.Fatalf("persisted next_run_at = %v, want %v", reloaded.NextRunAt, next)
	}
}

// ptr 返回给定时间值的指针。
func ptr(t time.Time) *time.Time { return &t }

// TestNextTrigger 测试下次触发时间计算：指定时区、严格晚于基准时刻与非法表达式。
func TestNextTrigger(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	after := time.Date(2026, 1, 1, 0, 0, 0, 0, loc)
	got, err := NextTrigger("0 9 * * *", after, loc)
	if err != nil {
		t.Fatalf("NextTrigger error = %v", err)
	}
	want := time.Date(2026, 1, 1, 9, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("NextTrigger() = %v, want %v", got, want)
	}
	got, err = NextTrigger("0 9 * * *", want, loc)
	if err != nil {
		t.Fatalf("NextTrigger(equal) error = %v", err)
	}
	if wantNext := want.Add(24 * time.Hour); !got.Equal(wantNext) {
		t.Fatalf("NextTrigger(equal) = %v, want strictly later %v", got, wantNext)
	}
	if _, err := NextTrigger("not-a-cron", after, loc); err == nil {
		t.Fatal("NextTrigger(invalid) error = nil, want an error")
	}
}
