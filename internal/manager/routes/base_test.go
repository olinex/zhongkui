package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	uv "github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

func TestMain(m *testing.M) {
	// 初始化全局 KeyDerivation，供 CryptoSerializer 使用
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	// 在 Gin binding 验证器上注册自定义验证规则，供请求体绑定校验使用
	registerBindingValidations()

	os.Exit(m.Run())
}

// registerBindingValidations 在 Gin binding 验证器上注册自定义验证规则
// 测试中通过 ShouldBindJSON 等绑定的请求体包含 binding 标签（如 strongPassword、
// alphanumspecial、gtCurrentTime），需确保这些自定义规则在测试环境已注册
func registerBindingValidations() {
	v, ok := binding.Validator.Engine().(*uv.Validate)
	if !ok {
		panic("binding validator is not valid")
	}
	for tag, fn := range map[string]uv.Func{
		validator.TagStrongPassword:         validator.ValidateStrongPassword,
		validator.TagWindowsFileName:        validator.ValidateWindowsFileName,
		validator.TagUnixFileName:           validator.ValidateUnixFileName,
		validator.TagFileName:               validator.ValidateFileName,
		validator.TagStrictFileName:         validator.ValidateStrictFileName,
		validator.TagDirpath:                validator.ValidateDirPath,
		validator.TagAlphanumSpecial:        validator.ValidateAlphanumSpecial,
		validator.TagGreaterThanCurrentTime: validator.ValidateGtCurrentTime,
		validator.TagLesserThanCurrentTime:  validator.ValidateLtCurrentTime,
	} {
		if err := v.RegisterValidation(tag, fn); err != nil {
			panic(err)
		}
	}
}

func newBaseTestLocalizer() *i18n.Localizer {
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
code.1100 = "username or password check failed"
code.1101 = "old password check failed"
code.1102 = "username or password check failed"
code.1103 = "username or email already exist"
code.1104 = "too many access key"
code.1105 = "tag event rule invalid"
code.200 = "ok"
code.404 = "not found"
error.emptyUrlParam = "empty url param: %s"
error.badUrlParam = "bad url param: %s"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

func newBaseTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回一个注入 Localizer 和 Translator 的 Gin 中间件
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(manager.LocalizerCtxName, newBaseTestLocalizer())
		c.Set(manager.TranslatorCtxName, newBaseTestTranslator())
		c.Next()
	}
}

type simpleStruct struct {
	Name    string `json:"name"`
	Age     int    `json:"age"`
	Email   string `json:"email"`
	Ignored string
}

type omitemptyStruct struct {
	Name     string  `json:"name,omitempty"`
	Balance  float64 `json:"balance,omitempty"`
	Excluded string  `json:"-"`
}

type embeddedBase struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type embeddedStruct struct {
	embeddedBase
	Name string `json:"name"`
}

type ptrFieldStruct struct {
	Name  *string `json:"name"`
	Count *int    `json:"count"`
}

type nestedEmbeddedStruct struct {
	embeddedBase
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type noJSONTagStruct struct {
	Field1 string
	Field2 int
}

type testSearcherIn struct{}

func (t *testSearcherIn) GetGreedy() bool                            { return false }
func (t *testSearcherIn) GetPage() uint                              { return 1 }
func (t *testSearcherIn) GetPageSize() uint                          { return 10 }
func (t *testSearcherIn) GetOrderBy() []utils.OrderByItem            { return nil }
func (t *testSearcherIn) GetFilterBy() map[string]utils.FilterByItem { return nil }
func (t *testSearcherIn) GetOptions() *utils.SearchOptions {
	return &utils.SearchOptions{
		OrderBy:  []string{"id", "name"},
		FilterBy: []*utils.FilterByOption{},
	}
}

type testTotalOut struct {
	utils.BaseTotalOut
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

func TestHeaderOption(t *testing.T) {
	Convey("测试 HeaderOption 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			opt := &HeaderOption{
				Name: "id",
				Type: "uint",
			}
			So(opt.Name, ShouldEqual, "id")
			So(opt.Type, ShouldEqual, "uint")
		})

		Convey("应正确序列化为 JSON", func() {
			opt := &HeaderOption{
				Name: "createdAt",
				Type: "time.Time",
			}
			data, err := json.Marshal(opt)
			So(err, ShouldBeNil)
			So(string(data), ShouldEqual, `{"name":"createdAt","type":"time.Time"}`)
		})
	})
}

