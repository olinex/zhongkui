// Package config 提供应用程序配置管理的基础设施，包括：
//   - 基于 Viper 的配置文件加载与环境变量绑定
//   - 基于反射的配置结构体元信息提取与描述
//   - 泛型配置提取与校验
//
// 该包是整个应用配置体系的核心，被 logging、etcd、redis、s3 等子包引用，
// 每个子包通过 DescribeConfig 和 ExtractConfig 泛型函数实现各自的配置描述与提取。
//
// 典型用法：
//
//	// 在子包中定义配置前缀
//	const PREFIX = "zap"
//
//	// 生成配置描述文档
//	descriptions := config.DescribeConfig[Config]("zap.default")
//
//	// 从 viper 实例提取配置
//	cfg, err := config.ExtractConfig[Config](v, "zap.default")
package config

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/creasty/defaults"
	"github.com/mitchellh/mapstructure"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// NewViper 创建一个新的 Viper 实例，用于加载和管理配置。
//
// 该函数封装了 Viper 的初始化流程，依次完成以下操作：
//  1. 设置环境变量前缀（通过 SetEnvPrefix），环境变量格式为 {ENVPREFIX}_{KEY}
//  2. 指定配置文件名和类型（如 config.toml）
//  3. 添加配置文件搜索路径：优先使用 configPath，其次搜索当前目录(.)和上层目录(..)
//  4. 启用自动环境变量绑定（AutomaticEnv），未在配置文件中设置的键可通过环境变量覆盖
//  5. 绑定命令行参数（BindPFlags），命令行参数具有最高优先级
//  6. 读取配置文件（ReadInConfig），若文件不存在或格式错误将触发 panic
//
// 参数：
//   - envPrefix:   环境变量前缀，例如 "zki" 表示环境变量以 SEI_ 开头
//   - configName:  配置文件名（不含扩展名），例如 "config"
//   - configType:  配置文件类型，例如 "toml"、"yaml"、"json"
//   - configPath:  默认配置文件搜索路径，例如 "/etc/zhongkui"
//   - flags:       命令行参数集合，可以为 nil
//
// 返回值：
//   - *viper.Viper: 初始化完成的 Viper 实例
//
// 注意：如果配置文件读取失败或命令行参数绑定失败，函数会 panic。
// 调用方应确保配置文件存在于搜索路径中，或在调用前做好 recover 处理。
func NewViper(
	envPrefix string,
	configName string,
	configType string,
	configPath string,
	flags *pflag.FlagSet,
) *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(envPrefix)
	v.AutomaticEnv()

	// 如果命令行通过 --config / -c 指定了配置文件路径，则直接加载该文件
	if flags != nil {
		if configFlag := flags.Lookup("config"); configFlag != nil {
			if configFile := configFlag.Value.String(); configFile != "" {
				v.SetConfigFile(configFile)
				if err := v.BindPFlags(flags); err != nil {
					panic(err)
				}
				if err := v.ReadInConfig(); err != nil {
					panic(err)
				}
				return v
			}
		}
	}

	v.SetConfigName(configName)
	v.SetConfigType(configType)
	v.AddConfigPath(configPath)
	v.AddConfigPath(".")
	v.AddConfigPath("..")

	if err := v.BindPFlags(flags); err != nil {
		panic(err)
	}
	if err := v.ReadInConfig(); err != nil {
		panic(err)
	}
	return v
}

// NewDefaultViper 创建一个使用默认参数的 Viper 实例。
//
// 默认参数说明：
//   - 环境变量前缀: base.GLOBAL_PREFIX（即 "zki"）
//   - 配置文件名:   "config"
//   - 配置文件类型: "toml"
//   - 默认搜索路径: "/etc/zhongkui"
//
// 参数：
//   - flags: 命令行参数集合，可以为 nil
//
// 返回值：
//   - *viper.Viper: 使用默认参数初始化的 Viper 实例
func NewDefaultViper(flags *pflag.FlagSet) *viper.Viper {
	return NewViper(base.GLOBAL_PREFIX, "config", "toml", "/etc/zhongkui", flags)
}

// NewDefaultViperWithCmd 基于 cobra.Command 创建默认的 Viper 实例。
//
// 该函数是 NewDefaultViper 的便捷封装，自动从 cobra.Command 提取 FlagSet，
// 避免调用方手动调用 cmd.Flags()。
//
// 参数：
//   - cmd: cobra.Command 实例，其 Flags() 将被提取并传递给 NewDefaultViper
//
// 返回值：
//   - *viper.Viper: 使用默认参数初始化的 Viper 实例
func NewDefaultViperWithCmd(cmd *cobra.Command) *viper.Viper {
	return NewDefaultViper(cmd.Flags())
}

