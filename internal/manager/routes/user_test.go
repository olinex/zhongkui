package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/crypto"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

func createUserSelfTestDB() *database.DB {
	cfg := &database.Config{
		Driver: "sqlite",
		DSN:    ":memory:",
	}
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

	db, err := database.NewDatabase(cfg, logger)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	if err := db.AutoMigrate(
		&core.User{},
		&core.Group{},
		&core.Namespace{},
		&core.RbacPolicy{},
		&core.AccessKey{},
	); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	return db
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
		Password:   hashedPassword,
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
	return manager.ZapLoggerMiddleware(logger)
}

func setupUserSelfTestRouterWithDB(db *database.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(func(c *gin.Context) {
		c.Set(manager.DATABASE_CTX_NAME, db)
		c.Next()
	})
	return r
}

func setupUserSelfTestRouterWithDBAndUser(db *database.DB, user *core.User) *gin.Engine {
	r := setupUserSelfTestRouterWithDB(db)
	r.Use(func(c *gin.Context) {
		c.Set(manager.USER_CTX_NAME, user)
		c.Next()
	})
	return r
}

func setupUserSelfTestRouterWithFullDeps(db *database.DB, user *core.User) *gin.Engine {
	r := setupUserSelfTestRouterWithDBAndUser(db, user)
	authConfig := createUserSelfTestAuthConfig()
	kd := createTestKeyDerivation()
	authenticator := manager.NewJwtAuthenticator(authConfig, db, kd)
	loggingLogger := createTestLoggingLogger()
	r.Use(func(c *gin.Context) {
		c.Set(manager.AUTH_CTX_NAME, authenticator)
		c.Set(manager.CONFIG_CTX_NAME, &manager.Config{
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
				Model: base.RBAC_MODEL,
				Policies: [][]string{
					{"p", "admin", "user", "read", "allow"},
					{"g", "user-union-id", "admin", "default", ""},
				},
				Namespaces: []string{"default", "production"},
			}
			So(out.Model, ShouldEqual, base.RBAC_MODEL)
			So(len(out.Policies), ShouldEqual, 2)
			So(out.Policies[0][0], ShouldEqual, "p")
			So(out.Namespaces, ShouldContain, "default")
			So(out.Namespaces, ShouldContain, "production")
		})

		Convey("应处理空策略列表", func() {
			out := AuthorizationInfoOut{
				Model:      base.RBAC_MODEL,
				Policies:   [][]string{},
				Namespaces: []string{},
			}
			So(len(out.Policies), ShouldEqual, 0)
			So(len(out.Namespaces), ShouldEqual, 0)
		})

		Convey("应正确序列化为 JSON", func() {
			out := AuthorizationInfoOut{
				Model:      base.RBAC_MODEL,
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

func TestUserRegister(t *testing.T) {
	Convey("测试 userRegister 函数", t, func() {
		db := createUserSelfTestDB()
		r := setupUserSelfTestRouterWithDB(db)
		r.POST("/api/v1/user/register", userRegister)

		Convey("应成功注册新用户", func() {
			reqBody := map[string]any{
				"nickleName": "注册用户名昵称",
				"email":      "register@example.com",
				"username":   "reguserone",
				"password":   "Str0ngP@ssw0rd",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var createdUser core.User
			err := db.First(&createdUser, "username = ?", "reguserone").Error
			So(err, ShouldBeNil)
			So(createdUser.NickleName, ShouldEqual, "注册用户名昵称")
			So(createdUser.Email, ShouldEqual, "register@example.com")
			So(createdUser.Active, ShouldBeTrue)

			var policy core.RbacPolicy
			err = db.First(&policy, "ptype = ? AND v0 = ?", base.GROUP_LABEL, createdUser.UnionID.String()).Error
			So(err, ShouldBeNil)
			So(policy.V1, ShouldEqual, base.DEFAULT_GROUP)
		})

		Convey("应返回错误当请求体缺少必需字段时", func() {
			reqBody := map[string]any{
				"nickleName": "昵称",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("应返回错误当用户名或邮箱已存在时", func() {
			createTestUserInDB(db, "existuser", "存在用户", "exist@example.com", "Str0ngP@ss", true)

			reqBody := map[string]any{
				"nickleName": "重复的用户名昵称",
				"email":      "exist@example.com",
				"username":   "existuser",
				"password":   "Str0ngP@ssw0rd",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)

			var response map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &response)
			So(err, ShouldBeNil)
			So(response, ShouldContainKey, "code")
		})

		Convey("应返回错误当密码格式不满足强密码要求时", func() {
			reqBody := map[string]any{
				"nickleName": "弱密码用户昵称",
				"email":      "weak@example.com",
				"username":   "weakusername",
				"password":   "weak",
			}
			body, _ := json.Marshal(reqBody)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/user/register", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestUserPasswordLogin(t *testing.T) {
	Convey("测试 userPasswordLogin 函数", t, func() {
		db := createUserSelfTestDB()
		authConfig := createUserSelfTestAuthConfig()
		kd := createTestKeyDerivation()
		authenticator := manager.NewJwtAuthenticator(authConfig, db, kd)
		loggingLogger := createTestLoggingLogger()
		r := setupUserSelfTestRouterWithDB(db)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.Use(func(c *gin.Context) {
			c.Set(manager.AUTH_CTX_NAME, authenticator)
			c.Next()
		})
		r.POST("/api/v1/user/password/login", userPasswordLogin)

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
	Convey("测试 userSelfPasswordReset 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "resetuser", "重置用户", "reset@example.com", "OldP@ssw0rd", true)
		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/password/reset", userSelfPasswordReset)

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
			So(crypto.CheckPassword(updatedUser.Password, "NewP@ssw0rd"), ShouldBeTrue)
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
	Convey("测试 userSelfProfileChange 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "profileuser", "原始昵称", "original@example.com", "Str0ngP@ss", true)
		r := setupUserSelfTestRouterWithFullDeps(db, user)
		r.POST("/api/v1/user/self/profile/change", userSelfProfileChange)

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
	Convey("测试 userSelfJwtRefresh 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "refreshuser", "刷新用户", "refresh@example.com", "Str0ngP@ss", true)
		authConfig := createUserSelfTestAuthConfig()
		kd := createTestKeyDerivation()
		authenticator := manager.NewJwtAuthenticator(authConfig, db, kd)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(func(c *gin.Context) {
			c.Set(manager.AUTH_CTX_NAME, authenticator)
			c.Next()
		})
		r.POST("/api/v1/user/self/jwt/refresh", userSelfJwtRefresh)

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
	})
}

func TestUserSelfAuthorizationInfoGet(t *testing.T) {
	Convey("测试 userSelfAuthorizationInfoGet 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "authuser", "授权用户", "auth@example.com", "Str0ngP@ss", true)
		loggingLogger := createTestLoggingLogger()

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.POST("/api/v1/user/self/authorization-info/get", userSelfAuthorizationInfoGet)

		Convey("应成功获取授权信息", func() {
			group := &core.Group{
				Name:        "testgroup",
				Description: "测试用户组",
			}
			db.Create(group)

			policy := core.NewUserPolicy(user.UnionID, "testgroup", "default")
			db.Create(policy)

			rbacPolicy := &core.RbacPolicy{
				Ptype: base.POLICY_LABEL,
				V0:    "testgroup",
				V1:    "user",
				V2:    "read",
				V3:    base.EFFECT_ALLOW,
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
	Convey("测试 userSelfAccessKeyListGet 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "aklistuser", "密钥用户", "aklist@example.com", "Str0ngP@ss", true)
		loggingLogger := createTestLoggingLogger()

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.Use(injectLoggerMiddleware(loggingLogger))
		r.POST("/api/v1/user/self/access-key/list", userSelfAccessKeyListGet)

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
	Convey("测试 userSelfAccessKeyCreate 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "akcreateuser", "创建密钥用户", "akcreate@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithFullDeps(db, user)
		r.POST("/api/v1/user/self/access-key/create", userSelfAccessKeyCreate)

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
	Convey("测试 userSelfAccessKeyRemove 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "akrmuser", "删除密钥用户", "akrm@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/access-key/:id/remove", userSelfAccessKeyRemove)

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
	Convey("测试 userSelfAccessKeyEnabled 函数", t, func() {
		db := createUserSelfTestDB()
		user := createTestUserInDB(db, "akenableuser", "启用密钥用户", "akenable@example.com", "Str0ngP@ss", true)

		r := setupUserSelfTestRouterWithDBAndUser(db, user)
		r.POST("/api/v1/user/self/access-key/:id/activate", userSelfAccessKeyEnabled(true))
		r.POST("/api/v1/user/self/access-key/:id/deactivate", userSelfAccessKeyEnabled(false))

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
	})
}
