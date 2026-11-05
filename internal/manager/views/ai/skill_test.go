package ai

import (
	// 标准库
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	skillstore "zhongkuisec.com/zhongkui/internal/ai/skill"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
)

// newSkillTestRouter 创建 AI 技能测试路由引擎，注入 i18n、日志、核心库、命名空间、当前用户
// 与 AI 运行时（需为启用态，requireAiEnabled 才放行）。
func newSkillTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
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
	group := r.Group("/ai/skill")
	group.POST("/list", AgentSkillListGet)
	group.POST("/options", AgentSkillOptionsGet)
	group.POST("/create", AgentSkillCreate)
	group.POST("/:id/get", AgentSkillDetailGet)
	group.POST("/:id/update", AgentSkillUpdate)
	group.POST("/delete", AgentSkillBatchDelete)
	group.POST("/:id/status", AgentSkillSetEnabled)
	return r
}

// skillPath 拼接技能子操作路径。
func skillPath(id uint, action string) string {
	return "/ai/skill/" + strconv.FormatUint(uint64(id), 10) + action
}

// skillDetailResponse 是技能创建 / 详情 / 更新响应体的最小解码结构。
type skillDetailResponse struct {
	// Code 业务响应码
	Code int `json:"code"`
	// Data 技能详情
	Data *AgentSkillDetailOut `json:"data"`
}

// createTestSkill 通过创建接口播种一条技能并返回其详情。
func createTestSkill(t *testing.T, r *gin.Engine, name, description, content string) *AgentSkillDetailOut {
	t.Helper()
	w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", map[string]any{
		"name":        name,
		"description": description,
		"content":     content,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("create skill %q: status = %d, body = %s", name, w.Code, w.Body.String())
	}
	var resp skillDetailResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode create skill response: %v", err)
	}
	if resp.Data == nil {
		t.Fatalf("create skill %q: empty data", name)
	}
	return resp.Data
}

