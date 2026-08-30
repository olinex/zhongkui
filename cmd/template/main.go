package main

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/go-playground/validator/v10"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	bench "zhongkui.com/zhongkui/internal/benchmark/capture"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
)

var (
	validate                  = validator.New(validator.WithRequiredStructEnabled())
	funcMaps template.FuncMap = template.FuncMap(map[string]any{
		"join":       strings.Join,
		"capitalize": serializer.Capitalize,
		// hasRequired 判断过滤字段集合中是否存在必选字段（Required=true）。
		// 存在时生成的 SearchIn.FilterBy 为非指针值类型，使必选校验在请求省略 filterBy 时同样生效。
		"hasRequired": func(fields []searchGenerateConfigFilterBy) bool {
			for _, field := range fields {
				if field.Required {
					return true
				}
			}
			return false
		},
	})
	StringActions map[string]any = map[string]any{
		"nil":   struct{}{},
		"nnil":  struct{}{},
		"eq":    struct{}{},
		"neq":   struct{}{},
		"pfx":   struct{}{},
		"npfx":  struct{}{},
		"ipfx":  struct{}{},
		"inpfx": struct{}{},
		"sfx":   struct{}{},
		"nsfx":  struct{}{},
		"isfx":  struct{}{},
		"insfx": struct{}{},
		"ctn":   struct{}{},
		"nctn":  struct{}{},
		"ictn":  struct{}{},
		"inctn": struct{}{},
	}
	NumberActions map[string]any = map[string]any{
		"nil":  struct{}{},
		"nnil": struct{}{},
		"eq":   struct{}{},
		"neq":  struct{}{},
		"lt":   struct{}{},
		"lte":  struct{}{},
		"gt":   struct{}{},
		"gte":  struct{}{},
	}
	BooleanActions = map[string]any{
		"nil":  struct{}{},
		"nnil": struct{}{},
		"eq":   struct{}{},
	}
	// HexUUIDActions 定义 serializer.HexUUID 类型支持的过滤动作。
	// UUID 为等值标识，仅支持精确匹配与不匹配。
	HexUUIDActions = map[string]any{
		"eq":  struct{}{},
		"neq": struct{}{},
	}
	// IPAddrActions 定义 serializer.IPv4Addr / IPv6Addr / IPAddr 类型支持的过滤动作。
	// IP 地址为等值标识，仅支持精确匹配与不匹配。
	IPAddrActions = map[string]any{
		"eq":  struct{}{},
		"neq": struct{}{},
	}
	DatetimeActions = NumberActions
	DateActions     = NumberActions
	TimeActions     = NumberActions
)

type searchGenerateConfigOption struct {
	Title string `toml:"title" validate:"required"`
	Value string `toml:"value" validate:"required"`
}

type searchGenerateConfigFilterBy struct {
	FieldName string   `toml:"fieldName" validate:"required"`
	FieldType string   `toml:"fieldType" validate:"required,oneof=string int uint int8 uint8 int16 uint16 int32 uint32 int64 uint64 float32 float64 bool time.Time serializer.TimeOnly serializer.DateOnly serializer.HexUUID serializer.IPv4Addr serializer.IPv6Addr serializer.IPAddr"`
	Column    string   `toml:"column"`
	Actions   []string `toml:"actions" validate:"required,oneof=eq neq pfx npfx ipfx inpfx sfx nsfx isfx insfx ctn nctn ictn inctn nil nnil lt lte gt gte"`
	Binding   string   `toml:"binding" validate:"required"`
	Required  bool     `toml:"required"` // 必选过滤字段：生成的 FilterBy 字段为非指针并带 binding:"required"
	// TextValue 是否以文本形式传值：为 true 时生成的 GetValue 返回值的 String() 文本。
	// 用于 ClickHouse 原生 IPv4/IPv6 类型列的字符串隐式比较（列存二进制，比较时自动提升/转换）
	TextValue bool `toml:"textValue"`
	// Enum 是否为枚举字段：为 true 时生成的 Type 为 "enum"，并携带 Options 下拉可选值。
	// fieldType 仍保持 "string"（枚举值以字符串存储），仅过滤 UI 形态变化。
	Enum    bool                         `toml:"enum"`
	Options []searchGenerateConfigOption `toml:"options"`
}

type searchGenerateConfigOrderBy struct {
	FieldName string `toml:"fieldName" validate:"required"`
	Column    string `toml:"column"`
}

type searchGenerateItem struct {
	OrderByFields  []searchGenerateConfigOrderBy  `toml:"orderByFields"`
	FilterByFields []searchGenerateConfigFilterBy `toml:"filterByFields"`
}

type searchGenerateConfig struct {
	PackageName string
	TimeModule  bool                          `toml:"timeModule"`
	Searchs     map[string]searchGenerateItem `toml:"searchs"`
}

