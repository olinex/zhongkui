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
}

type searchGenerateConfigOrderBy struct {
	FieldName string `toml:"fieldName" validate:"required"`
	Column    string `toml:"column"`
}

// searchGenerateConfigSelect 定义字段投影（selectFields）配置。
// FieldName 为输出 JSON 键（camelCase，与列表 Out 结构体字段一致），
// Column 为对应的数据库列名；两者共同构成字段投影的白名单。
type searchGenerateConfigSelect struct {
	FieldName string `toml:"fieldName" validate:"required"`
	Column    string `toml:"column"`
}

type searchGenerateItem struct {
	OrderByFields  []searchGenerateConfigOrderBy  `toml:"orderByFields"`
	FilterByFields []searchGenerateConfigFilterBy `toml:"filterByFields"`
	SelectFields   []searchGenerateConfigSelect   `toml:"selectFields"`
}

type searchGenerateConfig struct {
	PackageName string
	TimeModule  bool                          `toml:"timeModule"`
	Searchs     map[string]searchGenerateItem `toml:"searchs"`
	// EnumFields 结构体字段与枚举类的绑定声明（search.toml 顶层 [[enums]] 数组）。
	// 按结构体类型名组织，其字段通过 set 引用枚举注册表已注册的类名（可选项由该来源定义）。
	// 与 Searchs 同属视图层声明式配置：经 enum_bindings.tmpl 渲染为 enum_bindings.go，
	// 输出字段绑定表（enumBindings），供 meta 自省（enumType）使用。
	EnumFields []enumGenerateConfigType `toml:"enums" validate:"dive"`
}

// enumGenerateConfigField 声明结构体中某个枚举字段与枚举类的绑定关系。
// Set 引用枚举注册表已注册的类名（取值空间固定的枚举由归属包就地声明并注册；业务注册表
// 来源的枚举由所属业务包经 RegisterSource 声明），可选项由 enum 注册表统一定义，本处仅作
// "字段 → 枚举类名"绑定。
type enumGenerateConfigField struct {
	Field string `toml:"field" validate:"required"`
	Set   string `toml:"set" validate:"required"`
}

// enumGenerateConfigType 声明某个视图层结构体类型的全部枚举字段。
// TypeName 必须与视图层结构体 reflect.Type.Name() 精确一致（含匿名嵌入字段
// 递归展开时也按外层结构体类型名匹配）。
type enumGenerateConfigType struct {
	TypeName string                    `toml:"typeName" validate:"required"`
	Fields   []enumGenerateConfigField `toml:"fields" validate:"required,min=1,dive"`
}

// enumGenerateTemplateData 是 enum_bindings.tmpl 的渲染数据：[[enums]] 声明展开为
// 字段绑定表，模板只负责输出而无需感知引用关系。
type enumGenerateTemplateData struct {
	PackageName string
	Types       []enumGenerateTemplateType
}

// enumGenerateTemplateType 模板渲染用的结构体类型声明。
type enumGenerateTemplateType struct {
	TypeName string
	Fields   []enumGenerateTemplateField
}

