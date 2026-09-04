package views

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/manager/features"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	resp "zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/mail"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
	mailhandler "zhongkui.com/zhongkui/internal/worker/handlers/mail"
)

func createUserSelfTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

func createTestUserInDB(db *database.DB, username, nickleName, email, password string, active bool) *core.User {
	hashedPassword, err := crypto.HashPassword(password)
	if err != nil {
		panic("failed to hash password: " + err.Error())
	}
	user := &core.User{
		UnionID:    serializer.NewHexUUIDv4(),
		NickleName: nickleName,
		Username:   username,
		Password:   serializer.NewCrypto(hashedPassword),
		Email:      email,
		Active:     active,
	}
	if err := db.Create(user).Error; err != nil {
		panic("failed to create test user: " + err.Error())
	}
	return user
}

func createUserSelfTestAuthConfig() *manager.AuthConfig {
	return &manager.AuthConfig{
		Issuer:              "zhongkui-test",
		Subject:             "api-test",
		Audience:            []string{"web-test"},
		TokenPrefix:         "Bearer:",
		TokenTimeout:        time.Hour,
		TokenRefreshTimeout: 24 * time.Hour,
		HeaderName:          "X-Jwtoken",
		Cookie: manager.CookieConfig{
			Name:     "jwt-token",
			Domain:   "localhost",
			Path:     "/",
			Secure:   false,
			HttpOnly: true,
		},
		AccessKeyCount: 3,
	}
}

func createTestKeyDerivation() *key.KeyDerivation {
	provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
	book, err := key.LoadPasswordBook("")
	if err != nil {
		panic("failed to load password book: " + err.Error())
	}
	return key.NewKeyDerivation(provider, book)
}

func createTestLoggingLogger() *logging.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	return &logging.Logger{SugaredLogger: zapLogger.Sugar()}
}

func injectLoggerMiddleware(logger *logging.Logger) gin.HandlerFunc {
	return middlewares.ZapLoggerMiddleware(logger)
}

func setupUserSelfTestRouterWithDB(db *database.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Next()
	})
	return r
}

func setupUserSelfTestRouterWithDBAndUser(db *database.DB, user *core.User) *gin.Engine {
	r := setupUserSelfTestRouterWithDB(db)
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.UserCtxName, user)
		c.Next()
	})
	return r
}

func setupUserSelfTestRouterWithFullDeps(db *database.DB, user *core.User) *gin.Engine {
	r := setupUserSelfTestRouterWithDBAndUser(db, user)
	authConfig := createUserSelfTestAuthConfig()
	kd := createTestKeyDerivation()
	authenticator := middlewares.NewJwtAuthenticator(authConfig, db, kd)
	loggingLogger := createTestLoggingLogger()
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.AuthCtxName, authenticator)
		c.Set(manager.ConfigCtxName, &manager.Config{
			Auth: *authConfig,
		})
		c.Next()
	})
	r.Use(injectLoggerMiddleware(loggingLogger))
	return r
}

func uintToStr(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

func TestUserProfileChangeIn_Struct(t *testing.T) {
	Convey("测试 UserProfileChangeIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			in := UserProfileChangeIn{
				NickleName: "测试昵称",
				Email:      "test@example.com",
			}
			So(in.NickleName, ShouldEqual, "测试昵称")
			So(in.Email, ShouldEqual, "test@example.com")
		})

		Convey("应正确序列化为 JSON", func() {
			in := UserProfileChangeIn{
				NickleName: "newnickle",
				Email:      "new@example.com",
			}
			data, err := json.Marshal(in)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"nickleName":"newnickle"`)
			So(string(data), ShouldContainSubstring, `"email":"new@example.com"`)
		})
	})
}

func TestUserPasswordLoginIn_Struct(t *testing.T) {
	Convey("测试 UserPasswordLoginIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			in := UserPasswordLoginIn{
				Username: "testuser",
				Password: "Str0ngP@ss",
			}
			So(in.Username, ShouldEqual, "testuser")
			So(in.Password, ShouldEqual, "Str0ngP@ss")
		})

		Convey("应正确序列化为 JSON", func() {
			in := UserPasswordLoginIn{
				Username: "loginuser",
				Password: "An0therP@ss",
			}
			data, err := json.Marshal(in)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"username":"loginuser"`)
			So(string(data), ShouldContainSubstring, `"password":"An0therP@ss"`)
		})
	})
}

