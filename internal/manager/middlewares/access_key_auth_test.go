package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/manager/config"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/models/core"
	authpkg "zhongkuisec.com/zhongkui/internal/pkg/auth"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/redis"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// newAccessKeyTestAuthConfig 构造仅用于 JWT 分支的测试认证配置。
func newAccessKeyTestAuthConfig() *config.AuthConfig {
	return &config.AuthConfig{
		Issuer:              "zhongkui-test",
		Subject:             "api-test",
		Audience:            []string{"web-test"},
		TokenPrefix:         "Bearer:",
		TokenTimeout:        time.Hour,
		TokenRefreshTimeout: 24 * time.Hour,
		HeaderName:          "X-Jwtoken",
		Cookie: config.CookieConfig{
			Name:     "jwt-token",
			Domain:   "localhost",
			Path:     "/",
			Secure:   false,
			HttpOnly: true,
		},
		AccessKeyCount: 3,
	}
}

// newAccessKeyTestKeyDerivation 构造与生产同源、但使用测试主密钥的密钥派生器。
func newAccessKeyTestKeyDerivation() *key.KeyDerivation {
	provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
	book, err := key.LoadPasswordBook("")
	if err != nil {
		panic("failed to load password book: " + err.Error())
	}
	return key.NewKeyDerivation(provider, book)
}

var initAccessKeyTestKeyOnce sync.Once

// initAccessKeyTestKeyDerivation 初始化全局密钥派生：用户密码等字段经 crypto
// 序列化器加解密存储，测试须与生产一致地派生密钥，否则写库报 KeyDerivation not initialized。
func initAccessKeyTestKeyDerivation() {
	initAccessKeyTestKeyOnce.Do(func() {
		provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
		book, err := key.LoadPasswordBook("")
		if err != nil {
			panic("failed to load password book: " + err.Error())
		}
		key.InitGlobalKeyDerivation(provider, book)
	})
}

// accessKeyTestAuth 聚合测试用的多路鉴权中间件与独立的 JWT 认证器。
type accessKeyTestAuth struct {
	// middleware 统一的多路鉴权聚合器
	middleware *Authenticator
	// jwt 独立的 JWT 认证器，用于签发测试令牌
	jwt *JwtAuthenticator
}

// newAccessKeyTestEnv 为单个用例准备独立事务的测试库、miniredis 与多路鉴权聚合器。
// AccessKeyPool 的重放窗口取 1 分钟，与 router.go 的生产装配一致。
func newAccessKeyTestEnv(t *testing.T) (*database.DB, *accessKeyTestAuth) {
	t.Helper()
	initAccessKeyTestKeyDerivation()
	db := testdb.Core(t)
	mr := miniredis.RunT(t)
	cache := redis.NewClient(&redis.Config{Type: redis.NormalType, Addr: mr.Addr()})
	t.Cleanup(func() { _ = cache.Shutdown() })
	pool := core.NewAccessKeyPool(db, cache, time.Minute)
	jwtAuthenticator := NewJwtAuthenticator(newAccessKeyTestAuthConfig(), db, newAccessKeyTestKeyDerivation())
	accessKeyAuthenticator := NewAccessKeyAuthenticator(pool, db)
	return db, &accessKeyTestAuth{
		middleware: NewAuthenticator(accessKeyAuthenticator, jwtAuthenticator),
		jwt:        jwtAuthenticator,
	}
}

// createAccessKeyTestUser 在测试库中创建启用用户。
func createAccessKeyTestUser(t *testing.T, db *database.DB, username string) *core.User {
	t.Helper()
	user := &core.User{
		UnionID:    serializer.NewHexUUIDv4(),
		NickleName: username,
		Username:   username,
		Password:   serializer.NewCrypto([]byte("access-key-test-password")),
		Email:      username + "@example.com",
		Active:     true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return user
}

// createAccessKeyTestKey 创建并持久化访问密钥。
func createAccessKeyTestKey(t *testing.T, db *database.DB, user *core.User, enabled bool, expiredAt *time.Time) *core.AccessKey {
	t.Helper()
	key, err := core.NewAccessKey(user, enabled, "access key test", expiredAt)
	if err != nil {
		t.Fatalf("new access key: %v", err)
	}
	if err := db.Create(key).Error; err != nil {
		t.Fatalf("create access key: %v", err)
	}
	return key
}

// setupAccessKeyTestRouter 构造带认证中间件的测试路由，处理器回显注入用户的 UnionID。
func setupAccessKeyTestRouter(auth *accessKeyTestAuth, allowAnonymous bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	logger, _ := logging.NewDebugLogger()
	r.Use(func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newTestLocalizer())
		c.Set(response.TranslatorCtxName, newTestTranslator())
		if logger != nil {
			c.Set(LoggerCtxName, &ApiLogger{logger: logger})
		}
		c.Next()
	})
	r.Use(auth.middleware.AuthenticateMiddleware(allowAnonymous))
	r.GET("/api/v1/test", func(c *gin.Context) {
		user, err := GetUser(c)
		if err != nil {
			c.String(http.StatusOK, "missing")
			return
		}
		c.String(http.StatusOK, user.UnionID.String())
	})
	return r
}

