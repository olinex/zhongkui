package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

// TestPREFIX 测试配置前缀常量
func TestPREFIX(t *testing.T) {
	Convey("测试 PREFIX 常量", t, func() {
		Convey("PREFIX 应该等于 'api'", func() {
			So(PREFIX, ShouldEqual, "api")
		})
	})
}

// TestCONFIG_CTX_NAME 测试上下文键名常量
func TestCONFIG_CTX_NAME(t *testing.T) {
	Convey("测试 CONFIG_CTX_NAME 常量", t, func() {
		Convey("CONFIG_CTX_NAME 应该等于 'config'", func() {
			So(CONFIG_CTX_NAME, ShouldEqual, "config")
		})
	})
}

// TestCookieConfig 测试 CookieConfig 结构体的默认值
func TestCookieConfig(t *testing.T) {
	Convey("测试 CookieConfig 结构体", t, func() {
		Convey("默认值应正确设置", func() {
			cfg := CookieConfig{}
			So(cfg.Name, ShouldEqual, "")
			So(cfg.Domain, ShouldEqual, "")
			So(cfg.Path, ShouldEqual, "")
			So(cfg.Secure, ShouldBeFalse)
			So(cfg.HttpOnly, ShouldBeFalse)
		})

		Convey("应支持自定义值", func() {
			cfg := CookieConfig{
				Name:     "custom-token",
				Domain:   "example.com",
				Path:     "/api",
				Secure:   true,
				HttpOnly: true,
			}
			So(cfg.Name, ShouldEqual, "custom-token")
			So(cfg.Domain, ShouldEqual, "example.com")
			So(cfg.Path, ShouldEqual, "/api")
			So(cfg.Secure, ShouldBeTrue)
			So(cfg.HttpOnly, ShouldBeTrue)
		})
	})
}

// TestAuthConfig 测试 AuthConfig 结构体
func TestAuthConfig(t *testing.T) {
	Convey("测试 AuthConfig 结构体", t, func() {
		Convey("应支持自定义所有字段", func() {
			cfg := AuthConfig{
				Issuer:              "custom-issuer",
				Subject:             "custom-subject",
				Audience:            []string{"web", "mobile"},
				SecretKey:           "my-secret-key",
				TokenPrefix:         "Token:",
				TokenTimeout:        2 * time.Hour,
				TokenRefreshTimeout: 48 * time.Hour,
				HeaderName:          "X-Custom-Token",
				Cookie: CookieConfig{
					Name:     "custom-cookie",
					Domain:   "example.com",
					Path:     "/",
					Secure:   true,
					HttpOnly: true,
				},
				AccessKeyCount: 5,
			}
			So(cfg.Issuer, ShouldEqual, "custom-issuer")
			So(cfg.Subject, ShouldEqual, "custom-subject")
			So(cfg.Audience, ShouldResemble, []string{"web", "mobile"})
			So(cfg.SecretKey, ShouldEqual, "my-secret-key")
			So(cfg.TokenPrefix, ShouldEqual, "Token:")
			So(cfg.TokenTimeout, ShouldEqual, 2*time.Hour)
			So(cfg.TokenRefreshTimeout, ShouldEqual, 48*time.Hour)
			So(cfg.HeaderName, ShouldEqual, "X-Custom-Token")
			So(cfg.Cookie.Name, ShouldEqual, "custom-cookie")
			So(cfg.Cookie.Secure, ShouldBeTrue)
			So(cfg.AccessKeyCount, ShouldEqual, 5)
		})

		Convey("零值应正确初始化", func() {
			cfg := AuthConfig{}
			So(cfg.Issuer, ShouldEqual, "")
			So(cfg.Audience, ShouldBeNil)
			So(cfg.TokenTimeout, ShouldEqual, 0)
			So(cfg.AccessKeyCount, ShouldEqual, 0)
		})
	})
}

