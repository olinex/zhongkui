package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"
)

// newTestLocalizer 创建一个用于测试的 i18n 本地化器
// 注册了所有错误码和错误消息的翻译，确保 MustLocalizeMessage 不会 panic
func newTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.1000 = "unknown error"
code.1001 = "user not exist"
code.1002 = "bad body"
code.1003 = "bad query"
code.1004 = "bad url param"
code.1005 = "bad jwt token"
code.1006 = "timeout jwt token"
code.1007 = "authorize blank full path"
code.1008 = "authorize failed"
code.1009 = "namespace not exist"
code.1010 = "cannot delete cause relate record"
code.200 = "ok"
code.400 = "bad request"
code.401 = "unauthorized"
code.404 = "not found"
code.405 = "method not allowed"
code.500 = "internal server error"
code.1100 = "username or password check failed"
code.1101 = "old password check failed"
code.1102 = "username or email already exist"
code.1103 = "too many access key"
error.pageSize = "page size must be less than %d"
error.invalidID = "invalid id: %s"
error.requiredField = "%s is required"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newTestTranslator 创建一个用于测试的 ut.Translator
func newTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// TestErrorCodeConstants 测试基础错误码常量的值
func TestErrorCodeConstants(t *testing.T) {
	Convey("测试基础错误码常量", t, func() {
		Convey("基础错误码应从 1000 开始递增", func() {
			So(CodeUnknown, ShouldEqual, 1000)
			So(CodeUserNotExist, ShouldEqual, 1001)
			So(CodeBadBody, ShouldEqual, 1002)
			So(CodeBadQuery, ShouldEqual, 1003)
			So(CodeBadUrlParam, ShouldEqual, 1004)
			So(CodeBadJwtToken, ShouldEqual, 1005)
			So(CodeTimeoutJwtToken, ShouldEqual, 1006)
			So(CodeAuthorizeBlankFullPath, ShouldEqual, 1007)
			So(CodeAuthorizeFailed, ShouldEqual, 1008)
			So(CodeNamespaceNotExist, ShouldEqual, 1009)
			So(CodeCannotDeleteCauseRelateRecord, ShouldEqual, 1010)
		})
	})

	Convey("测试业务错误码常量", t, func() {
		Convey("业务错误码应从 1100 开始递增", func() {
			So(CodeUsernamePasswordCheckFailed, ShouldEqual, 1100)
			So(CodeOldPasswordCheckFailed, ShouldEqual, 1101)
			So(CodeUsernameOrEmailAlreadyExist, ShouldEqual, 1102)
			So(CodeTooManyAccessKey, ShouldEqual, 1103)
		})
	})
}

// TestPredefinedErrors 测试预定义的错误实例
func TestPredefinedErrors(t *testing.T) {
	Convey("测试预定义的错误实例", t, func() {
		Convey("SimpleOK 应该是成功响应", func() {
			So(SimpleOK, ShouldNotBeNil)
			localizer := newTestLocalizer()
			h := SimpleOK.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, http.StatusOK)
			So(h["summary"], ShouldEqual, "ok")
			So(h["details"], ShouldBeNil)
		})

		Convey("ErrDefaultPageNotFound 应该是 404 错误", func() {
			So(ErrDefaultPageNotFound, ShouldNotBeNil)
			localizer := newTestLocalizer()
			h := ErrDefaultPageNotFound.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, http.StatusNotFound)
			So(h["summary"], ShouldEqual, "not found")
		})

		Convey("ErrDefaultNoMethod 应该是 405 错误", func() {
			So(ErrDefaultNoMethod, ShouldNotBeNil)
			localizer := newTestLocalizer()
			h := ErrDefaultNoMethod.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, http.StatusMethodNotAllowed)
			So(h["summary"], ShouldEqual, "method not allowed")
		})

		Convey("ErrCannotDeleteCauseRelateRecord 应该是关联记录错误", func() {
			So(ErrCannotDeleteCauseRelateRecord, ShouldNotBeNil)
			localizer := newTestLocalizer()
			h := ErrCannotDeleteCauseRelateRecord.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, CodeCannotDeleteCauseRelateRecord)
			So(h["summary"], ShouldEqual, "cannot delete cause relate record")
		})
	})
}

