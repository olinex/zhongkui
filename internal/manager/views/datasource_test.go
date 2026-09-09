// Package views 提供 HTTP API 视图层处理器与 In/Out 结构定义。
// 本文件包含第三方数据源管理路由的集成测试：
// 覆盖类型枚举 schema、创建/详情、命名空间裸名 vtable、配置加密、非法名/非法类型拒绝、
// 启停生命周期（vtable 注册/注销）与批量删除；采用 redis 数据源（struct=string，
// 惰性不联网）避免依赖真实外部实例。
package views

import (
	// stdlib
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"

	// third-party
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// datasourceTestNamespace 第三方数据源测试专用命名空间，与迁移种子的 default 隔离。
const datasourceTestNamespace = "dstest"

// createDatasourceTestDB 返回共享 PostgreSQL 测试库（testdb.Core 迁移并清空全部业务表）。
// 进程内先确保测试专用命名空间存在（满足 datasources 的 namespace_name 外键约束）。
func createDatasourceTestDB(t *testing.T) *database.DB {
	db := testdb.Core(t)
	ns := core.Namespace{Name: datasourceTestNamespace}
	if err := db.Where(core.Namespace{Name: datasourceTestNamespace}).FirstOrCreate(&ns).Error; err != nil {
		t.Fatalf("seed datasource test namespace: %v", err)
	}
	return db
}

// registerDatasourceTestRoutes 注册第三方数据源管理路由，并注入按命名空间隔离的 sqlite
// vtable 池持有者与 etcd 客户端（显式 nil：测试环境无 etcd，写路径广播走
// etcd.ErrChangeUnavailable 告警降级，不阻断已提交的变更）。
func registerDatasourceTestRoutes(r *gin.Engine, pools *vtab.Pools) {
	group := r.Group("/datasource", func(c *gin.Context) {
		c.Set(middlewares.SQLiteVTabPoolsCtxName, pools)
		c.Set(middlewares.EtcdCtxName, (*etcd.EtcdClient)(nil))
		c.Next()
	})
	group.POST("/types", DatasourceTypeConfigGet)
	group.POST("/list", DatasourceListGet)
	group.POST("/create", DatasourceCreate)
	group.POST("/:id/get", DatasourceDetailGet)
	group.POST("/:id/update", DatasourceUpdate)
	group.POST("/delete", DatasourceBatchDelete)
	group.POST("/enable", DatasourceBatchSetEnabled(true))
	group.POST("/disable", DatasourceBatchSetEnabled(false))
}

// setupDatasourceTestRouterWithNamespace 创建测试路由引擎，注入 i18n、日志、数据库、
// sqlite vtable 池持有者与指定命名空间，用于验证多租户隔离。
func setupDatasourceTestRouterWithNamespace(db *database.DB, pools *vtab.Pools, namespace string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newTagRuleTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Next()
	})
	registerDatasourceTestRoutes(r, pools)
	return r
}

// doDatasourceRequest 执行一次第三方数据源 HTTP 请求并返回响应记录器。
func doDatasourceRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic("failed to marshal test request body: " + err.Error())
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// datasourceIDPath 拼接第三方数据源路由路径。
func datasourceIDPath(action string, id uint) string {
	return "/datasource/" + strconv.FormatUint(uint64(id), 10) + action
}

// newDatasourceTestPools 创建并登记清理的按命名空间 sqlite vtable 池持有者。
func newDatasourceTestPools(t *testing.T) *vtab.Pools {
	t.Helper()
	pools := vtab.NewPools()
	t.Cleanup(func() { pools.Close(context.Background()) })
	return pools
}

// waitDatasourceRegistered 断言指定命名空间池的裸名 vtable 是否注册。
func waitDatasourceRegistered(pools *vtab.Pools, ns, name string, want bool) {
	pool := pools.Pool(ns)
	So(slices.Contains(pool.Datasources(), name), ShouldEqual, want)
}

// TestDatasourceTypes 第三方数据源类型枚举返回 redis 与 mysql 及其配置 schema。
func TestDatasourceTypes(t *testing.T) {
	Convey("第三方数据源类型枚举", t, func() {
		db := createDatasourceTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		r := setupDatasourceTestRouterWithNamespace(db, newDatasourceTestPools(t), datasourceTestNamespace)

		w := doDatasourceRequest(r, http.MethodPost, "/datasource/types", nil)
		So(w.Code, ShouldEqual, http.StatusOK)
		var resp struct {
			Data DatasourceTypesOut `json:"data"`
		}
		So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
		byType := make(map[string]DatasourceTypeOut, len(resp.Data.Types))
		for _, typ := range resp.Data.Types {
			byType[typ.Type] = typ
		}

		redis, ok := byType[vtab.DatasourceTypeRedis]
		So(ok, ShouldBeTrue)
		names := make([]string, 0, len(redis.Config))
		for _, f := range redis.Config {
			names = append(names, f.Name)
		}
		So(names, ShouldResemble, []string{"addr", "port", "password", "db", "struct"})
		So(redis.Config[0].Required, ShouldBeTrue)
		So(redis.Config[1].DefaultValue, ShouldEqual, 6379)
		So(redis.Config[3].DefaultValue, ShouldEqual, 0)

		mysql, ok := byType[vtab.DatasourceTypeMySQL]
		So(ok, ShouldBeTrue)
		names = names[:0]
		for _, f := range mysql.Config {
			names = append(names, f.Name)
		}
		So(names, ShouldResemble, []string{"addr", "port", "user", "password", "dbName", "table"})
	})
}