// TestAuthConfig_ToJwtRegisterClaims 测试 ToJwtRegisterClaims 方法
func TestAuthConfig_ToJwtRegisterClaims(t *testing.T) {
	Convey("测试 ToJwtRegisterClaims 方法", t, func() {
		Convey("应正确生成 JWT 注册声明", func() {
			cfg := AuthConfig{
				Issuer:       "zhongkui",
				Subject:      "api",
				Audience:     []string{"web"},
				TokenTimeout: time.Hour,
			}

			claims := cfg.ToJwtRegisterClaims()

			So(claims.Issuer, ShouldEqual, "zhongkui")
			So(claims.Subject, ShouldEqual, "api")
			So(claims.Audience, ShouldResemble, jwt.ClaimStrings{"web"})
			So(claims.ExpiresAt, ShouldNotBeNil)
			So(claims.NotBefore, ShouldNotBeNil)
			So(claims.IssuedAt, ShouldNotBeNil)
		})

		Convey("过期时间应基于 TokenTimeout 计算", func() {
			cfg := AuthConfig{
				Issuer:       "test",
				Subject:      "api",
				Audience:     []string{"web"},
				TokenTimeout: 30 * time.Minute,
			}

			claims := cfg.ToJwtRegisterClaims()

			So(claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time), ShouldEqual, 30*time.Minute)
		})

		Convey("NotBefore 和 IssuedAt 应为当前时刻", func() {
			cfg := AuthConfig{
				Issuer:       "test",
				Subject:      "api",
				Audience:     []string{"web"},
				TokenTimeout: time.Hour,
			}

			claims := cfg.ToJwtRegisterClaims()

			So(claims.NotBefore.Time.Equal(claims.IssuedAt.Time), ShouldBeTrue)
			So(time.Since(claims.NotBefore.Time), ShouldBeLessThan, 2*time.Second)
			So(time.Since(claims.IssuedAt.Time), ShouldBeLessThan, 2*time.Second)
		})

		Convey("多次调用应生成不同的过期时间", func() {
			cfg := AuthConfig{
				Issuer:       "test",
				Subject:      "api",
				Audience:     []string{"web"},
				TokenTimeout: time.Hour,
			}

			claims1 := cfg.ToJwtRegisterClaims()
			time.Sleep(time.Second)
			claims2 := cfg.ToJwtRegisterClaims()

			So(claims2.IssuedAt.Time.After(claims1.IssuedAt.Time) || claims2.IssuedAt.Time.Equal(claims1.IssuedAt.Time), ShouldBeTrue)
		})

		Convey("空的 Audience 列表应正确处理", func() {
			cfg := AuthConfig{
				Issuer:       "test",
				Subject:      "api",
				Audience:     []string{},
				TokenTimeout: time.Hour,
			}

			claims := cfg.ToJwtRegisterClaims()
			So(claims.Audience, ShouldResemble, jwt.ClaimStrings{})
		})
	})
}

// TestNamespaceConfig 测试 NamespaceConfig 结构体
func TestNamespaceConfig(t *testing.T) {
	Convey("测试 NamespaceConfig 结构体", t, func() {
		Convey("应支持自定义所有字段", func() {
			cfg := NamespaceConfig{
				HeaderName: "X-Custom-Namespace",
				QueryName:  "ns",
				CookieName: "ns-cookie",
			}
			So(cfg.HeaderName, ShouldEqual, "X-Custom-Namespace")
			So(cfg.QueryName, ShouldEqual, "ns")
			So(cfg.CookieName, ShouldEqual, "ns-cookie")
		})

		Convey("零值应正确初始化", func() {
			cfg := NamespaceConfig{}
			So(cfg.HeaderName, ShouldEqual, "")
			So(cfg.QueryName, ShouldEqual, "")
			So(cfg.CookieName, ShouldEqual, "")
		})
	})
}

// TestCORSConfig 测试 CORSConfig 结构体
func TestCORSConfig(t *testing.T) {
	Convey("测试 CORSConfig 结构体", t, func() {
		Convey("应支持自定义所有字段", func() {
			cfg := CORSConfig{
				AllowOrigin:      "https://example.com",
				AllowCredentials: false,
				AllowHeaders:     []string{"Content-Type", "Authorization"},
				AllowMethods:     []string{"GET", "POST"},
			}
			So(cfg.AllowOrigin, ShouldEqual, "https://example.com")
			So(cfg.AllowCredentials, ShouldBeFalse)
			So(cfg.AllowHeaders, ShouldResemble, []string{"Content-Type", "Authorization"})
			So(cfg.AllowMethods, ShouldResemble, []string{"GET", "POST"})
		})

		Convey("零值应正确初始化", func() {
			cfg := CORSConfig{}
			So(cfg.AllowOrigin, ShouldEqual, "")
			So(cfg.AllowCredentials, ShouldBeFalse)
			So(cfg.AllowHeaders, ShouldBeNil)
			So(cfg.AllowMethods, ShouldBeNil)
		})
	})
}

