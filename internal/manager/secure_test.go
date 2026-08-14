package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/base"
)

// createTestSecureConfig 创建用于测试的安全响应头配置
func createTestSecureConfig() *SecureConfig {
	return &SecureConfig{
		XFrameOptions:           "DENY",
		ContentSecurityPolicy:   "default-src 'self'",
		XXSSProtection:          "1; mode=block",
		StrictTransportSecurity: "max-age=31536000; includeSubDomains",
		ReferrerPolicy:          "strict-origin",
		XContentTypeOptions:     "nosniff",
		PermissionsPolicy:       "geolocation=(),camera=()",
	}
}

// createTestCORSConfig 创建用于测试的 CORS 配置
func createTestCORSConfig() *CORSConfig {
	return &CORSConfig{
		AllowOrigins:     []string{"http://localhost:3000"},
		AllowCredentials: true,
		AllowHeaders:     []string{"Content-Type", "Authorization"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}
}

// setupRouter 创建带有中间件的测试路由引擎
func setupRouter(middleware ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	for _, m := range middleware {
		r.Use(m)
	}
	r.Any("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return r
}

// TestSecureHeadersMiddleware 测试安全响应头中间件
func TestSecureHeadersMiddleware(t *testing.T) {
	Convey("TestSecureHeadersMiddleware", t, func() {
		cfg := createTestSecureConfig()
		middleware := SecureHeadersMiddleware(cfg)

		Convey("应该返回非空的中间件函数", func() {
			So(middleware, ShouldNotBeNil)
		})

		Convey("应该设置 Content-Security-Policy 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrContentSecurityPolicy), ShouldEqual, "default-src 'self'")
		})

		Convey("应该设置 Permissions-Policy 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrPermissionsPolicy), ShouldEqual, "geolocation=(),camera=()")
		})

		Convey("应该设置 Referrer-Policy 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrReferrerPolicy), ShouldEqual, "strict-origin")
		})

		Convey("应该设置 Strict-Transport-Security 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrStrictTransportSecurity), ShouldEqual, "max-age=31536000; includeSubDomains")
		})

		Convey("应该设置 X-Content-Type-Options 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrXContentTypeOptions), ShouldEqual, "nosniff")
		})

		Convey("应该设置 X-Frame-Options 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrXFrameOptions), ShouldEqual, "DENY")
		})

		Convey("应该设置 X-XSS-Protection 响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)
			So(w.Header().Get(base.HdrXXssProtection), ShouldEqual, "1; mode=block")
		})

		Convey("应该同时设置所有 7 个安全响应头", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrContentSecurityPolicy), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrPermissionsPolicy), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrReferrerPolicy), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrStrictTransportSecurity), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrXContentTypeOptions), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrXFrameOptions), ShouldNotBeEmpty)
			So(w.Header().Get(base.HdrXXssProtection), ShouldNotBeEmpty)
		})

		Convey("当配置为空字符串时应设置空值响应头", func() {
			emptyCfg := &SecureConfig{}
			emptyMiddleware := SecureHeadersMiddleware(emptyCfg)
			r := setupRouter(emptyMiddleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrContentSecurityPolicy), ShouldEqual, "")
			So(w.Header().Get(base.HdrXFrameOptions), ShouldEqual, "")
		})

		Convey("安全响应头设置后请求应正常到达后续处理函数", func() {
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "ok")
		})
	})
}

// TestCORSHeadersMiddleware 测试 CORS 响应头中间件
func TestCORSHeadersMiddleware(t *testing.T) {
	Convey("TestCORSHeadersMiddleware", t, func() {
		cfg := createTestCORSConfig()

		Convey("应该返回非空的中间件函数", func() {
			middleware := CORSHeadersMiddleware(cfg)
			So(middleware, ShouldNotBeNil)
		})

		Convey("GET 请求应设置 CORS 响应头并返回 200", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://localhost:3000")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldEqual, "http://localhost:3000")
		})

		Convey("POST 请求应设置 CORS 响应头并返回 200", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://localhost:3000")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldEqual, "http://localhost:3000")
		})

		Convey("应正确设置 Access-Control-Allow-Origin 响应头", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://localhost:3000")
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldEqual, "http://localhost:3000")
		})

		Convey("Origin 不在允许列表时应不设置 Access-Control-Allow-Origin", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://evil.example.com")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldBeEmpty)
		})

		Convey("请求未携带 Origin 头时应不设置 Access-Control-Allow-Origin", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldBeEmpty)
		})

		Convey("应正确设置 Access-Control-Allow-Credentials 响应头", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowCredentials), ShouldEqual, "true")
		})

		Convey("当 AllowCredentials 为 false 时应设置响应头为 false", func() {
			cfgNoCreds := &CORSConfig{
				AllowOrigins:     []string{"*"},
				AllowCredentials: false,
				AllowHeaders:     []string{"Content-Type"},
				AllowMethods:     []string{"GET"},
			}
			middleware := CORSHeadersMiddleware(cfgNoCreds)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://example.com")
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowCredentials), ShouldEqual, "false")
			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldEqual, "http://example.com")
		})

		Convey("应正确设置 Access-Control-Allow-Headers 响应头", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowHeaders), ShouldEqual, "Content-Type, Authorization")
		})

		Convey("应正确设置 Access-Control-Allow-Methods 响应头", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowMethods), ShouldEqual, "GET, POST, PUT, DELETE, OPTIONS")
		})

		Convey("OPTIONS 预检请求（允许的方法）应返回 204 No Content", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set(base.HdrAccessControlRequestMethod, "POST")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusNoContent)
		})

		Convey("OPTIONS 预检请求（不允许的方法）应返回 403 Forbidden", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set(base.HdrAccessControlRequestMethod, "PATCH")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusForbidden)
		})

		Convey("OPTIONS 请求无 Access-Control-Request-Method 头应继续处理", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("OPTIONS 预检请求也应包含 CORS 响应头", func() {
			middleware := CORSHeadersMiddleware(cfg)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set(base.HdrOrigin, "http://localhost:3000")
			req.Header.Set(base.HdrAccessControlRequestMethod, "GET")
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowOrigin), ShouldEqual, "http://localhost:3000")
			So(w.Header().Get(base.HdrAccessControlAllowCredentials), ShouldEqual, "true")
			So(w.Header().Get(base.HdrAccessControlAllowHeaders), ShouldEqual, "Content-Type, Authorization")
			So(w.Header().Get(base.HdrAccessControlAllowMethods), ShouldEqual, "GET, POST, PUT, DELETE, OPTIONS")
		})

		Convey("AllowHeaders 为空时应设置空的 Access-Control-Allow-Headers", func() {
			cfgNoHeaders := &CORSConfig{
				AllowOrigins: []string{"*"},
				AllowHeaders: []string{},
				AllowMethods: []string{"GET"},
			}
			middleware := CORSHeadersMiddleware(cfgNoHeaders)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Header().Get(base.HdrAccessControlAllowHeaders), ShouldEqual, "")
		})

		Convey("AllowMethods 为空时预检请求应返回 403", func() {
			cfgNoMethods := &CORSConfig{
				AllowOrigins: []string{"*"},
				AllowHeaders: []string{"Content-Type"},
				AllowMethods: []string{},
			}
			middleware := CORSHeadersMiddleware(cfgNoMethods)
			r := setupRouter(middleware)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodOptions, "/test", nil)
			req.Header.Set(base.HdrAccessControlRequestMethod, "GET")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusForbidden)
		})
	})
}