// TestCreateCodeError 测试 CreateCodeError 工厂函数
func TestCreateCodeError(t *testing.T) {
	Convey("测试 CreateCodeError", t, func() {
		Convey("应正确创建无详情的错误", func() {
			err := CreateCodeError(http.StatusBadRequest, CodeBadQuery, nil)
			So(err, ShouldNotBeNil)
			So(err.statusCode, ShouldEqual, http.StatusBadRequest)
			So(err.code, ShouldEqual, CodeBadQuery)
			So(err.details, ShouldBeNil)
		})

		Convey("应正确创建带详情的错误", func() {
			detail := "some detail"
			err := CreateCodeError(http.StatusInternalServerError, CodeUnknown, detail)
			So(err, ShouldNotBeNil)
			So(err.statusCode, ShouldEqual, http.StatusInternalServerError)
			So(err.code, ShouldEqual, CodeUnknown)
			So(err.details, ShouldEqual, detail)
		})

		Convey("应支持不同的 HTTP 状态码", func() {
			err400 := CreateCodeError(http.StatusBadRequest, CodeBadBody, nil)
			So(err400.statusCode, ShouldEqual, http.StatusBadRequest)

			err401 := CreateCodeError(http.StatusUnauthorized, CodeBadJwtToken, nil)
			So(err401.statusCode, ShouldEqual, http.StatusUnauthorized)

			err500 := CreateCodeError(http.StatusInternalServerError, CodeUnknown, nil)
			So(err500.statusCode, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

// TestCreateBadBodyError 测试 CreateBadBodyError 工厂函数
func TestCreateBadBodyError(t *testing.T) {
	Convey("测试 CreateBadBodyError", t, func() {
		Convey("传入 validator.ValidationErrors 类型应正确创建错误", func() {
			type TestStruct struct {
				Name string `validate:"required"`
			}
			validate := validator.New()
			err := validate.Struct(TestStruct{})
			So(err, ShouldNotBeNil)

			vErr, ok := err.(validator.ValidationErrors)
			So(ok, ShouldBeTrue)

			codeErr := CreateBadBodyError(vErr)
			So(codeErr, ShouldNotBeNil)
			So(codeErr.statusCode, ShouldEqual, http.StatusBadRequest)
			So(codeErr.code, ShouldEqual, CodeBadBody)
			So(codeErr.details, ShouldEqual, vErr)
		})

		Convey("传入非 ValidationErrors 类型应返回 400 CodeBadBody 而非 panic", func() {
			plainErr := errors.New("not a validation error")
			codeErr := CreateBadBodyError(plainErr)
			So(codeErr, ShouldNotBeNil)
			So(codeErr.statusCode, ShouldEqual, http.StatusBadRequest)
			So(codeErr.code, ShouldEqual, CodeBadBody)
			So(codeErr.details, ShouldEqual, plainErr)
		})
	})
}

// TestCreateBadQueryError 测试 CreateBadQueryError 工厂函数
func TestCreateBadQueryError(t *testing.T) {
	Convey("测试 CreateBadQueryError", t, func() {
		Convey("应正确创建不带参数的查询参数错误", func() {
			err := CreateBadQueryError("invalidParam")
			So(err, ShouldNotBeNil)
			So(err.statusCode, ShouldEqual, http.StatusBadRequest)
			So(err.code, ShouldEqual, CodeBadQuery)
			So(err.details, ShouldNotBeNil)

			detail, ok := err.details.(*DetailContext)
			So(ok, ShouldBeTrue)
			So(detail.MsgID, ShouldEqual, "invalidParam")
			So(len(detail.args), ShouldEqual, 0)
		})

		Convey("应正确创建带参数的查询参数错误", func() {
			err := CreateBadQueryError("pageSize", 100)
			So(err, ShouldNotBeNil)

			detail, ok := err.details.(*DetailContext)
			So(ok, ShouldBeTrue)
			So(detail.MsgID, ShouldEqual, "pageSize")
			So(detail.args, ShouldResemble, []any{100})
		})

		Convey("应支持多个格式化参数", func() {
			err := CreateBadQueryError("rangeCheck", 1, 10)
			detail, ok := err.details.(*DetailContext)
			So(ok, ShouldBeTrue)
			So(detail.args, ShouldResemble, []any{1, 10})
		})
	})
}

// TestCreateBadUrlParamError 测试 CreateBadUrlParamError 工厂函数
func TestCreateBadUrlParamError(t *testing.T) {
	Convey("测试 CreateBadUrlParamError", t, func() {
		Convey("应正确创建不带参数的 URL 路径参数错误", func() {
			err := CreateBadUrlParamError("invalidID")
			So(err, ShouldNotBeNil)
			So(err.statusCode, ShouldEqual, http.StatusBadRequest)
			So(err.code, ShouldEqual, CodeBadUrlParam)

			detail, ok := err.details.(*DetailContext)
			So(ok, ShouldBeTrue)
			So(detail.MsgID, ShouldEqual, "invalidID")
			So(len(detail.args), ShouldEqual, 0)
		})

		Convey("应正确创建带参数的 URL 路径参数错误", func() {
			err := CreateBadUrlParamError("invalidID", "abc")
			So(err, ShouldNotBeNil)

			detail, ok := err.details.(*DetailContext)
			So(ok, ShouldBeTrue)
			So(detail.MsgID, ShouldEqual, "invalidID")
			So(detail.args, ShouldResemble, []any{"abc"})
		})
	})
}

// TestCreateAccidentError 测试 CreateAccidentError 工厂函数
func TestCreateAccidentError(t *testing.T) {
	Convey("测试 CreateAccidentError", t, func() {
		Convey("应正确创建意外内部错误", func() {
			originalErr := errors.New("database connection failed")
			err := CreateAccidentError(originalErr)
			So(err, ShouldNotBeNil)
			So(err.statusCode, ShouldEqual, http.StatusInternalServerError)
			So(err.code, ShouldEqual, CodeUnknown)
			So(err.details, ShouldEqual, originalErr)
		})

		Convey("详情应为原始错误对象", func() {
			originalErr := errors.New("file read error")
			err := CreateAccidentError(originalErr)
			So(err.details.(error).Error(), ShouldEqual, "file read error")
		})
	})
}

// TestDetailContextLocalize 测试 DetailContext 的 Localize 方法
func TestDetailContextLocalize(t *testing.T) {
	Convey("测试 DetailContext.Localize", t, func() {
		localizer := newTestLocalizer()

		Convey("应正确本地化带参数的消息模板", func() {
			d := &DetailContext{
				MsgID: "pageSize",
				args:  []any{100},
			}
			result := d.Localize(localizer)
			So(result, ShouldEqual, "page size must be less than 100")
		})

		Convey("应正确本地化带字符串参数的消息模板", func() {
			d := &DetailContext{
				MsgID: "invalidID",
				args:  []any{"abc"},
			}
			result := d.Localize(localizer)
			So(result, ShouldEqual, "invalid id: abc")
		})

		Convey("应正确本地化不带参数的消息模板", func() {
			d := &DetailContext{
				MsgID: "requiredField",
				args:  []any{"Name"},
			}
			result := d.Localize(localizer)
			So(result, ShouldEqual, "Name is required")
		})
	})
}

// TestSummaryLocalize 测试 summaryLocalize 函数
func TestSummaryLocalize(t *testing.T) {
	Convey("测试 summaryLocalize", t, func() {
		localizer := newTestLocalizer()

		Convey("应正确本地化错误码 1000", func() {
			result := summaryLocalize(localizer, CodeUnknown)
			So(result, ShouldEqual, "unknown error")
		})

		Convey("应正确本地化错误码 200", func() {
			result := summaryLocalize(localizer, 200)
			So(result, ShouldEqual, "ok")
		})

		Convey("应正确本地化业务错误码 1100", func() {
			result := summaryLocalize(localizer, CodeUsernamePasswordCheckFailed)
			So(result, ShouldEqual, "username or password check failed")
		})
	})
}

// TestCodeErrorSummary 测试 CodeError.Summary 方法
func TestCodeErrorSummary(t *testing.T) {
	Convey("测试 CodeError.Summary", t, func() {
		localizer := newTestLocalizer()

		Convey("应返回正确的本地化摘要", func() {
			err := CreateCodeError(http.StatusBadRequest, CodeBadQuery, nil)
			So(err.Summary(localizer), ShouldEqual, "bad query")
		})

		Convey("应正确处理未知错误码", func() {
			err := CreateCodeError(http.StatusInternalServerError, CodeUnknown, nil)
			So(err.Summary(localizer), ShouldEqual, "unknown error")
		})
	})
}

// TestCodeErrorToGinH 测试 CodeError.ToGinH 方法
func TestCodeErrorToGinH(t *testing.T) {
	Convey("测试 CodeError.ToGinH", t, func() {
		localizer := newTestLocalizer()

		Convey("无详情时应返回 code 和 summary", func() {
			err := CreateCodeError(http.StatusOK, http.StatusOK, nil)
			h := err.ToGinH(localizer, nil)
			So(h, ShouldContainKey, "code")
			So(h, ShouldContainKey, "summary")
			So(h, ShouldNotContainKey, "details")
			So(h["code"], ShouldEqual, http.StatusOK)
			So(h["summary"], ShouldEqual, "ok")
		})

		Convey("详情为 LocalizedError 时应返回本地化后的字符串详情", func() {
			err := CreateBadQueryError("pageSize", 100)
			h := err.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, CodeBadQuery)
			So(h["summary"], ShouldEqual, "bad query")
			So(h["details"], ShouldEqual, "page size must be less than 100")
		})

		Convey("详情为普通类型时应原样输出", func() {
			err := CreateCodeError(http.StatusInternalServerError, CodeUnknown, "raw error message")
			h := err.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, CodeUnknown)
			So(h["summary"], ShouldEqual, "unknown error")
			So(h["details"], ShouldEqual, "raw error message")
		})

		Convey("详情为 error 类型时应输出其消息字符串（避免序列化为空对象 {}）", func() {
			originalErr := errors.New("database connection failed")
			err := CreateAccidentError(originalErr)
			h := err.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, CodeUnknown)
			So(h["summary"], ShouldEqual, "unknown error")
			So(h["details"], ShouldEqual, "database connection failed")
		})

		Convey("详情为 map 类型时应原样输出", func() {
			detail := map[string]string{"field": "name", "reason": "too long"}
			err := CreateCodeError(http.StatusBadRequest, CodeBadBody, detail)
			h := err.ToGinH(localizer, nil)
			So(h["details"], ShouldResemble, map[string]string{"field": "name", "reason": "too long"})
		})
	})
}

// TestCodeErrorToGinHWithValidationErrors 测试 ToGinH 处理 ValidationErrors 详情
func TestCodeErrorToGinHWithValidationErrors(t *testing.T) {
	Convey("测试 ToGinH 处理 ValidationErrors", t, func() {
		localizer := newTestLocalizer()

		Convey("详情为 ValidationErrors 时应返回字段到翻译消息的映射", func() {
			type TestStruct struct {
				Name string `validate:"required"`
				Age  int    `validate:"min=18"`
			}
			validate := validator.New()
			err := validate.Struct(TestStruct{Age: 10})
			So(err, ShouldNotBeNil)

			vErr, ok := err.(validator.ValidationErrors)
			So(ok, ShouldBeTrue)

			codeErr := CreateBadBodyError(vErr)
			h := codeErr.ToGinH(localizer, nil)
			So(h["code"], ShouldEqual, CodeBadBody)
			So(h["summary"], ShouldEqual, "bad body")
			So(h["details"], ShouldNotBeNil)

			details, ok := h["details"].(map[string]string)
			So(ok, ShouldBeTrue)
			So(len(details), ShouldBeGreaterThan, 0)
		})
	})
}

// TestCodeErrorToGinW 测试 CodeError.ToGinW 方法
func TestCodeErrorToGinW(t *testing.T) {
	Convey("测试 CodeError.ToGinW", t, func() {
		localizer := newTestLocalizer()

		Convey("应在 ToGinH 基础上额外包含 data 字段", func() {
			err := CreateCodeError(http.StatusOK, http.StatusOK, nil)
			data := map[string]any{"id": 1, "name": "test"}
			h := err.ToGinW(localizer, nil, data)
			So(h, ShouldContainKey, "code")
			So(h, ShouldContainKey, "summary")
			So(h, ShouldContainKey, "data")
			So(h["code"], ShouldEqual, http.StatusOK)
			So(h["summary"], ShouldEqual, "ok")
			So(h["data"], ShouldResemble, data)
		})

		Convey("data 字段应支持任意类型", func() {
			err := CreateCodeError(http.StatusOK, http.StatusOK, nil)

			h1 := err.ToGinW(localizer, nil, "string data")
			So(h1["data"], ShouldEqual, "string data")

			h2 := err.ToGinW(localizer, nil, 42)
			So(h2["data"], ShouldEqual, 42)

			h3 := err.ToGinW(localizer, nil, []string{"a", "b"})
			So(h3["data"], ShouldResemble, []string{"a", "b"})
		})

		Convey("带详情的错误也应支持 ToGinW", func() {
			err := CreateBadQueryError("pageSize", 100)
			data := gin.H{"items": []string{}}
			h := err.ToGinW(localizer, nil, data)
			So(h["code"], ShouldEqual, CodeBadQuery)
			So(h["summary"], ShouldEqual, "bad query")
			So(h["details"], ShouldEqual, "page size must be less than 100")
			So(h["data"], ShouldResemble, gin.H{"items": []string{}})
		})
	})
}

// TestAbortWithGinH 测试 CodeError.AbortWithGinH 方法
func TestAbortWithGinH(t *testing.T) {
	Convey("测试 AbortWithGinH", t, func() {
		gin.SetMode(gin.TestMode)
		localizer := newTestLocalizer()
		translator := newTestTranslator()

		Convey("应正确中断请求并返回 JSON 响应", func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(LocalizerCtxName, localizer)
			c.Set(TranslatorCtxName, translator)

			err := CreateCodeError(http.StatusNotFound, http.StatusNotFound, nil)
			err.AbortWithGinH(c)

			So(w.Code, ShouldEqual, http.StatusNotFound)
			So(c.IsAborted(), ShouldBeTrue)

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(http.StatusNotFound))
			So(body["summary"], ShouldEqual, "not found")
		})

		Convey("SimpleOK 应返回 200 状态码", func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(LocalizerCtxName, localizer)
			c.Set(TranslatorCtxName, translator)

			SimpleOK.AbortWithGinH(c)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(c.IsAborted(), ShouldBeTrue)
		})

		Convey("带 LocalizedError 详情时应包含翻译后的详情", func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(LocalizerCtxName, localizer)
			c.Set(TranslatorCtxName, translator)

			err := CreateBadQueryError("pageSize", 100)
			err.AbortWithGinH(c)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(CodeBadQuery))
			So(body["details"], ShouldEqual, "page size must be less than 100")
		})
	})
}