func TestListOptions(t *testing.T) {
	Convey("测试 ListOptions 结构体", t, func() {
		Convey("应正确嵌入 SearchOptions", func() {
			opts := ListOptions{
				SearchOptions: utils.SearchOptions{
					OrderBy:  []string{"name", "createdAt"},
					FilterBy: []*utils.FilterByOption{{Name: "name", Type: "string", Actions: []string{"eq", "ctn"}}},
				},
				Headers: []*HeaderOption{
					{Name: "id", Type: "uint"},
					{Name: "name", Type: "string"},
				},
			}
			So(opts.OrderBy, ShouldResemble, []string{"name", "createdAt"})
			So(len(opts.FilterBy), ShouldEqual, 1)
			So(len(opts.Headers), ShouldEqual, 2)
		})

		Convey("应正确序列化为 JSON", func() {
			opts := ListOptions{
				SearchOptions: utils.SearchOptions{
					OrderBy:  []string{"name"},
					FilterBy: []*utils.FilterByOption{},
				},
				Headers: []*HeaderOption{
					{Name: "id", Type: "uint"},
				},
			}
			data, err := json.Marshal(opts)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)
			So(result["orderBy"], ShouldNotBeNil)
			So(result["headers"], ShouldNotBeNil)
		})
	})
}

func TestGetJSONTagAndType(t *testing.T) {
	Convey("测试 getJSONTagAndType 函数", t, func() {
		Convey("应提取简单结构体的 JSON 标签和类型", func() {
			opts := getJSONTagAndType(reflect.TypeOf(simpleStruct{}))
			So(len(opts), ShouldEqual, 3)

			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["name"], ShouldEqual, "string")
			So(names["age"], ShouldEqual, "int")
			So(names["email"], ShouldEqual, "string")
		})

		Convey("应忽略无 json 标签的字段", func() {
			opts := getJSONTagAndType(reflect.TypeOf(simpleStruct{}))
			for _, opt := range opts {
				So(opt.Name, ShouldNotEqual, "Ignored")
			}
		})

		Convey("应忽略 json:\"-\" 的字段", func() {
			opts := getJSONTagAndType(reflect.TypeOf(omitemptyStruct{}))
			for _, opt := range opts {
				So(opt.Name, ShouldNotEqual, "Excluded")
			}
		})

		Convey("应正确处理 omitempty 选项", func() {
			opts := getJSONTagAndType(reflect.TypeOf(omitemptyStruct{}))
			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["name"], ShouldEqual, "string")
			So(names["balance"], ShouldEqual, "float64")
		})

		Convey("应递归展开匿名嵌入的结构体字段", func() {
			opts := getJSONTagAndType(reflect.TypeOf(embeddedStruct{}))
			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["id"], ShouldEqual, "uint")
			So(names["createdAt"], ShouldEqual, "time.Time")
			So(names["name"], ShouldEqual, "string")
		})

		Convey("应正确处理指针类型的字段", func() {
			opts := getJSONTagAndType(reflect.TypeOf(ptrFieldStruct{}))
			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["name"], ShouldEqual, "string")
			So(names["count"], ShouldEqual, "int")
		})

		Convey("应支持传入指针类型作为参数", func() {
			opts := getJSONTagAndType(reflect.TypeOf(&simpleStruct{}))
			So(len(opts), ShouldEqual, 3)
			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["name"], ShouldEqual, "string")
		})

		Convey("所有字段均无 json 标签时应返回空切片", func() {
			opts := getJSONTagAndType(reflect.TypeOf(noJSONTagStruct{}))
			So(len(opts), ShouldEqual, 0)
		})

		Convey("应正确处理嵌套嵌入和 omitempty 组合", func() {
			opts := getJSONTagAndType(reflect.TypeOf(nestedEmbeddedStruct{}))
			names := make(map[string]string)
			for _, opt := range opts {
				names[opt.Name] = opt.Type
			}
			So(names["id"], ShouldEqual, "uint")
			So(names["createdAt"], ShouldEqual, "time.Time")
			So(names["name"], ShouldEqual, "string")
			So(names["email"], ShouldEqual, "string")
		})
	})
}

