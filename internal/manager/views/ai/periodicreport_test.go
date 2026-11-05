package ai

import (
	// 标准库
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/datatypes"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// periodicReportOtherTestNamespace 报告视图测试的跨命名空间用例命名空间。
const periodicReportOtherTestNamespace = "reportother"

// newPeriodicReportTestRouter 创建报告视图测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时（需为启用态，requireAiEnabled 才放行）；注册报告计划列表/创建、运行记录、关注与信息流路由。
func newPeriodicReportTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newKnowledgeTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Set(middlewares.AiRuntimeCtxName, rt)
		user := &core.User{}
		user.ID = sessionTestUserID
		c.Set(middlewares.UserCtxName, user)
		c.Next()
	})
	group := r.Group("/ai/periodic-report")
	group.POST("/plan/list", PeriodicReportPlanListGet)
	group.POST("/plan/create", PeriodicReportPlanCreate)
	group.POST("/run/:id/get", PeriodicReportRunDetailGet)
	group.POST("/run/:id/delete", PeriodicReportRunDelete)
	group.POST("/feed/list", PeriodicReportFeedListGet)
	group.POST("/watch/list", PeriodicReportWatchListGet)
	return r
}

// periodicReportRunPath 拼接报告运行记录子操作路径。
func periodicReportRunPath(id uint, action string) string {
	return "/ai/periodic-report/run/" + strconv.FormatUint(uint64(id), 10) + action
}

// seedPeriodicReportRun 在给定命名空间插入一条报告运行记录并返回其 ID；对象键为空时模拟尚无产出。
func seedPeriodicReportRun(t *testing.T, db *database.DB, namespace, objectKey string) uint {
	t.Helper()
	_, runID := seedPeriodicReportPlanAndRun(t, db, namespace, objectKey)
	return runID
}

