// Package tagengine 提供基于 Open Policy Agent(OPA) 的标签识别引擎。
// 规则以 Rego 源码形式存储于数据库，引擎按（目标模型类型键, 标签类型）分组
// 加载、编译并缓存，对结构体实例评估后产出标签集合。
package tagengine

import (
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// ErrUnsupportedTarget 目标不是（可解引用的）结构体时返回
	ErrUnsupportedTarget = errors.New("unsupported tag recognition target, expected a struct or pointer to struct")
	// ErrInvalidRego 规则源码不符合标签识别契约时返回
	ErrInvalidRego = errors.Newf("invalid rego rule, contract: package %s with a tags rule", base.GlobalPrefix)
)

// RecognizeResult 标签识别结果。
type RecognizeResult struct {
	// Tags 命中的标签，去重排序后的标签数组
	Tags []string
	// Matched 命中的规则名（RecognizeWithDetail 填充，Recognize 不填充）
	Matched []string
}

// Inputer 输入文档提供者接口。
// 目标模型可实现该接口自定义规则可见的输入文档（如字段投影、脱敏），
// 未实现时引擎默认将结构体 JSON 序列化作为输入文档。
type Inputer interface {
	// TagRecogInput 返回标签识别计算的输入文档
	TagRecogInput() (map[string]any, error)
}
