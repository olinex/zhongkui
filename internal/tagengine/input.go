package tagengine

import (
	"encoding/json"
	"reflect"

	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// extractNamespace 从目标实例中提取所属命名空间。
// 优先读取 NamespaceName 字符串字段（GORM 模型以嵌入 Struct 形式表达 Namespace 关联，
// 真正的命名空间字符串在 NamespaceName 列上）；其次尝试 Namespace 字段若其本身为字符串
// （无 NamespaceName 的简单模型），任一字段缺失/为空串时回退到全局默认命名空间
// base.DefaultNamespace，保证未显式归属租户的数据落入默认租户的规则分组。
func extractNamespace(target any) string {
	v := reflect.ValueOf(target)
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.IsNil() {
			return base.DefaultNamespace
		}
		v = v.Elem()
	}
	if !v.IsValid() || v.Kind() != reflect.Struct {
		return base.DefaultNamespace
	}
	for _, name := range []string{"NamespaceName", "Namespace"} {
		field := v.FieldByName(name)
		if field.IsValid() && field.Kind() == reflect.String {
			if ns := field.String(); ns != "" {
				return ns
			}
		}
	}
	return base.DefaultNamespace
}

// BuildInput 构建规则评估的输入文档。
// 目标模型实现 Inputer 接口时使用其自定义文档；否则将结构体 JSON 序列化
// 后反序列化为 map 作为输入文档（字段名即 json tag，Crypto 加密字段序列化为
// 空字符串，不向规则暴露明文）。
func BuildInput(target any) (map[string]any, error) {
	if inputer, ok := target.(Inputer); ok {
		return inputer.TagRecogInput()
	}
	raw, err := json.Marshal(target)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	var input map[string]any
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, errors.WithStack(err)
	}
	return input, nil
}
