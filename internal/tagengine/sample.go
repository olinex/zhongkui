package tagengine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/open-policy-agent/opa/v1/rego"

	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
)

// SampleRuntimeError 描述一条验证样例的运行期评估错误。
type SampleRuntimeError struct {
	// SampleName 出错样例的名称
	SampleName string `json:"sampleName"`
	// Error 错误消息文本（反序列化失败或规则评估报错）
	Error string `json:"error"`
}

// ErrModelNotRegistered 目标模型未在引擎注册表中注册，无法执行运行期样例校验。
// 未注册模型的规则引擎永远不会加载（按实际数据实例的类型键分组），属于死规则，
// 写入路径应在调用本函数前基于 IsRegistered 直接拒绝。
var ErrModelNotRegistered = errors.New("target model is not registered")

// ErrSampleNotProvisioned 目标模型已注册但尚未预置验证样例，运行期校验被跳过。
// 属于降级放行：调用方应记录告警后继续（规则仅有第一层语法/契约保障）。
var ErrSampleNotProvisioned = errors.New("no validation samples provisioned for target model")

// ValidateGroupRuntime 对候选规则组执行运行期样例校验（第二层合法性）。
// 依据组内规则的目标模型类型键（model）从 tag_rule_samples 表读取全部验证样例，
// 将每条样例 JSON 反序列化为目标模型实例，经 BuildInput 构建输入文档后
// 以组级合并查询（与引擎 Recognize 的评估方式一致）逐一评估；
// 任一样例反序列化失败或评估报错，即认为规则运行期不合法，返回对应错误列表。
// 返回值语义：
//   - 规则组为空：返回 nil, nil（无校验对象）；
//   - 目标模型未注册：返回 nil, ErrModelNotRegistered（无法校验）；
//   - 该模型无样例数据：返回 nil, ErrSampleNotProvisioned（降级，由调用方决定是否放行）；
//   - 正常校验：返回出错样例列表（空表示全部通过）与 nil 错误。
func ValidateGroupRuntime(ctx context.Context, db *database.DB, rules []*tag.TagRule) ([]SampleRuntimeError, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	target, ok := TargetInfo(rules[0].Model)
	if !ok {
		return nil, errors.WithStack(ErrModelNotRegistered)
	}
	var samples []*tag.TagRuleSample
	if err := db.WithContext(ctx).Model(&tag.TagRuleSample{}).
		Where("model = ?", rules[0].Model).
		Find(&samples).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	if len(samples) == 0 {
		return nil, errors.WithStack(ErrSampleNotProvisioned)
	}
	// 运行期样例校验启用 StrictBuiltinErrors：OPA 默认将 builtin 类型错误
	// （如 count(42)）视为规则体不满足而吞掉，严格模式让其浮出为评估错误，
	// 从而能捕捉"编译通过但作用到数据时运行期报错"的不合法规则。
	group, err := CompileGroup(ctx, rules, rego.StrictBuiltinErrors(true))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	// 基于注册目标的零值实例类型新建目标实例（指针自动解引用，兼容值类型注册）
	typ := reflect.TypeOf(target.Type)
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	issues := make([]SampleRuntimeError, 0, len(samples))
	for _, sample := range samples {
		instance := reflect.New(typ).Interface()
		if err := json.Unmarshal(sample.Input, instance); err != nil {
			issues = append(issues, SampleRuntimeError{
				SampleName: sample.Name,
				Error:      fmt.Sprintf("sample cannot be deserialized into target model: %v", err),
			})
			continue
		}
		input, err := BuildInput(instance)
		if err != nil {
			issues = append(issues, SampleRuntimeError{
				SampleName: sample.Name,
				Error:      fmt.Sprintf("build input failed: %v", err),
			})
			continue
		}
		if _, err := group.Recognize(ctx, rego.EvalInput(input)); err != nil {
			issues = append(issues, SampleRuntimeError{
				SampleName: sample.Name,
				Error:      err.Error(),
			})
		}
	}
	if len(issues) == 0 {
		return nil, nil
	}
	return issues, nil
}
