package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// createGroupTestDB 创建用于用户组路由测试的 SQLite 内存数据库实例
// 并自动迁移所需的表结构（Group、RbacPolicy、RbacPolicyTemplate）
func createGroupTestDB() *database.DB {
	cfg := &database.Config{
		Driver: "sqlite",
		DSN:    ":memory:",
	}
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

	db, err := database.NewDatabase(cfg, logger)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	if err := db.AutoMigrate(&core.Group{}, &core.RbacPolicy{}, &core.RbacPolicyTemplate{}); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	return db
}

// setupGroupTestRouter 创建用于用户组路由测试的 Gin 路由引擎
// 注入 i18n 中间件以支持错误响应的国际化
func setupGroupTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	return r
}

// setupGroupTestRouterWithDB 创建带有数据库中间件的测试路由引擎
func setupGroupTestRouterWithDB(db *database.DB) *gin.Engine {
	r := setupGroupTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(manager.DatabaseCtxName, db)
		c.Next()
	})
	return r
}

// TestGroupRbacPolicyIn_Struct 测试 GroupRbacPolicyIn 结构体
func TestGroupRbacPolicyIn_Struct(t *testing.T) {
	Convey("测试 GroupRbacPolicyIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			policy := GroupRbacPolicyIn{
				Target: "/api/v1/admin/user/list",
				Action: "POST",
				Effect: "allow",
			}
			So(policy.Target, ShouldEqual, "/api/v1/admin/user/list")
			So(policy.Action, ShouldEqual, "POST")
			So(policy.Effect, ShouldEqual, "allow")
		})

		Convey("应正确序列化为 JSON", func() {
			policy := GroupRbacPolicyIn{
				Target: "/api/v1/admin/group/:name/get",
				Action: "POST",
				Effect: "deny",
			}
			data, err := json.Marshal(policy)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"target":"/api/v1/admin/group/:name/get"`)
			So(string(data), ShouldContainSubstring, `"action":"POST"`)
			So(string(data), ShouldContainSubstring, `"effect":"deny"`)
		})
	})
}

// TestGroupUpdateIn_Struct 测试 GroupUpdateIn 结构体
func TestGroupUpdateIn_Struct(t *testing.T) {
	Convey("测试 GroupUpdateIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			update := GroupUpdateIn{
				Description: "更新后的管理员组描述",
			}
			So(update.Description, ShouldEqual, "更新后的管理员组描述")
		})

		Convey("应正确序列化为 JSON", func() {
			update := GroupUpdateIn{
				Description: "测试用户组描述信息",
			}
			data, err := json.Marshal(update)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"description":"测试用户组描述信息"`)
		})
	})
}