// ConfigDescription 描述单个配置项的元信息。
//
// 该结构体用于生成配置文档，每个实例对应配置结构体中的一个字段。
// 通过反射从结构体标签（mapstructure、default、comment）中提取信息填充。
//
// 字段说明：
//   - Path:    配置项的完整路径，例如 "zap.default.level"，
//     由前缀和 mapstructure 标签（或小写字段名）拼接而成
//   - Kind:    配置项的 Go 类型名称，例如 "string"、"bool"、"slice"
//   - Default: 配置项的默认值，来自 default 标签
//   - Comment: 配置项的说明文字，来自 comment 标签
type ConfigDescription struct {
	Path    string `json:"path"`
	Kind    string `json:"kind"`
	Default string `json:"default"`
	Comment string `json:"comment"`
}

// ConfigDescriptions 是 ConfigDescription 的切片类型，表示一组配置项描述。
//
// 该类型提供了合并、解析和格式化输出等方法，支持从任意配置结构体
// 通过反射自动提取配置描述信息。
type ConfigDescriptions []ConfigDescription

// Join 将另一个 ConfigDescriptions 合并到当前实例中。
//
// 该方法用于将多个配置模块的描述信息聚合到一起，便于统一输出文档。
// 例如，可以将 logging、etcd、redis 等模块的配置描述合并后再调用 Describe 输出。
//
// 参数：
//   - others: 待合并的配置描述集合
//
// 注意：该方法会修改接收者的内容（通过指针追加）。
func (descs *ConfigDescriptions) Join(others ConfigDescriptions) {
	*descs = append(*descs, others...)
}

// Describe 将配置描述以表格形式输出到标准输出。
//
// 输出包含四列：path（配置路径）、kind（类型）、default（默认值）、comment（说明）。
//
// 返回值：
//   - error: 表格写入或渲染过程中的错误，包装为 errors.WithStack
func (descs ConfigDescriptions) Describe() error {
	data := pterm.TableData{
		{"path", "kind", "default", "comment"},
	}
	for _, desc := range descs {
		data = append(data, []string{desc.Path, desc.Kind, desc.Default, desc.Comment})
	}
	if err := pterm.DefaultTable.WithHasHeader().WithData(data).Render(); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// parse 根据反射值的类型递归解析配置描述信息。
//
// 该方法根据值的 Kind 分派到不同的处理逻辑：
//   - reflect.Ptr:       解引用后递归解析
//   - reflect.Array/Slice: 以元素类型创建新实例递归解析，路径追加 "[]" 表示数组
//   - reflect.Struct:    调用 parseStruct 解析结构体字段
//   - 其他类型:          不做处理（基本类型已在 parseStruct 中作为叶节点记录）
//
// 参数：
//   - value:  当前待解析的反射值
//   - prefix: 当前配置路径前缀，例如 "zap.default"
func (descs *ConfigDescriptions) parse(value reflect.Value, prefix string) {
	switch kind := value.Kind(); kind {
	case reflect.Ptr:
		descs.parse(value.Elem(), prefix)
	case reflect.Array, reflect.Slice:
		descs.parse(reflect.New(value.Type().Elem()), fmt.Sprintf(`%s[]`, prefix))
	case reflect.Struct:
		descs.parseStruct(value, prefix)
	default:
	}
}

// parseStruct 解析结构体的每个公开字段，生成对应的 ConfigDescription。
//
// 对于每个公开字段，该函数会：
//  1. 读取 mapstructure 标签作为配置键名；若无该标签则使用小写字段名
//  2. 拼接前缀生成完整配置路径，例如 "zap.default.level"
//  3. 读取 default 标签作为默认值
//  4. 读取 comment 标签作为说明文字
//  5. 将字段信息追加到 ConfigDescriptions
//  6. 递归调用 parse 解析嵌套结构体、数组等复杂类型
//
// 私有字段（PkgPath != ""）将被跳过。
//
// 参数：
//   - value:  结构体的反射值
//   - prefix: 当前配置路径前缀
func (descs *ConfigDescriptions) parseStruct(value reflect.Value, prefix string) {
	typ := value.Type()
	for i := 0; i < value.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		defaultValue := field.Tag.Get("default")
		comment := field.Tag.Get("comment")
		mapstructure := field.Tag.Get("mapstructure")
		if mapstructure == "" {
			mapstructure = strings.ToLower(field.Name)
		}
		if prefix != "" {
			mapstructure = fmt.Sprintf("%s.%s", prefix, mapstructure)
		}
		fieldValue := value.Field(i)
		*descs = append(*descs, ConfigDescription{
			Path:    mapstructure,
			Kind:    fieldValue.Kind().String(),
			Default: defaultValue,
			Comment: comment,
		})
		descs.parse(fieldValue, mapstructure)
	}
}

