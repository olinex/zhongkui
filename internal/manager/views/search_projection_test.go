package views_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/smartystreets/goconvey/convey"

	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/manager/views/admin"
	"zhongkuisec.com/zhongkui/internal/manager/views/ai"
	"zhongkuisec.com/zhongkui/internal/manager/views/datasource"
	"zhongkuisec.com/zhongkui/internal/manager/views/llm"
	"zhongkuisec.com/zhongkui/internal/manager/views/notification"
	"zhongkuisec.com/zhongkui/internal/manager/views/tag"
	"zhongkuisec.com/zhongkui/internal/manager/views/traffic"
	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// projectionCase 描述一组列表接口的 SearchIn → Out 映射，
// 用于校验投影白名单（GetOptions().Fields）与 Out JSON 键集合的双向一致。
type projectionCase struct {
	name string
	in   any
	out  any
}

// searchProjectionCases 全部 24 个已实现字段投影的列表接口映射。
func searchProjectionCases() []projectionCase {
	return []projectionCase{
		{"UserListGet", &views.UserListGetSearchIn{}, &admin.UserListOut{}},
		{"RbacPolicyTemplateListGet", &views.RbacPolicyTemplateListGetSearchIn{}, &admin.RbacPolicyTemplateListOut{}},
		{"GroupListGet", &views.GroupListGetSearchIn{}, &admin.GroupListOut{}},
		{"NamespaceListGet", &views.NamespaceListGetSearchIn{}, &admin.NamespaceListOut{}},
		{"CapturerListGet", &views.CapturerListGetSearchIn{}, &admin.CapturerListOut{}},
		{"TagRuleListGet", &views.TagRuleListGetSearchIn{}, &tag.TagRuleListOut{}},
		{"TagRuleSampleListGet", &views.TagRuleSampleListGetSearchIn{}, &tag.TagRuleSampleListOut{}},
		{"TagEventRuleListGet", &views.TagEventRuleListGetSearchIn{}, &tag.TagEventRuleListOut{}},
		{"Ipv4TcpListGet", &views.Ipv4TcpListGetSearchIn{}, &traffic.Ipv4TcpListGetListItemOut{}},
		{"Ipv6TcpListGet", &views.Ipv6TcpListGetSearchIn{}, &traffic.Ipv6TcpListGetListItemOut{}},
		{"HttpTransportListGet", &views.HttpTransportListGetSearchIn{}, &traffic.HttpTransportListGetListItemOut{}},
		{"NetworkStackListGet", &views.NetworkStackListGetSearchIn{}, &traffic.NetworkStackListGetListItemOut{}},
		{"NotificationChannelListGet", &views.NotificationChannelListGetSearchIn{}, &notification.NotificationChannelListOut{}},
		{"NotificationTargetListGet", &views.NotificationTargetListGetSearchIn{}, &notification.NotificationTargetListOut{}},
		{"NotificationGroupListGet", &views.NotificationGroupListGetSearchIn{}, &notification.NotificationGroupListOut{}},
		{"NotificationTemplateListGet", &views.NotificationTemplateListGetSearchIn{}, &notification.NotificationTemplateListOut{}},
		{"NotificationRecordListGet", &views.NotificationRecordListGetSearchIn{}, &notification.NotificationRecordListOut{}},
		{"TagEventRecordListGet", &views.TagEventRecordListGetSearchIn{}, &tag.TagEventRecordListOut{}},
		{"DatasourceListGet", &views.DatasourceListGetSearchIn{}, &datasource.DatasourceListOut{}},
		{"LlmModelListGet", &views.LlmModelListGetSearchIn{}, &llm.LlmModelListOut{}},
		{"McpServerListGet", &views.McpServerListGetSearchIn{}, &llm.McpServerListOut{}},
		{"LlmToolListGet", &views.LlmToolListGetSearchIn{}, &llm.LlmToolListOut{}},
		{"KnowledgeDocumentListGet", &views.KnowledgeDocumentListGetSearchIn{}, &ai.KnowledgeDocumentListOut{}},
		{"AgentMemoryListGet", &views.AgentMemoryListGetSearchIn{}, &ai.AgentMemoryListOut{}},
	}
}

