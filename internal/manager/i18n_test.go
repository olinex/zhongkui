package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// newI18nTestLogger 创建一个基于 observer 的测试日志记录器
// 返回日志记录器和 observer 实例，后者可用于断言日志内容
func newI18nTestLogger() (*logging.Logger, *observer.ObservedLogs) {
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core).Sugar()
	return logger, recorded
}

// newI18nInstance 创建一个用于测试的 I18n 实例
// 使用内置的本地化文件（英语和简体中文）
func newI18nInstance(logger *logging.Logger) *translate.I18n {
	enLang, _ := translate.ParseLanguage("en")
	zhLang, _ := translate.ParseLanguage("zh-Hans")
	cfg := &translate.Config{
		DefaultLocale:  *enLang,
		SupportLocales: []translate.Language{*enLang, *zhLang},
		RootPath:       "locales",
	}
	return translate.InitI18n(cfg, logger)
}

// newI18nGinContext 创建一个用于测试的 gin.Context
// 可以通过 opts 设置请求头等参数
func newI18nGinContext(method, path string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c.Request = req
	return c
}

// TestLOCALIZER_CTX_NAME 测试常量 LOCALIZER_CTX_NAME 的值
func TestLOCALIZER_CTX_NAME(t *testing.T) {
	Convey("TestLOCALIZER_CTX_NAME", t, func() {
		Convey("LOCALIZER_CTX_NAME 应等于 i18n-localizer", func() {
			So(LOCALIZER_CTX_NAME, ShouldEqual, "i18n-localizer")
		})
	})
}

// TestTRANSLATOR_CTX_NAME 测试常量 TRANSLATOR_CTX_NAME 的值
func TestTRANSLATOR_CTX_NAME(t *testing.T) {
	Convey("TestTRANSLATOR_CTX_NAME", t, func() {
		Convey("TRANSLATOR_CTX_NAME 应等于 i18n-translator", func() {
			So(TRANSLATOR_CTX_NAME, ShouldEqual, "i18n-translator")
		})
	})
}

// TestGetLanguage 测试 GetLanguage 函数
func TestGetLanguage(t *testing.T) {
	Convey("TestGetLanguage", t, func() {
		Convey("当 Accept-Language 为英语时应返回英语", func() {
			c := newI18nGinContext(http.MethodGet, "/", map[string]string{
				base.HDR_ACCEPT_LANGUAGE: "en",
			})
			lang := GetLanguage(c)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "en")
		})

		Convey("当 Accept-Language 为简体中文时应返回简体中文", func() {
			c := newI18nGinContext(http.MethodGet, "/", map[string]string{
				base.HDR_ACCEPT_LANGUAGE: "zh-Hans",
			})
			lang := GetLanguage(c)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "zh-Hans")
		})

		Convey("当 Accept-Language 为带地区的语言时应正确解析", func() {
			c := newI18nGinContext(http.MethodGet, "/", map[string]string{
				base.HDR_ACCEPT_LANGUAGE: "en-US",
			})
			lang := GetLanguage(c)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, "en-US")
		})

		Convey("当 Accept-Language 为空时应返回默认语言（英语）", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			lang := GetLanguage(c)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, translate.DefaultLang.String())
		})

		Convey("当 Accept-Language 为无效值时应返回默认语言（英语）", func() {
			c := newI18nGinContext(http.MethodGet, "/", map[string]string{
				base.HDR_ACCEPT_LANGUAGE: "invalid-language-tag-!!!",
			})
			lang := GetLanguage(c)
			So(lang, ShouldNotBeNil)
			So(lang.String(), ShouldEqual, translate.DefaultLang.String())
		})
	})
}

// TestGetLocalizer 测试 GetLocalizer 函数
func TestGetLocalizer(t *testing.T) {
	Convey("TestGetLocalizer", t, func() {
		Convey("当上下文中存在 Localizer 时应正确返回", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			bundle := i18n.NewBundle(language.English)
			localizer := i18n.NewLocalizer(bundle, "en")
			c.Set(LOCALIZER_CTX_NAME, localizer)

			result := GetLocalizer(c)
			So(result, ShouldNotBeNil)
			So(result, ShouldHaveSameTypeAs, &i18n.Localizer{})
		})

		Convey("当上下文中不存在 Localizer 时应该 panic", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			c.Keys = make(map[string]any)

			So(func() { GetLocalizer(c) }, ShouldPanicWith, "i18n localizer does not exists")
		})

		Convey("当上下文中存储了错误类型时应该 panic", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			c.Keys = make(map[string]any)
			c.Keys[LOCALIZER_CTX_NAME] = "not a localizer"

			So(func() { GetLocalizer(c) }, ShouldPanic)
		})
	})
}

