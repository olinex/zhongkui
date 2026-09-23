package admin

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
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// createGroupTestDB 创建用于用户组路由测试的共享 PostgreSQL 测试数据库
func createGroupTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
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
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	// 注入 RBAC enforcer，供 GroupRbacPolicyAdd/Remove 等 handler 经 GetRbacPolicy 使用
	r.Use(middlewares.RbacPolicyMiddleware(newTestRbacEnforcer(db)))
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

// groupTestNamePrefix 本测试专用用户组名前缀，用于与迁移基线种子组
// （superuser / admin / default，见 002_fixtures.sql）隔离：
// 基线组为已提交的持久行，测试若直接断言「表为空」或插入同名 admin 组
// 会撞唯一约束而失败（且仅当数据库被重新迁移时才暴露）。
// 各子测试统一经 filterBy.name 前缀过滤操作自身数据。
const groupTestNamePrefix = "zk-test-group-"

// TestGroupListGet 测试 GroupListGet 函数
func TestGroupListGet(t *testing.T) {
	Convey("测试 GroupListGet 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/list", GroupListGet)

		Convey("应返回空列表当无匹配数据时", func() {
			// 准备请求体：按测试专用前缀过滤（groups 表存在基线种子组，非空）
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
				"filterBy": map[string]any{
					"name": map[string]any{"action": "pfx", "value": groupTestNamePrefix},
				},
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

		Convey("应返回用户组列表当有匹配数据时", func() {
			// 创建测试数据（使用测试专用前缀，避开基线种子组）
			groups := []core.Group{
				{Name: groupTestNamePrefix + "admin", Description: "管理员组"},
				{Name: groupTestNamePrefix + "users", Description: "普通用户组"},
				{Name: groupTestNamePrefix + "guests", Description: "访客组"},
			}
			for _, group := range groups {
				db.Create(&group)
			}

			// 准备请求体
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
				"filterBy": map[string]any{
					"name": map[string]any{"action": "pfx", "value": groupTestNamePrefix},
				},
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
			// 创建测试数据（使用测试专用前缀）
			for i := 1; i <= 15; i++ {
				db.Create(&core.Group{
					Name:        fmt.Sprintf("%s%02d", groupTestNamePrefix, i), // 生成 zk-test-group-01, ... 作为组名
					Description: "测试组",
				})
			}

			// 第一页，每页10条
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
				"greedy":   false,
				"filterBy": map[string]any{
					"name": map[string]any{"action": "pfx", "value": groupTestNamePrefix},
				},
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

// TestGroupDetailGet 测试 GroupDetailGet 函数
func TestGroupDetailGet(t *testing.T) {
	Convey("测试 GroupDetailGet 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/get", GroupDetailGet)

		Convey("应返回404当用户组不存在时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/nonexistent/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应返回用户组详情当用户组存在时", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
				Description: "管理员用户组",
			}
			db.Create(&group)

			// 创建关联的RBAC策略
			policy := core.NewGroupPolicy(groupName, "/api/v1/admin/user/list", "POST", base.EffectAllow)
			db.Create(policy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, 200)

			data := response["data"].(map[string]any)
			So(data["name"], ShouldEqual, groupName)
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

// TestGroupUpdate 测试 GroupUpdate 函数
func TestGroupUpdate(t *testing.T) {
	Convey("测试 GroupUpdate 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁（如重复插入同名的 admin 组），
		// 引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/update", GroupUpdate)

		Convey("应更新用户组描述信息", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
				Description: "原始描述",
			}
			db.Create(&group)

			// 准备更新请求
			reqBody := map[string]any{
				"description": "更新后的描述信息",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库更新
			var updatedGroup core.Group
			db.First(&updatedGroup, "name = ?", groupName)
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
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
				Description: "原始描述",
			}
			db.Create(&group)

			// 无效的请求体（缺少description字段）
			reqBody := map[string]any{}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestGroupCreate 测试 GroupCreate 函数
func TestGroupCreate(t *testing.T) {
	Convey("测试 GroupCreate 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/create", GroupCreate)

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

// TestGroupDelete 测试 GroupBatchDelete 函数
func TestGroupDelete(t *testing.T) {
	Convey("测试 GroupBatchDelete 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/delete", GroupBatchDelete)

		Convey("应成功删除用户组", func() {
			// 创建测试数据
			group := core.Group{
				Name:        "tobedeleted",
				Description: "将被删除的组",
			}
			db.Create(&group)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/delete",
				bytes.NewReader([]byte(`{"names":"tobedeleted"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                  `json:"code"`
				Data views.BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(len(resp.Data.Failed), ShouldEqual, 0)

			// 验证数据库记录已删除
			var deletedGroup core.Group
			err := db.First(&deletedGroup, "name = ?", "tobedeleted").Error
			So(err, ShouldEqual, gorm.ErrRecordNotFound)
		})

		Convey("应返回成功当用户组不存在时（幂等性）", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/delete",
				bytes.NewReader([]byte(`{"names":"nonexistent"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 删除不存在的资源视为成功（批量汇总 succeeded=1）
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data views.BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
		})

		Convey("应记入失败当用户组有关联策略时", func() {
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/delete",
				bytes.NewReader([]byte(`{"names":["admin"]}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			// 有引用时该条记入失败明细，HTTP 200 表达批量结果
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                  `json:"code"`
				Data views.BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 0)
			So(len(resp.Data.Failed), ShouldEqual, 1)
			So(resp.Data.Failed[0].ID, ShouldEqual, "admin")
			So(resp.Data.Failed[0].Reason, ShouldNotBeEmpty)
		})
	})
}

// TestGroupRbacPolicyAdd 测试 GroupRbacPolicyAdd 函数
func TestGroupRbacPolicyAdd(t *testing.T) {
	Convey("测试 GroupRbacPolicyAdd 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/add-rbac-policy", GroupRbacPolicyAdd)

		Convey("应成功添加RBAC策略", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/add-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库记录
			var policy core.RbacPolicy
			err := db.First(&policy, "ptype = ? AND v0 = ?", base.PolicyLabel, groupName).Error
			So(err, ShouldBeNil)
			So(policy.V1, ShouldEqual, "/api/v1/admin/user/list")
			So(policy.V2, ShouldEqual, "POST")
			So(policy.V3, ShouldEqual, "allow")
		})

		Convey("应返回错误当策略模板不存在时", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/add-rbac-policy", bytes.NewReader(body))
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

// TestGroupRbacPolicyRemove 测试 GroupRbacPolicyRemove 函数
func TestGroupRbacPolicyRemove(t *testing.T) {
	Convey("测试 GroupRbacPolicyRemove 函数", t, func() {
		// 创建测试数据库和路由
		db := createGroupTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupGroupTestRouterWithDB(db)

		// 注册测试路由
		r.POST("/api/v1/admin/group/:name/remove-rbac-policy", GroupRbacPolicyRemove)

		Convey("应成功移除RBAC策略", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
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
			policy := core.NewGroupPolicy(groupName, "/api/v1/admin/user/list", "POST", base.EffectAllow)
			db.Create(policy)

			// 验证策略存在
			var count int64
			db.Model(&core.RbacPolicy{}).Where("ptype = ? AND v0 = ?", base.PolicyLabel, groupName).Count(&count)
			So(count, ShouldEqual, 1)

			// 准备移除策略请求
			reqBody := map[string]any{
				"target": "/api/v1/admin/user/list",
				"action": "POST",
				"effect": "allow",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/remove-rbac-policy", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证策略已删除
			db.Model(&core.RbacPolicy{}).Where("ptype = ? AND v0 = ?", base.PolicyLabel, groupName).Count(&count)
			So(count, ShouldEqual, 0)
		})

		Convey("应返回错误当策略不存在时", func() {
			// 使用测试专用前缀的组名（避开基线种子组 admin，避免唯一约束冲突导致事务中止）
			groupName := groupTestNamePrefix + "admin"
			group := core.Group{
				Name:        groupName,
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/group/"+groupName+"/remove-rbac-policy", bytes.NewReader(body))
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