// seedPeriodicReportPlanAndRun 在给定命名空间准备外键依赖（命名空间、技能、计划）并插入一条成功的
// 报告运行记录，返回计划 ID 与运行记录 ID。
func seedPeriodicReportPlanAndRun(t *testing.T, db *database.DB, namespace, objectKey string) (uint, uint) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO namespaces (name, description) VALUES (?, '') ON CONFLICT (name) DO NOTHING`,
		namespace,
	).Error; err != nil {
		t.Fatalf("seed periodic report run namespace %q: %v", namespace, err)
	}
	var skillID uint
	if err := db.Raw(
		`INSERT INTO agent_skills (namespace_name, source, name, description, content, enabled, periodic_capable, input_fields, fixed_fields)
		 VALUES (?, 'custom', ?, '', NULL, true, true, '[{"name":"nowTime","type":"date","required":true,"description":""}]'::jsonb, '{"timeOffset":"-24h","timeWindow":"24h"}'::jsonb)
		 RETURNING id`,
		namespace, fmt.Sprintf("seed-skill-%d", time.Now().UnixNano()),
	).Scan(&skillID).Error; err != nil {
		t.Fatalf("seed periodic report run skill: %v", err)
	}
	plan := &modelsai.PeriodicReportPlan{
		NamespaceName: namespace,
		Name:          "seed plan",
		SkillID:       skillID,
		CronSpec:      "0 8 * * *",
		UserID:        sessionTestUserID,
		RetentionDays: 90,
	}
	if err := db.Create(plan).Error; err != nil {
		t.Fatalf("seed periodic report plan: %v", err)
	}
	run := &modelsai.PeriodicReportRun{
		NamespaceName: namespace,
		PlanID:        plan.ID,
		TriggerAt:     time.Now().Truncate(time.Second),
		Status:        modelsai.PeriodicReportRunStatusSuccess,
		ObjectKey:     objectKey,
		Title:         "daily report",
	}
	if err := db.Create(run).Error; err != nil {
		t.Fatalf("seed periodic report run: %v", err)
	}
	return plan.ID, run.ID
}

// TestNewPeriodicReportPlanItemOut 覆盖计划映射：input_values 按原始 JSON 类型解码、坏 JSON 与空值均回退为空映射。
func TestNewPeriodicReportPlanItemOut(t *testing.T) {
	Convey("报告计划映射", t, func() {
		expiresAt := time.Now().Add(time.Hour)
		nextRunAt := time.Now().Add(time.Minute)
		m := &modelsai.PeriodicReportPlan{
			Name:          "daily",
			Description:   "说明",
			SkillID:       7,
			CronSpec:      "0 8 * * *",
			InputValues:   datatypes.JSON([]byte(`{"threshold":0.75,"onlyHigh":true,"region":"cn"}`)),
			Enabled:       true,
			RetentionDays: 30,
			ExpiresAt:     &expiresAt,
			NextRunAt:     &nextRunAt,
		}
		m.ID = 3
		out := newPeriodicReportPlanItemOut(m)
		So(out.ID, ShouldEqual, 3)
		So(out.Name, ShouldEqual, "daily")
		So(out.SkillID, ShouldEqual, 7)
		So(out.Enabled, ShouldBeTrue)
		So(out.RetentionDays, ShouldEqual, 30)
		So(out.ExpiresAt, ShouldEqual, &expiresAt)
		So(out.NextRunAt, ShouldEqual, &nextRunAt)
		So(string(out.InputValues["threshold"]), ShouldEqual, "0.75")
		So(string(out.InputValues["onlyHigh"]), ShouldEqual, "true")
		So(string(out.InputValues["region"]), ShouldEqual, `"cn"`)

		m.InputValues = datatypes.JSON([]byte(`{`))
		out = newPeriodicReportPlanItemOut(m)
		So(out.InputValues, ShouldNotBeNil)
		So(len(out.InputValues), ShouldEqual, 0)

		m.InputValues = nil
		out = newPeriodicReportPlanItemOut(m)
		So(out.InputValues, ShouldNotBeNil)
		So(len(out.InputValues), ShouldEqual, 0)
	})
}

// TestNewPeriodicReportRunItemOut 覆盖运行记录映射：状态、时间与可空字段逐字段透传。
func TestNewPeriodicReportRunItemOut(t *testing.T) {
	Convey("报告运行记录映射", t, func() {
		startedAt := time.Now().Add(-time.Minute)
		finishedAt := time.Now()
		m := &modelsai.PeriodicReportRun{
			PlanID:     11,
			Status:     modelsai.PeriodicReportRunStatusFailed,
			Title:      "weekly",
			ObjectKey:  "periodic-report/11.tar.gz",
			SizeBytes:  1024,
			Error:      "boom",
			TriggerAt:  finishedAt,
			StartedAt:  &startedAt,
			FinishedAt: &finishedAt,
		}
		m.ID = 5
		out := newPeriodicReportRunItemOut(m)
		So(out.ID, ShouldEqual, 5)
		So(out.PlanID, ShouldEqual, 11)
		So(out.Status, ShouldEqual, string(modelsai.PeriodicReportRunStatusFailed))
		So(out.Title, ShouldEqual, "weekly")
		So(out.ObjectKey, ShouldEqual, "periodic-report/11.tar.gz")
		So(out.SizeBytes, ShouldEqual, 1024)
		So(out.Error, ShouldEqual, "boom")
		So(out.TriggerAt, ShouldEqual, finishedAt)
		So(out.StartedAt, ShouldEqual, &startedAt)
		So(out.FinishedAt, ShouldEqual, &finishedAt)
	})
}

// TestNewPeriodicReportSkillOptionOut 覆盖技能选项映射：输入字段与固定参数按原始类型解析，坏 JSON 回退为空值。
func TestNewPeriodicReportSkillOptionOut(t *testing.T) {
	Convey("报告技能选项映射", t, func() {
		m := &modelsai.AgentSkill{
			Name:        "skill-a",
			Description: "周期技能",
			InputFields: datatypes.JSON([]byte(`[{"name":"nowTime","type":"date","required":true,"description":"运行时注入"},{"name":"note","type":"string","required":false}]`)),
			FixedFields: datatypes.JSON([]byte(`{"timeOffset":"-24h","timeWindow":"24h","threshold":0.75,"onlyHigh":true}`)),
		}
		m.ID = 8
		out := newPeriodicReportSkillOptionOut(m)
		So(out.ID, ShouldEqual, 8)
		So(out.Name, ShouldEqual, "skill-a")
		So(out.Description, ShouldEqual, "周期技能")
		So(out.InputFields, ShouldHaveLength, 2)
		So(out.InputFields[0].Name, ShouldEqual, "nowTime")
		So(out.InputFields[0].Type, ShouldEqual, "date")
		So(out.InputFields[0].Required, ShouldBeTrue)
		So(out.InputFields[0].Description, ShouldEqual, "运行时注入")
		So(out.InputFields[1].Required, ShouldBeFalse)
		So(string(out.FixedFields["timeOffset"]), ShouldEqual, `"-24h"`)
		So(string(out.FixedFields["threshold"]), ShouldEqual, "0.75")
		So(string(out.FixedFields["onlyHigh"]), ShouldEqual, "true")

		m.InputFields = datatypes.JSON([]byte(`[`))
		m.FixedFields = datatypes.JSON([]byte(`[`))
		out = newPeriodicReportSkillOptionOut(m)
		So(out.InputFields, ShouldBeEmpty)
		So(out.FixedFields, ShouldBeEmpty)
	})
}

// periodicReportRunDetailResponse 是报告运行详情响应的最小解码结构。
type periodicReportRunDetailResponse struct {
	// Code 业务响应码
	Code int `json:"code"`
	// Data 运行详情
	Data *PeriodicReportRunDetailOut `json:"data"`
}

// projectedListResponse 是列表接口（投影出参）的最小解码结构。
type projectedListResponse struct {
	// Code 业务响应码
	Code int `json:"code"`
	// Data 投影列表
	Data struct {
		// Total 满足条件的记录总数
		Total uint `json:"total"`
		// Items 当前页条目（键为投影白名单字段）
		Items []map[string]any `json:"items"`
	} `json:"data"`
}

// TestPeriodicReportPlanListSearch 覆盖报告计划列表的搜索契约：keyword 过滤走 search.toml 白名单列，
// 命中时 skillName 由第二趟按 skill_id 回填，未命中时返回空列表。
func TestPeriodicReportPlanListSearch(t *testing.T) {
	Convey("报告计划列表关键词过滤", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		seedPeriodicReportPlanAndRun(t, db, knowledgeTestNamespace, "")

		search := func(keyword string) *projectedListResponse {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/periodic-report/plan/list", map[string]any{
				"greedy":   false,
				"page":     1,
				"pageSize": 10,
				"orderBy":  []map[string]any{{"name": "createdAt", "order": -1}},
				"filterBy": map[string]any{
					"keyword": map[string]any{"action": "ictn", "value": keyword},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp projectedListResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			return &resp
		}

		hit := search("seed")
		So(hit.Data.Total, ShouldEqual, 1)
		So(hit.Data.Items, ShouldHaveLength, 1)
		So(hit.Data.Items[0]["name"], ShouldEqual, "seed plan")
		So(hit.Data.Items[0]["skillName"], ShouldNotBeEmpty)

		miss := search("no-such-plan")
		So(miss.Data.Total, ShouldEqual, 0)
		So(miss.Data.Items, ShouldBeEmpty)
	})
}

// TestPeriodicReportFeedListWatchedSuccess 覆盖信息流的关注与状态过滤：未关注任何计划时返回空列表，
// 关注后只返回该计划的成功运行记录并回填计划名称。
func TestPeriodicReportFeedListWatchedSuccess(t *testing.T) {
	Convey("报告信息流只返回关注计划的成功运行记录", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		planID, runID := seedPeriodicReportPlanAndRun(t, db, knowledgeTestNamespace, "")
		if err := db.Create(&modelsai.PeriodicReportRun{
			NamespaceName: knowledgeTestNamespace,
			PlanID:        planID,
			TriggerAt:     time.Now().Add(time.Minute),
			Status:        modelsai.PeriodicReportRunStatusFailed,
			Title:         "failed report",
		}).Error; err != nil {
			t.Fatalf("seed failed periodic report run: %v", err)
		}

		feed := func() *projectedListResponse {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/periodic-report/feed/list", map[string]any{
				"greedy":   false,
				"page":     1,
				"pageSize": 10,
				"orderBy":  []map[string]any{{"name": "createdAt", "order": -1}},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp projectedListResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			return &resp
		}

		empty := feed()
		So(empty.Data.Total, ShouldEqual, 0)
		So(empty.Data.Items, ShouldBeEmpty)

		if err := db.Create(&modelsai.PeriodicReportPlanWatch{
			UserID:        sessionTestUserID,
			NamespaceName: knowledgeTestNamespace,
			PlanID:        planID,
		}).Error; err != nil {
			t.Fatalf("seed periodic report plan watch: %v", err)
		}
		filled := feed()
		So(filled.Data.Total, ShouldEqual, 1)
		So(filled.Data.Items, ShouldHaveLength, 1)
		So(filled.Data.Items[0]["runID"], ShouldEqual, float64(runID))
		So(filled.Data.Items[0]["planName"], ShouldEqual, "seed plan")
		So(filled.Data.Items[0]["status"], ShouldEqual, string(modelsai.PeriodicReportRunStatusSuccess))
	})
}

// TestPeriodicReportWatchListSearch 覆盖关注计划候选列表的搜索契约：keyword 过滤走 search.toml
// 白名单列，watched 由当前用户关注关系回填，返回统一的分页 total/items 结构。
func TestPeriodicReportWatchListSearch(t *testing.T) {
	Convey("关注计划候选列表搜索", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		planID, _ := seedPeriodicReportPlanAndRun(t, db, knowledgeTestNamespace, "")

		search := func(keyword string) *projectedListResponse {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/periodic-report/watch/list", map[string]any{
				"greedy":   false,
				"page":     1,
				"pageSize": 10,
				"orderBy":  []map[string]any{{"name": "createdAt", "order": -1}},
				"filterBy": map[string]any{
					"keyword": map[string]any{"action": "ictn", "value": keyword},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp projectedListResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			return &resp
		}

		hit := search("seed")
		So(hit.Data.Total, ShouldEqual, 1)
		So(hit.Data.Items, ShouldHaveLength, 1)
		So(hit.Data.Items[0]["id"], ShouldEqual, float64(planID))
		So(hit.Data.Items[0]["watched"], ShouldEqual, false)

		if err := db.Create(&modelsai.PeriodicReportPlanWatch{
			UserID:        sessionTestUserID,
			NamespaceName: knowledgeTestNamespace,
			PlanID:        planID,
		}).Error; err != nil {
			t.Fatalf("seed periodic report plan watch: %v", err)
		}
		watched := search("seed")
		So(watched.Data.Items, ShouldHaveLength, 1)
		So(watched.Data.Items[0]["watched"], ShouldEqual, true)

		miss := search("no-such-plan")
		So(miss.Data.Total, ShouldEqual, 0)
		So(miss.Data.Items, ShouldBeEmpty)
	})
}

// TestPeriodicReportPlanCreateRetention 覆盖保留天数在视图层的默认值与范围校验：缺省填充 90 天，
// 越界值在绑定阶段被拒绝，不会进入领域层。
func TestPeriodicReportPlanCreateRetention(t *testing.T) {
	Convey("报告计划保留天数视图层校验", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		planID, _ := seedPeriodicReportPlanAndRun(t, db, knowledgeTestNamespace, "")
		var seeded modelsai.PeriodicReportPlan
		if err := db.First(&seeded, planID).Error; err != nil {
			t.Fatalf("load seeded plan: %v", err)
		}

		create := func(retention any, include bool) *httptest.ResponseRecorder {
			body := map[string]any{
				"name":        "retention plan",
				"skillID":     seeded.SkillID,
				"cronSpec":    "0 8 * * *",
				"inputValues": map[string]any{},
			}
			if include {
				body["retentionDays"] = retention
			}
			return doKnowledgeJSONRequest(r, http.MethodPost, "/ai/periodic-report/plan/create", body)
		}

		w := create(nil, false)
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Data struct {
				RetentionDays int `json:"retentionDays"`
			} `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data.RetentionDays, ShouldEqual, 90)

		for _, retention := range []int{0, -1, 3651} {
			w := create(retention, true)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		}
	})
}

