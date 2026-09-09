package middlewares

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// newTestAuthorizeEnforcer 基于内存 adapter 创建 Casbin enforcer（使用 base.RbacModel）。
// 与 provider/rbac.go 使用同一模型定义，仅以内存策略代替数据库 adapter，
// 便于在中间件单测中自由注入/清空策略。
func newTestAuthorizeEnforcer() *casbin.Enforcer {
	mdl, err := model.NewModelFromString(base.RbacModel)
	if err != nil {
		panic("failed to create rbac model: " + err.Error())
	}
	enforcer, err := casbin.NewEnforcer(mdl)
	if err != nil {
		panic("failed to create rbac enforcer: " + err.Error())
	}
	return enforcer
}

// newTestUser 构造注入上下文所需的测试用户（随机 UnionID）
func newTestUser() *core.User {
	return &core.User{UnionID: serializer.NewHexUUIDv4()}
}

// setupAuthorizeTestRouter 构造用于授权中间件测试的 Gin 路由。
// 中间件链：注入 policy / namespace / user → RbacPolicyAuthorizeMiddleware → 处理函数；
// 同时注入 Localizer / Translator / Logger，支撑 AbortWithGinH 与授权失败日志。
func setupAuthorizeTestRouter(enforcer *casbin.Enforcer, user *core.User, namespace string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newTestLocalizer())
		c.Set(response.TranslatorCtxName, newTestTranslator())
		if logger, err := logging.NewDebugLogger(); err == nil {
			c.Set(LoggerCtxName, &ApiLogger{logger: logger})
		}
		c.Next()
	})
	router.Use(RbacPolicyMiddleware(enforcer))
	router.Use(func(c *gin.Context) {
		c.Set(NamespaceCtxName, namespace)
		c.Next()
	})
	router.Use(func(c *gin.Context) {
		c.Set(UserCtxName, user)
		c.Next()
	})
	router.Use(RbacPolicyAuthorizeMiddleware())
	router.POST("/test/get", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return router
}

// TestAuthorize 测试 Authorize 动态授权函数
func TestAuthorize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 Authorize 动态授权函数", t, func() {
		Convey("当请求缺失命名空间时应返回命名空间错误", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(RbacPolicyCtxName, enforcer)
			c.Set(UserCtxName, user)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test/get", nil)
			_, span := StartMiddlewareSpan(c, "test-authorize")
			defer span.End()

			err := Authorize(c, span)
			So(err, ShouldEqual, ErrNampspaceDoesNotExist)
		})

		Convey("当请求路径为空时应返回空白路径错误", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(RbacPolicyCtxName, enforcer)
			c.Set(NamespaceCtxName, base.DefaultNamespace)
			c.Set(UserCtxName, user)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test/get", nil)
			_, span := StartMiddlewareSpan(c, "test-authorize")
			defer span.End()

			err := Authorize(c, span)
			So(err, ShouldEqual, ErrAuthorizeBlankFullPath)
		})

		Convey("当请求缺失用户时应返回用户错误", func() {
			enforcer := newTestAuthorizeEnforcer()
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(RbacPolicyCtxName, enforcer)
			c.Set(NamespaceCtxName, base.DefaultNamespace)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/test/get", nil)
			_, span := StartMiddlewareSpan(c, "test-authorize")
			defer span.End()

			err := Authorize(c, span)
			So(err, ShouldNotBeNil)
			So(err, ShouldEqual, ErrUserNotExist)
		})
	})
}

// TestRbacPolicyAuthorizeMiddleware 测试 RbacPolicyAuthorizeMiddleware 中间件
func TestRbacPolicyAuthorizeMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 RbacPolicyAuthorizeMiddleware 中间件", t, func() {
		Convey("拥有权限点的用户应通过授权并到达处理函数", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			userID := user.UnionID.String()
			// default 组授予 /test/get POST 权限，用户经分组规则绑定到 default 命名空间
			_, err := enforcer.AddPolicy(base.DefaultGroup, "api:/test/get", http.MethodPost, base.EffectAllow)
			So(err, ShouldBeNil)
			_, err = enforcer.AddGroupingPolicy(userID, base.DefaultGroup, base.DefaultNamespace)
			So(err, ShouldBeNil)

			router := setupAuthorizeTestRouter(enforcer, user, base.DefaultNamespace)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/test/get", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(w.Body.String(), ShouldEqual, "ok")
		})

		Convey("无权限点的用户应被拒绝并返回 403", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			// 未添加任何策略与分组规则

			router := setupAuthorizeTestRouter(enforcer, user, base.DefaultNamespace)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/test/get", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusForbidden)

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(response.CodeAuthorizeFailed))
		})

		Convey("HTTP 方法不匹配时应被拒绝", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			userID := user.UnionID.String()
			// 仅授予 POST，请求 GET 应失败
			_, err := enforcer.AddPolicy(base.DefaultGroup, "api:/test/get", http.MethodPost, base.EffectAllow)
			So(err, ShouldBeNil)
			_, err = enforcer.AddGroupingPolicy(userID, base.DefaultGroup, base.DefaultNamespace)
			So(err, ShouldBeNil)

			router := setupAuthorizeTestRouter(enforcer, user, base.DefaultNamespace)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test/get", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusForbidden)
		})

		Convey("其他命名空间的分组不应放行默认命名空间请求", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			userID := user.UnionID.String()
			_, err := enforcer.AddPolicy(base.DefaultGroup, "api:/test/get", http.MethodPost, base.EffectAllow)
			So(err, ShouldBeNil)
			// 用户绑定到 tenant-a 命名空间，请求 default 命名空间应被拒绝
			_, err = enforcer.AddGroupingPolicy(userID, base.DefaultGroup, "tenant-a")
			So(err, ShouldBeNil)

			router := setupAuthorizeTestRouter(enforcer, user, base.DefaultNamespace)
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodPost, "/test/get", nil)
			router.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusForbidden)
		})
	})
}

// TestStaticAuthorize 测试 StaticAuthorize 静态授权函数
func TestStaticAuthorize(t *testing.T) {
	gin.SetMode(gin.TestMode)

	Convey("测试 StaticAuthorize 静态授权函数", t, func() {
		Convey("拥有静态权限点的用户应授权成功", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()
			userID := user.UnionID.String()
			_, err := enforcer.AddPolicy(base.DefaultGroup, "plaintext:crypto:http-transport:reqHdrCookie", base.ActionRead, base.EffectAllow)
			So(err, ShouldBeNil)
			_, err = enforcer.AddGroupingPolicy(userID, base.DefaultGroup, base.DefaultNamespace)
			So(err, ShouldBeNil)

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(RbacPolicyCtxName, enforcer)
			c.Set(UserCtxName, user)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/plaintext/crypto", nil)
			_, span := StartMiddlewareSpan(c, "test-static-authorize")
			defer span.End()

			err = StaticAuthorize(
				c, span, base.DefaultNamespace,
				"plaintext:crypto:http-transport:reqHdrCookie", base.ActionRead,
			)
			So(err, ShouldBeNil)
		})

		Convey("无静态权限点的用户应授权失败", func() {
			enforcer := newTestAuthorizeEnforcer()
			user := newTestUser()

			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set(RbacPolicyCtxName, enforcer)
			c.Set(UserCtxName, user)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/plaintext/crypto", nil)
			_, span := StartMiddlewareSpan(c, "test-static-authorize")
			defer span.End()

			err := StaticAuthorize(
				c, span, base.DefaultNamespace,
				"plaintext:crypto:http-transport:reqHdrCookie", base.ActionRead,
			)
			So(err, ShouldEqual, ErrAuthorizeFailed)
		})
	})
}
