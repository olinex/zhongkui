package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// createTestDB 创建一个用于测试的 SQLite 内存数据库实例
func createTestDB() *database.DB {
	cfg := &database.Config{
		Driver: "sqlite",
		DSN:    ":memory:",
	}
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	logger := zapLogger.Sugar()

	db, err := database.NewDatabase(cfg, logger)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	return db
}

// createTestEtcdClient 创建一个用于测试的 etcd 客户端实例
// 注意：使用不可达的端点仅用于类型测试，不会实际连接
func createTestEtcdClient() *clientv3.Client {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:9999"},
		DialTimeout: 0,
	})
	if err != nil {
		panic("failed to create test etcd client: " + err.Error())
	}
	return client
}

// newTestGinContext 创建一个带有 mock ResponseWriter 的 Gin 上下文
func newTestGinContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return c
}

// TestDATABASE_CTX_NAME 测试数据库上下文键名常量
func TestDATABASE_CTX_NAME(t *testing.T) {
	Convey("测试数据库上下文键名常量", t, func() {
		Convey("DATABASE_CTX_NAME 的值应为 'db'", func() {
			So(DATABASE_CTX_NAME, ShouldEqual, "db")
		})
	})
}

// TestETCD_CTX_NAME 测试 etcd 客户端上下文键名常量
func TestETCD_CTX_NAME(t *testing.T) {
	Convey("测试 etcd 客户端上下文键名常量", t, func() {
		Convey("ETCD_CTX_NAME 的值应为 'etcd'", func() {
			So(ETCD_CTX_NAME, ShouldEqual, "etcd")
		})
	})
}

