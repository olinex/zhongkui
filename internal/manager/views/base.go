// Package views 提供 API 视图层的通用基础设施，包括：
//   - 列表视图列的元数据描述（HeaderOption）
//   - 通过反射提取结构体 JSON 标签与类型的工具函数（getJSONTagAndType）
//   - URL 路径参数解析的辅助函数（getRawParam、getUint64Param、getUUIDParam）
//
// 视图层包含全部 HTTP 处理函数与 In/Out 结构体定义；路由注册见 internal/manager/router。
package views

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

// HeaderOption 描述列表视图中单个列的元数据信息。
// 前端可根据此结构动态渲染表格列，包括列名和对应的数据类型。
type HeaderOption struct {
	Name string `json:"name"` // 列名，对应输出结构体中字段的 JSON 标签名
	Type string `json:"type"` // 列的数据类型，如 string、int、time.Time 等
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
		response.CreateBadUrlParamError("emptyUrlParam", name).AbortWithGinH(c)
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
	idStr := c.Param(name)
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		response.CreateBadUrlParamError("badUrlParam", name, err).AbortWithGinH(c)
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
		response.CreateBadUrlParamError("emptyUrlParam", name).AbortWithGinH(c)
		return binUUID, false
	}
	return binUUID, true
}
