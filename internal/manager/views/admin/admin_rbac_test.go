package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// createRbacTestDB 创建用于 RBAC 路由测试的共享 PostgreSQL 测试数据库
func createRbacTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// setupRbacTestRouter 创建用于RBAC路由测试的Gin路由引擎
// 返回配置好的Gin引擎，包含必要的中间件
func setupRbacTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(func(c *gin.Context) {
		// 设置数据库上下文键
		c.Set(middlewares.CoreDatabaseCtxName, nil)
		c.Next()
	})
	return r
}

// setupRbacTestRouterWithDB 创建带有数据库中间件的测试路由引擎
func setupRbacTestRouterWithDB(db *database.DB) *gin.Engine {
	r := setupRbacTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	return r
}

// TestRbacPolicyTemplateListGet 测试RBAC策略模板列表获取功能
// 验证RbacPolicyTemplateListGet函数的正确性和健壮性
func TestRbacPolicyTemplateListGet(t *testing.T) {
	// 创建测试数据库
	db := createRbacTestDB(t)
	// 创建测试数据
	testTemplates := []*core.RbacPolicyTemplate{
		core.NewRbacPolicyTemplate("user", "read"),
		core.NewRbacPolicyTemplate("user", "write"),
		core.NewRbacPolicyTemplate("document", "read"),
		core.NewRbacPolicyTemplate("document", "write"),
		core.NewRbacPolicyTemplate("settings", "update"),
	}

	// 插入测试数据
	for _, template := range testTemplates {
		if err := db.Create(template).Error; err != nil {
			t.Fatalf("failed to create test template: %v", err)
		}
	}

	Convey("测试RBAC策略模板列表获取", t, func() {
		Convey("成功获取所有策略模板", func() {
			// 创建测试路由
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 创建测试请求
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// 执行请求
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// 验证响应状态码
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("带分页获取策略模板", func() {
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 请求第一页，每页2条
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("按目标对象筛选策略模板", func() {
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 筛选目标对象为"user"的模板
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"filterBy": map[string]any{
					"target": map[string]any{
						"action": "eq",
						"value":  "user",
					},
				},
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("按创建时间排序策略模板", func() {
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 按创建时间升序排序
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"orderBy": []map[string]any{
					{
						"name":  "createdAt",
						"order": 1,
					},
				},
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("处理无效的验证参数", func() {
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 发送无效的pageSize值（5不在允许的10,20,50,100中）
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 5, // 无效值，应该是10,20,50,100之一
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// 应该返回400 Bad Request（验证错误）
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("处理贪婪搜索模式", func() {
			r := setupRbacTestRouterWithDB(db)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			// 使用贪婪搜索模式
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   true,
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("处理空数据库的情况", func() {
			// 创建新的空数据库。
			// 注意：testdb.Core 按同一测试复用事务句柄并在下次调用时回滚上一个，
			// 故此子测试必须放在使用共享 db 的其他子测试之后，避免提前回滚共享事务。
			emptyDB := createRbacTestDB(t)

			r := setupRbacTestRouterWithDB(emptyDB)
			r.POST("/rbac-policy-template/list", RbacPolicyTemplateListGet)

			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
			}
			body, _ := json.Marshal(reqBody)
			req, _ := http.NewRequest("POST", "/rbac-policy-template/list", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestGroupRbacPolicyDetailOut 测试组RBAC策略详情输出结构
// 验证GroupRbacPolicyDetailOut结构体的正确性
func TestGroupRbacPolicyDetailOut(t *testing.T) {
	Convey("测试GroupRbacPolicyDetailOut结构体", t, func() {
		Convey("创建组策略详情输出实例", func() {
			policy := &GroupRbacPolicyDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        123,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				V1: "user",
				V2: "read",
				V3: "allow",
			}

			So(policy.ID, ShouldEqual, uint(123))
			So(policy.V1, ShouldEqual, "user")
			So(policy.V2, ShouldEqual, "read")
			So(policy.V3, ShouldEqual, "allow")
		})

		Convey("验证JSON标签", func() {
			policy := &GroupRbacPolicyDetailOut{
				V1: "document",
				V2: "write",
				V3: "deny",
			}

			// 序列化为JSON
			jsonData, err := json.Marshal(policy)
			So(err, ShouldBeNil)

			// 解析JSON验证字段名
			var result map[string]any
			err = json.Unmarshal(jsonData, &result)
			So(err, ShouldBeNil)
			So(result["target"], ShouldEqual, "document")
			So(result["action"], ShouldEqual, "write")
			So(result["effect"], ShouldEqual, "deny")
		})
	})
}

// TestUserRbacPolicyDetailOut 测试用户RBAC策略详情输出结构
// 验证UserRbacPolicyDetailOut结构体的正确性
func TestUserRbacPolicyDetailOut(t *testing.T) {
	Convey("测试UserRbacPolicyDetailOut结构体", t, func() {
		Convey("创建用户策略详情输出实例", func() {
			policy := &UserRbacPolicyDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        456,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				V1: "john.doe",
				V2: "default",
			}

			So(policy.ID, ShouldEqual, uint(456))
			So(policy.V1, ShouldEqual, "john.doe")
			So(policy.V2, ShouldEqual, "default")
		})

		Convey("验证JSON标签", func() {
			policy := &UserRbacPolicyDetailOut{
				V1: "alice",
				V2: "production",
			}

			jsonData, err := json.Marshal(policy)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(jsonData, &result)
			So(err, ShouldBeNil)
			So(result["name"], ShouldEqual, "alice")
			So(result["namespace"], ShouldEqual, "production")
		})
	})
}

// TestRbacPolicyTemplateDetailOut 测试RBAC策略模板详情输出结构
// 验证RbacPolicyTemplateDetailOut结构体的正确性
func TestRbacPolicyTemplateDetailOut(t *testing.T) {
	Convey("测试RbacPolicyTemplateDetailOut结构体", t, func() {
		Convey("创建策略模板详情输出实例", func() {
			template := &RbacPolicyTemplateDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        789,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				Target: "settings",
				Action: "update",
			}

			So(template.ID, ShouldEqual, uint(789))
			So(template.Target, ShouldEqual, "settings")
			So(template.Action, ShouldEqual, "update")
		})

		Convey("验证JSON标签", func() {
			template := &RbacPolicyTemplateDetailOut{
				Target: "audit",
				Action: "view",
			}

			jsonData, err := json.Marshal(template)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(jsonData, &result)
			So(err, ShouldBeNil)
			So(result["target"], ShouldEqual, "audit")
			So(result["action"], ShouldEqual, "view")
		})
	})
}

// TestRbacPolicyTemplateListOut 测试RBAC策略模板列表输出结构
// 验证RbacPolicyTemplateListOut结构体的正确性
func TestRbacPolicyTemplateListOut(t *testing.T) {
	Convey("测试RbacPolicyTemplateListOut结构体", t, func() {
		Convey("创建策略模板列表输出实例", func() {
			template := &RbacPolicyTemplateListOut{
				RbacPolicyTemplateDetailOut: RbacPolicyTemplateDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        999,
						CreatedAt: time.Now(),
						UpdatedAt: time.Now(),
					},
					Target: "api",
					Action: "call",
				},
				BaseTotalOut: utils.BaseTotalOut{
					Total: 100,
				},
			}

			So(template.ID, ShouldEqual, uint(999))
			So(template.Target, ShouldEqual, "api")
			So(template.Action, ShouldEqual, "call")
			So(template.Total, ShouldEqual, 100)
		})
	})
}
