package manager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/models/core"
)

// setupNamespaceTestRouter 创建用于命名空间中间件测试的 Gin 路由
// 注册 NamespaceMiddleware 和一个测试处理函数，用于验证命名空间是否正确注入上下文。
// 同时注入 Localizer 和 Translator 中间件，以支持 AbortWithGinH 的国际化功能。
func setupNamespaceTestRouter(cfg *NamespaceConfig) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(LOCALIZER_CTX_NAME, newTestLocalizer())
		c.Set(TRANSLATOR_CTX_NAME, newTestTranslator())
		c.Next()
	})
	router.Use(NamespaceMiddleware(cfg))
	router.GET("/test", func(c *gin.Context) {
		ns, err := GetNamespace(c)
		if err != nil {
			err.AbortWithGinH(c)
			return
		}
		c.String(http.StatusOK, ns)
	})
	return router
}

// setupNamespaceContext 设置命名空间测试所需的 Gin 上下文，
// 包括 Localizer 和 Translator，以支持 AbortWithGinH 的国际化功能
func setupNamespaceContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(LOCALIZER_CTX_NAME, newTestLocalizer())
	c.Set(TRANSLATOR_CTX_NAME, newTestTranslator())
	return c, w
}

// setupNamespaceCache 设置测试所需的命名空间缓存
// 向全局缓存中添加指定的命名空间名称，返回一个清理函数，
// 测试结束后应调用该函数恢复原始缓存状态
func setupNamespaceCache(namespaces ...string) func() {
	originalCache := core.Cache
	testCache := core.NewNamespaceCache()
	for _, ns := range namespaces {
		testCache.Add(ns)
	}
	core.Cache = testCache
	return func() {
		core.Cache = originalCache
	}
}

// TestDefaultNamespaceConstant 测试 DEFAULT_NAMESPACE 常量
func TestDefaultNamespaceConstant(t *testing.T) {
	Convey("测试 DEFAULT_NAMESPACE 常量", t, func() {
		Convey("默认命名空间应为 'default'", func() {
			So(DEFAULT_NAMESPACE, ShouldEqual, "default")
		})
	})
}

// TestNamespaceCtxNameConstant 测试 NAMESPACE_CTX_NAME 常量
func TestNamespaceCtxNameConstant(t *testing.T) {
	Convey("测试 NAMESPACE_CTX_NAME 常量", t, func() {
		Convey("上下文键名应为 'namespace'", func() {
			So(NAMESPACE_CTX_NAME, ShouldEqual, "namespace")
		})
	})
}

// TestErrNampspaceDoesNotExist 测试 ErrNampspaceDoesNotExist 错误变量
func TestErrNampspaceDoesNotExist(t *testing.T) {
	Convey("测试 ErrNampspaceDoesNotExist 错误变量", t, func() {
		Convey("错误不应为 nil", func() {
			So(ErrNampspaceDoesNotExist, ShouldNotBeNil)
		})

		Convey("AbortWithGinH 应返回 400 状态码和正确的业务错误码", func() {
			c, w := setupNamespaceContext()

			ErrNampspaceDoesNotExist.AbortWithGinH(c)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			So(c.IsAborted(), ShouldBeTrue)

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(CodeNamespaceNotExist))
		})
	})
}