// enumGenerateTemplateField 模板渲染用的字段绑定声明，Set 为枚举类名。
type enumGenerateTemplateField struct {
	Field string
	Set   string
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
		enumBindingsTmplPath, err := flags.GetString("enumBindingsTemplatePath")
		if err != nil {
			panic(fmt.Sprintf("Invalid enum bindings template path: %v", err))
		}
		enumBindingsOutputPath, err := flags.GetString("enumBindingsOutputPath")
		if err != nil {
			panic(fmt.Sprintf("Invalid enum bindings output path: %v", err))
		}
		// 枚举绑定模板与输出路径须同时提供：仅当两者齐全时才生成 enum_bindings.go
		genEnum := enumBindingsTmplPath != "" && enumBindingsOutputPath != ""

		tempBytes, err := os.ReadFile(tmplPath)
		if err != nil {
			panic(fmt.Sprintf("Invalid template file: %v", err))
		}
		tmpl, err := template.New("search-generate").Funcs(funcMaps).Parse(string(tempBytes))
		if err != nil {
			panic(fmt.Sprintf("Invalid template file: %v", err))
		}
		var enumTmpl *template.Template
		if genEnum {
			enumBytes, err := os.ReadFile(enumBindingsTmplPath)
			if err != nil {
				panic(fmt.Sprintf("Invalid enum template file: %v", err))
			}
			enumTmpl, err = template.New("enum-generate").Parse(string(enumBytes))
			if err != nil {
				panic(fmt.Sprintf("Invalid enum template file: %v", err))
			}
		}
		cfgBytes, err := os.ReadFile(cfgPath)
		if err != nil {
			panic(fmt.Sprintf("Invalid configs file: %v", err))
		}
		cfg := searchGenerateConfig{
			PackageName: packageName,
			TimeModule:  true,
		}
		// Searchs 与 EnumFields 分两次解析：go-toml 无法在同一文档中混排顶层命名表
		// （[XxxListGet]）与 [[enums]] 数组表，故：
		//   - EnumFields 经 wrapper 结构体直接解析（其余顶层表被宽松忽略）
		//   - Searchs 先按通用 map 解码、剔除 enums 键后回写再解析
		var enumsDoc struct {
			Enums []enumGenerateConfigType `toml:"enums"`
		}
		if err = toml.Unmarshal(cfgBytes, &enumsDoc); err != nil {
			panic(fmt.Sprintf("Invalid toml config: %v", err))
		}
		cfg.EnumFields = enumsDoc.Enums
		var doc map[string]any
		if err = toml.Unmarshal(cfgBytes, &doc); err != nil {
			panic(fmt.Sprintf("Invalid toml config: %v", err))
		}
		delete(doc, "enums")
		if searchBytes, err := toml.Marshal(doc); err != nil {
			panic(fmt.Sprintf("Invalid toml config: %v", err))
		} else if err = toml.Unmarshal(searchBytes, &cfg.Searchs); err != nil {
			panic(fmt.Sprintf("Invalid toml config: %v", err))
		}
		if err = validate.Struct(&cfg); err != nil {
			panic(fmt.Sprintf("Invalid configs: %v", err))
		}
		for fieldName, item := range cfg.Searchs {
			for i := range item.FilterByFields {
				field := &item.FilterByFields[i]
				if err = actionValidate(field.FieldType, field.Actions); err != nil {
					panic(fmt.Sprintf("Invalid action for field %s: %v", fieldName, err))
				}
			}
			// selectFields 必填校验：字段投影为接口层必需能力，每个搜索类型必须配置非空白名单
			if len(item.SelectFields) == 0 {
				panic(fmt.Sprintf("Search %s must define selectFields", fieldName))
			}
			// selectFields 校验：fieldName 非空（validate 已保证）且在同一类型内不得重复
			seenSelect := make(map[string]struct{}, len(item.SelectFields))
			for _, field := range item.SelectFields {
				if _, dup := seenSelect[field.FieldName]; dup {
					panic(fmt.Sprintf("Duplicate selectField %s in %s", field.FieldName, fieldName))
				}
				seenSelect[field.FieldName] = struct{}{}
			}
		}
		// 同一结构体类型内字段名去重，避免生成的枚举绑定表产生歧义；
		// [[enums]] 的 set 仅作为"字段 → 枚举类名"绑定引用，类可来自归属包的就地声明
		// 或业务包经 RegisterSource 声明的运行时类，故此处不校验 set 是否声明。
		templateTypes := make([]enumGenerateTemplateType, 0, len(cfg.EnumFields))
		for _, t := range cfg.EnumFields {
			seen := make(map[string]struct{}, len(t.Fields))
			templateFields := make([]enumGenerateTemplateField, 0, len(t.Fields))
			for _, f := range t.Fields {
				if _, dup := seen[f.Field]; dup {
					panic(fmt.Sprintf("Duplicate enum field %q in type %s", f.Field, t.TypeName))
				}
				seen[f.Field] = struct{}{}
				templateFields = append(templateFields, enumGenerateTemplateField{
					Field: f.Field,
					Set:   f.Set,
				})
			}
			templateTypes = append(templateTypes, enumGenerateTemplateType{
				TypeName: t.TypeName,
				Fields:   templateFields,
			})
		}
		outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			panic(fmt.Sprintf("Invalid output file: %v", err))
		}
		err = tmpl.Execute(outputFile, cfg)
		if err != nil {
			outputFile.Close()
			panic(fmt.Sprintf("Failed to execute template: %v", err))
		}
		outputFile.Close()
		// 可选：渲染枚举字段绑定表 enum_bindings.go（由 search.toml 的 [[enums]] 声明驱动）
		if !genEnum {
			return
		}
		enumOutputFile, err := os.OpenFile(enumBindingsOutputPath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
		if err != nil {
			panic(fmt.Sprintf("Invalid enum output file: %v", err))
		}
		defer enumOutputFile.Close()
		err = enumTmpl.Execute(enumOutputFile, enumGenerateTemplateData{
			PackageName: packageName,
			Types:       templateTypes,
		})
		if err != nil {
			panic(fmt.Sprintf("Failed to execute enum bindings template: %v", err))
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
	// 枚举字段绑定表生成（可选）：同时提供时才渲染 enum_bindings.go，数据源为同一 configPath 的 [[enums]] 声明
	flags.String("enumBindingsTemplatePath", "", "Path to the enum bindings template file")
	flags.String("enumBindingsOutputPath", "", "Path where enum bindings will be generated")

	genFallbackFlags := genFallbackPcapCmd.Flags()
	genFallbackFlags.String("output", "", "Output path for the pcap file")
	genFallbackFlags.Int("numPackets", 0, "Number of packets to generate")

	RootCmd.AddCommand(searchGenerateCmd)
	RootCmd.AddCommand(genFallbackPcapCmd)
}

func main() {
	RootCmd.Execute()
}