// TestAbortWithGinW 测试 CodeError.AbortWithGinW 方法
func TestAbortWithGinW(t *testing.T) {
	Convey("测试 AbortWithGinW", t, func() {
		gin.SetMode(gin.TestMode)
		localizer := newTestLocalizer()
		translator := newTestTranslator()

		Convey("应正确中断请求并返回包含 data 的 JSON 响应", func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(LocalizerCtxName, localizer)
			c.Set(TranslatorCtxName, translator)

			data := map[string]any{"id": 1, "name": "test"}
			SimpleOK.AbortWithGinW(c, data)

			So(w.Code, ShouldEqual, http.StatusOK)
			So(c.IsAborted(), ShouldBeTrue)

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["code"], ShouldEqual, float64(http.StatusOK))
			So(body["summary"], ShouldEqual, "ok")
			So(body["data"], ShouldNotBeNil)
		})

		Convey("data 字段应为传入的数据", func() {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(LocalizerCtxName, localizer)
			c.Set(TranslatorCtxName, translator)

			SimpleOK.AbortWithGinW(c, "hello")

			var body map[string]any
			So(json.Unmarshal(w.Body.Bytes(), &body), ShouldBeNil)
			So(body["data"], ShouldEqual, "hello")
		})
	})
}

// TestLocalizedErrorInterface 测试 LocalizedError 接口实现
func TestLocalizedErrorInterface(t *testing.T) {
	Convey("测试 LocalizedError 接口", t, func() {
		localizer := newTestLocalizer()

		Convey("DetailContext 应实现 LocalizedError 接口", func() {
			var _ LocalizedError = &DetailContext{}
		})

		Convey("通过接口调用应返回正确的本地化结果", func() {
			var localized LocalizedError = &DetailContext{
				MsgID: "invalidID",
				args:  []any{"xyz"},
			}
			result := localized.Localize(localizer)
			So(result, ShouldEqual, "invalid id: xyz")
		})
	})
}

