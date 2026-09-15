package tagengine

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"zhongkui.com/zhongkui/internal/models/network"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	// registeredTargets 全局模型注册表索引：类型键 → 目标模型元信息。
	registeredTargets sync.Map

	// expectedIDType 目标模型 ID 字段的共享类型锚点（UUID）。
	// 所有可打标目标模型必须具有唯一 ID 字段且类型必须为该锚点，
	// 保证 tag_summings.ids（Array(UUID)）列类型跨模型统一。
	expectedIDType = reflect.TypeFor[serializer.HexUUID]()
)

// Target 表示一个已注册的可打标目标模型。
// 目标模型在全局注册表中登记后，其类型键（pkgpath.TypeName 完整路径）
// 方可作为 TagRule.Model / TagSumming.Model / TagEventCondition.Model 等
// 模型字段的合法取值；展示 label 不再由注册表人工维护，统一经 tagRuleModel
// 枚举类（Key 取 ModelKey，见 enum_registry.go）按 enum.tagRuleModel.<ModelKey> 推导。
// 每个目标模型必须具有唯一 ID 字段且类型为 serializer.HexUUID（共享类型锚点），
// 与 ClickHouse tag_summings.ids（Array(UUID)）列类型保持一致，保证打标结果
// 跨模型统一以 UUID 锚定数据行。
type Target struct {
	// Type 目标模型的零值实例（指针），用于计算类型键（如 *network.HttpTransport）
	Type any
	// Dimensions 该模型合法标签维度取值列表（snake_case 列名，对应
	// tag_summings.dimension 的存储值，须与 ClickHouse 物化视图的展开列保持一致）。
	// 未登记任何维度的模型不参与标签维度统计，dimension 下拉不出选项。
	Dimensions []string
}

// targets 可打标目标模型的静态注册清单，包初始化时构建索引。
// 新增可打标模型时在此登记；展示 label 无需人工维护，经 tagRuleModel 枚举类
// 按 enum.tagRuleModel.<ModelKey> 推导。
// 参与标签维度统计的模型须同时登记 Dimensions（与 ClickHouse 物化视图展开列一致）。
// 所有登记模型必须满足 Register 的 ID 约束：具有唯一 ID 字段且类型为
// serializer.HexUUID（对应 ClickHouse tag_summings.ids Array(UUID) 列）。
var targets = []Target{
	{
		Type: &network.HttpTransport{},
		Dimensions: []string{
			"fpTcpClientSig",
			"fpTcpServerSig",
		},
	},
}

// Register 运行时注册一个可打标目标模型。
// 注册表默认由静态清单（targets）在包初始化时构建；本函数供测试注入自定义
// 模型或未来运行时扩展使用。目标模型必须具有唯一 ID 字段且类型为
// serializer.HexUUID（与 ClickHouse tag_summings.ids Array(UUID) 列对齐），
// 否则直接 panic。类型键无法计算或重复注册时同样 panic，
// 与静态注册语义保持一致，避免静默的未知模型。
func Register(target Target) {
	typeKey := MustTypeKey(target.Type)
	t := reflect.TypeOf(target.Type)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	idField, ok := t.FieldByName("ID")
	if !ok {
		panic(fmt.Sprintf("tagengine: target %s must have a unique ID field", typeKey))
	}
	if idField.Type != expectedIDType {
		panic(fmt.Sprintf("tagengine: target %s ID field type %s must be %s", typeKey, idField.Type, expectedIDType))
	}
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

// ModelKey 返回目标模型类型键的短展示键（camelCase），作为该模型枚举 Option.Key 的唯一来源。
// 取类型键（pkgpath.TypeName）末段并将首字母转小写（如 …HttpTransport → httpTransport）；
// 对已是枚举键的输入幂等（不含 `.` 时回退原串再转小驼峰），因此可安全处理
// 完整类型键 / 枚举键两种入参。与前端 modelTypeKeyToI18nKey 保持同一推导规则，前后端单源一致。
func ModelKey(typeKey string) string {
	last := typeKey
	if i := strings.LastIndexByte(typeKey, '.'); i >= 0 {
		last = typeKey[i+1:]
	}
	if last == "" {
		return ""
	}
	return strings.ToLower(last[:1]) + last[1:]
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

// Dimensions 返回已注册模型合法标签维度取值列表（snake_case 列名，
// 对应 tag_summings.dimension 的存储值），未注册模型返回 nil。
// 供接口层作为 dimension 下拉选择的数据源，保证前后端维度配置同步。
func Dimensions(typeKey string) []string {
	if target, ok := TargetInfo(typeKey); ok {
		return target.Dimensions
	}
	return nil
}

// AllDimensions 返回全部已注册目标模型的合法标签维度取值（snake_case 列名）的
// 跨模型去重并集，按取值升序排序。供视图层组装 tagEventDimension 枚举类条目
// （Key/Value 均取维度取值，数据库存储语义）与参数校验使用；维度展示 label 不在
// 此输出，统一经 tagEventDimension 枚举类按 enum.tagEventDimension.<取值> 本地化。
func AllDimensions() []string {
	seen := make(map[string]struct{})
	for _, typeKey := range TypeKeys() {
		for _, dim := range Dimensions(typeKey) {
			seen[dim] = struct{}{}
		}
	}
	dims := make([]string, 0, len(seen))
	for d := range seen {
		dims = append(dims, d)
	}
	sort.Strings(dims)
	return dims
}

// TypeKeys 返回全部已注册目标模型的类型键列表，按类型键升序排序。
// 供接口层组装 tagRuleModel 枚举类条目（Key 取 ModelKey(typeKey)、Value 取完整
// 类型键）与参数校验使用；模型展示 label 不在此输出，统一经 tagRuleModel 枚举类
// 按 enum.tagRuleModel.<ModelKey> 经统一枚举接口本地化。
func TypeKeys() []string {
	keys := make([]string, 0)
	registeredTargets.Range(func(k, _ any) bool {
		keys = append(keys, k.(string))
		return true
	})
	sort.Strings(keys)
	return keys
}

// init 构建全局模型注册表索引。
// 注册项的类型键无法计算（非结构体）或类型键重复时直接 panic，
// 避免启动后出现静默的未知模型，保证注册表恒为合法状态的单一事实来源。
func init() {
	for _, target := range targets {
		Register(target)
	}
}