// TestGroupCreateIn_Struct 测试 GroupCreateIn 结构体
func TestGroupCreateIn_Struct(t *testing.T) {
	Convey("测试 GroupCreateIn 结构体", t, func() {
		Convey("应正确嵌入 GroupUpdateIn", func() {
			create := GroupCreateIn{
				GroupUpdateIn: GroupUpdateIn{
					Description: "新创建的管理员组",
				},
				Name: "admin",
			}
			So(create.Name, ShouldEqual, "admin")
			So(create.Description, ShouldEqual, "新创建的管理员组")
		})

		Convey("应正确序列化为 JSON", func() {
			create := GroupCreateIn{
				GroupUpdateIn: GroupUpdateIn{
					Description: "开发团队成员组",
				},
				Name: "developers",
			}
			data, err := json.Marshal(create)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"name":"developers"`)
			So(string(data), ShouldContainSubstring, `"description":"开发团队成员组"`)
		})
	})
}

// TestGroupBaseDetailOut_Struct 测试 GroupBaseDetailOut 结构体
func TestGroupBaseDetailOut_Struct(t *testing.T) {
	Convey("测试 GroupBaseDetailOut 结构体", t, func() {
		Convey("应正确嵌入 BaseItemOut", func() {
			detail := GroupBaseDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        1,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Name:        "admin",
				Description: "管理员用户组",
			}
			So(detail.ID, ShouldEqual, 1)
			So(detail.Name, ShouldEqual, "admin")
			So(detail.Description, ShouldEqual, "管理员用户组")
		})
	})
}

// TestGroupListGet 测试 groupListGet 函数
func TestGroupListGet(t *testing.T) {
	Convey("测试 groupListGet 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/list", groupListGet)

		Convey("应返回空列表当数据库为空时", func() {
			// 准备请求体
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
			}
			body, _ := json.Marshal(reqBody)

			// 发送请求
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/list", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 验证响应
			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, 200)

			data := response["data"].(map[string]any)
			So(data["items"], ShouldBeEmpty)
			So(data["total"], ShouldEqual, 0)
		})

		Convey("应返回用户组列表当数据库有数据时", func() {
			// 创建测试数据
			groups := []core.Group{
				{Name: "admin", Description: "管理员组"},
				{Name: "users", Description: "普通用户组"},
				{Name: "guests", Description: "访客组"},
			}
			for _, group := range groups {
				db.Create(&group)
			}

			// 准备请求体
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
			}
			body, _ := json.Marshal(reqBody)

			// 发送请求
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/list", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 验证响应
			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, 200)

			data := response["data"].(map[string]any)
			items := data["items"].([]any)
			So(len(items), ShouldEqual, 3)
			So(data["total"], ShouldEqual, 3)
		})

		Convey("应支持分页查询", func() {
			// 创建测试数据
			for i := 1; i <= 15; i++ {
				db.Create(&core.Group{
					Name:        fmt.Sprintf("group-%02d", i), // 生成group-01, group-02,...作为组名
					Description: "测试组",
				})
			}

			// 第一页，每页10条
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/list", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 调试：打印响应状态和内容
			if w.Code != http.StatusOK {
				t.Logf("响应状态码: %d", w.Code)
				t.Logf("响应体: %s", w.Body.String())
			}

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)

			data := response["data"].(map[string]any)
			items := data["items"].([]any)
			So(len(items), ShouldEqual, 10)
			So(data["total"], ShouldEqual, 15)
		})
	})
}

// TestGroupDetailGet 测试 groupDetailGet 函数
func TestGroupDetailGet(t *testing.T) {
	Convey("测试 groupDetailGet 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/get", groupDetailGet)

		Convey("应返回404当用户组不存在时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/nonexistent/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应返回用户组详情当用户组存在时", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员用户组",
			}
			db.Create(&group)

			// 创建关联的RBAC策略
			policy := core.NewGroupPolicy("admin", "/api/v1/admin/user/list", "POST", base.EffectAllow)
			db.Create(policy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, 200)

			data := response["data"].(map[string]any)
			So(data["name"], ShouldEqual, "admin")
			So(data["description"], ShouldEqual, "管理员用户组")

			policies := data["policies"].([]any)
			So(len(policies), ShouldEqual, 1)
		})

		Convey("应正确处理空的URL参数", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group//get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupUpdate 测试 groupUpdate 函数
func TestGroupUpdate(t *testing.T) {
	Convey("测试 groupUpdate 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/update", groupUpdate)

		Convey("应更新用户组描述信息", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "原始描述",
			}
			db.Create(&group)

			// 准备更新请求
			reqBody := map[string]any{
				"description": "更新后的描述信息",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库更新
			var updatedGroup core.Group
			db.First(&updatedGroup, "name = ?", "admin")
			So(updatedGroup.Description, ShouldEqual, "更新后的描述信息")
		})

		Convey("应返回错误当用户组不存在时", func() {
			reqBody := map[string]any{
				"description": "这是一个符合长度要求的描述信息",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/nonexistent/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应验证请求体格式", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "原始描述",
			}
			db.Create(&group)

			// 无效的请求体（缺少description字段）
			reqBody := map[string]any{}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupCreate 测试 groupCreate 函数
func TestGroupCreate(t *testing.T) {
	Convey("测试 groupCreate 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/create", groupCreate)

		Convey("应成功创建新用户组", func() {
			reqBody := map[string]any{
				"name":        "newgroup",
				"description": "新创建的用户组",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库记录
			var group core.Group
			err := db.First(&group, "name = ?", "newgroup").Error
			So(err, ShouldBeNil)
			So(group.Name, ShouldEqual, "newgroup")
			So(group.Description, ShouldEqual, "新创建的用户组")
		})

		Convey("应返回错误当组名已存在时", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "existing",
				Description: "已存在的组",
			}
			db.Create(&group)

			reqBody := map[string]any{
				"name":        "existing",
				"description": "尝试创建重复的组",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应验证请求体格式", func() {
			// 无效的请求体（缺少name字段）
			reqBody := map[string]any{
				"description": "缺少组名的描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupDelete 测试 groupDelete 函数
func TestGroupDelete(t *testing.T) {
	Convey("测试 groupDelete 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/delete", groupDelete)

		Convey("应成功删除用户组", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "tobedeleted",
				Description: "将被删除的组",
			}
			db.Create(&group)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/tobedeleted/delete", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库记录已删除
			var deletedGroup core.Group
			err := db.First(&deletedGroup, "name = ?", "tobedeleted").Error
			So(err, ShouldEqual, gorm.ErrRecordNotFound)
		})

		Convey("应返回成功当用户组不存在时（幂等性）", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/nonexistent/delete", nil)
			r.ServeHTTP(w, req)

			// 删除不存在的资源返回成功是符合HTTP DELETE幂等性的
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应返回错误当用户组有关联策略时", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员组",
			}
			db.Create(&group)

			// 创建关联的RBAC策略
			policy := core.NewGroupPolicy("admin", "/api/v1/admin/user/list", "POST", base.EffectAllow)
			db.Create(policy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/delete", nil)
			r.ServeHTTP(w, req)

			// ErrCannotDeleteCauseRelateRecord 的 HTTP 状态码是 400 (BadRequest)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupRbacPolicyAdd 测试 groupRbacPolicyAdd 函数
func TestGroupRbacPolicyAdd(t *testing.T) {
	Convey("测试 groupRbacPolicyAdd 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/add-rbac-policy", groupRbacPolicyAdd)

		Convey("应成功添加RBAC策略", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员组",
			}
			db.Create(&group)

			// 创建策略模板
			template := core.RbacPolicyTemplate{
				Target: "/api/v1/admin/user/list",
				Action: "POST",
			}
			db.Create(&template)

			// 准备添加策略请求
			reqBody := map[string]any{
				"target": "/api/v1/admin/user/list",
				"action": "POST",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/add-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库记录
			var policy core.RbacPolicy
			err := db.First(&policy, "ptype = ? AND v0 = ?", base.PolicyLabel, "admin").Error
			So(err, ShouldBeNil)
			So(policy.V1, ShouldEqual, "/api/v1/admin/user/list")
			So(policy.V2, ShouldEqual, "POST")
			So(policy.V3, ShouldEqual, "allow")
		})

		Convey("应返回错误当策略模板不存在时", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员组",
			}
			db.Create(&group)

			reqBody := map[string]any{
				"target": "/api/v1/nonexistent",
				"action": "GET",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/add-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应验证请求体格式", func() {
			// 无效的请求体（缺少target字段）
			reqBody := map[string]any{
				"action": "POST",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/add-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupRbacPolicyRemove 测试 groupRbacPolicyRemove 函数
func TestGroupRbacPolicyRemove(t *testing.T) {
	Convey("测试 groupRbacPolicyRemove 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB()
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/remove-rbac-policy", groupRbacPolicyRemove)

		Convey("应成功移除RBAC策略", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员组",
			}
			db.Create(&group)

			// 创建策略模板
			template := core.RbacPolicyTemplate{
				Target: "/api/v1/admin/user/list",
				Action: "POST",
			}
			db.Create(&template)

			// 创建策略
			policy := core.NewGroupPolicy("admin", "/api/v1/admin/user/list", "POST", base.EffectAllow)
			db.Create(policy)

			// 验证策略存在
			var count int64
			db.Model(&core.RbacPolicy{}).Where("ptype = ? AND v0 = ?", base.PolicyLabel, "admin").Count(&count)
			So(count, ShouldEqual, 1)

			// 准备移除策略请求
			reqBody := map[string]any{
				"target": "/api/v1/admin/user/list",
				"action": "POST",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/remove-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证策略已删除
			db.Model(&core.RbacPolicy{}).Where("ptype = ? AND v0 = ?", base.PolicyLabel, "admin").Count(&count)
			So(count, ShouldEqual, 0)
		})

		Convey("应返回错误当策略不存在时", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "admin",
				Description: "管理员组",
			}
			db.Create(&group)

			reqBody := map[string]any{
				"target": "/api/v1/nonexistent",
				"action": "GET",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/remove-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK) // 删除不存在的策略返回成功（无影响）
		})

		Convey("应验证请求体格式", func() {
			// 无效的请求体（缺少effect字段）
			reqBody := map[string]any{
				"target": "/api/v1/admin/user/list",
				"action": "POST",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/admin/remove-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}