func TestUserRegisterIn_Struct(t *testing.T) {
	Convey("测试 UserRegisterIn 结构体", t, func() {
		Convey("应正确嵌入 UserProfileChangeIn 和 UserPasswordLoginIn", func() {
			in := UserRegisterIn{
				UserProfileChangeIn: UserProfileChangeIn{
					NickleName: "注册昵称",
					Email:      "register@example.com",
				},
				UserPasswordLoginIn: UserPasswordLoginIn{
					Username: "reguser",
					Password: "Str0ngP@ssw0rd",
				},
			}
			So(in.NickleName, ShouldEqual, "注册昵称")
			So(in.Email, ShouldEqual, "register@example.com")
			So(in.Username, ShouldEqual, "reguser")
			So(in.Password, ShouldEqual, "Str0ngP@ssw0rd")
		})

		Convey("应正确序列化为 JSON", func() {
			in := UserRegisterIn{
				UserProfileChangeIn: UserProfileChangeIn{
					NickleName: "序列昵称",
					Email:      "serial@example.com",
				},
				UserPasswordLoginIn: UserPasswordLoginIn{
					Username: "serialuser",
					Password: "Ser1alP@ss",
				},
			}
			data, err := json.Marshal(in)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"nickleName":"序列昵称"`)
			So(string(data), ShouldContainSubstring, `"username":"serialuser"`)
		})
	})
}

func TestUserPasswordResetIn_Struct(t *testing.T) {
	Convey("测试 UserPasswordResetIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			in := UserPasswordResetIn{
				OldPassword: "OldP@ssw0rd",
				NewPassword: "NewP@ssw0rd",
			}
			So(in.OldPassword, ShouldEqual, "OldP@ssw0rd")
			So(in.NewPassword, ShouldEqual, "NewP@ssw0rd")
		})

		Convey("应正确序列化为 JSON", func() {
			in := UserPasswordResetIn{
				OldPassword: "oldpass",
				NewPassword: "newpass",
			}
			data, err := json.Marshal(in)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"oldPassword":"oldpass"`)
			So(string(data), ShouldContainSubstring, `"newPassword":"newpass"`)
		})
	})
}

func TestUserAccessKeyCreateIn_Struct(t *testing.T) {
	Convey("测试 UserAccessKeyCreateIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			expiredAt := time.Now().Add(24 * time.Hour)
			in := UserAccessKeyCreateIn{
				Enabled:     true,
				Description: "测试访问密钥描述",
				ExpiredAt:   &expiredAt,
			}
			So(in.Enabled, ShouldBeTrue)
			So(in.Description, ShouldEqual, "测试访问密钥描述")
			So(in.ExpiredAt, ShouldNotBeNil)
		})

		Convey("应处理无过期时间的情况", func() {
			in := UserAccessKeyCreateIn{
				Enabled:     false,
				Description: "永不过期的密钥",
				ExpiredAt:   nil,
			}
			So(in.Enabled, ShouldBeFalse)
			So(in.ExpiredAt, ShouldBeNil)
		})
	})
}

