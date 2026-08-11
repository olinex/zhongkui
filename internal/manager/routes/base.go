// Package routes 提供 API 路由层的通用基础设施，包括：
//   - 列表选项的元数据描述（HeaderOption、ListOptions）
//   - 通过反射提取结构体 JSON 标签与类型的工具函数（getJSONTagAndType）
//   - URL 路径参数解析的辅助函数（getRawParam、getUint64Param、getUUIDParam）
//   - 列表选项查询接口的 Gin 处理器（ListOptionsGinHandler）
package routes

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// HeaderOption 描述列表视图中单个列的元数据信息。
// 前端可根据此结构动态渲染表格列，包括列名和对应的数据类型。
type HeaderOption struct {
	Name string `json:"name"` // 列名，对应输出结构体中字段的 JSON 标签名
	Type string `json:"type"` // 列的数据类型，如 string、int、time.Time 等
}

// ListOptions 描述列表查询接口的完整选项配置。
// 嵌入了 SearchOptions 以提供排序和过滤的元数据，
// 并额外包含 Headers 用于描述输出结构体各字段的列信息。
// 典型场景：前端在发起列表查询前，先调用 ListOptionsGinHandler 获取此配置，
// 以动态渲染表格列和搜索/过滤控件。
type ListOptions struct {
	utils.SearchOptions                 // 嵌入搜索选项，包含可排序字段（OrderBy）和可过滤字段（FilterBy）
	Headers             []*HeaderOption `json:"headers"` // 列表列的元数据信息列表
}

// getJSONTagAndType 通过反射递归提取结构体中所有字段的 JSON 标签名和 Go 类型名。
// 对于指针类型会自动解引用，对于匿名嵌入字段会递归展开，
// 忽略无 json 标签或标签为 "-" 的字段，并正确处理 json 标签中的 omitempty 等选项。
//
// 参数：
//   - t: 目标结构体的 reflect.Type，支持传入指针类型
//
// 返回值：
//   - opts: 提取到的所有字段的 HeaderOption 列表
func getJSONTagAndType(t reflect.Type) (opts []*HeaderOption) {
	// 如果传入的是指针类型，解引用获取实际类型
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	for field := range t.Fields() {
		// 处理匿名嵌入字段：递归展开其内部字段
		if field.Anonymous {
			embeddedType := field.Type
			// 嵌入字段为指针时，解引用获取实际类型
			if embeddedType.Kind() == reflect.Pointer {
				embeddedType = embeddedType.Elem()
			}
			// 仅对结构体类型的嵌入字段递归展开
			if embeddedType.Kind() == reflect.Struct {
				opts = append(opts, getJSONTagAndType(embeddedType)...)
				continue
			}
		}

		// 获取 json 标签，忽略无标签或显式标记为 "-" 的字段
		jsonTag := field.Tag.Get("json")
		if jsonTag == "" || jsonTag == "-" {
			continue
		}

		// 处理 json 标签中的选项部分（如 "name,omitempty"），只取逗号前的名称
		if commaIdx := strings.Index(jsonTag, ","); commaIdx != -1 {
			jsonTag = jsonTag[:commaIdx]
		}

		// 获取字段类型名称，指针类型取其元素类型名称
		fieldType := field.Type.String()
		if field.Type.Kind() == reflect.Pointer {
			fieldType = field.Type.Elem().String()
		}

		opts = append(opts, &HeaderOption{
			Name: jsonTag,
			Type: fieldType,
		})
	}
	return
}

// getRawParam 从 Gin 上下文中获取原始字符串类型的 URL 路径参数。
// 若参数值为空字符串，则创建 URL 参数错误并中断请求。
//
// 参数：
//   - c: Gin 上下文
//   - name: URL 路径参数名（如路由 "/api/group/:name" 中的 "name"）
//
// 返回值：
//   - string: 参数值
//   - bool: 是否成功获取（true 表示参数有效，false 表示参数为空且请求已被中断）
func getRawParam(c *gin.Context, name string) (string, bool) {
	rawStr := c.Param(name)
	if len(rawStr) == 0 {
		manager.CreateBadUrlParamError("emptyUrlParam", name).AbortWithGinH(c)
		return rawStr, false
	}
	return rawStr, true
}

// getUint64Param 从 Gin 上下文中获取 uint64 类型的 URL 路径参数。
// 将路径参数字符串解析为 64 位无符号整数，若解析失败则创建 URL 参数错误并中断请求。
//
// 参数：
//   - c: Gin 上下文
//   - name: URL 路径参数名（如路由 "/api/user/:id" 中的 "id"）
//
// 返回值：
//   - uint64: 解析后的参数值
//   - bool: 是否成功解析（true 表示参数有效，false 表示解析失败且请求已被中断）
func getUint64Param(c *gin.Context, name string) (uint64, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		manager.CreateBadUrlParamError("badUrlParam", name, err).AbortWithGinH(c)
		return 0, false
	}
	return id, true
}

// getUUIDParam 从 Gin 上下文中获取 UUID 类型的 URL 路径参数。
// 将路径参数字符串解析为 HexUUID（二进制 UUID），若解析失败或 UUID 为空/零值，
// 则创建 URL 参数错误并中断请求。
//
// 参数：
//   - c: Gin 上下文
//   - name: URL 路径参数名（如路由 "/api/task/:id" 中的 "id"）
//
// 返回值：
//   - serializer.HexUUID: 解析后的二进制 UUID
//   - bool: 是否成功解析（true 表示 UUID 有效，false 表示无效且请求已被中断）
func getUUIDParam(c *gin.Context, name string) (serializer.HexUUID, bool) {
	uuidStr := c.Param(name)
	binUUID, err := serializer.HexUUIDv4FromString(uuidStr)
	if err != nil || binUUID.IsNil() || binUUID.IsEmpty() {
		manager.CreateBadUrlParamError("emptyUrlParam", name).AbortWithGinH(c)
		return binUUID, false
	}
	return binUUID, true
}

// ListOptionsGinHandler 是一个泛型 Gin 处理器，用于返回列表查询接口的选项配置。
// 前端在发起列表查询前，先调用此处理器获取列定义（Headers）和搜索配置（SearchOptions），
// 以便动态渲染表格列和搜索/过滤控件。
//
// 类型参数：
//   - M: 实现了 utils.SearcherIn 接口的搜索输入类型，用于获取搜索选项配置
//   - N: 实现了 utils.TotalOut 接口的列表输出项类型，用于通过反射提取列元数据
//
// 响应体结构：
//
//	{
//	  "code": 200,
//	  "data": {
//	    "orderBy": ["field1", "field2"],
//	    "filterBy": [{"name": "field1", "type": "string", "actions": ["eq", "ctn"]}],
//	    "headers": [{"name": "id", "type": "uint"}, {"name": "createdAt", "type": "time.Time"}]
//	  }
//	}
func ListOptionsGinHandler[M utils.SearcherIn, N utils.TotalOut](c *gin.Context) {
	var in M
	var out N
	itemType := reflect.TypeOf(out)
	headers := getJSONTagAndType(itemType)
	searchOpts := in.GetOptions()
	manager.SimpleOK.AbortWithGinW(c, ListOptions{
		SearchOptions: *searchOpts,
		Headers:       headers,
	})
}