// TestDatasourceCreateAndDetail 创建（redis，惰性不联网）与详情查询全链路。
func TestDatasourceCreateAndDetail(t *testing.T) {
	Convey("第三方数据源创建与详情", t, func() {
		db := createDatasourceTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		pools := newDatasourceTestPools(t)
		r := setupDatasourceTestRouterWithNamespace(db, pools, datasourceTestNamespace)

		Convey("合法 redis 数据源创建成功，裸名 vtable 注册且 config 为密文", func() {
			cfg := `{"addr":"127.0.0.1","port":6379,"db":0,"struct":"string"}`
			w := doDatasourceRequest(r, http.MethodPost, "/datasource/create", map[string]any{
				"name":        "myredis",
				"type":        vtab.DatasourceTypeRedis,
				"enabled":     true,
				"config":      cfg,
				"description": "main redis",
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var ds core.Datasource
			So(db.Where("name = ?", "myredis").First(&ds).Error, ShouldBeNil)
			// 启用状态下数据源以裸名注册在 dstest 命名空间池
			waitDatasourceRegistered(pools, datasourceTestNamespace, "myredis", true)

			w = doDatasourceRequest(r, http.MethodPost, datasourceIDPath("/get", ds.ID), nil)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Data *DatasourceDetailOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Name, ShouldEqual, "myredis")
			So(resp.Data.Type, ShouldEqual, vtab.DatasourceTypeRedis)
			So(resp.Data.Enabled, ShouldBeTrue)
			So(resp.Data.Description, ShouldEqual, "main redis")
			// config 返回密文而非明文
			var raw struct {
				Data struct {
					Config string `json:"config"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &raw), ShouldBeNil)
			So(raw.Data.Config, ShouldNotBeEmpty)
			So(raw.Data.Config, ShouldNotEqual, cfg)
		})

		Convey("非法本地名（非合法 SQL 标识符）被拒绝且不落库、不注册", func() {
			w := doDatasourceRequest(r, http.MethodPost, "/datasource/create", map[string]any{
				"name":   "1bad",
				"type":   vtab.DatasourceTypeRedis,
				"config": `{"addr":"127.0.0.1","struct":"string"}`,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.Datasource{}).Where("name = ?", "1bad").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "1bad", false)
		})

		Convey("未支持的类型被拒绝且不落库", func() {
			w := doDatasourceRequest(r, http.MethodPost, "/datasource/create", map[string]any{
				"name":   "click",
				"type":   "clickhouse",
				"config": `{}`,
			})
			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var count int64
			So(db.Model(&core.Datasource{}).Where("name = ?", "click").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})
	})
}

// TestDatasourceEnableDisable 启停生命周期：停用注销 vtable、启用重新注册。
func TestDatasourceEnableDisable(t *testing.T) {
	Convey("第三方数据源启停生命周期", t, func() {
		db := createDatasourceTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		pools := newDatasourceTestPools(t)
		r := setupDatasourceTestRouterWithNamespace(db, pools, datasourceTestNamespace)

		Convey("创建即停用：不常驻 vtable；启用后注册、停用后注销", func() {
			w := doDatasourceRequest(r, http.MethodPost, "/datasource/create", map[string]any{
				"name":    "kv",
				"type":    vtab.DatasourceTypeRedis,
				"enabled": false,
				"config":  `{"addr":"127.0.0.1","struct":"string"}`,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var ds core.Datasource
			So(db.Where("name = ?", "kv").First(&ds).Error, ShouldBeNil)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "kv", false)

			w = doDatasourceRequest(r, http.MethodPost, "/datasource/enable", map[string]any{"ids": []uint64{uint64(ds.ID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "kv", true)

			w = doDatasourceRequest(r, http.MethodPost, "/datasource/disable", map[string]any{"ids": []uint64{uint64(ds.ID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "kv", false)
		})
	})
}

// TestDatasourceDeleteAndList 批量删除注销 vtable；列表包含创建行。
func TestDatasourceDeleteAndList(t *testing.T) {
	Convey("第三方数据源删除与列表", t, func() {
		db := createDatasourceTestDB(t)
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		pools := newDatasourceTestPools(t)
		r := setupDatasourceTestRouterWithNamespace(db, pools, datasourceTestNamespace)

		Convey("创建后列表命中，删除后注销且列表为空", func() {
			w := doDatasourceRequest(r, http.MethodPost, "/datasource/create", map[string]any{
				"name":    "dropped",
				"type":    vtab.DatasourceTypeRedis,
				"enabled": true,
				"config":  `{"addr":"127.0.0.1","struct":"set"}`,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var ds core.Datasource
			So(db.Where("name = ?", "dropped").First(&ds).Error, ShouldBeNil)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "dropped", true)

			w = doDatasourceRequest(r, http.MethodPost, "/datasource/list", map[string]any{
				"page":     1,
				"pageSize": 10,
			})
			So(w.Code, ShouldEqual, http.StatusOK)
			var listResp struct {
				Data struct {
					Total uint                `json:"total"`
					Items []DatasourceListOut `json:"items"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &listResp), ShouldBeNil)
			So(listResp.Data.Total, ShouldEqual, 1)
			So(listResp.Data.Items, ShouldHaveLength, 1)

			w = doDatasourceRequest(r, http.MethodPost, "/datasource/delete", map[string]any{"ids": []uint64{uint64(ds.ID)}})
			So(w.Code, ShouldEqual, http.StatusOK)
			var count int64
			So(db.Model(&core.Datasource{}).Where("name = ?", "dropped").Count(&count).Error, ShouldBeNil)
			So(count, ShouldEqual, 0)
			waitDatasourceRegistered(pools, datasourceTestNamespace, "dropped", false)
		})
	})
}
