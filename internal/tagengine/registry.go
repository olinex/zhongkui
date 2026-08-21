package tagengine

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"zhongkui.com/zhongkui/internal/models/network"
)

// Target 表示一个已注册的可打标目标模型。
// 目标模型在全局注册表中登记后，其类型键（pkgpath.TypeName 完整路径）
// 方可作为 TagRule.Model / TagSumming.Model / TagEventCondition.Model 等
// 模型字段的合法取值；DisplayName 为展示名称的 i18n 键，供接口层本地化展示。
type Target struct {
	// Type 目标模型的零值实例（指针），用于计算类型键（如 *network.HttpTransport）
	Type any
	// DisplayName 展示名称的 i18n 键（如 model.network.httpTransport），
	// 翻译文案维护在 internal/pkg/translate/locales 下
	DisplayName string
}

// Option 表示注册表导出的选项项，供接口层作为参数校验（options 列表）与下拉选择的数据源。
type Option struct {
	// Value 模型类型键（完整路径，如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Value string `json:"value"`
	// Label 展示名称的 i18n 键，由接口层本地化后展示
	Label string `json:"label"`
}

// targets 可打标目标模型的静态注册清单，包初始化时构建索引。
// 新增可打标模型时在此登记，并补充对应的 i18n displayname 文案。
var targets = []Target{
	{Type: &network.HttpTransport{}, DisplayName: "model.network.httpTransport"},
}

// registeredTargets 全局模型注册表索引：类型键 → 目标模型元信息。
var registeredTargets sync.Map

// Register 运行时注册一个可打标目标模型。
// 注册表默认由静态清单（targets）在包初始化时构建；本函数供测试注入自定义
// 模型或未来运行时扩展使用。类型键无法计算或重复注册时直接 panic，
// 与静态注册语义保持一致，避免静默的未知模型。
func Register(target Target) {
	typeKey := MustTypeKey(target.Type)
	if _, loaded := registeredTargets.LoadOrStore(typeKey, &target); loaded {
		panic(fmt.Sprintf("tagengine: duplicate target type key: %s", typeKey))
	}
}

// TypeKey 计算目标模型实例的类型键。
// 类型键格式为 pkgpath.TypeName（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport），
// 作为数据库规则的模型分组标识（TagRule.Model 等模型字段的取值）。
// 指针自动解引用。可打标模型须在全局注册表（见 registry.go）中登记。
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

// MustTypeKey 计算目标模型实例的类型键，失败即 panic。
func MustTypeKey(target any) string {
	typeKey, err := TypeKey(target)
	if err != nil {
		panic(err)
	}
	return typeKey
}

// IsRegistered 判断模型类型键是否已注册。
// 接口层校验 TagRule.Model / TagSumming.Model 等模型字段取值时使用。
func IsRegistered(typeKey string) bool {
	_, ok := registeredTargets.Load(typeKey)
	return ok
}

// TargetInfo 返回类型键对应的注册目标模型元信息，未注册时返回 false。
func TargetInfo(typeKey string) (*Target, bool) {
	v, ok := registeredTargets.Load(typeKey)
	if !ok {
		return nil, false
	}
	return v.(*Target), true
}

// DisplayName 返回类型键对应的展示名称 i18n 键，未注册时返回空串。
func DisplayName(typeKey string) string {
	if target, ok := TargetInfo(typeKey); ok {
		return target.DisplayName
	}
	return ""
}

// Options 返回全部已注册目标模型的选项列表，按类型键排序。
// 供接口层作为参数校验（options 列表）与下拉选择的数据源；
// Label 为 i18n 键，展示前需经本地化翻译。
func Options() []Option {
	options := make([]Option, 0)
	registeredTargets.Range(func(k, v any) bool {
		target := v.(*Target)
		options = append(options, Option{Value: k.(string), Label: target.DisplayName})
		return true
	})
	sort.Slice(options, func(i, j int) bool { return options[i].Value < options[j].Value })
	return options
}

// init 构建全局模型注册表索引。
// 注册项的类型键无法计算（非结构体）或类型键重复时直接 panic，
// 避免启动后出现静默的未知模型，保证注册表恒为合法状态的单一事实来源。
func init() {
	for _, target := range targets {
		Register(target)
	}
}