// DescribeConfig 通过反射提取泛型类型 C 的配置描述信息。
//
// 该函数是配置描述体系的核心入口，被各子包（logging、etcd 等）的
// DescribeConfig 函数间接调用。它会创建类型 C 的零值实例，
// 然后通过反射递归遍历所有公开字段，提取 mapstructure、default、comment 标签信息。
//
// 类型参数：
//   - C: 配置结构体类型，通常包含 mapstructure、default、comment 等结构体标签
//
// 参数：
//   - prefix: 配置路径前缀，例如 "zap.default"、"etcd.{primary,secondary}"
//     前缀会被拼接到每个字段的配置路径中
//
// 返回值：
//   - ConfigDescriptions: 提取到的配置描述集合，按结构体字段声明顺序排列
//
// 使用示例：
//
//	type DBConfig struct {
//	    Host     string `mapstructure:"host" default:"localhost" comment:"database host"`
//	    Port     int    `mapstructure:"port" default:"5432" comment:"database port"`
//	    ReadOnly bool   `mapstructure:"readOnly" default:"false" comment:"read only mode"`
//	}
//	descs := DescribeConfig[DBConfig]("db.primary")
//	// 结果:
//	// [{Path:"db.primary.host", Kind:"string", Default:"localhost", Comment:"database host"},
//	//  {Path:"db.primary.port", Kind:"int", Default:"5432", Comment:"database port"},
//	//  {Path:"db.primary.readOnly", Kind:"bool", Default:"false", Comment:"read only mode"}]
func DescribeConfig[C any](prefix string) ConfigDescriptions {
	descs := make(ConfigDescriptions, 0)
	descs.parse(reflect.New(reflect.TypeFor[C]()), prefix)
	return descs
}

// ExtractConfig 从 Viper 实例中提取并校验泛型类型的配置。
//
// 该函数执行以下四个步骤：
//  1. 应用默认值：通过 defaults.Set 将结构体 default 标签的值设置到零值实例上
//  2. 提取子配置：通过 viper.Sub(prefix) 获取指定前缀下的子配置树
//  3. 反序列化：将子配置树通过 Unmarshal 映射到配置结构体（覆盖默认值）
//  4. 校验：通过 validator.Struct 对配置结构体执行结构化校验（如 required、min 等标签）
//
// 任何步骤失败都会返回错误，不会继续执行后续步骤。
// 若 viper.Sub 返回 nil（即指定前缀在配置中不存在），返回格式化的错误信息。
//
// 类型参数：
//   - C: 配置结构体类型，应包含 mapstructure、default 和 validate 等标签
//
// 参数：
//   - v:      已加载配置的 Viper 实例
//   - prefix: 配置键前缀，例如 "zap.default"、"etcd.main"
//
// 返回值：
//   - *C:    成功时返回填充并校验通过的配置结构体指针
//   - error: 任一步骤失败时返回的错误，均通过 errors.WithStack 或 errors.Newf 包装
//
// 使用示例：
//
//	v := viper.New()
//	v.Set("db.primary.host", "10.0.0.1")
//	v.Set("db.primary.port", 3306)
//
//	cfg, err := ExtractConfig[DBConfig](v, "db.primary")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(cfg.Host) // "10.0.0.1"
func ExtractConfig[C any](v *viper.Viper, prefix string) (*C, error) {
	var cfg C
	validate := validator.MustValidate()
	if err := defaults.Set(&cfg); err != nil {
		return nil, errors.WithStack(err)
	} else if sub := v.Sub(prefix); sub == nil {
		return nil, errors.Newf(`Cannot get %s %T config from nil`, prefix, cfg)
	} else if err := sub.Unmarshal(&cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.TextUnmarshallerHookFunc(),
			mapstructure.StringToTimeDurationHookFunc(),
			mapstructure.StringToSliceHookFunc(","),
		),
	)); err != nil {
		return nil, errors.WithStack(err)
	} else if err := validate.Struct(&cfg); err != nil {
		return nil, errors.WithStack(err)
	}
	return &cfg, nil
}