// TestSelectFieldsCompleteness 校验 24 个列表接口的投影白名单与 Out JSON 键集合双向一致：
// GetOptions().Fields 中的每个键都必须能在 Out 上序列化输出，
// Out 的每个 JSON 键也都必须在投影白名单中（否则前端列设置无法勾选该列）。
func TestSelectFieldsCompleteness(t *testing.T) {
	convey.Convey("投影白名单应与 Out JSON 键集合双向一致", t, func() {
		for _, tc := range searchProjectionCases() {
			convey.Convey(tc.name, func() {
				searcher, ok := tc.in.(utils.SearcherIn)
				convey.So(ok, convey.ShouldBeTrue)
				projector, ok := tc.in.(utils.FieldProjector)
				convey.So(ok, convey.ShouldBeTrue)
				convey.So(projector, convey.ShouldNotBeNil)

				options := searcher.GetOptions()
				convey.So(options, convey.ShouldNotBeNil)

				fieldSet := stringSet(options.Fields)
				outKeys := collectJSONKeys(tc.out)

				// 白名单多余键：Options.Fields 有而 Out 无
				var extra []string
				for _, f := range options.Fields {
					if _, exists := outKeys[f]; !exists {
						extra = append(extra, f)
					}
				}
				// Out 缺失键：Out 有而白名单无
				var missing []string
				for k := range outKeys {
					if _, exists := fieldSet[k]; !exists {
						missing = append(missing, k)
					}
				}
				convey.So(extra, convey.ShouldBeEmpty)
				convey.So(missing, convey.ShouldBeEmpty)
			})
		}
	})
}

// TestResolveSelectColumns 校验投影列解析的行为契约：
// 空 fields 返回 nil、未知字段静默忽略、重复字段去重、不强制包含 id 列。
func TestResolveSelectColumns(t *testing.T) {
	convey.Convey("Fields 为空时返回 nil", t, func() {
		in := &views.HttpTransportListGetSearchIn{}
		convey.So(in.ResolveSelectColumns(), convey.ShouldBeNil)
	})

	convey.Convey("未知字段被静默忽略", t, func() {
		in := &views.HttpTransportListGetSearchIn{Fields: []string{"unknown"}}
		convey.So(in.ResolveSelectColumns(), convey.ShouldBeEmpty)
	})

	convey.Convey("重复字段去重", t, func() {
		in := &views.HttpTransportListGetSearchIn{Fields: []string{"timestamp", "timestamp"}}
		columns := in.ResolveSelectColumns()
		count := 0
		for _, column := range columns {
			if column == "timestamp" {
				count++
			}
		}
		convey.So(count, convey.ShouldEqual, 1)
	})

	convey.Convey("不强制包含 id 列：Fields 不含 id 时结果不含 id", t, func() {
		in := &views.HttpTransportListGetSearchIn{Fields: []string{"url"}}
		columns := in.ResolveSelectColumns()
		convey.So(len(columns), convey.ShouldEqual, 1)
		convey.So(columns[0], convey.ShouldEqual, "url")
	})

	convey.Convey("NetworkStack 白名单不含 id（无唯一 ID），解析结果不含 id 列", t, func() {
		in := &views.NetworkStackListGetSearchIn{}
		convey.So(in.ResolveSelectColumns(), convey.ShouldBeNil)
		in.Fields = []string{"timestamp", "seq"}
		columns := in.ResolveSelectColumns()
		convey.So(len(columns), convey.ShouldBeGreaterThan, 0)
		for _, column := range columns {
			convey.So(column, convey.ShouldNotEqual, "id")
		}
	})
}

// TestTrafficDetailInBinding 校验三个流量行详情接口的入参绑定契约：
// namespace 与 id 均为必填，缺失任一字段绑定失败。
func TestTrafficDetailInBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)

	convey.Convey("完整请求体应绑定成功", t, func() {
		body := `{"namespace":"default","id":"0c0a2b3c4d5e6f708192a3b4c5d6e7f8"}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in traffic.HttpTransportDetailGetIn
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldBeNil)
		convey.So(in.Namespace, convey.ShouldEqual, "default")
		convey.So(in.ID.String(), convey.ShouldEqual, "0c0a2b3c-4d5e-6f70-8192-a3b4c5d6e7f8")
	})

	convey.Convey("缺失 id 应绑定失败", t, func() {
		body := `{"namespace":"default"}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in traffic.Ipv4TcpDetailGetIn
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldNotBeNil)
	})

	convey.Convey("缺失 namespace 应绑定失败", t, func() {
		body := `{"id":"0c0a2b3c4d5e6f708192a3b4c5d6e7f8"}`

		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
		c.Request.Header.Set("Content-Type", "application/json")

		var in traffic.Ipv6TcpDetailGetIn
		convey.So(c.ShouldBindBodyWithJSON(&in), convey.ShouldNotBeNil)
	})
}

