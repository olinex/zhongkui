package periodicreport

import (
	// 标准库
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	// 第三方
	"github.com/hibiken/asynq"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
)

// fakeTaskEnqueuer 记录被投递任务的测试替身，实现 pipeline 内部投递接口。
type fakeTaskEnqueuer struct {
	// tasks 记录所有经 EnqueueContext 投递的任务。
	tasks []*asynq.Task
	// err 预设投递错误，非 nil 时一律返回该错误。
	err error
}

// EnqueueContext 记录任务并返回空任务信息（或预设错误）。
func (f *fakeTaskEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.tasks = append(f.tasks, task)
	return nil, nil
}

// Close 关闭投递客户端，测试替身无需释放资源。
func (f *fakeTaskEnqueuer) Close() error { return nil }

// newFakePipelineClient 构造投递行为被桩替换的 *pipeline.Client。
// pipeline.Client 的投递字段为非导出字段且 NewClient 需要真实 Redis，单元测试经
// reflect.NewAt + unsafe 注入假实现，使 EnqueueContext 直接命中桩而不触达 Redis。
func newFakePipelineClient(f *fakeTaskEnqueuer) *pipeline.Client {
	client := &pipeline.Client{}
	field := reflect.ValueOf(client).Elem().FieldByName("cli")
	addressable := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	addressable.Set(reflect.ValueOf(f))
	return client
}

// newDispatchHandlerForTest 构造注入假投递客户端的报告分发处理器。
func newDispatchHandlerForTest(t *testing.T, db *database.DB, enq *pipeline.Client, now time.Time) *DispatchHandler {
	t.Helper()
	store, err := periodic.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("NewDefaultLogger error = %v", err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("LoadLocation error = %v", err)
	}
	return &DispatchHandler{
		store:  store,
		enq:    enq,
		loc:    loc,
		logger: logger,
		now:    func() time.Time { return now },
	}
}

// seedPlanNextRunAt 为计划写入下次执行时间。
func seedPlanNextRunAt(t *testing.T, db *database.DB, planID uint, next time.Time) {
	t.Helper()
	if err := db.Exec(`UPDATE periodic_report_plans SET next_run_at = ? WHERE id = ?`, next, planID).Error; err != nil {
		t.Fatalf("seed next_run_at for plan %d: %v", planID, err)
	}
}

// dispatchReadPlan 读取计划当前状态。
func dispatchReadPlan(t *testing.T, db *database.DB, planID uint) (nextRunAt *time.Time, enabled bool) {
	t.Helper()
	store, err := periodic.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	plan, err := store.GetPlanByID(context.Background(), planID)
	if err != nil {
		t.Fatalf("GetPlanByID error = %v", err)
	}
	return plan.NextRunAt, plan.Enabled
}

// TestDispatchHandler_ComputesPendingPlan 验证 next_run_at 为 NULL 的计划只计算不投递。
func TestDispatchHandler_ComputesPendingPlan(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dispatch-skill", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dispatch-skill", "待计算计划", true, nil)

	enq := &fakeTaskEnqueuer{}
	handler := newDispatchHandlerForTest(t, db, newFakePipelineClient(enq), nowTime)
	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskDispatch}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("enqueued %d tasks, want none for a pending plan", len(enq.tasks))
	}
	next, _ := dispatchReadPlan(t, db, planID)
	if next == nil || !next.After(nowTime) {
		t.Fatalf("next_run_at = %v, want a time after %v", next, nowTime)
	}
}

// TestDispatchHandler_DispatchesDuePlan 验证到期计划投递一次并把下次执行时间推进到未来。
func TestDispatchHandler_DispatchesDuePlan(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dispatch-skill-due", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dispatch-skill-due", "到期计划", true, nil)
	triggerAt := nowTime.Add(-time.Minute)
	seedPlanNextRunAt(t, db, planID, triggerAt)

	enq := &fakeTaskEnqueuer{}
	handler := newDispatchHandlerForTest(t, db, newFakePipelineClient(enq), nowTime)
	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskDispatch}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if len(enq.tasks) != 1 {
		t.Fatalf("enqueued %d tasks, want exactly 1", len(enq.tasks))
	}
	if got := enq.tasks[0].Type(); got != TaskPlanRun {
		t.Fatalf("task type = %q, want %q", got, TaskPlanRun)
	}
	gotTrigger, payload, err := pipeline.ParseCursorTaskPayload(enq.tasks[0].Payload())
	if err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if !gotTrigger.Equal(triggerAt) {
		t.Fatalf("payload triggerAt = %v, want %v", gotTrigger, triggerAt)
	}
	dispatchedID, err := parsePlanID(payload)
	if err != nil || dispatchedID != planID {
		t.Fatalf("payload planId = %d, %v, want %d", dispatchedID, err, planID)
	}
	next, _ := dispatchReadPlan(t, db, planID)
	if next == nil || !next.After(nowTime) {
		t.Fatalf("next_run_at = %v, want advanced after %v", next, nowTime)
	}
}

// TestDispatchHandler_SkipsNotDuePlan 验证未到期计划不投递且下次执行时间不变。
func TestDispatchHandler_SkipsNotDuePlan(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dispatch-skill-future", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dispatch-skill-future", "未到期计划", true, nil)
	future := nowTime.Add(time.Hour)
	seedPlanNextRunAt(t, db, planID, future)

	enq := &fakeTaskEnqueuer{}
	handler := newDispatchHandlerForTest(t, db, newFakePipelineClient(enq), nowTime)
	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskDispatch}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("enqueued %d tasks, want none for a not due plan", len(enq.tasks))
	}
	next, _ := dispatchReadPlan(t, db, planID)
	if next == nil || !next.Equal(future) {
		t.Fatalf("next_run_at = %v, want unchanged %v", next, future)
	}
}

// TestDispatchHandler_SkipsExpiredPlan 验证过期计划不投递。
func TestDispatchHandler_SkipsExpiredPlan(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dispatch-skill-expired", true, true)
	expiresAt := nowTime.Add(-time.Hour)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dispatch-skill-expired", "过期计划", true, &expiresAt)
	seedPlanNextRunAt(t, db, planID, nowTime.Add(-time.Minute))

	enq := &fakeTaskEnqueuer{}
	handler := newDispatchHandlerForTest(t, db, newFakePipelineClient(enq), nowTime)
	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskDispatch}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	if len(enq.tasks) != 0 {
		t.Fatalf("enqueued %d tasks, want none for an expired plan", len(enq.tasks))
	}
}

// TestDispatchHandler_KeepsNextRunAtOnEnqueueFailure 验证投递失败时不推进下次执行时间。
func TestDispatchHandler_KeepsNextRunAtOnEnqueueFailure(t *testing.T) {
	db := periodicReportRunTestDB(t)
	ctx := context.Background()
	nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "dispatch-skill-fail", true, true)
	planID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "dispatch-skill-fail", "投递失败计划", true, nil)
	triggerAt := nowTime.Add(-time.Minute)
	seedPlanNextRunAt(t, db, planID, triggerAt)

	enq := &fakeTaskEnqueuer{err: errors.New("fake enqueue failure")}
	handler := newDispatchHandlerForTest(t, db, newFakePipelineClient(enq), nowTime)
	if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskDispatch}); err != nil {
		t.Fatalf("Do error = %v", err)
	}
	next, _ := dispatchReadPlan(t, db, planID)
	if next == nil || !next.Equal(triggerAt) {
		t.Fatalf("next_run_at = %v, want unchanged %v", next, triggerAt)
	}
}