// TestGetTranslator 测试 GetTranslator 函数
func TestGetTranslator(t *testing.T) {
	Convey("TestGetTranslator", t, func() {
		Convey("当上下文中不存在 Translator 时应该 panic", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			c.Keys = make(map[string]any)

			So(func() { GetTranslator(c) }, ShouldPanicWith, "i18n translator does not exists")
		})

		Convey("当上下文中存储了错误类型时应该 panic", func() {
			c := newI18nGinContext(http.MethodGet, "/", nil)
			c.Keys = make(map[string]any)
			c.Keys[TRANSLATOR_CTX_NAME] = "not a translator"

			So(func() { GetTranslator(c) }, ShouldPanic)
		})
	})
}

// TestTranslateMiddleware 测试 TranslateMiddleware 中间件
func TestTranslateMiddleware(t *testing.T) {
	Convey("TestTranslateMiddleware", t, func() {
		logger, _ := newI18nTestLogger()
		i18nInstance := newI18nInstance(logger)
		middleware := TranslateMiddleware(i18nInstance)

		Convey("应在上下文中设置 Localizer", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				localizer, exists := c.Get(LOCALIZER_CTX_NAME)
				So(exists, ShouldBeTrue)
				So(localizer, ShouldHaveSameTypeAs, &i18n.Localizer{})
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "en")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("应在上下文中设置 Translator", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				translator, exists := c.Get(TRANSLATOR_CTX_NAME)
				So(exists, ShouldBeTrue)
				So(translator, ShouldNotBeNil)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "en")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("当 Accept-Language 为英语时应正确设置英文本地化组件", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				lang := GetLanguage(c)
				So(lang.String(), ShouldEqual, "en")
				localizer := GetLocalizer(c)
				So(localizer, ShouldNotBeNil)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "en")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("当 Accept-Language 为简体中文时应正确设置中文本地化组件", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				lang := GetLanguage(c)
				So(lang.String(), ShouldEqual, "zh-Hans")
				localizer := GetLocalizer(c)
				So(localizer, ShouldNotBeNil)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "zh-Hans")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("当 Accept-Language 为不支持的语言时请求不应中断且应正确设置本地化组件", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				lang := GetLanguage(c)
				So(lang.String(), ShouldEqual, "fr")
				localizer := GetLocalizer(c)
				So(localizer, ShouldNotBeNil)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "fr")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("当 Accept-Language 为空时应使用默认语言", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				lang := GetLanguage(c)
				So(lang.String(), ShouldEqual, translate.DefaultLang.String())
				localizer := GetLocalizer(c)
				So(localizer, ShouldNotBeNil)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("中间件应正确调用 c.Next() 使请求继续传递", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			nextCalled := false
			r.Use(ZapLoggerMiddleware(logger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				nextCalled = true
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(nextCalled, ShouldBeTrue)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

// TestTranslateMiddlewareWithLogger 测试 TranslateMiddleware 在获取 Translator 失败时的日志记录行为
func TestTranslateMiddlewareWithLogger(t *testing.T) {
	Convey("TestTranslateMiddlewareWithLogger", t, func() {
		Convey("当获取 Translator 失败时应记录警告日志", func() {
			observedLogger, recorded := newI18nTestLogger()
			i18nInstance := newI18nInstance(observedLogger)
			middleware := TranslateMiddleware(i18nInstance)

			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(ZapLoggerMiddleware(observedLogger))
			r.Use(middleware)
			r.GET("/test", func(c *gin.Context) {
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set(base.HDR_ACCEPT_LANGUAGE, "fr")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(recorded.Len(), ShouldBeGreaterThan, 0)

			hasWarning := false
			for _, entry := range recorded.All() {
				if entry.Level == zapcore.WarnLevel && entry.Message == "cannot get translator cause: Could not find fr in universal translator" {
					hasWarning = true
					break
				}
			}
			So(hasWarning, ShouldBeTrue)
		})
	})
}