// TestProjectionOutputKeys 校验投影输出键集合 == 请求字段 ∩ Out 键：
// 未知字段被忽略、id 仅在请求携带时输出。
func TestProjectionOutputKeys(t *testing.T) {
	convey.Convey("投影输出键集合应等于请求字段与 Out 键的交集，且不强制含 id", t, func() {
		row := network.HttpTransport{
			RowMeta: network.RowMeta{
				ID: serializer.MustHexUUIDv4FromString("0c0a2b3c4d5e6f708192a3b4c5d6e7f8"),
			},
		}
		out := traffic.NewHttpTransportListGetListItemOut(&row)

		m := utils.ProjectItemFields(out, []string{"timestamp", "url", "unknownField"})
		keys := stringSet(nil)
		for k := range m {
			keys[k] = struct{}{}
		}
		convey.So(len(m), convey.ShouldEqual, 2)
		convey.So(hasKeys(keys, "timestamp", "url"), convey.ShouldBeTrue)
		convey.So(hasKeys(keys, "id"), convey.ShouldBeFalse)
	})
}

// TestProjectionCryptoFieldString 校验投影输出中 crypto 字段经 MarshalJSON 输出 base64 密文串，
// 而非退化为 {} 空对象：serializer.Crypto 的 MarshalJSON 为指针接收器，
// 投影取值若被解引用为值类型则丢失自定义序列化（unexported 字段被跳过）。
// 前端据此直传 ciphertext 调用 /plaintext/crypto，空对象将导致 "[object Object]" 传参。
func TestProjectionCryptoFieldString(t *testing.T) {
	convey.Convey("投影输出中 crypto 字段应为 base64 密文串", t, func() {
		row := network.HttpTransport{
			RowMeta: network.RowMeta{
				ID: serializer.MustHexUUIDv4FromString("0c0a2b3c4d5e6f708192a3b4c5d6e7f8"),
			},
		}
		out := traffic.NewHttpTransportListGetListItemOut(&row)
		var cookie serializer.Crypto
		convey.So(json.Unmarshal([]byte(`"aGVsbG8="`), &cookie), convey.ShouldBeNil)
		out.ReqHdrCookie = &cookie

		projected := utils.ProjectSearchItems(
			[]*traffic.HttpTransportListGetListItemOut{out},
			&views.HttpTransportListGetSearchIn{Fields: []string{"reqHdrCookie"}},
			func(o **traffic.HttpTransportListGetListItemOut) any { return *o },
		)
		convey.So(projected, convey.ShouldHaveLength, 1)

		b, err := json.Marshal(projected[0])
		convey.So(err, convey.ShouldBeNil)
		convey.So(string(b), convey.ShouldEqual, `{"reqHdrCookie":"aGVsbG8="}`)
	})
}

// collectJSONKeys 递归收集结构体的全部 JSON 键：
// 匿名嵌入结构体展开、跳过 json:"-" 字段、无 json tag 时使用字段名。
func collectJSONKeys(v any) map[string]struct{} {
	keys := make(map[string]struct{})
	t := reflect.TypeOf(v)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return keys
	}
	collectStructJSONKeys(t, keys)
	return keys
}

// collectStructJSONKeys 递归展开结构体字段并注册 JSON 键。
func collectStructJSONKeys(t reflect.Type, keys map[string]struct{}) {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// 匿名嵌入的 struct/指针 struct 递归展开
		if field.Anonymous {
			ft := field.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				collectStructJSONKeys(ft, keys)
				continue
			}
		}
		// 不可导出字段不参与 JSON 序列化
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		if jsonTag, ok := field.Tag.Lookup("json"); ok {
			if idx := strings.Index(jsonTag, ","); idx != -1 {
				jsonTag = jsonTag[:idx]
			}
			switch jsonTag {
			case "-":
				continue
			case "":
			default:
				name = jsonTag
			}
		}
		keys[name] = struct{}{}
	}
}

// stringSet 将字符串切片转为集合。
func stringSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		set[item] = struct{}{}
	}
	return set
}

// hasKeys 断言集合包含全部给定键。
func hasKeys(set map[string]struct{}, keys ...string) bool {
	for _, k := range keys {
		if _, exists := set[k]; !exists {
			return false
		}
	}
	return true
}
