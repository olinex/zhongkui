// Package tagengine 提供基于 Open Policy Agent(OPA) 的标签识别引擎与基于标签聚合统计的标签事件识别能力。
// 规则以 Rego 源码形式存储于数据库，引擎按目标模型类型键分组
// 加载、编译并缓存，对结构体实例评估后产出标签集合；
// 标签事件识别基于 tag_summings 聚合统计（Detect）产出标签事件结果，供定时检测与 dry-run 复用。
package tagengine

import (
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	// ErrInvalidRego 规则源码不符合标签识别契约时返回
	ErrInvalidRego = errors.Newf("invalid rego rule, contract: package %s with a tags rule", base.GlobalPrefix)
	// ErrUnsupportedTarget 目标类型不支持（空类型或非结构体）时返回
	ErrUnsupportedTarget = errors.New("tagengine: unsupported target type")
)

// RecognizeResult 标签识别结果。
type RecognizeResult struct {
	// Tags 命中的标签，去重排序后的标签数组
	Tags []string
	// Matched 命中的规则名（RecognizeWithDetail 填充，Recognize 不填充）
	Matched []string
}

// RecognizeDetailResult 标签识别详细结果。
type RecognizeDetailResult struct {
	RecognizeResult
	// Error 识别过程中遇到的错误（RecognizeWithDetail 填充，Recognize 不填充）
	Error map[string]error
	// Duration 识别耗时
	Duration serializer.DurationNS
}

// Inputer 输入文档提供者接口。
// 目标模型可实现该接口自定义规则可见的输入文档（如字段投影、脱敏），
// 未实现时引擎默认将结构体 JSON 序列化作为输入文档。
type Inputer interface {
	// TagRecogInput 返回标签识别计算的输入文档
	TagRecogInput() (map[string]any, error)
}