// accessKeyResponseCode 提取响应体中的业务码。
func accessKeyResponseCode(w *httptest.ResponseRecorder) float64 {
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	code, _ := body["code"].(float64)
	return code
}

// sendAccessKeyRequest 构造并发送一次带 X-Access-Key 的请求。
func sendAccessKeyRequest(router *gin.Engine, token string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	req.Header.Set(authpkg.AccessKeyHeaderName, token)
	router.ServeHTTP(w, req)
	return w
}

// TestAccessKeyAuthentication 测试 X-Access-Key 签名鉴权的成功与各类失败场景。
func TestAccessKeyAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("访问密钥签名鉴权", t, func() {
		Convey("合法签名应通过并注入对应用户", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-valid")
			key := createAccessKeyTestKey(t, db, user, true, nil)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret(), key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, user.UnionID.String())
		})

		Convey("签名不符应返回 401 与访问密钥错误码", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-badsig")
			key := createAccessKeyTestKey(t, db, user, true, nil)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret()+"tampered", key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("密钥不存在应返回 401", func() {
			_, auth := newAccessKeyTestEnv(t)

			token, err := authpkg.SignHTTPAccessKey(
				"any-secret", "missing-access-key", http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("密钥停用应返回 401", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-disabled")
			key := createAccessKeyTestKey(t, db, user, false, nil)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret(), key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("密钥已过期应返回 401", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-expired")
			expiredAt := time.Now().Add(-time.Hour)
			key := createAccessKeyTestKey(t, db, user, true, &expiredAt)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret(), key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("nonce 重复（重放）应返回 401", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-replay")
			key := createAccessKeyTestKey(t, db, user, true, nil)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret(), key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			router := setupAccessKeyTestRouter(auth, false)
			first := sendAccessKeyRequest(router, token)
			So(first.Code, ShouldEqual, http.StatusOK)

			second := sendAccessKeyRequest(router, token)
			So(second.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(second), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("密钥所属用户不存在应返回 401", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-goneuser")
			key := createAccessKeyTestKey(t, db, user, true, nil)
			// 软删除用户后密钥行仍在，但 Preload("User") 不再返回该用户
			if err := db.Delete(user).Error; err != nil {
				t.Fatalf("soft delete user: %v", err)
			}

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret(), key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, false), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})

		Convey("allowAnonymous 为 true 时显式凭据失败同样返回 401", func() {
			db, auth := newAccessKeyTestEnv(t)
			user := createAccessKeyTestUser(t, db, "ak-anonfail")
			key := createAccessKeyTestKey(t, db, user, true, nil)

			token, err := authpkg.SignHTTPAccessKey(
				key.Secret()+"tampered", key.AccessKeyID, http.MethodGet, "/api/v1/test", time.Now().UnixMilli(),
			)
			So(err, ShouldBeNil)

			w := sendAccessKeyRequest(setupAccessKeyTestRouter(auth, true), token)
			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})
	})
}

// TestJwtAuthenticationWithoutAccessKeyHeader 验证无 X-Access-Key 时 JWT 与匿名逻辑保持不变。
func TestJwtAuthenticationWithoutAccessKeyHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("无 X-Access-Key 时保持既有 JWT 行为", t, func() {
		db, auth := newAccessKeyTestEnv(t)
		user := createAccessKeyTestUser(t, db, "jwt-user")

		Convey("有效 JWT 应通过", func() {
			token, codeErr := auth.jwt.Sign(user)
			So(codeErr, ShouldBeNil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			req.Header.Set(auth.jwt.cfg.HeaderName, token)
			setupAccessKeyTestRouter(auth, false).ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, user.UnionID.String())
		})

		Convey("allowAnonymous 为 true 且无凭据时应注入匿名用户", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			setupAccessKeyTestRouter(auth, true).ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, anonymous.UnionID.String())
		})

		Convey("allowAnonymous 为 false 且无凭据时应返回 401", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			setupAccessKeyTestRouter(auth, false).ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadJwtToken))
		})

		Convey("携带无效 X-Access-Key 时不回退到有效 JWT", func() {
			token, codeErr := auth.jwt.Sign(user)
			So(codeErr, ShouldBeNil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
			req.Header.Set(auth.jwt.cfg.HeaderName, token)
			req.Header.Set(authpkg.AccessKeyHeaderName, "not-a-valid-token")
			setupAccessKeyTestRouter(auth, false).ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusUnauthorized)
			So(accessKeyResponseCode(w), ShouldEqual, float64(response.CodeBadAccessKey))
		})
	})
}
