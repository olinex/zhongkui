// Package views 提供 HTTP 接口的请求输入（In）与响应输出（Out）视图结构体及处理器。
// 本文件包含全局功能开关列表接口（FeaturesListGet）的集成测试。
package views

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"zhongkuisec.com/zhongkui/internal/manager/features"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/mail"
	"zhongkuisec.com/zhongkui/internal/pkg/message"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
)

// createUserSelfTestDB 创建用于测试的核心数据库
// （原定义于 user 子包测试，本测试作为基座包测试需自包含）
func createUserSelfTestDB(t *testing.T) *database.DB {
	return testdb.Core(t)
}

// createTestLoggingLogger 创建测试专用的日志器
func createTestLoggingLogger() *logging.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	return &logging.Logger{SugaredLogger: zapLogger.Sugar()}
}

// setupUserSelfTestRouterWithDB 创建注入核心数据库的测试路由
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

func TestFeaturesListGet(t *testing.T) {
	Convey("测试 FeaturesListGet 函数", t, func() {
		db := createUserSelfTestDB(t)
		route := func(cfg *mail.Config) *gin.Engine {
			r := setupUserSelfTestRouterWithDB(db)
			r.Use(middlewares.FeatureRegistryMiddleware(features.NewRegistry(features.PasswordReset(mail.NewSender(cfg, createTestLoggingLogger(), message.NewPool())))))
			r.POST("/api/v1/features/list", FeaturesListGet)
			return r
		}

		Convey("SMTP 配置完整时返回 passwordReset=true", func() {
			r := route(&mail.Config{Enabled: true, Host: "smtp.example.com", From: "no-reply@example.com"})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/features/list", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
			var rsp struct {
				Code int `json:"code"`
				Data []struct {
					Key     string `json:"key"`
					Enabled bool   `json:"enabled"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			So(rsp.Code, ShouldEqual, http.StatusOK)
			var passwordReset *bool
			for i := range rsp.Data {
				if rsp.Data[i].Key == features.PasswordResetKey {
					passwordReset = &rsp.Data[i].Enabled
				}
			}
			So(passwordReset, ShouldNotBeNil)
			So(*passwordReset, ShouldBeTrue)
		})

		Convey("SMTP 未启用时返回 passwordReset=false", func() {
			r := route(&mail.Config{Enabled: false})
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/features/list", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
			var rsp struct {
				Code int `json:"code"`
				Data []struct {
					Key     string `json:"key"`
					Enabled bool   `json:"enabled"`
				} `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &rsp), ShouldBeNil)
			var found bool
			for i := range rsp.Data {
				if rsp.Data[i].Key == features.PasswordResetKey {
					found = true
					So(rsp.Data[i].Enabled, ShouldBeFalse)
				}
			}
			So(found, ShouldBeTrue)
		})
	})
}
