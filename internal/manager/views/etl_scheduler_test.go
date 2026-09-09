// Package views 提供 ETL 管道监控和管理相关的 API 路由处理函数。
// 此文件为定时任务管理接口的互斥校验逻辑 validateSchedulerSchedule 提供单元测试。
package views

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

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
