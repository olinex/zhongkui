package periodicreport

import (
	// 标准库
	"context"
	"testing"
	"time"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/reports/periodic"
	"zhongkuisec.com/zhongkui/internal/ai/session"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
)

// fakePeriodicReportRemover 是报告对象删除替身：fail 为 true 时 Remove 一律失败，否则记录已删除键。
type fakePeriodicReportRemover struct {
	// removed 成功删除的对象键
	removed []string
	// fail 是否模拟删除失败
	fail bool
}

// Remove 删除对象：fail 为 true 时返回错误，否则记录对象键。
func (f *fakePeriodicReportRemover) Remove(_ context.Context, objectKey string, _ []byte) error {
	if f.fail {
		return errors.New("fake object removal failure")
	}
	f.removed = append(f.removed, objectKey)
	return nil
}

// newCleanupHandler 构造注入假依赖的报告清理处理器。
func newCleanupHandler(t *testing.T, db *database.DB, objects periodic.ObjectRemover, now time.Time) *CleanupHandler {
	t.Helper()
	store, err := periodic.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	sessions, err := session.NewStore(db)
	if err != nil {
		t.Fatalf("session.NewStore error = %v", err)
	}
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatalf("NewDefaultLogger error = %v", err)
	}
	return &CleanupHandler{
		store:    store,
		objects:  objects,
		sessions: sessions,
		logger:   logger,
		now:      func() time.Time { return now },
	}
}

// TestCleanupHandler 验证到期计划停用、超期对象与记录清理，以及对象删除失败时保留记录。
func TestCleanupHandler(t *testing.T) {
	cases := []struct {
		// name 用例名称
		name string
		// failRemoval 是否模拟对象删除失败
		failRemoval bool
	}{
		{"对象删除成功", false},
		{"对象删除失败保留记录", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := periodicReportRunTestDB(t)
			ctx := context.Background()
			nowTime := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
			skillID := insertPeriodicReportSkill(t, db, periodicReportHandlerTestNamespace, "cleanup-skill", true, true)
			activePlanID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "cleanup-skill", "活跃计划", true, nil)
			expiresAt := nowTime.Add(-time.Hour)
			expiredPlanID := insertPeriodicReportPlan(t, db, periodicReportHandlerTestNamespace, skillID, "cleanup-skill", "到期计划", true, &expiresAt)

			store, err := periodic.NewStore(db)
			if err != nil {
				t.Fatalf("NewStore error = %v", err)
			}
			sessions, err := session.NewStore(db)
			if err != nil {
				t.Fatalf("session.NewStore error = %v", err)
			}
			oldSession, err := sessions.CreateSession(ctx, periodicReportHandlerTestNamespace, "cleanup-run-session", "", modelsai.AgentKindKnowledge, 1)
			if err != nil {
				t.Fatalf("CreateSession error = %v", err)
			}
			oldRun := &modelsai.PeriodicReportRun{
				NamespaceName: periodicReportHandlerTestNamespace,
				PlanID:        activePlanID,
				TriggerAt:     nowTime.Add(-48 * time.Hour),
				Status:        modelsai.PeriodicReportRunStatusSuccess,
				ObjectKey:     serializer.NewS3Crypto("periodic-report/old.md"),
				SessionID:     &oldSession.ID,
			}
			if _, err := store.CreateRunIfAbsent(ctx, oldRun); err != nil {
				t.Fatalf("CreateRunIfAbsent(old) error = %v", err)
			}
			if err := db.Exec(`UPDATE periodic_report_runs SET created_at = ? WHERE id = ?`, nowTime.Add(-48*time.Hour), oldRun.ID).Error; err != nil {
				t.Fatalf("backdate old run: %v", err)
			}
			recentRun := &modelsai.PeriodicReportRun{
				NamespaceName: periodicReportHandlerTestNamespace,
				PlanID:        activePlanID,
				TriggerAt:     nowTime.Add(-time.Hour),
				Status:        modelsai.PeriodicReportRunStatusSuccess,
				ObjectKey:     serializer.NewS3Crypto("periodic-report/recent.md"),
			}
			if _, err := store.CreateRunIfAbsent(ctx, recentRun); err != nil {
				t.Fatalf("CreateRunIfAbsent(recent) error = %v", err)
			}

			remover := &fakePeriodicReportRemover{fail: tc.failRemoval}
			handler := newCleanupHandler(t, db, remover, nowTime)

			if err := handler.Do(ctx, &periodicReportTaskStub{typ: TaskRunCleanup, payload: periodicReportRunTaskPayload(t, 0, nowTime)}); err != nil {
				t.Fatalf("Do error = %v", err)
			}
			plan, err := store.GetPlanByID(ctx, expiredPlanID)
			if err != nil {
				t.Fatalf("GetPlanByID(expired) error = %v", err)
			}
			if plan.Enabled {
				t.Fatal("expired plan is still enabled, want disabled")
			}
			_, err = store.GetRun(ctx, periodicReportHandlerTestNamespace, oldRun.ID)
			_, sessionErr := sessions.GetSession(ctx, oldSession.ID, periodicReportHandlerTestNamespace, 1)
			if tc.failRemoval {
				if err != nil {
					t.Fatalf("old run should be retained on removal failure, got error %v", err)
				}
				if len(remover.removed) != 0 {
					t.Fatalf("removed objects = %v, want none", remover.removed)
				}
				if sessionErr != nil {
					t.Fatalf("old session should be retained on removal failure, got error %v", sessionErr)
				}
			} else {
				if !errors.Is(err, periodic.ErrRunNotFound) {
					t.Fatalf("old run error = %v, want ErrRunNotFound", err)
				}
				if len(remover.removed) != 1 || remover.removed[0] != oldRun.ObjectKey.Key() {
					t.Fatalf("removed objects = %v, want [%q]", remover.removed, oldRun.ObjectKey.Key())
				}
				if !errors.Is(sessionErr, session.ErrSessionNotFound) {
					t.Fatalf("old session error = %v, want ErrSessionNotFound", sessionErr)
				}
			}
			if _, err := store.GetRun(ctx, periodicReportHandlerTestNamespace, recentRun.ID); err != nil {
				t.Fatalf("recent run should be retained, got error %v", err)
			}
		})
	}
}
