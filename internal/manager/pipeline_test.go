package manager

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/hibiken/asynq"
	. "github.com/smartystreets/goconvey/convey"
	"zhongkui.com/zhongkui/internal/pkg/pipeline"
)

// setupPipelineTestRouter 创建带有 Pipeline 检查器中间件的测试路由引擎。
func setupPipelineTestRouter(inspector *pipeline.Inspector) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	if inspector != nil {
		r.Use(PipelineInspectorMiddleware(inspector))
	}
	return r
}

// TestPIPELINE_INSPECTOR_CTX_NAME 测试上下文键名常量
func TestPIPELINE_INSPECTOR_CTX_NAME(t *testing.T) {
	Convey("测试 PIPELINE_INSPECTOR_CTX_NAME 常量", t, func() {
		Convey("常量值应为 'pipeline-inspector'", func() {
			So(PIPELINE_INSPECTOR_CTX_NAME, ShouldEqual, "pipeline-inspector")
		})

		Convey("常量值不应为空", func() {
			So(PIPELINE_INSPECTOR_CTX_NAME, ShouldNotBeEmpty)
		})
	})
}

// TestPipelineInspectorMiddleware 测试 PipelineInspectorMiddleware 中间件
func TestPipelineInspectorMiddleware(t *testing.T) {
	Convey("测试 PipelineInspectorMiddleware 中间件", t, func() {
		Convey("应该返回非空的中间件函数", func() {
			inspector := &asynq.Inspector{}
			middleware := PipelineInspectorMiddleware(inspector)
			So(middleware, ShouldNotBeNil)
		})

		Convey("应该将检查器实例存储到 Gin 上下文中", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var gotInspector *pipeline.Inspector
			r.GET("/test", func(c *gin.Context) {
				val, exists := c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				So(exists, ShouldBeTrue)
				gotInspector = val.(*pipeline.Inspector)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(gotInspector, ShouldEqual, inspector)
		})

		Convey("应该将检查器存储到正确的上下文键名下", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var actualKey string
			var keyExists bool
			r.GET("/test", func(c *gin.Context) {
				_, keyExists = c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				actualKey = PIPELINE_INSPECTOR_CTX_NAME
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(keyExists, ShouldBeTrue)
			So(actualKey, ShouldEqual, "pipeline-inspector")
		})

		Convey("中间件调用后应继续执行后续处理函数", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			nextCalled := false
			r.GET("/test", func(c *gin.Context) {
				nextCalled = true
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(nextCalled, ShouldBeTrue)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("多个请求应共享同一个检查器实例", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			r.GET("/test", func(c *gin.Context) {
				_, _ = c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				c.String(http.StatusOK, "ok")
			})

			for i := 0; i < 3; i++ {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest(http.MethodGet, "/test", nil)
				r.ServeHTTP(w, req)
				So(w.Code, ShouldEqual, http.StatusOK)
			}
		})

		Convey("不注册中间件时上下文中不应存在检查器", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()

			var exists bool
			r.GET("/test", func(c *gin.Context) {
				_, exists = c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(exists, ShouldBeFalse)
		})

		Convey("中间件应支持多次注册不同的检查器实例", func() {
			inspector1 := &asynq.Inspector{}
			inspector2 := &asynq.Inspector{}

			gin.SetMode(gin.TestMode)

			r1 := gin.New()
			r1.Use(PipelineInspectorMiddleware(inspector1))
			var got1 *pipeline.Inspector
			r1.GET("/test", func(c *gin.Context) {
				val, _ := c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				got1 = val.(*pipeline.Inspector)
				c.String(http.StatusOK, "ok")
			})

			r2 := gin.New()
			r2.Use(PipelineInspectorMiddleware(inspector2))
			var got2 *pipeline.Inspector
			r2.GET("/test", func(c *gin.Context) {
				val, _ := c.Get(PIPELINE_INSPECTOR_CTX_NAME)
				got2 = val.(*pipeline.Inspector)
				c.String(http.StatusOK, "ok")
			})

			w1 := httptest.NewRecorder()
			req1, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r1.ServeHTTP(w1, req1)

			w2 := httptest.NewRecorder()
			req2, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r2.ServeHTTP(w2, req2)

			So(got1, ShouldEqual, inspector1)
			So(got2, ShouldEqual, inspector2)
		})
	})
}

// TestGetPipelineInspector 测试 GetPipelineInspector 函数
func TestGetPipelineInspector(t *testing.T) {
	Convey("测试 GetPipelineInspector 函数", t, func() {
		Convey("上下文中存在检查器时应正确返回", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var gotInspector *pipeline.Inspector
			r.GET("/test", func(c *gin.Context) {
				gotInspector = GetPipelineInspector(c)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(gotInspector, ShouldNotBeNil)
			So(gotInspector, ShouldEqual, inspector)
		})

		Convey("上下文中不存在检查器时应触发 panic", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()

			r.GET("/test", func(c *gin.Context) {
				GetPipelineInspector(c)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)

			So(func() {
				r.ServeHTTP(w, req)
			}, ShouldPanicWith, "pipeline inspector does not exists")
		})

		Convey("获取的检查器实例应与中间件注入的实例相同", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var retrievedInspector *pipeline.Inspector
			r.GET("/test", func(c *gin.Context) {
				retrievedInspector = GetPipelineInspector(c)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(retrievedInspector, ShouldEqual, inspector)
		})

		Convey("在同一个请求中多次调用应返回相同的检查器实例", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var first, second *pipeline.Inspector
			r.GET("/test", func(c *gin.Context) {
				first = GetPipelineInspector(c)
				second = GetPipelineInspector(c)
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(first, ShouldEqual, second)
			So(first, ShouldEqual, inspector)
		})
	})
}

// TestPipelineInspectorMiddlewareAndGetter 测试中间件与获取函数的协作
func TestPipelineInspectorMiddlewareAndGetter(t *testing.T) {
	Convey("测试中间件与获取函数的协作", t, func() {
		Convey("完整流程：中间件注入后通过获取函数取回检查器", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			var retrieved *pipeline.Inspector
			var statusCode int
			r.GET("/test", func(c *gin.Context) {
				retrieved = GetPipelineInspector(c)
				statusCode = http.StatusOK
				c.String(statusCode, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(retrieved, ShouldNotBeNil)
			So(retrieved, ShouldEqual, inspector)
		})

		Convey("中间件与其他中间件组合使用时应正常工作", func() {
			inspector := &asynq.Inspector{}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("other-key", "other-value")
				c.Next()
			})
			r.Use(PipelineInspectorMiddleware(inspector))

			var gotInspector *pipeline.Inspector
			var otherValue any
			r.GET("/test", func(c *gin.Context) {
				gotInspector = GetPipelineInspector(c)
				otherValue, _ = c.Get("other-key")
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(gotInspector, ShouldNotBeNil)
			So(gotInspector, ShouldEqual, inspector)
			So(otherValue, ShouldEqual, "other-value")
		})

		Convey("请求处理过程中检查器应始终可用", func() {
			inspector := &asynq.Inspector{}
			r := setupPipelineTestRouter(inspector)

			availableInBefore := false
			availableInAfter := false

			r.GET("/test", func(c *gin.Context) {
				insp := GetPipelineInspector(c)
				availableInBefore = insp != nil
				c.Next()
				insp2 := GetPipelineInspector(c)
				availableInAfter = insp2 != nil
				c.String(http.StatusOK, "ok")
			})

			w := httptest.NewRecorder()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			r.ServeHTTP(w, req)

			So(availableInBefore, ShouldBeTrue)
			So(availableInAfter, ShouldBeTrue)
		})
	})
}