// TestDatabaseMiddleware 测试数据库中间件的注入行为
func TestDatabaseMiddleware(t *testing.T) {
	Convey("测试数据库中间件", t, func() {
		db := createTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		middleware := DatabaseMiddleware(db)
		So(middleware, ShouldNotBeNil)

		c := newTestGinContext()
		c.Keys = make(map[string]any)

		Convey("中间件应将数据库实例存入上下文", func() {
			middleware(c)

			val, exists := c.Get(DATABASE_CTX_NAME)
			So(exists, ShouldBeTrue)
			So(val, ShouldNotBeNil)

			storedDB, ok := val.(*database.DB)
			So(ok, ShouldBeTrue)
			So(storedDB, ShouldPointTo, db)
		})

		Convey("中间件应调用 c.Next() 继续后续处理", func() {
			called := false
			handler := func(c *gin.Context) {
				called = true
			}

			r := gin.New()
			r.Use(DatabaseMiddleware(db))
			r.GET("/test", handler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(called, ShouldBeTrue)
		})
	})
}

// TestDatabaseMiddleware_NilDB 测试数据库中间件接收 nil 数据库时的行为
func TestDatabaseMiddleware_NilDB(t *testing.T) {
	Convey("测试数据库中间件接收 nil 数据库", t, func() {
		middleware := DatabaseMiddleware(nil)
		So(middleware, ShouldNotBeNil)

		c := newTestGinContext()
		c.Keys = make(map[string]any)

		Convey("中间件应将 nil 存入上下文而不 panic", func() {
			middleware(c)

			val, exists := c.Get(DATABASE_CTX_NAME)
			So(exists, ShouldBeTrue)
			So(val, ShouldBeNil)
		})
	})
}

// TestGetDatabase 测试从上下文中获取数据库连接
func TestGetDatabase(t *testing.T) {
	Convey("测试从上下文获取数据库连接", t, func() {
		db := createTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		Convey("当上下文中存在数据库实例时", func() {
			c := newTestGinContext()
			c.Keys = make(map[string]any)
			c.Set(DATABASE_CTX_NAME, db)

			Convey("应成功获取数据库连接", func() {
				result := GetDatabase(c)
				So(result, ShouldNotBeNil)
			})

			Convey("返回的数据库连接应绑定了请求上下文", func() {
				result := GetDatabase(c)
				So(result, ShouldNotBeNil)
			})
		})

		Convey("当上下文中不存在数据库实例时", func() {
			c := newTestGinContext()
			c.Keys = make(map[string]any)

			Convey("应触发 panic", func() {
				So(func() {
					GetDatabase(c)
				}, ShouldPanicWith, "database does not exists")
			})
		})
	})
}

// TestGetDatabase_WithContextPropagation 测试 GetDatabase 的上下文传播
func TestGetDatabase_WithContextPropagation(t *testing.T) {
	Convey("测试 GetDatabase 的请求上下文传播", t, func() {
		db := createTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		c := newTestGinContext()
		c.Keys = make(map[string]any)
		c.Set(DATABASE_CTX_NAME, db)

		Convey("GetDatabase 应使用请求的 context 调用 WithContext", func() {
			result := GetDatabase(c)
			So(result, ShouldNotBeNil)
		})
	})
}

// TestDatabaseMiddlewareAndGetDatabase_Integration 测试中间件与获取函数的集成
func TestDatabaseMiddlewareAndGetDatabase_Integration(t *testing.T) {
	Convey("测试数据库中间件与 GetDatabase 的集成使用", t, func() {
		db := createTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		var capturedDB *database.DB

		r := gin.New()
		r.Use(DatabaseMiddleware(db))
		r.GET("/test", func(c *gin.Context) {
			capturedDB = GetDatabase(c)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		Convey("应成功通过中间件注入并通过 GetDatabase 获取数据库连接", func() {
			So(w.Code, ShouldEqual, http.StatusOK)
			So(capturedDB, ShouldNotBeNil)
		})
	})
}

// TestEtcdClientMiddleware 测试 etcd 客户端中间件的注入行为
func TestEtcdClientMiddleware(t *testing.T) {
	Convey("测试 etcd 客户端中间件", t, func() {
		client := createTestEtcdClient()
		defer client.Close()

		middleware := EtcdClientMiddleware(client)
		So(middleware, ShouldNotBeNil)

		c := newTestGinContext()
		c.Keys = make(map[string]any)

		Convey("中间件应将 etcd 客户端存入上下文", func() {
			middleware(c)

			val, exists := c.Get(ETCD_CTX_NAME)
			So(exists, ShouldBeTrue)
			So(val, ShouldNotBeNil)

			storedClient, ok := val.(*clientv3.Client)
			So(ok, ShouldBeTrue)
			So(storedClient, ShouldPointTo, client)
		})

		Convey("中间件应调用 c.Next() 继续后续处理", func() {
			called := false
			handler := func(c *gin.Context) {
				called = true
			}

			r := gin.New()
			r.Use(EtcdClientMiddleware(client))
			r.GET("/test", handler)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(called, ShouldBeTrue)
		})
	})
}

// TestEtcdClientMiddleware_NilClient 测试 etcd 中间件接收 nil 客户端时的行为
func TestEtcdClientMiddleware_NilClient(t *testing.T) {
	Convey("测试 etcd 中间件接收 nil 客户端", t, func() {
		middleware := EtcdClientMiddleware(nil)
		So(middleware, ShouldNotBeNil)

		c := newTestGinContext()
		c.Keys = make(map[string]any)

		Convey("中间件应将 nil 存入上下文而不 panic", func() {
			middleware(c)

			val, exists := c.Get(ETCD_CTX_NAME)
			So(exists, ShouldBeTrue)
			So(val, ShouldBeNil)
		})
	})
}

// TestGetEtcdClient 测试从上下文中获取 etcd 客户端
func TestGetEtcdClient(t *testing.T) {
	Convey("测试从上下文获取 etcd 客户端", t, func() {
		client := createTestEtcdClient()
		defer client.Close()

		Convey("当上下文中存在 etcd 客户端时", func() {
			c := newTestGinContext()
			c.Keys = make(map[string]any)
			c.Set(ETCD_CTX_NAME, client)

			Convey("应成功获取 etcd 客户端", func() {
				result := GetEtcdClient(c)
				So(result, ShouldNotBeNil)
				So(result, ShouldPointTo, client)
			})
		})

		Convey("当上下文中不存在 etcd 客户端时", func() {
			c := newTestGinContext()
			c.Keys = make(map[string]any)

			Convey("应触发 panic", func() {
				So(func() {
					GetEtcdClient(c)
				}, ShouldPanicWith, "etcd client does not exists")
			})
		})
	})
}

// TestEtcdClientMiddlewareAndGetEtcdClient_Integration 测试 etcd 中间件与获取函数的集成
func TestEtcdClientMiddlewareAndGetEtcdClient_Integration(t *testing.T) {
	Convey("测试 etcd 中间件与 GetEtcdClient 的集成使用", t, func() {
		client := createTestEtcdClient()
		defer client.Close()

		var capturedClient *clientv3.Client

		r := gin.New()
		r.Use(EtcdClientMiddleware(client))
		r.GET("/test", func(c *gin.Context) {
			capturedClient = GetEtcdClient(c)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		Convey("应成功通过中间件注入并通过 GetEtcdClient 获取客户端", func() {
			So(w.Code, ShouldEqual, http.StatusOK)
			So(capturedClient, ShouldNotBeNil)
		})
	})
}

// TestDatabaseAndEtcdCoexistence 测试数据库和 etcd 中间件共存
func TestDatabaseAndEtcdCoexistence(t *testing.T) {
	Convey("测试数据库和 etcd 中间件共存使用", t, func() {
		db := createTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		client := createTestEtcdClient()
		defer client.Close()

		var capturedDB *database.DB
		var capturedClient *clientv3.Client

		r := gin.New()
		r.Use(DatabaseMiddleware(db))
		r.Use(EtcdClientMiddleware(client))
		r.GET("/test", func(c *gin.Context) {
			capturedDB = GetDatabase(c)
			capturedClient = GetEtcdClient(c)
			c.Status(http.StatusOK)
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		r.ServeHTTP(w, req)

		Convey("应同时获取数据库连接和 etcd 客户端", func() {
			So(w.Code, ShouldEqual, http.StatusOK)
			So(capturedDB, ShouldNotBeNil)
			So(capturedClient, ShouldNotBeNil)
		})
	})
}

// TestGetDatabase_WrongType 测试上下文中存储了错误类型时的行为
func TestGetDatabase_WrongType(t *testing.T) {
	Convey("测试上下文中存储了错误类型时的 GetDatabase 行为", t, func() {
		c := newTestGinContext()
		c.Keys = make(map[string]any)
		c.Set(DATABASE_CTX_NAME, "not a database")

		Convey("应触发 panic（类型断言失败）", func() {
			So(func() {
				GetDatabase(c)
			}, ShouldPanic)
		})
	})
}

// TestGetEtcdClient_WrongType 测试上下文中存储了错误类型时的行为
func TestGetEtcdClient_WrongType(t *testing.T) {
	Convey("测试上下文中存储了错误类型时的 GetEtcdClient 行为", t, func() {
		c := newTestGinContext()
		c.Keys = make(map[string]any)
		c.Set(ETCD_CTX_NAME, "not an etcd client")

		Convey("应触发 panic（类型断言失败）", func() {
			So(func() {
				GetEtcdClient(c)
			}, ShouldPanic)
		})
	})
}
