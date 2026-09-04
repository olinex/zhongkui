package views

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// createNamespaceTestDB 创建用于命名空间路由测试的共享 PostgreSQL 测试数据库
func createNamespaceTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// setupNamespaceTestRouter 创建用于命名空间路由测试的 Gin 路由引擎
func setupNamespaceTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	return r
}

// setupNamespaceTestRouterWithDB 创建带有数据库中间件的测试路由引擎
func setupNamespaceTestRouterWithDB(db *database.DB) *gin.Engine {
	r := setupNamespaceTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	return r
}

// TestNamespaceUpdateIn_Struct 测试 NamespaceUpdateIn 结构体
func TestNamespaceUpdateIn_Struct(t *testing.T) {
	Convey("测试 NamespaceUpdateIn 结构体", t, func() {
		update := NamespaceUpdateIn{
			Description: "更新后的命名空间描述",
		}
		So(update.Description, ShouldEqual, "更新后的命名空间描述")
	})
}

// TestNamespaceCreateIn_Struct 测试 NamespaceCreateIn 结构体
func TestNamespaceCreateIn_Struct(t *testing.T) {
	Convey("测试 NamespaceCreateIn 结构体", t, func() {
		create := NamespaceCreateIn{
			NamespaceUpdateIn: NamespaceUpdateIn{
				Description: "新创建的命名空间",
			},
			Name: "test-namespace",
		}
		So(create.Name, ShouldEqual, "test-namespace")
		So(create.Description, ShouldEqual, "新创建的命名空间")
	})
}