func TestAuthorizationInfoOut_Struct(t *testing.T) {
	Convey("测试 AuthorizationInfoOut 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			out := AuthorizationInfoOut{
				Model: base.RbacModel,
				Policies: [][]string{
					{"p", "admin", "user", "read", "allow"},
					{"g", "user-union-id", "admin", "default", ""},
				},
				Namespaces: []string{"default", "production"},
			}
			So(out.Model, ShouldEqual, base.RbacModel)
			So(len(out.Policies), ShouldEqual, 2)
			So(out.Policies[0][0], ShouldEqual, "p")
			So(out.Namespaces, ShouldContain, "default")
			So(out.Namespaces, ShouldContain, "production")
		})

		Convey("应处理空策略列表", func() {
			out := AuthorizationInfoOut{
				Model:      base.RbacModel,
				Policies:   [][]string{},
				Namespaces: []string{},
			}
			So(len(out.Policies), ShouldEqual, 0)
			So(len(out.Namespaces), ShouldEqual, 0)
		})

		Convey("应正确序列化为 JSON", func() {
			out := AuthorizationInfoOut{
				Model:      base.RbacModel,
				Policies:   [][]string{{"g", "uid", "admin", "default", ""}},
				Namespaces: []string{"default"},
			}
			data, err := json.Marshal(out)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"model"`)
			So(string(data), ShouldContainSubstring, `"policies"`)
			So(string(data), ShouldContainSubstring, `"namespaces"`)
		})
	})
}

func TestUserAccessKeyOut_Struct(t *testing.T) {
	Convey("测试 UserAccessKeyOut 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			now := time.Now()
			expiredAt := now.Add(24 * time.Hour)
			out := UserAccessKeyOut{
				Enabled:         true,
				AccessKeyID:     "testAKID1234567890",
				AccessKeySecret: "testAKSecret1234567890abcdef",
				Description:     "测试密钥",
				ExpiredAt:       &expiredAt,
				LastAccessAt:    &now,
			}
			So(out.Enabled, ShouldBeTrue)
			So(out.AccessKeyID, ShouldEqual, "testAKID1234567890")
			So(out.AccessKeySecret, ShouldEqual, "testAKSecret1234567890abcdef")
			So(out.Description, ShouldEqual, "测试密钥")
		})

		Convey("应正确序列化为 JSON", func() {
			out := UserAccessKeyOut{
				Enabled:         true,
				AccessKeyID:     "akid",
				AccessKeySecret: "secret",
				Description:     "desc",
			}
			data, err := json.Marshal(out)
			So(err, ShouldBeNil)
			So(string(data), ShouldContainSubstring, `"accessKeyID":"akid"`)
			So(string(data), ShouldContainSubstring, `"enabled":true`)
		})
	})
}

func TestUserPasswordLogin(t *testing.T) {
	Convey("测试 UserPasswordLogin 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		authConfig := createUserSelfTestAuthConfig()
		kd := createTestKeyDerivation()
		authenticator := middlewares.NewJwtAuthenticator(authConfig, db, kd)
		loggingLogger := createTestLoggingLogger()
		r := setupUserSelfTestRouterWithDB(db)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.Use(func(c *gin.Context) {
			c.Set(middlewares.AuthCtxName, authenticator)
			c.Next()
		})
		r.POST("/api/v1/user/password/login", UserPasswordLogin)

		Convey("应成功登录并返回令牌", func() {
			createTestUserInDB(db, "loginuser", "登录用户", "login@example.com", "Str0ngP@ss", true)

			reqBody := map[string]any{
				"username": "loginuser",
				"password": "Str0ngP@ss",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			// 响应体应返回 JWT 令牌，供跨源前端持久化使用
			var rspBody map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rspBody), ShouldBeNil)
			data, ok := rspBody["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["token"], ShouldNotBeEmpty)

			cookie := w.Result().Cookies()
			So(len(cookie), ShouldBeGreaterThan, 0)
			So(cookie[0].Name, ShouldEqual, "jwt-token")
			So(cookie[0].Value, ShouldNotBeEmpty)
		})

		Convey("应返回错误当用户名不存在时", func() {
			reqBody := map[string]any{
				"username": "nonexistent",
				"password": "Str0ngP@ss",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当密码错误时", func() {
			createTestUserInDB(db, "wrongpwduser", "密码错误用户", "wrongpwd@example.com", "CorrectP@ss1", true)

			reqBody := map[string]any{
				"username": "wrongpwduser",
				"password": "WrongP@ss1",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当用户被禁用时", func() {
			user := createTestUserInDB(db, "inactiveuser2", "禁用用户", "inactive2@example.com", "Str0ngP@ss", true)
			db.Model(user).Update("active", false)

			reqBody := map[string]any{
				"username": "inactiveuser2",
				"password": "Str0ngP@ss",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			reqBody := map[string]any{
				"username": "someuser",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/login", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestUserSelfPasswordReset(t *testing.T) {
	Convey("测试 UserSelfPasswordReset 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "resetuser", "重置用户", "reset@example.com", "OldP@ssw0rd", true)
		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/password/reset", UserSelfPasswordReset)

		Convey("应成功重置密码", func() {
			reqBody := map[string]any{
				"oldPassword": "OldP@ssw0rd",
				"newPassword": "NewP@ssw0rd",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var updatedUser core.User
			db.First(&updatedUser, "union_id = ?", user.UnionID)
			So(crypto.CheckPassword(updatedUser.Password.Value(), "NewP@ssw0rd"), ShouldBeTrue)
		})

		Convey("应返回错误当旧密码错误时", func() {
			reqBody := map[string]any{
				"oldPassword": "WrongOldP@ss",
				"newPassword": "NewP@ssw0rd",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			reqBody := map[string]any{
				"oldPassword": "OldP@ssw0rd",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestUserSelfProfileChange(t *testing.T) {
	Convey("测试 UserSelfProfileChange 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "profileuser", "原始昵称", "original@example.com", "Str0ngP@ss", true)
		r := setupUserSelfTestRouterWithFullDeps(db, user)
		r.POST("/api/v1/user/self/profile/change", UserSelfProfileChange)

		Convey("应成功修改用户资料", func() {
			reqBody := map[string]any{
				"nickleName": "更新后的昵称",
				"email":      "updated@example.com",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/profile/change", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var updatedUser core.User
			db.First(&updatedUser, "union_id = ?", user.UnionID)
			So(updatedUser.NickleName, ShouldEqual, "更新后的昵称")
			So(updatedUser.Email, ShouldEqual, "updated@example.com")
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			reqBody := map[string]any{
				"nickleName": "只有昵称的测试",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/profile/change", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

func TestUserSelfJwtRefresh(t *testing.T) {
	Convey("测试 UserSelfJwtRefresh 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "refreshuser", "刷新用户", "refresh@example.com", "Str0ngP@ss", true)
		authConfig := createUserSelfTestAuthConfig()
		kd := createTestKeyDerivation()
		authenticator := middlewares.NewJwtAuthenticator(authConfig, db, kd)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(func(c *gin.Context) {
			c.Set(middlewares.AuthCtxName, authenticator)
			c.Next()
		})
		r.POST("/api/v1/user/self/jwt/refresh", UserSelfJwtRefresh)

		Convey("应返回错误当请求中没有有效JWT时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/jwt/refresh", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusUnauthorized)
		})

		Convey("应成功刷新当请求中有有效JWT时", func() {
			token, codeErr := authenticator.Sign(user)
			So(codeErr, ShouldBeNil)
			So(token, ShouldNotBeEmpty)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/jwt/refresh", nil)
			req.Header.Set(authConfig.HeaderName, token)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应成功刷新已过期但在刷新窗口内的令牌", func() {
			expiredConfig := createUserSelfTestAuthConfig()
			expiredConfig.TokenTimeout = -time.Second // 签发即过期
			expiredAuth := middlewares.NewJwtAuthenticator(expiredConfig, db, kd)
			token, codeErr := expiredAuth.Sign(user)
			So(codeErr, ShouldBeNil)
			So(token, ShouldNotBeEmpty)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/jwt/refresh", nil)
			req.Header.Set(expiredConfig.HeaderName, token)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var rspBody map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rspBody), ShouldBeNil)
			data, ok := rspBody["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			newToken, ok := data["token"].(string)
			So(ok, ShouldBeTrue)
			// 过期令牌应被重新签发，而非原样返回
			So(newToken, ShouldNotBeEmpty)
			So(newToken, ShouldNotEqual, token)
		})

		Convey("应返回错误当令牌已过期且超过刷新窗口时", func() {
			expiredConfig := createUserSelfTestAuthConfig()
			expiredConfig.TokenTimeout = -1 * 25 * time.Hour // 过期远超刷新窗口（24h）
			expiredAuth := middlewares.NewJwtAuthenticator(expiredConfig, db, kd)
			token, codeErr := expiredAuth.Sign(user)
			So(codeErr, ShouldBeNil)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/jwt/refresh", nil)
			req.Header.Set(expiredConfig.HeaderName, token)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusUnauthorized)
		})
	})
}

func TestUserSelfAuthorizationInfoGet(t *testing.T) {
	Convey("测试 UserSelfAuthorizationInfoGet 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "authuser", "授权用户", "auth@example.com", "Str0ngP@ss", true)
		loggingLogger := createTestLoggingLogger()

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.POST("/api/v1/user/self/authorization-info/get", UserSelfAuthorizationInfoGet)

		Convey("应成功获取授权信息", func() {
			group := &core.Group{
				Name:        "testgroup",
				Description: "测试用户组",
			}
			db.Create(group)

			policy := core.NewUserPolicy(user.UnionID, "testgroup", "default")
			db.Create(policy)

			rbacPolicy := &core.RbacPolicy{
				Ptype: base.PolicyLabel,
				V0:    "testgroup",
				V1:    "user",
				V2:    "read",
				V3:    base.EffectAllow,
			}
			db.Create(rbacPolicy)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/authorization-info/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response, ShouldContainKey, "data")

			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data, ShouldContainKey, "model")
			So(data, ShouldContainKey, "policies")
			So(data, ShouldContainKey, "namespaces")
		})

		Convey("应返回空策略列表当用户没有用户组时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/authorization-info/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)

			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)

			policies, ok := data["policies"].([]any)
			So(ok, ShouldBeTrue)
			So(len(policies), ShouldEqual, 0)
		})
	})
}

func TestUserSelfAccessKeyListGet(t *testing.T) {
	Convey("测试 UserSelfAccessKeyListGet 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "aklistuser", "密钥用户", "aklist@example.com", "Str0ngP@ss", true)
		loggingLogger := createTestLoggingLogger()

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.POST("/api/v1/user/self/access-key/list", UserSelfAccessKeyListGet)

		Convey("应返回空列表当用户没有访问密钥时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/list", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)

			data, ok := response["data"].([]any)
			So(ok, ShouldBeTrue)
			So(len(data), ShouldEqual, 0)
		})

		Convey("应返回用户的所有访问密钥", func() {
			key1, err := core.NewAccessKey(user, true, "第一个密钥描述内容", nil)
			So(err, ShouldBeNil)
			db.Create(key1)

			key2, err := core.NewAccessKey(user, false, "第二个密钥描述内容", nil)
			So(err, ShouldBeNil)
			db.Create(key2)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/list", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err2 := json.Unmarshal(w.Body.Bytes(), &response)
			So(err2, ShouldBeNil)

			data, ok := response["data"].([]any)
			So(ok, ShouldBeTrue)
			So(len(data), ShouldEqual, 2)
		})
	})
}

func TestUserSelfAccessKeyCreate(t *testing.T) {
	Convey("测试 UserSelfAccessKeyCreate 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "akcreateuser", "创建密钥用户", "akcreate@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithFullDeps(db, user)
		r.POST("/api/v1/user/self/access-key/create", UserSelfAccessKeyCreate)

		Convey("应成功创建访问密钥", func() {
			reqBody := map[string]any{
				"enabled":     true,
				"description": "新创建的访问密钥描述",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response, ShouldContainKey, "data")

			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data, ShouldContainKey, "accessKeyID")
			So(data, ShouldContainKey, "accessKeySecret")
			So(data["enabled"], ShouldEqual, true)
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			reqBody := map[string]any{
				"enabled": true,
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当访问密钥数量超过限制时", func() {
			for i := 0; i < 3; i++ {
				key, err := core.NewAccessKey(user, true, "已有密钥描述内容填充", nil)
				So(err, ShouldBeNil)
				db.Create(key)
			}

			reqBody := map[string]any{
				"enabled":     true,
				"description": "超额密钥描述内容",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response, ShouldContainKey, "code")
		})

		Convey("应成功创建带过期时间的访问密钥", func() {
			expiredAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
			reqBody := map[string]any{
				"enabled":     true,
				"description": "带过期时间的密钥描述",
				"expiredAt":   expiredAt,
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/create", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

func TestUserSelfAccessKeyRemove(t *testing.T) {
	Convey("测试 UserSelfAccessKeyRemove 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "akrmuser", "删除密钥用户", "akrm@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/access-key/:id/remove", UserSelfAccessKeyRemove)

		Convey("应成功删除访问密钥", func() {
			key, err := core.NewAccessKey(user, true, "待删除的密钥描述", nil)
			So(err, ShouldBeNil)
			db.Create(key)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/"+uintToStr(key.ID)+"/remove", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var count int64
			db.Model(&core.AccessKey{}).Where("id = ?", key.ID).Count(&count)
			So(count, ShouldEqual, 0)
		})

		Convey("应成功当删除不存在的密钥时（幂等性）", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/99999/remove", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应返回错误当URL参数id格式无效时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/invalid/remove", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestUserSelfAccessKeyEnabled(t *testing.T) {
	Convey("测试 UserSelfAccessKeyEnabled 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "akenableuser", "启用密钥用户", "akenable@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/access-key/:id/activate", UserSelfAccessKeyEnabled(true))
		r.POST("/api/v1/user/self/access-key/:id/deactivate", UserSelfAccessKeyEnabled(false))

		Convey("应成功启用访问密钥", func() {
			key, err := core.NewAccessKey(user, false, "待启用密钥描述内容", nil)
			So(err, ShouldBeNil)
			db.Create(key)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/"+uintToStr(key.ID)+"/activate", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err2 := json.Unmarshal(w.Body.Bytes(), &response)
			So(err2, ShouldBeNil)
			So(response, ShouldContainKey, "data")

			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["enabled"], ShouldEqual, true)
		})

		Convey("应成功禁用访问密钥", func() {
			key, err := core.NewAccessKey(user, true, "待禁用密钥描述内容", nil)
			So(err, ShouldBeNil)
			db.Create(key)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/"+uintToStr(key.ID)+"/deactivate", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var response map[string]any
			err2 := json.Unmarshal(w.Body.Bytes(), &response)
			So(err2, ShouldBeNil)

			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["enabled"], ShouldEqual, false)
		})

		Convey("应返回错误当URL参数id格式无效时", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/access-key/invalid/activate", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestErrVariables(t *testing.T) {
	Convey("测试 user.go 中的错误变量", t, func() {
		Convey("ErrUsernamePasswordCheckFailed 应不为nil", func() {
			So(ErrUsernamePasswordCheckFailed, ShouldNotBeNil)
		})

		Convey("ErrOldPasswordCheckFailed 应不为nil", func() {
			So(ErrOldPasswordCheckFailed, ShouldNotBeNil)
		})

		Convey("ErrUsernameOrEmailAlreadyExist 应不为nil", func() {
			So(ErrUsernameOrEmailAlreadyExist, ShouldNotBeNil)
		})

		Convey("ErrTooManyAccessKey 应不为nil", func() {
			So(ErrTooManyAccessKey, ShouldNotBeNil)
		})

		Convey("ErrFrontendConfigConflict 应不为nil", func() {
			So(ErrFrontendConfigConflict, ShouldNotBeNil)
		})

		Convey("ErrFrontendConfigSyncDisabled 应不为nil", func() {
			So(ErrFrontendConfigSyncDisabled, ShouldNotBeNil)
		})
	})
}

func TestUserSelfFrontendConfig(t *testing.T) {
	Convey("测试前端配置同步接口", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "fcsyncuser", "配置同步用户", "fcsync@example.com", "Str0ngP@ss", true)
		loggingLogger := createTestLoggingLogger()

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.POST("/api/v1/user/self/frontend-config/get", UserSelfFrontendConfigGet)
		r.POST("/api/v1/user/self/frontend-config/sync", UserSelfFrontendConfigSync)
		r.POST("/api/v1/user/self/frontend-config/sync-enabled/set", UserSelfFrontendConfigSyncEnabledSet)

		Convey("未同步过时 get 应返回默认空状态", func() {
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["syncEnabled"], ShouldEqual, false)
			// PostgreSQL 的 CHAR(64)（bpchar）会把空哈希补成 64 个空格，按去空白后为空断言
			So(strings.TrimSpace(data["hash"].(string)), ShouldEqual, "")
			So(data["version"], ShouldEqual, float64(0))
			So(data["config"], ShouldEqual, "")
		})

		Convey("同步开关未开启时 sync 应返回 400", func() {
			body, _ := json.Marshal(map[string]any{
				"prevHash":    "",
				"prevVersion": 0,
				"config":      `{"theme":"light"}`,
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, float64(resp.CodeFrontendConfigSyncDisabled))
		})

		Convey("开启同步开关后首次 sync 应成功并返回新哈希与版本", func() {
			body, _ := json.Marshal(map[string]any{"enabled": true})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync-enabled/set", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)

			config := `{"theme":"light","navPinned":true}`
			body, _ = json.Marshal(map[string]any{
				"prevHash":    "",
				"prevVersion": 0,
				"config":      config,
			})
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			data, ok := response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["version"], ShouldEqual, float64(1))
			So(data["hash"], ShouldEqual, frontendConfigHash(config))
			So(data["config"], ShouldEqual, config)

			// 再次 get 应返回已保存的配置
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/get", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
			response = map[string]any{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			data, ok = response["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["syncEnabled"], ShouldEqual, true)
			So(data["version"], ShouldEqual, float64(1))
		})

		Convey("版本号不匹配时 sync 应返回 409 冲突", func() {
			body, _ := json.Marshal(map[string]any{"enabled": true})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync-enabled/set", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)

			// 首次同步成功，版本号变为 1
			body, _ = json.Marshal(map[string]any{
				"prevHash":    "",
				"prevVersion": 0,
				"config":      `{"theme":"light"}`,
			})
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)

			// 携带过期版本号 0 再次同步应冲突
			body, _ = json.Marshal(map[string]any{
				"prevHash":    frontendConfigHash(`{"theme":"light"}`),
				"prevVersion": 0,
				"config":      `{"theme":"dark"}`,
			})
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusConflict)
			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response["code"], ShouldEqual, float64(resp.CodeFrontendConfigConflict))
		})

		Convey("哈希不匹配时 sync 应返回 409 冲突", func() {
			body, _ := json.Marshal(map[string]any{"enabled": true})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync-enabled/set", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)

			// 首次同步成功
			body, _ = json.Marshal(map[string]any{
				"prevHash":    "",
				"prevVersion": 0,
				"config":      `{"theme":"light"}`,
			})
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)

			// 携带错误哈希但正确版本号 1 再次同步应冲突
			body, _ = json.Marshal(map[string]any{
				"prevHash":    "wronghash",
				"prevVersion": 1,
				"config":      `{"theme":"dark"}`,
			})
			w = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusConflict)
		})

		Convey("请求体缺少 config 时 sync 应返回 400", func() {
			body, _ := json.Marshal(map[string]any{
				"prevHash":    "",
				"prevVersion": 0,
			})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/self/frontend-config/sync", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

// setupPasswordResetRouter 创建一个用于密码重置接口（forgot/reset）的测试引擎，
// 注入核心数据库、日志、功能开关注册表（由 [smtp] 配置派生）、项目级配置与假 pipeline 投递客户端。
func setupPasswordResetRouter(db *database.DB, enq *fakePipelineEnqueuer, mailCfg *mail.Config) *gin.Engine {
	r := setupUserSelfTestRouterWithDB(db)
	r.Use(injectLoggerMiddleware(createTestLoggingLogger()))
	r.Use(middlewares.PipelineClientMiddleware(newFakePipelineClient(enq)))
	r.Use(middlewares.FeatureRegistryMiddleware(features.NewRegistry(features.PasswordReset(mail.NewSender(mailCfg, createTestLoggingLogger())))))
	r.Use(func(c *gin.Context) {
		c.Set(manager.ConfigCtxName, &manager.Config{
			FrontendBaseURL: "http://127.0.0.1:8000",
			PasswordReset:   manager.PasswordResetConfig{TokenTTL: 30 * time.Minute},
		})
		c.Next()
	})
	return r
}

func TestUserPasswordForgot(t *testing.T) {
	Convey("测试 UserPasswordForgot 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "forgotuser", "忘记密码用户", "forgot@example.com", "OldP@ssw0rd", true)
		enq := &fakePipelineEnqueuer{}
		enabledCfg := &mail.Config{Enabled: true, Host: "smtp.example.com", From: "no-reply@example.com"}
		r := setupPasswordResetRouter(db, enq, enabledCfg)
		r.POST("/api/v1/user/password/forgot", UserPasswordForgot)

		Convey("SMTP 未启用时返回 400 CodePasswordResetUnavailable", func() {
			disabledCfg := &mail.Config{Enabled: false}
			r2 := setupPasswordResetRouter(db, enq, disabledCfg)
			r2.POST("/api/v1/user/password/forgot", UserPasswordForgot)

			body, _ := json.Marshal(map[string]any{"username": user.Username, "email": user.Email})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/forgot", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r2.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var rsp map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			So(rsp["code"], ShouldEqual, float64(resp.CodePasswordResetUnavailable))
		})

		Convey("账号名邮箱一致时返回成功并创建重置记录且入队邮件任务", func() {
			body, _ := json.Marshal(map[string]any{"username": user.Username, "email": user.Email})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/forgot", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var count int64
			db.Model(&core.UserPasswordReset{}).Where("user_id = ?", user.ID).Count(&count)
			So(count, ShouldEqual, 1)

			// 应入队 mail:send 任务，携带 password.reset 模板与收件人
			So(enq.lastTask, ShouldNotBeNil)
			var input mailhandler.MailSendInput
			So(json.Unmarshal(enq.lastTask.Payload(), &input), ShouldBeNil)
			So(input.To, ShouldEqual, user.Email)
			So(input.Message.Template, ShouldEqual, "password.reset")
			So(input.Message.Params, ShouldContainKey, "ResetURL")
		})

		Convey("账号名邮箱不一致时统一返回成功且不创建重置记录（避免账号枚举）", func() {
			body, _ := json.Marshal(map[string]any{"username": user.Username, "email": "wrong@example.com"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/forgot", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var count int64
			db.Model(&core.UserPasswordReset{}).Where("user_id = ?", user.ID).Count(&count)
			So(count, ShouldEqual, 0)
		})

		Convey("请求体缺少必需字段时返回 400", func() {
			body, _ := json.Marshal(map[string]any{"username": user.Username})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/forgot", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestUserPasswordResetByToken(t *testing.T) {
	Convey("测试 UserPasswordResetByToken 函数", t, func() {
		db := createUserSelfTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		user := createTestUserInDB(db, "resettokenuser", "令牌重置用户", "resettoken@example.com", "OldP@ssw0rd", true)
		r := setupPasswordResetRouter(db, &fakePipelineEnqueuer{}, &mail.Config{Enabled: true, Host: "smtp.example.com", From: "no-reply@example.com"})
		r.POST("/api/v1/user/password/reset", UserPasswordResetByToken)

		// 生成有效令牌并入库；仅哈希入库
		createReset := func(token string, expiredAt time.Time, used bool) {
			sum := sha256.Sum256([]byte(token))
			reset := core.UserPasswordReset{
				UserID:    user.ID,
				Email:     user.Email,
				TokenHash: hex.EncodeToString(sum[:]),
				ExpiredAt: expiredAt,
				Used:      used,
			}
			So(db.Create(&reset).Error, ShouldBeNil)
		}

		Convey("有效令牌应成功重置密码且令牌标记为已使用", func() {
			token := "abcdef1234567890-valid-token"
			createReset(token, time.Now().Add(time.Hour), false)

			body, _ := json.Marshal(map[string]any{"token": token, "newPassword": "NewP@ssw0rd"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var updatedUser core.User
			db.First(&updatedUser, "union_id = ?", user.UnionID)
			So(crypto.CheckPassword(updatedUser.Password.Value(), "NewP@ssw0rd"), ShouldBeTrue)

			var reset core.UserPasswordReset
			db.First(&reset, "token_hash = ?", userPasswordResetTokenHash(token))
			So(reset.Used, ShouldBeTrue)
		})

		Convey("令牌不存在时返回 400 CodePasswordResetTokenInvalid", func() {
			body, _ := json.Marshal(map[string]any{"token": "totally-missing-token-1234567890", "newPassword": "NewP@ssw0rd"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var rsp map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			So(rsp["code"], ShouldEqual, float64(resp.CodePasswordResetTokenInvalid))
		})

		Convey("令牌已过期时返回 400 CodePasswordResetTokenInvalid", func() {
			token := "expired-token-1234567890-abcd"
			createReset(token, time.Now().Add(-time.Minute), false)

			body, _ := json.Marshal(map[string]any{"token": token, "newPassword": "NewP@ssw0rd"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var rsp map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			So(rsp["code"], ShouldEqual, float64(resp.CodePasswordResetTokenInvalid))
		})

		Convey("令牌已使用时不返回重复启用密码（单次有效）", func() {
			token := "used-token-1234567890-efgh"
			createReset(token, time.Now().Add(time.Hour), true)

			body, _ := json.Marshal(map[string]any{"token": token, "newPassword": "NewP@ssw0rd"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/password/reset", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var rsp map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			So(rsp["code"], ShouldEqual, float64(resp.CodePasswordResetTokenInvalid))
		})
	})
}

// userPasswordResetTokenHash 计算令牌的 SHA-256 十六进制摘要，与视图层保持一致。
func userPasswordResetTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