// TestPeriodicReportRunDelete 覆盖运行记录删除：本命名空间删除成功且随后查询 404；跨命名空间删除
// 一律 404，不删除他命名空间的数据。
func TestPeriodicReportRunDelete(t *testing.T) {
	Convey("报告运行记录删除", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		Convey("本命名空间删除后详情返回 404", func() {
			id := seedPeriodicReportRun(t, db, knowledgeTestNamespace, "")
			w := doKnowledgeJSONRequest(r, http.MethodPost, periodicReportRunPath(id, "/delete"), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			w = doKnowledgeJSONRequest(r, http.MethodPost, periodicReportRunPath(id, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("跨命名空间删除返回 404", func() {
			id := seedPeriodicReportRun(t, db, periodicReportOtherTestNamespace, "")
			w := doKnowledgeJSONRequest(r, http.MethodPost, periodicReportRunPath(id, "/delete"), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})
	})
}

// TestPeriodicReportRunDetailIsolation 覆盖运行详情接口的命名空间隔离与无产出分支：跨命名空间一律
// 404；无对象键时返回条目且正文为空。不依赖对象存储与调度组件。
func TestPeriodicReportRunDetailIsolation(t *testing.T) {
	Convey("报告运行详情命名空间隔离", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newPeriodicReportTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		Convey("跨命名空间读取返回 404", func() {
			id := seedPeriodicReportRun(t, db, periodicReportOtherTestNamespace, "")
			w := doKnowledgeJSONRequest(r, http.MethodPost, periodicReportRunPath(id, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("无产出时返回空正文", func() {
			id := seedPeriodicReportRun(t, db, knowledgeTestNamespace, "")
			w := doKnowledgeJSONRequest(r, http.MethodPost, periodicReportRunPath(id, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp periodicReportRunDetailResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.ID, ShouldEqual, id)
			So(resp.Data.Content, ShouldBeEmpty)
		})
	})
}
