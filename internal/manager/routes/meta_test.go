package routes

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
	"zhongkui.com/zhongkui/internal/manager"
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

// newMetaTestLocalizer 返回包含测试字段翻译 key 的 Localizer
func newMetaTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.200 = "ok"

["model.metaTestHeader.field.token"]
other = "JWT Token"

["model.metaTestIn.field.username"]
other = "Username (required, 6-32 chars)"

["model.metaTestIn.field.age"]
other = "Age (min 1)"

["model.metaTestBase.field.id"]
other = "Primary key ID"

["model.metaTestBase.field.createdAt"]
other = "Created time"

["model.metaTestOut.field.name"]
other = "Name"

["model.metaTestOut.field.count"]
other = "Count"

["model.metaTestOut.field.tags"]
other = "Tags"

["model.metaTestSearchIn.field.page"]
other = "Page number"

["model.metaTestSearchIn.field.pageSize"]
other = "Page size"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// setupMetaI18nMiddleware 返回一个注入测试 Localizer 和 Translator 的 Gin 中间件
func setupMetaI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(manager.LocalizerCtxName, newMetaTestLocalizer())
		c.Set(manager.TranslatorCtxName, newBaseTestTranslator())
		c.Next()
	}
}

func TestGetStructMeta(t *testing.T) {
	Convey("测试 getStructMeta 函数", t, func() {
		lc := newMetaTestLocalizer()

		Convey("应提取结构体字段的 JSON 字段名、binding 规则、翻译后 comment 与类型", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestIn{}), lc)
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestIn")
			So(len(meta.Fields), ShouldEqual, 2)

			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["username"].Type, ShouldEqual, "string")
			So(fields["username"].Binding, ShouldEqual, "required,min=6,max=32")
			So(fields["username"].Comment, ShouldEqual, "Username (required, 6-32 chars)")
			So(fields["age"].Type, ShouldEqual, "int")
			So(fields["age"].Binding, ShouldEqual, "min=1")
			So(fields["age"].Comment, ShouldEqual, "Age (min 1)")
		})

		Convey("应忽略无 json 标签与 json:\"-\" 的字段", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestIn{}), lc)
			for _, f := range meta.Fields {
				So(f.Name, ShouldNotEqual, "Ignored")
			}
			out := getStructMeta(reflect.TypeOf(metaTestOut{}), lc)
			for _, f := range out.Fields {
				So(f.Name, ShouldNotEqual, "Secret")
			}
		})

		Convey("应递归展开匿名嵌入的结构体字段并翻译其 comment", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestOut{}), lc)
			names := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				names[f.Name] = f
			}
			So(names["id"].Comment, ShouldEqual, "Primary key ID")
			So(names["createdAt"].Type, ShouldEqual, "time.Time")
			So(names["name"].Binding, ShouldEqual, "required")
			So(names["name"].Comment, ShouldEqual, "Name")
		})

		Convey("应正确处理 omitempty 选项与指针、切片类型", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestOut{}), lc)
			names := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				names[f.Name] = f
			}
			So(names["name"].Name, ShouldEqual, "name")
			So(names["count"].Type, ShouldEqual, "int")
			So(names["tags"].Type, ShouldEqual, "[]string")
			So(names["tags"].Binding, ShouldEqual, "max=5")
			So(names["tags"].Comment, ShouldEqual, "Tags")
		})

		Convey("应支持传入指针类型作为参数", func() {
			meta := getStructMeta(reflect.TypeOf(&metaTestIn{}), lc)
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestIn")
			So(len(meta.Fields), ShouldEqual, 2)
		})

		Convey("应支持 mapstructure 标签作为字段 key 的回退", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestSearchIn{}), lc)
			So(meta, ShouldNotBeNil)
			So(meta.Name, ShouldEqual, "metaTestSearchIn")
			So(len(meta.Fields), ShouldEqual, 3)

			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["page"].Type, ShouldEqual, "uint")
			So(fields["page"].Binding, ShouldEqual, "min=1,max=100")
			So(fields["page"].Comment, ShouldEqual, "Page number")
			So(fields["pageSize"].Binding, ShouldEqual, "oneof=10 20 50 100")
			So(fields["pageSize"].Comment, ShouldEqual, "Page size")
			So(fields["greedy"].Type, ShouldEqual, "bool")
			So(fields["Internal"], ShouldBeNil)
		})

		Convey("翻译 key 未命中时应回退返回 key 本身", func() {
			meta := getStructMeta(reflect.TypeOf(metaTestIn{}), newBaseTestLocalizer())
			fields := make(map[string]*FieldMeta)
			for _, f := range meta.Fields {
				fields[f.Name] = f
			}
			So(fields["username"].Comment, ShouldEqual, "model.metaTestIn.field.username")
		})

		Convey("未指定类型时应返回 nil", func() {
			So(getStructMeta(nil, lc), ShouldBeNil)
		})
	})
}

func TestStructMetaGinHandler(t *testing.T) {
	Convey("测试 StructMetaGinHandler 泛型处理器", t, func() {
		Convey("应按 HTTP 方法分组返回元信息并翻译 comment", func() {
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
			So(headerField["comment"], ShouldEqual, "JWT Token")

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
