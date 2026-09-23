package etl

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// newETLTestLocalizer 返回测试专用的 go-i18n Localizer
// 内置本包测试所需的最小错误消息键，供调度器校验错误渲染使用
func newETLTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.200 = "ok"
code.1000 = "unknown error"
code.1112 = "scheduler task config invalid"
error.schedulerScheduleConflict = "cronSpec and interval are mutually exclusive, set only one"
error.schedulerScheduleEmpty = "either cronSpec or interval must be set"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newETLTestTranslator 返回测试专用的 universal-translator
func newETLTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回一个注入 Localizer 和 Translator 的 Gin 中间件
// 该辅助函数在本测试包内自包含，供校验定时任务调度模式错误的场景使用
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newETLTestLocalizer())
		c.Set(response.TranslatorCtxName, newETLTestTranslator())
		c.Next()
	}
}

// TestValidateSchedulerSchedule 测试定时任务调度模式互斥校验。
// 验证 cronSpec 与 interval 二选一：同时设置（冲突）或同时为空均返回 false 并写 400，
// 单模式配置返回 true。
func TestValidateSchedulerSchedule(t *testing.T) {
	Convey("测试 validateSchedulerSchedule 互斥校验", t, func() {
		Convey("同时设置 cronSpec 与 interval 应判定非法", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/x", func(c *gin.Context) {
				ok := validateSchedulerSchedule(c, "0 * * * *", serializer.DurationNS(time.Second))
				So(ok, ShouldBeFalse)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("cronSpec 与 interval 均为空应判定非法", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/x", func(c *gin.Context) {
				ok := validateSchedulerSchedule(c, "", 0)
				So(ok, ShouldBeFalse)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("仅设置 cronSpec 应判定合法", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			var valid bool
			r.GET("/x", func(c *gin.Context) {
				valid = validateSchedulerSchedule(c, "0 */5 * * * *", 0)
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
			So(valid, ShouldBeTrue)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("仅设置 interval 应判定合法", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			var valid bool
			r.GET("/x", func(c *gin.Context) {
				valid = validateSchedulerSchedule(c, "", serializer.DurationNS(30*time.Second))
			})
			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
			So(valid, ShouldBeTrue)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}