func TestGetRawParam(t *testing.T) {
	Convey("测试 getRawParam 函数", t, func() {
		Convey("参数非空时应正确返回参数值", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/group/:name", func(c *gin.Context) {
				val, ok := getRawParam(c, "name")
				So(ok, ShouldBeTrue)
				So(val, ShouldEqual, "admin")
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/group/admin", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("参数为空字符串时应返回 false 并中断请求", func() {
			gin.SetMode(gin.TestMode)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Set(manager.LocalizerCtxName, newBaseTestLocalizer())
			c.Set(manager.TranslatorCtxName, newBaseTestTranslator())
			c.Request = httptest.NewRequest(http.MethodGet, "/api/group/", nil)
			c.Params = gin.Params{{Key: "name", Value: ""}}

			val, ok := getRawParam(c, "name")
			So(ok, ShouldBeFalse)
			So(val, ShouldEqual, "")
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestGetUint64Param(t *testing.T) {
	Convey("测试 getUint64Param 函数", t, func() {
		Convey("有效的 uint64 参数应正确解析", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/user/:id", func(c *gin.Context) {
				id, ok := getUint64Param(c, "id")
				So(ok, ShouldBeTrue)
				So(id, ShouldEqual, uint64(12345))
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/user/12345", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("非数字参数应返回 false 并中断请求", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/user/:id", func(c *gin.Context) {
				id, ok := getUint64Param(c, "id")
				So(ok, ShouldBeFalse)
				So(id, ShouldEqual, uint64(0))
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/user/abc", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("负数参数应返回 false 并中断请求", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/user/:id", func(c *gin.Context) {
				id, ok := getUint64Param(c, "id")
				So(ok, ShouldBeFalse)
				So(id, ShouldEqual, uint64(0))
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/user/-1", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("零值参数应正确解析为 0", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/user/:id", func(c *gin.Context) {
				id, ok := getUint64Param(c, "id")
				So(ok, ShouldBeTrue)
				So(id, ShouldEqual, uint64(0))
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/user/0", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		})
	})
}

func TestGetUUIDParam(t *testing.T) {
	Convey("测试 getUUIDParam 函数", t, func() {
		Convey("有效的 UUID 参数应正确解析", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/task/:id", func(c *gin.Context) {
				uuid, ok := getUUIDParam(c, "id")
				So(ok, ShouldBeTrue)
				So(uuid.String(), ShouldEqual, "123e4567-e89b-12d3-a456-426614174000")
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/task/123e4567-e89b-12d3-a456-426614174000", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusOK)
		})

		Convey("无效的 UUID 参数应返回 false 并中断请求", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/task/:id", func(c *gin.Context) {
				_, ok := getUUIDParam(c, "id")
				So(ok, ShouldBeFalse)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/task/not-a-uuid", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("空 UUID（全零）应返回 false 并中断请求", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/task/:id", func(c *gin.Context) {
				_, ok := getUUIDParam(c, "id")
				So(ok, ShouldBeFalse)
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/task/00000000-0000-0000-0000-000000000000", nil)
			r.ServeHTTP(w, req)
			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})
	})
}

func TestListOptionsGinHandler(t *testing.T) {
	Convey("测试 ListOptionsGinHandler 泛型处理器", t, func() {
		Convey("应正确返回列表选项配置", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/options", ListOptionsGinHandler[*testSearcherIn, *testTotalOut])

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/options", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)
			So(resp["code"], ShouldEqual, float64(200))

			data, ok := resp["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["headers"], ShouldNotBeNil)

			headers, ok := data["headers"].([]any)
			So(ok, ShouldBeTrue)
			So(len(headers), ShouldBeGreaterThan, 0)
		})

		Convey("返回的 headers 应包含输出结构体的字段信息", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/options", ListOptionsGinHandler[*testSearcherIn, *testTotalOut])

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/options", nil)
			r.ServeHTTP(w, req)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)

			data := resp["data"].(map[string]any)
			headers := data["headers"].([]any)

			headerNames := make(map[string]bool)
			for _, h := range headers {
				header := h.(map[string]any)
				name := header["name"].(string)
				headerNames[name] = true
			}
			So(headerNames["id"], ShouldBeTrue)
			So(headerNames["name"], ShouldBeTrue)
		})

		Convey("返回的 SearchOptions 应包含搜索配置", func() {
			searchOpts := &utils.SearchOptions{
				OrderBy: []string{"id", "name"},
				FilterBy: []*utils.FilterByOption{
					{Name: "name", Type: "string", Actions: []string{"eq", "ctn"}},
				},
			}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupI18nMiddleware())
			r.GET("/api/options", ListOptionsGinHandler[*testSearcherIn, *testTotalOut])

			_ = searchOpts

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/options", nil)
			r.ServeHTTP(w, req)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)

			data := resp["data"].(map[string]any)
			So(data["orderBy"], ShouldNotBeNil)
			So(data["filterBy"], ShouldNotBeNil)
		})
	})
}