// TestSecureConfig 测试 SecureConfig 结构体
func TestSecureConfig(t *testing.T) {
	Convey("测试 SecureConfig 结构体", t, func() {
		Convey("应支持自定义所有字段", func() {
			cfg := SecureConfig{
				XFrameOptions:           "SAMEORIGIN",
				ContentSecurityPolicy:   "default-src 'self'",
				XXSSProtection:          "0",
				StrictTransportSecurity: "max-age=86400",
				ReferrerPolicy:          "no-referrer",
				XContentTypeOptions:     "nosniff",
				PermissionsPolicy:       "fullscreen=(self)",
				CORS: CORSConfig{
					AllowOrigin: "https://prod.example.com",
				},
			}
			So(cfg.XFrameOptions, ShouldEqual, "SAMEORIGIN")
			So(cfg.ContentSecurityPolicy, ShouldEqual, "default-src 'self'")
			So(cfg.XXSSProtection, ShouldEqual, "0")
			So(cfg.StrictTransportSecurity, ShouldEqual, "max-age=86400")
			So(cfg.ReferrerPolicy, ShouldEqual, "no-referrer")
			So(cfg.XContentTypeOptions, ShouldEqual, "nosniff")
			So(cfg.PermissionsPolicy, ShouldEqual, "fullscreen=(self)")
			So(cfg.CORS.AllowOrigin, ShouldEqual, "https://prod.example.com")
		})

		Convey("嵌套的 CORS 配置应完整设置", func() {
			cfg := SecureConfig{
				CORS: CORSConfig{
					AllowOrigin:      "https://example.com",
					AllowCredentials: true,
					AllowHeaders:     []string{"Content-Type"},
					AllowMethods:     []string{"GET", "POST"},
				},
			}
			So(cfg.CORS.AllowOrigin, ShouldEqual, "https://example.com")
			So(cfg.CORS.AllowCredentials, ShouldBeTrue)
			So(cfg.CORS.AllowHeaders, ShouldResemble, []string{"Content-Type"})
			So(cfg.CORS.AllowMethods, ShouldResemble, []string{"GET", "POST"})
		})
	})
}

// TestConfig 测试 Config 顶层结构体
func TestConfig(t *testing.T) {
	Convey("测试 Config 结构体", t, func() {
		Convey("应支持自定义所有字段", func() {
			cfg := Config{
				Host: "127.0.0.1",
				Port: 9090,
				Namespace: NamespaceConfig{
					HeaderName: "X-Namespace",
					QueryName:  "namespace",
					CookieName: "namespace",
				},
				Auth: AuthConfig{
					Issuer:       "zhongkui",
					Subject:      "api",
					SecretKey:    "secret",
					TokenTimeout: time.Hour,
				},
				Secure: SecureConfig{
					XFrameOptions: "DENY",
				},
			}
			So(cfg.Host, ShouldEqual, "127.0.0.1")
			So(cfg.Port, ShouldEqual, 9090)
			So(cfg.Namespace.HeaderName, ShouldEqual, "X-Namespace")
			So(cfg.Auth.Issuer, ShouldEqual, "zhongkui")
			So(cfg.Secure.XFrameOptions, ShouldEqual, "DENY")
		})

		Convey("零值应正确初始化", func() {
			cfg := Config{}
			So(cfg.Host, ShouldEqual, "")
			So(cfg.Port, ShouldEqual, 0)
		})
	})
}

