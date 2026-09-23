package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/manager/config"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
)

// newTestLocalizer 创建一个用于测试的 i18n 本地化器
// 注册了所有错误码和错误消息的翻译，确保 AbortWithGinH 的国际化功能可用
func newTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.1000 = "unknown error"
code.1001 = "user not exist"
code.1002 = "bad body"
code.1003 = "bad query"
code.1004 = "bad url param"
code.1005 = "bad jwt token"
code.1006 = "timeout jwt token"
code.1007 = "authorize blank full path"
code.1008 = "authorize failed"
code.1009 = "namespace not exist"
code.1010 = "cannot delete cause relate record"
code.200 = "ok"
code.400 = "bad request"
code.401 = "unauthorized"
code.404 = "not found"
code.405 = "method not allowed"
code.500 = "internal server error"
code.1100 = "username or password check failed"
code.1101 = "old password check failed"
code.1102 = "username or email already exist"
code.1103 = "too many access key"
error.pageSize = "page size must be less than %d"
error.invalidID = "invalid id: %s"
error.requiredField = "%s is required"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newTestTranslator 创建一个用于测试的 ut.Translator
func newTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupNamespaceTestRouter 创建用于命名空间中间件测试的 Gin 路由
// 注册 NamespaceMiddleware 和一个测试处理函数，用于验证命名空间是否正确注入上下文。
// 同时注入 Localizer 和 Translator 中间件，以支持 AbortWithGinH 的国际化功能。
func setupNamespaceTestRouter(cfg *config.NamespaceConfig) *gin.Engine {
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newTestLocalizer())
		c.Set(response.TranslatorCtxName, newTestTranslator())
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
	c.Set(response.LocalizerCtxName, newTestLocalizer())
	c.Set(response.TranslatorCtxName, newTestTranslator())
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

// TestDefaultNamespaceConstant 测试 DefaultNamespace 常量
func TestDefaultNamespaceConstant(t *testing.T) {
	Convey("测试 DefaultNamespace 常量", t, func() {
		Convey("默认命名空间应为 'default'", func() {
			So(DefaultNamespace, ShouldEqual, "default")
		})
	})
}

// TestNamespaceCtxNameConstant 测试 NamespaceCtxName 常量
func TestNamespaceCtxNameConstant(t *testing.T) {
	Convey("测试 NamespaceCtxName 常量", t, func() {
		Convey("上下文键名应为 'namespace'", func() {
			So(NamespaceCtxName, ShouldEqual, "namespace")
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
			So(body["code"], ShouldEqual, float64(response.CodeNamespaceNotExist))
		})
	})
}

// TestNamespaceMiddleware 测试 NamespaceMiddleware 中间件
func TestNamespaceMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 NamespaceMiddleware 中间件", t, func() {
		cfg := &config.NamespaceConfig{
			HeaderName: "X-Namespace",
			QueryName:  "namespace",
			CookieName: "namespace",
		}

		Convey("应使用默认命名空间当请求中未携带任何命名空间标识", func() {
			cleanup := setupNamespaceCache(DefaultNamespace)
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, DefaultNamespace)
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

		Convey("应从请求头中提取 URL 编码的中文命名空间", func() {
			// 中文命名空间写入请求头前先经前端 encodeURIComponent 编码，
			// 中间件需解码还原后按名称匹配
			cleanup := setupNamespaceCache("测试空间")
			defer cleanup()

			router := setupNamespaceTestRouter(cfg)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-Namespace", "%E6%B5%8B%E8%AF%95%E7%A9%BA%E9%97%B4")
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "测试空间")
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
			cleanup := setupNamespaceCache(DefaultNamespace)
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
			So(body["code"], ShouldEqual, float64(response.CodeNamespaceNotExist))
		})

		Convey("中间件应调用 c.Next() 使后续处理函数继续执行", func() {
			cleanup := setupNamespaceCache(DefaultNamespace)
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
			customCfg := &config.NamespaceConfig{
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
			c.Set(NamespaceCtxName, "test-namespace")

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

		Convey("应返回 DefaultNamespace 的值", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(NamespaceCtxName, DefaultNamespace)

			ns, err := GetNamespace(c)
			So(err, ShouldBeNil)
			So(ns, ShouldEqual, "default")
		})

		Convey("应返回自定义命名空间的值", func() {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(NamespaceCtxName, "my-custom-ns")

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
		cfg := &config.NamespaceConfig{
			HeaderName: "X-Namespace",
			QueryName:  "namespace",
			CookieName: "namespace",
		}

		Convey("完整的命名空间识别和提取流程应正常工作", func() {
			cleanup := setupNamespaceCache("integration-ns")
			defer cleanup()

			var extractedNS string
			var extractErr *response.CodeError

			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(response.LocalizerCtxName, newTestLocalizer())
				c.Set(response.TranslatorCtxName, newTestTranslator())
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
