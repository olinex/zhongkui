package views

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/text/language"

	"zhongkuisec.com/zhongkui/internal/manager/response"
)

type metaTestHeader struct {
	Token string `json:"token" binding:"required"`
}

type metaTestIn struct {
	Username string `json:"username" binding:"required,min=6,max=32"`
	Age      int    `json:"age" binding:"min=1"`
	Ignored  string // 无 json 标签，应被忽略
}

type metaTestBase struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"createdAt"`
}

type metaTestOut struct {
	metaTestBase
	Name   string   `json:"name,omitempty" binding:"required"`
	Secret string   `json:"-"` // json:"-"，应被忽略
	Count  *int     `json:"count"`
	Tags   []string `json:"tags" binding:"max=5"`
}

// metaTestSearchIn 模拟搜索输入结构体：仅使用 mapstructure 标签
type metaTestSearchIn struct {
	Page     uint   `mapstructure:"page" binding:"min=1,max=100"`
	PageSize uint   `mapstructure:"pageSize" binding:"oneof=10 20 50 100"`
	Greedy   bool   `mapstructure:"greedy"`
	Internal string // 无 json 与 mapstructure 标签，应被忽略
}

// metaTestForm 覆盖 form 标签 kind 覆盖、default 解析与 required 的测试结构体
type metaTestForm struct {
	Host     string            `json:"host" binding:"required"`
	Port     int               `json:"port" default:"25"`
	Enabled  bool              `json:"enabled"`
	Password string            `json:"password" form:"kind=password"`
	Headers  map[string]string `json:"headers"`
	Skip     string            // 无 json 标签，应被忽略
}

// metaTestEnumIn 覆盖可选枚举值自省字段的测试结构体：
// model 为声明的枚举字段，nonEnum 不在枚举注册表内，应保留非枚举形态。
type metaTestEnumIn struct {
	Model   string `json:"model"`
	NonEnum string `json:"nonEnum"`
}

// newMetaTestLocalizer 返回包含测试基础翻译 key 的 Localizer
func newMetaTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.200 = "ok"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// setupMetaI18nMiddleware 返回一个注入测试 Localizer 和 Translator 的 Gin 中间件
func setupMetaI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newMetaTestLocalizer())
		c.Set(response.TranslatorCtxName, newBaseTestTranslator())
		c.Next()
	}
}

func TestGetStructMeta(t *testing.T) {
	Convey("测试 getStructMeta 函数", t, func() {
		Convey("应提取结构体字段的 JSON 字段名、binding 规则、kind 与类型", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestIn{}))
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestIn")
			So(len(meta.Fields), ShouldEqual, 2)

			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["username"].Type, ShouldEqual, "string")
			So(fields["username"].Binding, ShouldEqual, "required,min=6,max=32")
			So(fields["username"].Kind, ShouldEqual, KindString)
			So(fields["username"].Required, ShouldBeTrue)
			So(fields["age"].Type, ShouldEqual, "int")
			So(fields["age"].Binding, ShouldEqual, "min=1")
			So(fields["age"].Kind, ShouldEqual, KindNumber)
			So(fields["age"].Required, ShouldBeFalse)
		})

		Convey("应忽略无 json 标签与 json:\"-\" 的字段", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestIn{}))
			for _, f := range meta.Fields {
				So(f.Name, ShouldNotEqual, "Ignored")
			}
			out := getStructMeta(reflect.TypeOf(metaTestOut{}))
			for _, f := range out.Fields {
				So(f.Name, ShouldNotEqual, "Secret")
			}
		})

		Convey("应递归展开匿名嵌入的结构体字段并按类型推断 kind", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestOut{}))
			names := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				names[f.Name] = f
			}
			So(names["createdAt"].Type, ShouldEqual, "time.Time")
			So(names["createdAt"].Kind, ShouldEqual, KindObject)
			So(names["name"].Binding, ShouldEqual, "required")
			So(names["name"].Required, ShouldBeTrue)
			So(names["id"].Kind, ShouldEqual, KindNumber)
		})

		Convey("应正确处理 omitempty 选项与指针、切片类型", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestOut{}))
			names := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				names[f.Name] = f
			}
			So(names["name"].Name, ShouldEqual, "name")
			So(names["count"].Type, ShouldEqual, "int")
			So(names["tags"].Type, ShouldEqual, "[]string")
			So(names["tags"].Binding, ShouldEqual, "max=5")
			So(names["tags"].Kind, ShouldEqual, KindObject)
		})

		Convey("应支持传入指针类型作为参数", func() {
			meta := getStructMeta(reflect.TypeOf(&metaTestIn{}))
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestIn")
			So(len(meta.Fields), ShouldEqual, 2)
		})

		Convey("应支持 mapstructure 标签作为字段 key 的回退", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestSearchIn{}))
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestSearchIn")
			So(len(meta.Fields), ShouldEqual, 3)

			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["page"].Type, ShouldEqual, "uint")
			So(fields["page"].Binding, ShouldEqual, "min=1,max=100")
			So(fields["pageSize"].Binding, ShouldEqual, "oneof=10 20 50 100")
			So(fields["greedy"].Type, ShouldEqual, "bool")
			So(fields["greedy"].Kind, ShouldEqual, KindBoolean)
			So(fields["Internal"], ShouldBeNil)
		})

		Convey("应依据 form 标签覆盖 kind 并解析默认值", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestForm{}))
			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["host"].Kind, ShouldEqual, KindString)
			So(fields["host"].Required, ShouldBeTrue)
			So(fields["port"].Kind, ShouldEqual, KindNumber)
			So(fields["port"].DefaultValue, ShouldEqual, 25)
			So(fields["enabled"].Kind, ShouldEqual, KindBoolean)
			So(fields["password"].Kind, ShouldEqual, KindPassword)
			So(fields["headers"].Kind, ShouldEqual, KindObject)
		})

		Convey("未指定类型时应返回 nil", func() {
			So(getStructMeta(nil), ShouldBeNil)
			So(GetStructFields(nil), ShouldBeNil)
		})

		Convey("命中枚举类绑定表的字段应附带 enumType 并标记为枚举", func() {
			typeName := reflect.TypeOf(metaTestEnumIn{}).Name()
			registry := enumBindings
			enumBindings = map[string]map[string]string{
				typeName: {"model": "testEnumClass"},
			}
			defer func() { enumBindings = registry }()

			meta := getStructMeta(reflect.TypeOf(metaTestEnumIn{}))
			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["model"].Enum, ShouldBeTrue)
			So(fields["model"].EnumType, ShouldEqual, "testEnumClass")
			// 未声明字段保持非枚举形态
			So(fields["nonEnum"].Enum, ShouldBeFalse)
			So(fields["nonEnum"].EnumType, ShouldEqual, "")
		})
	})
}

func TestStructMetaGinHandler(t *testing.T) {
	Convey("测试 StructMetaGinHandler 泛型处理器", t, func() {
		Convey("应按 HTTP 方法分组返回元信息", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupMetaI18nMiddleware())
			r.GET("/api/meta", StructMetaGinHandler(
				WithMethod(http.MethodPost,
					WithHeaderInput[metaTestHeader](),
					WithBodyInput[metaTestIn](),
					WithBodyOutput[metaTestOut](),
				),
				WithMethod(http.MethodOptions),
			))

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)
			So(resp["code"], ShouldEqual, float64(200))

			data, ok := resp["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			// 只包含注册的两个方法
			So(len(data), ShouldEqual, 2)
			So(data["GET"], ShouldBeNil)

			post := data["POST"].(map[string]any)
			headerInput := post["headerInput"].(map[string]any)
			So(headerInput["name"], ShouldEqual, "metaTestHeader")
			headerFields := headerInput["fields"].([]any)
			So(len(headerFields), ShouldEqual, 1)
			headerField := headerFields[0].(map[string]any)
			So(headerField["name"], ShouldEqual, "token")
			So(headerField["binding"], ShouldEqual, "required")
			So(headerField["kind"], ShouldEqual, "string")
			So(headerField["required"], ShouldBeTrue)

			bodyInput := post["bodyInput"].(map[string]any)
			So(bodyInput["name"], ShouldEqual, "metaTestIn")

			bodyOutput := post["bodyOutput"].(map[string]any)
			So(bodyOutput["name"], ShouldEqual, "metaTestOut")
			outputFields := bodyOutput["fields"].([]any)
			So(len(outputFields), ShouldEqual, 5)

			// OPTIONS 方法未指定任何结构体，四类均输出为 null
			options := data["OPTIONS"].(map[string]any)
			So(options["headerInput"], ShouldBeNil)
			So(options["headerOutput"], ShouldBeNil)
			So(options["bodyInput"], ShouldBeNil)
			So(options["bodyOutput"], ShouldBeNil)
		})

		Convey("方法未指定结构体时应输出为 null", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupMetaI18nMiddleware())
			r.GET("/api/meta", StructMetaGinHandler(
				WithMethod(http.MethodPost, WithBodyInput[metaTestIn]()),
			))

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
			r.ServeHTTP(w, req)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)

			data := resp["data"].(map[string]any)
			post := data["POST"].(map[string]any)
			So(post["headerInput"], ShouldBeNil)
			So(post["headerOutput"], ShouldBeNil)
			So(post["bodyInput"], ShouldNotBeNil)
			So(post["bodyOutput"], ShouldBeNil)
		})

		Convey("不传任何选项时应返回空 map", func() {
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(setupMetaI18nMiddleware())
			r.GET("/api/meta", StructMetaGinHandler())

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
			r.ServeHTTP(w, req)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)

			data := resp["data"].(map[string]any)
			So(len(data), ShouldEqual, 0)
		})
	})
}