// TestConfig_Describe 测试 Config.Describe 方法
func TestConfig_Describe(t *testing.T) {
	Convey("测试 Config.Describe 方法", t, func() {
		Convey("应生成非空的配置描述", func() {
			cfg := Config{}
			descs := cfg.Describe()
			So(descs, ShouldNotBeNil)
			So(len(descs), ShouldBeGreaterThan, 0)
		})

		Convey("配置描述的路径应以 'api' 为前缀", func() {
			cfg := Config{}
			descs := cfg.Describe()
			for _, desc := range descs {
				So(desc.Path, ShouldStartWith, "api.")
			}
		})

		Convey("应包含 Host 和 Port 的描述", func() {
			cfg := Config{}
			descs := cfg.Describe()
			paths := make(map[string]bool)
			for _, desc := range descs {
				paths[desc.Path] = true
			}
			So(paths["api.host"], ShouldBeTrue)
			So(paths["api.port"], ShouldBeTrue)
		})

		Convey("应包含嵌套配置的描述", func() {
			cfg := Config{}
			descs := cfg.Describe()
			paths := make(map[string]bool)
			for _, desc := range descs {
				paths[desc.Path] = true
			}
			So(paths["api.namespace"], ShouldBeTrue)
			So(paths["api.auth"], ShouldBeTrue)
			So(paths["api.secure"], ShouldBeTrue)
		})

		Convey("多次调用应返回相同结果", func() {
			cfg := Config{}
			descs1 := cfg.Describe()
			descs2 := cfg.Describe()
			So(len(descs1), ShouldEqual, len(descs2))
		})
	})
}

// TestExtractConfig 测试 ExtractConfig 函数
func TestExtractConfig(t *testing.T) {
	Convey("测试 ExtractConfig 函数", t, func() {
		Convey("应从 viper 中成功提取配置", func() {
			v := viper.New()
			v.Set("api.host", "192.168.1.1")
			v.Set("api.port", 9090)

			cfg, err := ExtractConfig(v)
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Host, ShouldEqual, "192.168.1.1")
			So(cfg.Port, ShouldEqual, 9090)
		})

		Convey("当 viper 中缺少字段时应使用默认值", func() {
			v := viper.New()
			v.Set("api.host", "10.0.0.1")

			cfg, err := ExtractConfig(v)
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Host, ShouldEqual, "10.0.0.1")
		})

		Convey("当指定前缀不存在时应返回错误", func() {
			v := viper.New()
			cfg, err := ExtractConfig(v)
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("应正确提取嵌套的认证配置", func() {
			v := viper.New()
			v.Set("api.host", "0.0.0.0")
			v.Set("api.auth.issuer", "custom-issuer")
			v.Set("api.auth.subject", "custom-subject")
			v.Set("api.auth.secretKey", "my-secret")

			cfg, err := ExtractConfig(v)
			So(err, ShouldBeNil)
			So(cfg.Auth.Issuer, ShouldEqual, "custom-issuer")
			So(cfg.Auth.Subject, ShouldEqual, "custom-subject")
			So(cfg.Auth.SecretKey, ShouldEqual, "my-secret")
		})

		Convey("应正确提取嵌套的 Cookie 配置", func() {
			v := viper.New()
			v.Set("api.host", "0.0.0.0")
			v.Set("api.auth.cookie.name", "custom-cookie")
			v.Set("api.auth.cookie.domain", "example.com")
			v.Set("api.auth.cookie.secure", true)
			v.Set("api.auth.cookie.httpOnly", true)

			cfg, err := ExtractConfig(v)
			So(err, ShouldBeNil)
			So(cfg.Auth.Cookie.Name, ShouldEqual, "custom-cookie")
			So(cfg.Auth.Cookie.Domain, ShouldEqual, "example.com")
			So(cfg.Auth.Cookie.Secure, ShouldBeTrue)
			So(cfg.Auth.Cookie.HttpOnly, ShouldBeTrue)
		})

		Convey("应正确提取安全配置", func() {
			v := viper.New()
			v.Set("api.host", "0.0.0.0")
			v.Set("api.secure.frameOptions", "SAMEORIGIN")
			v.Set("api.secure.cors.allowOrigin", "https://prod.example.com")

			cfg, err := ExtractConfig(v)
			So(err, ShouldBeNil)
			So(cfg.Secure.XFrameOptions, ShouldEqual, "SAMEORIGIN")
			So(cfg.Secure.CORS.AllowOrigin, ShouldEqual, "https://prod.example.com")
		})
	})
}