// TestNamespaceDetailOut_Struct 测试 NamespaceDetailOut 结构体
func TestNamespaceDetailOut_Struct(t *testing.T) {
	Convey("测试 NamespaceDetailOut 结构体", t, func() {
		now := time.Now()
		detail := NamespaceDetailOut{
			BaseItemOut: utils.BaseItemOut{
				ID:        1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			Name:        "default",
			Description: "默认命名空间",
		}
		So(detail.ID, ShouldEqual, 1)
		So(detail.Name, ShouldEqual, "default")
		So(detail.Description, ShouldEqual, "默认命名空间")
	})
}

// TestNamespaceListGet 测试 NamespaceListGet 函数
func TestNamespaceListGet(t *testing.T) {
	Convey("测试 NamespaceListGet 函数", t, func() {
		db := createNamespaceTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNamespaceTestRouterWithDB(db)
		r.POST("/api/v1/admin/namespace/list", NamespaceListGet)

		Convey("应返回空列表当数据库为空时", func() {
			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/list", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应返回包含数据的列表当数据库有记录时", func() {
			namespaces := []core.Namespace{
				{Name: "staging", Description: "预发布环境"},
				{Name: "development", Description: "开发环境"},
			}
			for _, ns := range namespaces {
				db.Create(&ns)
			}

			reqBody := map[string]any{
				"page":     1,
				"pageSize": 10,
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/list", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestNamespaceDetailGet 测试 NamespaceDetailGet 函数
func TestNamespaceDetailGet(t *testing.T) {
	Convey("测试 NamespaceDetailGet 函数", t, func() {
		db := createNamespaceTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNamespaceTestRouterWithDB(db)
		r.GET("/api/v1/admin/namespace/:name/get", NamespaceDetailGet)

		Convey("应返回命名空间详情当命名空间存在时", func() {
			ns := core.Namespace{
				Name:        "development",
				Description: "开发环境",
			}
			db.Create(&ns)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/namespace/development/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应返回错误当命名空间不存在时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/namespace/nonexistent/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

// TestNamespaceCreate 测试 NamespaceCreate 函数
func TestNamespaceCreate(t *testing.T) {
	Convey("测试 NamespaceCreate 函数", t, func() {
		db := createNamespaceTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNamespaceTestRouterWithDB(db)
		r.POST("/api/v1/admin/namespace/create", NamespaceCreate)

		Convey("应成功创建命名空间", func() {
			reqBody := map[string]any{
				"name":        "test-namespace",
				"description": "测试命名空间描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 命名空间创建后应自动种子化全部占位符通知渠道（如 webhook），每个占位类型一条
			for _, typ := range message.PlaceholderTypes() {
				var channel core.NotificationChannel
				err := db.Where("namespace_name = ? AND type = ?", "test-namespace", typ).First(&channel).Error
				So(err, ShouldBeNil)
				So(channel.Name, ShouldEqual, typ)
				So(channel.Enabled, ShouldBeTrue)
			}
		})

		Convey("应返回错误当名称已存在时", func() {
			existingNs := core.Namespace{
				Name:        "existing-namespace",
				Description: "已存在的命名空间",
			}
			db.Create(&existingNs)

			reqBody := map[string]any{
				"name":        "existing-namespace",
				"description": "新的描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)

			// 创建失败不应遗留任何占位渠道
			var count int64
			So(db.Model(&core.NotificationChannel{}).Where("namespace_name = ?", "existing-namespace").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestNamespaceUpdate 测试 NamespaceUpdate 函数
func TestNamespaceUpdate(t *testing.T) {
	Convey("测试 NamespaceUpdate 函数", t, func() {
		db := createNamespaceTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNamespaceTestRouterWithDB(db)
		r.POST("/api/v1/admin/namespace/:name/update", NamespaceUpdate)

		Convey("应成功更新命名空间描述", func() {
			// 先创建测试数据
			ns := core.Namespace{
				Name:        "development",
				Description: "原始描述",
			}
			db.Create(&ns)

			// 准备更新请求
			reqBody := map[string]any{
				"description": "更新后的描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/development/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 验证数据库中的记录已更新
			var updatedNs core.Namespace
			db.First(&updatedNs, "name = ?", "development")
			So(updatedNs.Description, ShouldEqual, "更新后的描述")
		})

		Convey("应返回错误当命名空间不存在时", func() {
			reqBody := map[string]any{
				"description": "新的描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/nonexistent/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("应返回错误当描述长度小于最小值时", func() {
			ns := core.Namespace{
				Name:        "test",
				Description: "原始描述",
			}
			db.Create(&ns)

			reqBody := map[string]any{
				"description": "ab", // 长度2，小于最小值3
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/test/update", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// TestNamespaceDelete 测试 NamespaceBatchDelete 函数
func TestNamespaceDelete(t *testing.T) {
	Convey("测试 NamespaceBatchDelete 函数", t, func() {
		db := createNamespaceTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupNamespaceTestRouterWithDB(db)
		r.POST("/api/v1/admin/namespace/delete", NamespaceBatchDelete)

		Convey("应成功删除命名空间当没有引用时", func() {
			// 先创建测试数据
			ns := core.Namespace{
				Name:        "test-namespace",
				Description: "测试命名空间",
			}
			db.Create(&ns)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/delete",
				bytes.NewReader([]byte(`{"names":"test-namespace"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
			So(len(resp.Data.Failed), ShouldEqual, 0)

			// 验证数据库中的记录已删除
			var deletedNs core.Namespace
			err := db.First(&deletedNs, "name = ?", "test-namespace").Error
			So(err, ShouldEqual, gorm.ErrRecordNotFound)
		})

		Convey("应记入失败当命名空间被RBAC策略引用时", func() {
			// 先创建命名空间
			ns := core.Namespace{
				Name:        "referenced-namespace",
				Description: "被引用的命名空间",
			}
			db.Create(&ns)

			// 创建引用该命名空间的RBAC策略
			policy := core.RbacPolicy{
				Ptype: base.GroupLabel,
				V0:    "group1",
				V1:    "role1",
				V2:    "referenced-namespace",
			}
			db.Create(&policy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/delete",
				bytes.NewReader([]byte(`{"names":["referenced-namespace"]}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 0)
			So(len(resp.Data.Failed), ShouldEqual, 1)
			So(resp.Data.Failed[0].ID, ShouldEqual, "referenced-namespace")
			So(resp.Data.Failed[0].Reason, ShouldNotBeEmpty)
		})

		Convey("命名空间不存在时幂等成功", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/namespace/delete",
				bytes.NewReader([]byte(`{"names":"nonexistent"}`)))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int            `json:"code"`
				Data BatchResultOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data.Succeeded, ShouldEqual, 1)
		})
	})
}
