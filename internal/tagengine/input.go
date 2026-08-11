package tagengine

import (
	"encoding/json"
	"reflect"

	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// extractNamespace 从目标实例中提取所属命名空间。
// 目标模型内嵌 BaseMeta（含 Namespace 字段）时返回其值（如 *network.HttpTransport）；
// 目标缺失 Namespace 字段、字段为空串或字段类型非字符串时，回退到全局默认命名空间
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
	field := v.FieldByName("Namespace")
	if !field.IsValid() || field.Kind() != reflect.String {
		return base.DefaultNamespace
	}
	if ns := field.String(); ns != "" {
		return ns
	}
	return base.DefaultNamespace
}

// BuildInput 构建规则评估的输入文档。
// 目标模型实现 Inputer 接口时使用其自定义文档；否则将结构体 JSON 序列化
// 后反序列化为 map 作为输入文档（字段名即 json tag，Crypto 加密字段序列化为
// 空对象，不向规则暴露明文）。
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
