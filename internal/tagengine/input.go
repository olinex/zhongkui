package tagengine

import (
	"encoding/json"
	"fmt"
	"reflect"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// TypeKey 计算目标模型实例的类型键。
// 类型键格式为 pkgpath.TypeName（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport），
// 作为数据库规则的 TargetType 分组标识。指针自动解引用。
func TypeKey(target any) (string, error) {
	t := reflect.TypeOf(target)
	if t == nil {
		return "", ErrUnsupportedTarget
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return "", ErrUnsupportedTarget
	}
	return fmt.Sprintf(`%s.%s`, t.PkgPath(), t.Name()), nil
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