func actionValidate(fieldType string, actions []string) error {
	for _, action := range actions {
		var allowActions map[string]any
		switch fieldType {
		case "string":
			allowActions = StringActions
		case "int", "uint", "int8", "uint8", "int16", "uint16", "int32", "uint32", "int64", "uint64", "float32", "float64":
			allowActions = NumberActions
		case "bool":
			allowActions = BooleanActions
		case "serializer.HexUUID":
			allowActions = HexUUIDActions
		case "serializer.IPv4Addr", "serializer.IPv6Addr", "serializer.IPAddr":
			allowActions = IPAddrActions
		case "time.Time":
			allowActions = DatetimeActions
		case "serializer.TimeOnly":
			allowActions = TimeActions
		case "serializer.DateOnly":
			allowActions = DateActions
		default:
			return errors.Newf("Invalid action: %s for %s type", action, fieldType)
		}
		if value := allowActions[action]; value != nil {
			continue
		}
	}
	return nil
}

var searchGenerateCmd = &cobra.Command{
	Use:   "search-generate",
	Short: "Generate search models",
	Run: func(cmd *cobra.Command, arg []string) {
		flags := cmd.Flags()
		packageName, err := flags.GetString("packageName")
		if err != nil {
			panic(fmt.Sprintf("Invalid package name: %v", err))
		}
		if packageName == "" {
			panic("package name is required")
		}
		tmplPath, err := flags.GetString("templatePath")
		if err != nil {
			panic(fmt.Sprintf("Invalid template path: %v", err))
		}
		if tmplPath == "" {
			panic("template path is required")
		}
		cfgPath, err := flags.GetString("configPath")
		if err != nil {
			panic(fmt.Sprintf("Invalid configs path: %v", err))
		}
		if cfgPath == "" {
			panic("configs path is required")
		}
		outputPath, err := flags.GetString("outputPath")
		if err != nil {
			panic(fmt.Sprintf("Invalid output path: %v", err))
		}
		if outputPath == "" {
			panic("output path is required")
		}
		tempBytes, err := os.ReadFile(tmplPath)
		if err != nil {
			panic(fmt.Sprintf("Invalid template file: %v", err))
		}
		tmpl, err := template.New("search-generate").Funcs(funcMaps).Parse(string(tempBytes))
		if err != nil {
			panic(fmt.Sprintf("Invalid template file: %v", err))
		}
		cfgBytes, err := os.ReadFile(cfgPath)
		if err != nil {
			panic(fmt.Sprintf("Invalid configs file: %v", err))
		}
		cfg := searchGenerateConfig{
			PackageName: packageName,
			TimeModule:  true,
		}
		err = toml.Unmarshal(cfgBytes, &cfg.Searchs)
		if err != nil {
			panic(fmt.Sprintf("Invalid toml config: %v", err))
		}
		err = validate.Struct(&cfg)
		if err != nil {
			panic(fmt.Sprintf("Invalid configs: %v", err))
		}
		for fieldName, item := range cfg.Searchs {
			for _, field := range item.FilterByFields {
				if err = actionValidate(field.FieldType, field.Actions); err != nil {
					panic(fmt.Sprintf("Invalid action for field %s: %v", fieldName, err))
				}
				// 枚举字段必须携带下拉可选值，非枚举字段不得携带
				if field.Enum && len(field.Options) == 0 {
					panic(fmt.Sprintf("Enum field %s must define options", fieldName))
				}
				if !field.Enum && len(field.Options) > 0 {
					panic(fmt.Sprintf("Options defined for non-enum field %s", fieldName))
				}
			}
		}
		outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			panic(fmt.Sprintf("Invalid output file: %v", err))
		}
		defer outputFile.Close()
		err = tmpl.Execute(outputFile, cfg)
		if err != nil {
			panic(fmt.Sprintf("Failed to execute template: %v", err))
		}
	},
}

var RootCmd = &cobra.Command{
	Use:   "api",
	Short: "The seismograph api group command lines",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

var genFallbackPcapCmd = &cobra.Command{
	Use:   "gen-fallback-pcap",
	Short: "Generate fallback pcap file",
	Run: func(cmd *cobra.Command, args []string) {
		flags := cmd.Flags()
		outputPath, err := flags.GetString("output")
		if err != nil {
			panic(fmt.Sprintf("Invalid output path: %v", err))
		}
		if outputPath == "" {
			panic("output path is required")
		}
		numPackets, err := flags.GetInt("numPackets")
		if err != nil {
			panic(fmt.Sprintf("Invalid numPackets: %v", err))
		}
		if err := bench.GenerateFallbackPcap(outputPath, numPackets); err != nil {
			panic(err)
		}
	},
}

func init() {
	flags := searchGenerateCmd.Flags()
	flags.String("packageName", "", "Package name")
	flags.String("templatePath", "", "Path to the template file")
	flags.String("configPath", "", "Path to the toml configs file")
	flags.String("outputPath", "", "Path to the directory where the models will be generated")

	genFallbackFlags := genFallbackPcapCmd.Flags()
	genFallbackFlags.String("output", "", "Output path for the pcap file")
	genFallbackFlags.Int("numPackets", 0, "Number of packets to generate")

	RootCmd.AddCommand(searchGenerateCmd)
	RootCmd.AddCommand(genFallbackPcapCmd)
}

func main() {
	RootCmd.Execute()
}