// TestCreateBadQueryAndUrlParamDifferences 测试 CreateBadQueryError 和 CreateBadUrlParamError 的区别
func TestCreateBadQueryAndUrlParamDifferences(t *testing.T) {
	Convey("测试 CreateBadQueryError 和 CreateBadUrlParamError 的区别", t, func() {
		Convey("两者应使用不同的错误码", func() {
			queryErr := CreateBadQueryError("test")
			urlParamErr := CreateBadUrlParamError("test")
			So(queryErr.code, ShouldEqual, CodeBadQuery)
			So(urlParamErr.code, ShouldEqual, CodeBadUrlParam)
			So(queryErr.code, ShouldNotEqual, urlParamErr.code)
		})

		Convey("两者都应使用 DetailContext 作为详情", func() {
			queryErr := CreateBadQueryError("testMsg")
			urlParamErr := CreateBadUrlParamError("testMsg")

			_, queryOk := queryErr.details.(*DetailContext)
			_, urlParamOk := urlParamErr.details.(*DetailContext)
			So(queryOk, ShouldBeTrue)
			So(urlParamOk, ShouldBeTrue)
		})

		Convey("两者的 HTTP 状态码应均为 400", func() {
			queryErr := CreateBadQueryError("test")
			urlParamErr := CreateBadUrlParamError("test")
			So(queryErr.statusCode, ShouldEqual, http.StatusBadRequest)
			So(urlParamErr.statusCode, ShouldEqual, http.StatusBadRequest)
		})
	})
}