// TestNamespaceMiddleware 测试 NamespaceMiddleware 中间件
func TestNamespaceMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 NamespaceMiddleware 中间件", t, func() {
		cfg := &NamespaceConfig{
			HeaderName: "X-Namespace",
			QueryName:  "namespace",
			CookieName: "namespace",
		}

		Convey("应使用默认命名空间当请求中未携带任何命名空间标识", func() {
			cleanup := setupNamespaceCache(DEFAULT_NAMESPACE)
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, DEFAULT_NAMESPACE)
		})

		Convey("应从请求头中提取命名空间", func() {
			cleanup := setupNamespaceCache("tenant-a")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Namespace", "tenant-a")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "tenant-a")
		})

		Convey("应从查询参数中提取命名空间（当请求头未提供时）", func() {
			cleanup := setupNamespaceCache("tenant-b")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test?namespace=tenant-b", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "tenant-b")
		})

		Convey("请求头优先级应高于查询参数", func() {
			cleanup := setupNamespaceCache("header-ns", "query-ns")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test?namespace=query-ns", nil)
			req.Header.Set("X-Namespace", "header-ns")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "header-ns")
		})

		Convey("应从 Cookie 中提取命名空间（当请求头和查询参数均未提供时）", func() {
			cleanup := setupNamespaceCache("cookie-ns")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			req.AddCookie(&http.Cookie{Name: "namespace", Value: "cookie-ns"})
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "cookie-ns")
		})

		Convey("查询参数优先级应高于 Cookie", func() {
			cleanup := setupNamespaceCache("query-ns", "cookie-ns")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test?namespace=query-ns", nil)
			req.AddCookie(&http.Cookie{Name: "namespace", Value: "cookie-ns"})
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "query-ns")
		})

		Convey("当命名空间不存在时应中断请求并返回错误", func() {
			cleanup := setupNamespaceCache(DEFAULT_NAMESPACE)
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			router.GET("/test2", func(c *gin.Context) {
				c.String(http.StatusOK, "should not reach here")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test2", nil)
			req.Header.Set("X-Namespace", "nonexistent-ns")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(CodeNamespaceNotExist))
		})

		Convey("中间件应调用 c.Next() 使后续处理函数继续执行", func() {
			cleanup := setupNamespaceCache(DEFAULT_NAMESPACE)
			defer cleanup()

			nextCalled := false
			router := setupNamespaceTestRouter(cfg)
			router.GET("/test-next", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test-next", nil)
			router.ServeHTTP(w, req)

			So(nextCalled, ShouldBeTrue)
		})

		Convey("当命名空间不存在时后续处理函数不应被执行", func() {
			cleanup := setupNamespaceCache()
			defer cleanup()

			nextCalled := false
			router := setupNamespaceTestRouter(cfg)
			router.GET("/test-abort", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test-abort", nil)
			router.ServeHTTP(w, req)

			So(nextCalled, ShouldBeFalse)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应使用自定义配置的键名提取命名空间", func() {
			customCfg := &NamespaceConfig{
				HeaderName: "X-Custom-Ns",
				QueryName:  "ns",
				CookieName: "ns-cookie",
			}
			cleanup := setupNamespaceCache("custom-tenant")
			defer cleanup()

			router := setupNamespaceTestRouter(customCfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Custom-Ns", "custom-tenant")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "custom-tenant")
		})
	})
}

// TestGetNamespace 测试 GetNamespace 函数
func TestGetNamespace(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 GetNamespace 函数", t, func() {
		Convey("当上下文中存在命名空间时应正确返回", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(NAMESPACE_CTX_NAME, "test-namespace")

			ns, err := GetNamespace(c)
			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "test-namespace")
		})

		Convey("当上下文中不存在命名空间时应返回错误", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())

			ns, err := GetNamespace(c)
			So(err, ShouldNotBeNil)
			So(err, ShouldEqual, ErrNampspaceDoesNotExist)
			So(ns, ShouldEqual, "")
		})

		Convey("应返回 DEFAULT_NAMESPACE 的值", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(NAMESPACE_CTX_NAME, DEFAULT_NAMESPACE)

			ns, err := GetNamespace(c)
			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "default")
		})

		Convey("应返回自定义命名空间的值", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(NAMESPACE_CTX_NAME, "my-custom-ns")

			ns, err := GetNamespace(c)
			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "my-custom-ns")
		})
	})
}

// TestNamespaceMiddlewareAndGetNamespace_Integration 测试 NamespaceMiddleware 和 GetNamespace 的集成
func TestNamespaceMiddlewareAndGetNamespace_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 NamespaceMiddleware 和 GetNamespace 集成", t, func() {
		cfg := &NamespaceConfig{
			HeaderName: "X-Namespace",
			QueryName:  "namespace",
			CookieName: "namespace",
		}

		Convey("完整的命名空间识别和提取流程应正常工作", func() {
			cleanup := setupNamespaceCache("integration-ns")
			defer cleanup()

			var extractedNS string
			var extractErr *CodeError

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(LOCALIZER_CTX_NAME, newTestLocalizer())
				c.Set(TRANSLATOR_CTX_NAME, newTestTranslator())
				c.Next()
			})
			router.Use(NamespaceMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				extractedNS, extractErr = GetNamespace(c)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Namespace", "integration-ns")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(extractErr, ShouldBeNil)
			So(extractedNS, ShouldEqual, "integration-ns")
		})

		Convey("多个请求应各自获取正确的命名空间", func() {
			cleanup := setupNamespaceCache("tenant-x", "tenant-y")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)

			w1 := httptest.NewRecorder()
			req1, _ := http.NewRequest("GET", "/test", nil)
			req1.Header.Set("X-Namespace", "tenant-x")
			router.ServeHTTP(w1, req1)

			w2 := httptest.NewRecorder()
			req2, _ := http.NewRequest("GET", "/test", nil)
			req2.Header.Set("X-Namespace", "tenant-y")
			router.ServeHTTP(w2, req2)

			So(w1.Code, ShouldEqual, http.StatusOK)
			So(w1.Body.String(), ShouldEqual, "tenant-x")
			So(w2.Code, ShouldEqual, http.StatusOK)
			So(w2.Body.String(), ShouldEqual, "tenant-y")
		})
	})
}