// TestAgentSkillCRUD 覆盖技能的创建、重名拒绝、详情 404、更新、启停与批量删除语义。
func TestAgentSkillCRUD(t *testing.T) {
	Convey("技能 CRUD 接口", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		Convey("创建成功返回详情并加密落库", func() {
			out := createTestSkill(t, r, "triager", "按五元组排查", "先看五元组")
			So(out.ID, ShouldBeGreaterThan, 0)
			So(out.NamespaceName, ShouldEqual, knowledgeTestNamespace)
			So(out.Name, ShouldEqual, "triager")
			So(out.Description, ShouldEqual, "按五元组排查")
			So(out.Content, ShouldEqual, "先看五元组")
			So(out.Enabled, ShouldBeTrue)

			var row modelsai.AgentSkill
			So(db.Where("id = ?", out.ID).First(&row).Error, ShouldBeNil)
			So(row.NamespaceName, ShouldEqual, knowledgeTestNamespace)
			// 读取经 crypto 序列化器解密，应还原明文
			So(string(row.Content.Value()), ShouldEqual, "先看五元组")
		})

		Convey("重名创建返回 400", func() {
			createTestSkill(t, r, "duplicate", "", "正文")
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", map[string]any{
				"name": "duplicate", "content": "另一份正文",
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&modelsai.AgentSkill{}).
				Where("namespace_name = ? AND name = ?", knowledgeTestNamespace, "duplicate").
				Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 1)
		})

		Convey("详情查询不存在返回 404", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, skillPath(999999, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusNotFound)
		})

		Convey("更新覆盖名称、描述与正文", func() {
			out := createTestSkill(t, r, "before", "旧描述", "旧正文")
			w := doKnowledgeJSONRequest(r, http.MethodPost, skillPath(out.ID, "/update"), map[string]any{
				"name":        "after",
				"description": "新描述",
				"content":     "新正文",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp skillDetailResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.ID, ShouldEqual, out.ID)
			So(resp.Data.Name, ShouldEqual, "after")
			So(resp.Data.Description, ShouldEqual, "新描述")
			So(resp.Data.Content, ShouldEqual, "新正文")

			w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(out.ID, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Name, ShouldEqual, "after")
			So(resp.Data.Content, ShouldEqual, "新正文")
		})

		Convey("启停技能并落库", func() {
			out := createTestSkill(t, r, "toggle", "", "正文")
			w := doKnowledgeJSONRequest(r, http.MethodPost, skillPath(out.ID, "/status"), map[string]any{
				"enabled": false,
			})
			So(w.Code, ShouldEqual, http.StatusOK)

			var row modelsai.AgentSkill
			So(db.Where("id = ?", out.ID).First(&row).Error, ShouldBeNil)
			So(row.Enabled, ShouldBeFalse)

			w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(out.ID, "/get"), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp skillDetailResponse
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Enabled, ShouldBeFalse)
		})

		Convey("批量删除返回成功汇总并软删除", func() {
			first := createTestSkill(t, r, "first", "", "正文一")
			second := createTestSkill(t, r, "second", "", "正文二")
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/delete", map[string]any{
				"ids": []uint{first.ID, second.ID, 999999},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *struct {
					Succeeded int `json:"succeeded"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			// 不存在的 ID 计入失败明细，两条存在记录成功删除
			So(resp.Data.Succeeded, ShouldEqual, 2)

			var count int64
			So(db.Model(&modelsai.AgentSkill{}).
				Where("namespace_name = ?", knowledgeTestNamespace).Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestAgentSkillDefinitionFields 覆盖用户填写的周期能力、输入字段与固定参数随创建落库，详情与
// 列表都透出固定参数并保留数字 / 布尔等原始 JSON 类型，同时校验周期技能必须包含 nowTime。
func TestAgentSkillDefinitionFields(t *testing.T) {
	Convey("技能定义字段创建与周期约束", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", map[string]any{
			"name":            "fixed-output",
			"content":         "正文",
			"periodicCapable": true,
			"inputFields": []map[string]any{
				{"name": "nowTime", "type": "date", "required": true, "description": "当前时间"},
			},
			"fixedFields": map[string]any{
				"timeOffset": "-24h",
				"timeWindow": "168h",
				"threshold":  0.75,
				"onlyHigh":   true,
			},
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		var created skillDetailResponse
		So(json.Unmarshal(w.Body.Bytes(), &created), ShouldBeNil)
		So(created.Data, ShouldNotBeNil)
		So(created.Data.PeriodicCapable, ShouldBeTrue)
		So(created.Data.InputFields, ShouldHaveLength, 1)
		So(string(created.Data.FixedFields["threshold"]), ShouldEqual, "0.75")

		w = doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/list", map[string]any{
			"page": 1, "pageSize": 10,
			"fields": []string{"id", "name", "fixedFields", "periodicCapable"},
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		data := decodeSkillList(t, w.Body.Bytes())
		var listed map[string]any
		for _, item := range data.Items {
			if item["name"] == "fixed-output" {
				listed = item
				break
			}
		}
		So(listed, ShouldNotBeNil)
		So(listed["periodicCapable"], ShouldEqual, true)
		fixed, ok := listed["fixedFields"].(map[string]any)
		So(ok, ShouldBeTrue)
		So(fixed["threshold"], ShouldEqual, 0.75)
		So(fixed["onlyHigh"], ShouldEqual, true)

		// 周期技能缺少 nowTime：创建与更新都必须返回 400。
		Convey("周期技能缺少 nowTime 被拒绝", func() {
			w = doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", map[string]any{
				"name": "missing-now", "content": "正文", "periodicCapable": true,
				"inputFields": []map[string]any{
					{"name": "region", "type": "string", "required": false, "description": ""},
				},
				"fixedFields": map[string]any{"timeOffset": "-24h", "timeWindow": "24h"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)

			w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(created.Data.ID, "/update"), map[string]any{
				"name": "fixed-output", "content": "正文", "periodicCapable": true,
				"inputFields": []map[string]any{},
				"fixedFields": map[string]any{"timeOffset": "-24h", "timeWindow": "24h"},
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		// 周期技能固定参数缺少 / 非法时间窗口：创建与更新接口都必须返回 400。
		Convey("周期技能缺少合法时间窗口被拒绝", func() {
			cases := []struct {
				name  string
				fixed map[string]any
			}{
				{"missing-time-offset", map[string]any{"timeWindow": "24h"}},
				{"missing-time-window", map[string]any{"timeOffset": "-24h"}},
				{"invalid-time-offset", map[string]any{"timeOffset": "1d", "timeWindow": "24h"}},
				{"non-positive-time-window", map[string]any{"timeOffset": "-24h", "timeWindow": "0s"}},
			}
			periodicInputs := []map[string]any{
				{"name": "nowTime", "type": "date", "required": true, "description": ""},
			}
			for _, tc := range cases {
				Convey(tc.name+" 创建被拒绝", func() {
					w = doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", map[string]any{
						"name": tc.name, "content": "正文", "periodicCapable": true,
						"inputFields": periodicInputs,
						"fixedFields": tc.fixed,
					})
					So(w.Code, ShouldEqual, http.StatusBadRequest)
				})
				Convey(tc.name+" 更新被拒绝", func() {
					w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(created.Data.ID, "/update"), map[string]any{
						"name": "fixed-output", "content": "正文", "periodicCapable": true,
						"inputFields": periodicInputs,
						"fixedFields": tc.fixed,
					})
					So(w.Code, ShouldEqual, http.StatusBadRequest)
				})
			}
		})
	})
}

// TestAgentSkillCreateInputValidation 校验技能创建的名称 / 描述 / 正文约束统一由视图层
// binding 承担（存储层不再重复校验），并确认描述去空白后落库、正文原样保留。
func TestAgentSkillCreateInputValidation(t *testing.T) {
	Convey("技能创建输入 binding 校验", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		cases := []struct {
			name string
			body map[string]any
		}{
			{"blank name", map[string]any{"name": "   ", "content": "正文"}},
			{"padded name", map[string]any{"name": " triager ", "content": "正文"}},
			{"too long name", map[string]any{"name": strings.Repeat("技", 65), "content": "正文"}},
			{"too long description", map[string]any{
				"name": "ok", "description": strings.Repeat("d", 257), "content": "正文",
			}},
			{"blank content", map[string]any{"name": "ok", "content": "   "}},
			{"too long content", map[string]any{"name": "ok", "content": strings.Repeat("a", 32769)}},
		}
		for _, tc := range cases {
			Convey(tc.name+" 返回 400", func() {
				w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/create", tc.body)
				So(w.Code, ShouldEqual, http.StatusBadRequest)
			})
		}

		Convey("描述去空白后落库、正文保留原样", func() {
			out := createTestSkill(t, r, "trimmed", "  按五元组排查  ", " 正文 ")
			So(out.Description, ShouldEqual, "按五元组排查")
			So(out.Content, ShouldEqual, " 正文 ")
		})
	})
}

// TestAgentSkillListFilterAndProjection 覆盖技能列表的 keyword / enabled 过滤、排序与字段投影：
// 投影只输出请求字段，未指定字段时回退白名单全集，列表不含加密正文。
func TestAgentSkillListFilterAndProjection(t *testing.T) {
	Convey("技能列表过滤与字段投影", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))
		alpha := createTestSkill(t, r, "alpha", "第一位", "正文 alpha")
		createTestSkill(t, r, "beta", "第二位", "正文 beta")
		gamma := createTestSkill(t, r, "gamma", "第三位", "正文 gamma")
		// 停用 gamma，覆盖 enabled 过滤分支
		So(doKnowledgeJSONRequest(r, http.MethodPost, skillPath(gamma.ID, "/status"), map[string]any{
			"enabled": false,
		}).Code, ShouldEqual, http.StatusOK)

		Convey("keyword 按名称模糊过滤", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/list", map[string]any{
				"filterBy": map[string]any{
					"keyword": map[string]any{"action": "ctn", "value": "alp"},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			data := decodeSkillList(t, w.Body.Bytes())
			So(data.Total, ShouldEqual, 1)
			So(len(data.Items), ShouldEqual, 1)
			So(data.Items[0]["id"], ShouldEqual, float64(alpha.ID))
			So(data.Items[0]["name"], ShouldEqual, "alpha")
			// 列表不返回正文
			_, exists := data.Items[0]["content"]
			So(exists, ShouldBeFalse)
		})

		Convey("enabled 过滤仅返回停用技能", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/list", map[string]any{
				"filterBy": map[string]any{
					"enabled": map[string]any{"action": "eq", "value": false},
				},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			data := decodeSkillList(t, w.Body.Bytes())
			So(data.Total, ShouldEqual, 1)
			So(len(data.Items), ShouldEqual, 1)
			So(data.Items[0]["name"], ShouldEqual, "gamma")
			So(data.Items[0]["enabled"], ShouldEqual, false)
		})

		Convey("字段投影只输出请求字段", func() {
			w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/list", map[string]any{
				"page": 1, "pageSize": 10,
				"fields": []string{"id", "name"},
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			data := decodeSkillList(t, w.Body.Bytes())
			So(data.Total, ShouldEqual, 5)
			So(len(data.Items), ShouldEqual, 5)
			for _, item := range data.Items {
				So(len(item), ShouldEqual, 2)
				_, hasID := item["id"]
				_, hasName := item["name"]
				So(hasID, ShouldBeTrue)
				So(hasName, ShouldBeTrue)
			}
		})
	})
}

// skillListData 是技能列表响应体的数据段。
type skillListData struct {
	// Total 过滤后的总数
	Total uint `json:"total"`
	// Items 当前页的字段投影项
	Items []map[string]any `json:"items"`
}

// TestAgentSkillOptionsMergeBuiltinAndEnabledCustom 校验选择器选项接口：内置技能在前且跨命名
// 空间恒可用，自定义技能只含当前命名空间已启用项，且均以统一引用表达。
func TestAgentSkillOptionsMergeBuiltinAndEnabledCustom(t *testing.T) {
	Convey("技能选项合并内置与已启用自定义技能", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		// 技能列表用例只关心选择器语义，这里直接写库模拟一条已启用的技能行。
		enabled := modelsai.AgentSkill{
			NamespaceName: knowledgeTestNamespace,
			Source:        modelsai.SkillSourceCustom,
			Name:          "option-enabled",
			Description:   "启用技能",
			Enabled:       true,
		}
		So(db.Create(&enabled).Error, ShouldBeNil)
		disabled := modelsai.AgentSkill{
			NamespaceName: knowledgeTestNamespace,
			Source:        modelsai.SkillSourceCustom,
			Name:          "option-disabled",
			Description:   "停用技能",
			Enabled:       true,
		}
		So(db.Create(&disabled).Error, ShouldBeNil)
		statusResp := doKnowledgeJSONRequest(r, http.MethodPost, skillPath(disabled.ID, "/status"), map[string]any{"enabled": false})
		So(statusResp.Code, ShouldEqual, http.StatusOK)

		// 内置技能注册行由 fixture 注入系统命名空间；测试内直接写库模拟出厂行，并追加代码正文。
		for _, row := range []modelsai.AgentSkill{
			{NamespaceName: base.SystemNamespace, Source: modelsai.SkillSourceBuiltin, Name: "option_builtin", Description: "内置说明"},
			{NamespaceName: base.SystemNamespace, Source: modelsai.SkillSourceBuiltin, Name: "option_builtin_disabled", Description: "不应出现"},
		} {
			row := row
			So(db.Create(&row).Error, ShouldBeNil)
		}
		// enabled 的 gorm default:true 会在 Create 时替换零值，故停用状态单独更新落库。
		So(db.Model(&modelsai.AgentSkill{}).
			Where("namespace_name = ? AND name = ?", base.SystemNamespace, "option_builtin_disabled").
			Update("enabled", false).Error, ShouldBeNil)
		original := append([]skillstore.BuiltinSkill(nil), skillstore.BuiltinSkills...)
		defer func() { skillstore.BuiltinSkills = original }()
		skillstore.BuiltinSkills = append(skillstore.BuiltinSkills,
			skillstore.BuiltinSkill{Name: "option_builtin", Content: "正文"},
			skillstore.BuiltinSkill{Name: "option_builtin_disabled", Content: "正文"},
		)

		var builtinRow modelsai.AgentSkill
		So(db.Where("namespace_name = ? AND name = ?", base.SystemNamespace, "option_builtin").
			First(&builtinRow).Error, ShouldBeNil)

		w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/options", nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Code int                      `json:"code"`
			Data *AgentSkillOptionListOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		So(resp.Data, ShouldNotBeNil)
		So(resp.Data.Items, ShouldHaveLength, 4)
		// fixture 注入的两个出厂内置技能排在最前；随后是测试追加的内置技能与已启用自定义技能。
		So(resp.Data.Items[2].ID, ShouldEqual, builtinRow.ID)
		So(resp.Data.Items[2].Name, ShouldEqual, "option_builtin")
		So(resp.Data.Items[3].ID, ShouldEqual, enabled.ID)
		So(resp.Data.Items[3].Name, ShouldEqual, enabled.Name)
	})
}

// TestAgentSkillBuiltinReadonlyAndEnabled 校验内置技能与工具一致的管理语义：列表可见且以
// source='builtin' 表达只读，编辑与批量删除整批拒绝，启停可用，停用后不再出现在选择器选项中。
func TestAgentSkillBuiltinReadonlyAndEnabled(t *testing.T) {
	Convey("内置技能只读与启停", t, func() {
		db := createKnowledgeTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := newSkillTestRouter(db, knowledgeTestNamespace, newEnabledAiRuntime(t, db))

		row := modelsai.AgentSkill{
			NamespaceName: base.SystemNamespace,
			Source:        modelsai.SkillSourceBuiltin,
			Name:          "readonly_builtin",
			Description:   "内置只读技能",
		}
		So(db.Create(&row).Error, ShouldBeNil)
		original := append([]skillstore.BuiltinSkill(nil), skillstore.BuiltinSkills...)
		defer func() { skillstore.BuiltinSkills = original }()
		skillstore.BuiltinSkills = append(skillstore.BuiltinSkills,
			skillstore.BuiltinSkill{Name: "readonly_builtin", Content: "## 正文"},
		)

		// 列表纳入内置行，source 直接表达只读语义
		w := doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/list", map[string]any{
			"greedy": false, "page": 1, "pageSize": 10,
			"fields": []string{"id", "name", "source", "enabled"},
		})
		So(w.Code, ShouldEqual, http.StatusOK)
		data := decodeSkillList(t, w.Body.Bytes())
		var listed map[string]any
		for _, item := range data.Items {
			if item["name"] == "readonly_builtin" {
				listed = item
				break
			}
		}
		So(listed, ShouldNotBeNil)
		So(listed["source"], ShouldEqual, string(modelsai.SkillSourceBuiltin))

		// 内置技能禁止编辑
		w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(row.ID, "/update"), map[string]any{
			"name": "renamed", "description": "d", "content": "c",
		})
		So(w.Code, ShouldEqual, http.StatusBadRequest)

		// 内置技能禁止删除，且整批拒绝
		w = doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/delete", map[string]any{"ids": []uint{row.ID}})
		So(w.Code, ShouldEqual, http.StatusBadRequest)

		// 启停可用
		w = doKnowledgeJSONRequest(r, http.MethodPost, skillPath(row.ID, "/status"), map[string]any{"enabled": false})
		So(w.Code, ShouldEqual, http.StatusOK)

		// 停用后不再进入选择器选项
		w = doKnowledgeJSONRequest(r, http.MethodPost, "/ai/skill/options", nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var options struct {
			Code int                      `json:"code"`
			Data *AgentSkillOptionListOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &options), ShouldBeNil)
		So(options.Data, ShouldNotBeNil)
		// fixture 注入的出厂内置技能仍可用；此处只断言被停用的测试内置技能不再出现。
		for _, item := range options.Data.Items {
			So(item.Name, ShouldNotEqual, "readonly_builtin")
		}
	})
}

// decodeSkillList 解码技能列表响应体并返回数据段。
func decodeSkillList(t *testing.T, body []byte) *skillListData {
	t.Helper()
	var resp struct {
		Code int            `json:"code"`
		Data *skillListData `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode skill list response: %v", err)
	}
	if resp.Data == nil {
		t.Fatalf("skill list response has empty data: %s", string(body))
	}
	return resp.Data
}