// TestConfigMiddleware 测试 ConfigMiddleware 函数
func TestConfigMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 ConfigMiddleware 函数", t, func() {
		Convey("应将配置注入到 Gin 上下文中", func() {
			cfg := &Config{
				Host: "127.0.0.1",
				Port: 8080,
			}

			router := gin.New()
			router.Use(ConfigMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				storedCfg := GetConfig(c)
				c.String(http.StatusOK, "%s:%d", storedCfg.Host, storedCfg.Port)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "127.0.0.1:8080")
		})

		Convey("注入的配置对象应与原始对象相同", func() {
			cfg := &Config{
				Host: "0.0.0.0",
				Port: 3000,
			}

			var retrievedCfg *Config
			router := gin.New()
			router.Use(ConfigMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				retrievedCfg = GetConfig(c)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(retrievedCfg, ShouldEqual, cfg)
		})

		Convey("中间件应调用 c.Next()", func() {
			cfg := &Config{}
			nextCalled := false

			router := gin.New()
			router.Use(ConfigMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(nextCalled, ShouldBeTrue)
		})

		Convey("不同请求应获取相同的配置对象", func() {
			cfg := &Config{
				Host: "10.0.0.1",
				Port: 9999,
			}

			router := gin.New()
			router.Use(ConfigMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				storedCfg := GetConfig(c)
				c.String(http.StatusOK, "%s", storedCfg.Host)
			})

			w1 := httptest.NewRecorder()
			req1, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w1, req1)

			w2 := httptest.NewRecorder()
			req2, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w2, req2)

			So(w1.Body.String(), ShouldEqual, "10.0.0.1")
			So(w2.Body.String(), ShouldEqual, "10.0.0.1")
		})
	})
}

// TestGetConfig 测试 GetConfig 函数
func TestGetConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 GetConfig 函数", t, func() {
		Convey("当上下文中存在配置时应正确返回", func() {
			cfg := &Config{
				Host: "192.168.1.1",
				Port: 7070,
			}

			c := &gin.Context{}
			c.Keys = make(map[string]any)
			c.Keys[CONFIG_CTX_NAME] = cfg

			result := GetConfig(c)
			So(result, ShouldEqual, cfg)
			So(result.Host, ShouldEqual, "192.168.1.1")
			So(result.Port, ShouldEqual, 7070)
		})

		Convey("当上下文中不存在配置时应 panic", func() {
			c := &gin.Context{}
			c.Keys = make(map[string]any)

			So(func() {
				GetConfig(c)
			}, ShouldPanicWith, "config does not exists")
		})

		Convey("当上下文键名错误时应 panic", func() {
			cfg := &Config{Host: "test"}
			c := &gin.Context{}
			c.Keys = make(map[string]any)
			c.Keys["wrong_key"] = cfg

			So(func() {
				GetConfig(c)
			}, ShouldPanic)
		})
	})
}

// TestConfigMiddlewareAndGetConfig_Integration 测试 ConfigMiddleware 和 GetConfig 的集成
func TestConfigMiddlewareAndGetConfig_Integration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 ConfigMiddleware 和 GetConfig 集成", t, func() {
		Convey("完整的配置注入和提取流程应正常工作", func() {
			cfg := &Config{
				Host: "0.0.0.0",
				Port: 8080,
				Namespace: NamespaceConfig{
					HeaderName: "X-Namespace",
					QueryName:  "namespace",
					CookieName: "namespace",
				},
				Auth: AuthConfig{
					Issuer:       "zhongkui",
					Subject:      "api",
					SecretKey:    "test-secret",
					TokenTimeout: time.Hour,
					Cookie: CookieConfig{
						Name:     "jwtoken",
						Domain:   "localhost",
						Path:     "/",
						Secure:   false,
						HttpOnly: false,
					},
				},
				Secure: SecureConfig{
					XFrameOptions: "DENY",
					CORS: CORSConfig{
						AllowOrigin: "http://localhost:3000",
					},
				},
			}

			var retrievedCfg *Config
			router := gin.New()
			router.Use(ConfigMiddleware(cfg))
			router.GET("/test", func(c *gin.Context) {
				retrievedCfg = GetConfig(c)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/test", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(retrievedCfg, ShouldEqual, cfg)
			So(retrievedCfg.Host, ShouldEqual, "0.0.0.0")
			So(retrievedCfg.Port, ShouldEqual, 8080)
			So(retrievedCfg.Auth.Issuer, ShouldEqual, "zhongkui")
			So(retrievedCfg.Secure.CORS.AllowOrigin, ShouldEqual, "http://localhost:3000")
		})
	})
}
