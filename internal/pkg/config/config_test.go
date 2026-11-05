package config

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/go-playground/validator/v10"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// TestConfigDescription 测试 ConfigDescription 结构体
func TestConfigDescription(t *testing.T) {
	Convey("测试 ConfigDescription 结构体", t, func() {
		Convey("应正确创建并填充所有字段", func() {
			desc := ConfigDescription{
				Path:    "app.server.host",
				Kind:    "string",
				Default: "localhost",
				Comment: "服务器监听地址",
			}
			So(desc.Path, ShouldEqual, "app.server.host")
			So(desc.Kind, ShouldEqual, "string")
			So(desc.Default, ShouldEqual, "localhost")
			So(desc.Comment, ShouldEqual, "服务器监听地址")
		})

		Convey("零值 ConfigDescription 的字段应为空", func() {
			var desc ConfigDescription
			So(desc.Path, ShouldEqual, "")
			So(desc.Kind, ShouldEqual, "")
			So(desc.Default, ShouldEqual, "")
			So(desc.Comment, ShouldEqual, "")
		})

		Convey("应支持 JSON 序列化标签", func() {
			desc := ConfigDescription{
				Path:    "db.host",
				Kind:    "string",
				Default: "127.0.0.1",
				Comment: "数据库地址",
			}
			jsonBytes, err := json.Marshal(desc)
			So(err, ShouldBeNil)
			So(string(jsonBytes), ShouldContainSubstring, `"path"`)
			So(string(jsonBytes), ShouldContainSubstring, `"kind"`)
			So(string(jsonBytes), ShouldContainSubstring, `"default"`)
			So(string(jsonBytes), ShouldContainSubstring, `"comment"`)
		})
	})
}

// TestConfigDescriptions_Join 测试 ConfigDescriptions.Join 方法
func TestConfigDescriptions_Join(t *testing.T) {
	Convey("测试 ConfigDescriptions.Join 方法", t, func() {
		Convey("应将两组配置描述合并", func() {
			descs1 := ConfigDescriptions{
				{Path: "app.host", Kind: "string", Default: "localhost", Comment: "应用地址"},
			}
			descs2 := ConfigDescriptions{
				{Path: "db.host", Kind: "string", Default: "127.0.0.1", Comment: "数据库地址"},
				{Path: "db.port", Kind: "int", Default: "5432", Comment: "数据库端口"},
			}
			descs1.Join(descs2)
			So(len(descs1), ShouldEqual, 3)
			So(descs1[0].Path, ShouldEqual, "app.host")
			So(descs1[1].Path, ShouldEqual, "db.host")
			So(descs1[2].Path, ShouldEqual, "db.port")
		})

		Convey("合并空的配置描述不应改变原始切片", func() {
			descs := ConfigDescriptions{
				{Path: "app.host", Kind: "string", Default: "localhost", Comment: "应用地址"},
			}
			empty := ConfigDescriptions{}
			descs.Join(empty)
			So(len(descs), ShouldEqual, 1)
			So(descs[0].Path, ShouldEqual, "app.host")
		})

		Convey("向空切片合并配置描述应正常工作", func() {
			descs := ConfigDescriptions{}
			others := ConfigDescriptions{
				{Path: "app.host", Kind: "string", Default: "localhost", Comment: "应用地址"},
			}
			descs.Join(others)
			So(len(descs), ShouldEqual, 1)
			So(descs[0].Path, ShouldEqual, "app.host")
		})

		Convey("多次合并应累加而非替换", func() {
			descs := ConfigDescriptions{
				{Path: "first", Kind: "string", Default: "", Comment: "第一组"},
			}
			descs.Join(ConfigDescriptions{
				{Path: "second", Kind: "int", Default: "0", Comment: "第二组"},
			})
			descs.Join(ConfigDescriptions{
				{Path: "third", Kind: "bool", Default: "false", Comment: "第三组"},
			})
			So(len(descs), ShouldEqual, 3)
		})
	})
}

// TestConfigDescriptions_Describe 测试 ConfigDescriptions.Describe 方法
func TestConfigDescriptions_Describe(t *testing.T) {
	Convey("测试 ConfigDescriptions.Describe 方法", t, func() {
		Convey("应在标准输出渲染表格", func() {
			descs := ConfigDescriptions{
				{Path: "app.host", Kind: "string", Default: "localhost", Comment: "应用地址"},
				{Path: "app.port", Kind: "int", Default: "8080", Comment: "应用端口"},
			}
			err := descs.Describe(nil)
			So(err, ShouldBeNil)
		})

		Convey("空的配置描述也应正常渲染", func() {
			descs := ConfigDescriptions{}
			err := descs.Describe(nil)
			So(err, ShouldBeNil)
		})

		Convey("包含特殊字符的注释应正常渲染", func() {
			descs := ConfigDescriptions{
				{Path: "app.name", Kind: "string", Default: "zhongkui", Comment: "应用名称 (ref: https://example.com)"},
			}
			err := descs.Describe(nil)
			So(err, ShouldBeNil)
		})
	})
}

// 用于 DescribeConfig 和 ExtractConfig 测试的配置结构体
type simpleConfig struct {
	Host string `mapstructure:"host" default:"localhost" comment:"服务器监听地址"`
	Port int    `mapstructure:"port" default:"8080" comment:"服务器监听端口"`
}

type nestedInnerConfig struct {
	Enabled  bool   `mapstructure:"enabled" default:"true" comment:"是否启用"`
	Endpoint string `mapstructure:"endpoint" default:"localhost:9090" comment:"服务端点"`
}

type nestedConfig struct {
	Host  string            `mapstructure:"host" default:"0.0.0.0" comment:"监听地址"`
	Inner nestedInnerConfig `mapstructure:"inner" comment:"内部配置"`
}

type sliceConfig struct {
	Tags    []string `mapstructure:"tags" default:"[]" comment:"标签列表"`
	Numbers []int    `mapstructure:"numbers" default:"[]" comment:"数字列表"`
}

type structSliceConfig struct {
	Servers []nestedInnerConfig `mapstructure:"servers" comment:"服务器列表"`
}

type noMapstructureConfig struct {
	Host    string `default:"127.0.0.1" comment:"服务器地址"`
	PortNum int    `default:"3000" comment:"端口号"`
}

type privateFieldConfig struct {
	Public  string `mapstructure:"public" default:"visible" comment:"公开字段"`
	private string
}

type requiredConfig struct {
	Name string `mapstructure:"name" validate:"required" comment:"必填名称"`
}

// TestDescribeConfig 测试 DescribeConfig 泛型函数
func TestDescribeConfig(t *testing.T) {
	Convey("测试 DescribeConfig 泛型函数", t, func() {
		Convey("简单结构体应生成正确的配置描述", func() {
			descs := DescribeConfig[simpleConfig]("app")
			So(len(descs), ShouldEqual, 2)
			So(descs[0].Path, ShouldEqual, "app.host")
			So(descs[0].Kind, ShouldEqual, "string")
			So(descs[0].Default, ShouldEqual, "localhost")
			So(descs[0].Comment, ShouldEqual, "服务器监听地址")
			So(descs[1].Path, ShouldEqual, "app.port")
			So(descs[1].Kind, ShouldEqual, "int")
			So(descs[1].Default, ShouldEqual, "8080")
			So(descs[1].Comment, ShouldEqual, "服务器监听端口")
		})

		Convey("嵌套结构体应递归生成配置描述", func() {
			descs := DescribeConfig[nestedConfig]("server")
			So(len(descs), ShouldBeGreaterThan, 2)

			hostDesc := descs[0]
			So(hostDesc.Path, ShouldEqual, "server.host")
			So(hostDesc.Kind, ShouldEqual, "string")
			So(hostDesc.Default, ShouldEqual, "0.0.0.0")

			innerDesc := descs[1]
			So(innerDesc.Path, ShouldEqual, "server.inner")
			So(innerDesc.Kind, ShouldEqual, "struct")

			innerEnabledDesc := descs[2]
			So(innerEnabledDesc.Path, ShouldEqual, "server.inner.enabled")
			So(innerEnabledDesc.Kind, ShouldEqual, "bool")
			So(innerEnabledDesc.Default, ShouldEqual, "true")
		})

		Convey("基本类型切片字段应生成切片描述但不递归解析元素", func() {
			descs := DescribeConfig[sliceConfig]("data")
			So(len(descs), ShouldEqual, 2)

			tagsDesc := descs[0]
			So(tagsDesc.Path, ShouldEqual, "data.tags")
			So(tagsDesc.Kind, ShouldEqual, "slice")

			numbersDesc := descs[1]
			So(numbersDesc.Path, ShouldEqual, "data.numbers")
			So(numbersDesc.Kind, ShouldEqual, "slice")
		})

		Convey("结构体切片应递归解析元素类型的字段", func() {
			descs := DescribeConfig[structSliceConfig]("cluster")
			So(len(descs), ShouldEqual, 3)

			serversDesc := descs[0]
			So(serversDesc.Path, ShouldEqual, "cluster.servers")
			So(serversDesc.Kind, ShouldEqual, "slice")

			serversElemEnabledDesc := descs[1]
			So(serversElemEnabledDesc.Path, ShouldEqual, "cluster.servers[].enabled")
			So(serversElemEnabledDesc.Kind, ShouldEqual, "bool")

			serversElemEndpointDesc := descs[2]
			So(serversElemEndpointDesc.Path, ShouldEqual, "cluster.servers[].endpoint")
			So(serversElemEndpointDesc.Kind, ShouldEqual, "string")
		})

		Convey("无 mapstructure 标签时应使用小写字段名", func() {
			descs := DescribeConfig[noMapstructureConfig]("test")
			So(len(descs), ShouldEqual, 2)
			So(descs[0].Path, ShouldEqual, "test.host")
			So(descs[1].Path, ShouldEqual, "test.portnum")
		})

		Convey("私有字段应被跳过", func() {
			descs := DescribeConfig[privateFieldConfig]("test")
			So(len(descs), ShouldEqual, 1)
			So(descs[0].Path, ShouldEqual, "test.public")
		})

		Convey("空前缀时应直接使用字段名作为路径", func() {
			descs := DescribeConfig[simpleConfig]("")
			So(len(descs), ShouldEqual, 2)
			So(descs[0].Path, ShouldEqual, "host")
			So(descs[1].Path, ShouldEqual, "port")
		})

		Convey("复合前缀应正确拼接", func() {
			descs := DescribeConfig[simpleConfig]("module.{primary,secondary}")
			So(len(descs), ShouldEqual, 2)
			So(descs[0].Path, ShouldEqual, "module.{primary,secondary}.host")
			So(descs[1].Path, ShouldEqual, "module.{primary,secondary}.port")
		})
	})
}

// TestExtractConfig 测试 ExtractConfig 泛型函数
func TestExtractConfig(t *testing.T) {
	validate := validator.New()

	Convey("测试 ExtractConfig 泛型函数", t, func() {
		Convey("应从 viper 中成功提取简单配置", func() {
			v := viper.New()
			v.Set("app.host", "10.0.0.1")
			v.Set("app.port", 9090)

			cfg, err := ExtractConfig[simpleConfig](v, validate, "app")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Host, ShouldEqual, "10.0.0.1")
			So(cfg.Port, ShouldEqual, 9090)
		})

		Convey("当 viper 中缺少字段时应使用默认值", func() {
			v := viper.New()
			v.Set("app.host", "192.168.1.1")

			cfg, err := ExtractConfig[simpleConfig](v, validate, "app")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Host, ShouldEqual, "192.168.1.1")
			So(cfg.Port, ShouldEqual, 8080)
		})

		Convey("当指定前缀不存在时应返回错误", func() {
			v := viper.New()
			cfg, err := ExtractConfig[simpleConfig](v, validate, "nonexistent")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("错误信息应包含配置类型名称", func() {
			v := viper.New()
			_, err := ExtractConfig[simpleConfig](v, validate, "missing")
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "simpleConfig")
		})

		Convey("应正确提取嵌套配置", func() {
			v := viper.New()
			v.Set("server.host", "0.0.0.0")
			v.Set("server.inner.enabled", true)
			v.Set("server.inner.endpoint", "10.0.0.1:9999")

			cfg, err := ExtractConfig[nestedConfig](v, validate, "server")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Host, ShouldEqual, "0.0.0.0")
			So(cfg.Inner.Enabled, ShouldBeTrue)
			So(cfg.Inner.Endpoint, ShouldEqual, "10.0.0.1:9999")
		})

		Convey("嵌套配置缺失时应使用默认值", func() {
			v := viper.New()
			v.Set("server.host", "0.0.0.0")

			cfg, err := ExtractConfig[nestedConfig](v, validate, "server")
			So(err, ShouldBeNil)
			So(cfg, ShouldNotBeNil)
			So(cfg.Inner.Enabled, ShouldBeTrue)
			So(cfg.Inner.Endpoint, ShouldEqual, "localhost:9090")
		})

		Convey("当配置不满足验证规则时应返回错误", func() {
			v := viper.New()

			cfg, err := ExtractConfig[requiredConfig](v, validate, "app")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})
	})
}

// TestNewDefaultViper_Panic 测试 NewDefaultViper 在无配置文件时 panic
func TestNewDefaultViper_Panic(t *testing.T) {
	Convey("测试 NewDefaultViper 在无配置文件时 panic", t, func() {
		Convey("当配置文件不存在时应触发 panic", func() {
			So(func() {
				NewDefaultViper(pflag.NewFlagSet("test", pflag.ContinueOnError))
			}, ShouldPanic)
		})
	})
}

// TestNewViper_Panic 测试 NewViper 在无配置文件时 panic
func TestNewViper_Panic(t *testing.T) {
	Convey("测试 NewViper 在无配置文件时 panic", t, func() {
		Convey("当配置文件不存在时应触发 panic", func() {
			So(func() {
				NewViper("test", "nonexistent", "toml", "/nonexistent/path", nil)
			}, ShouldPanic)
		})
	})
}

// TestNewDefaultViperWithCmd 测试 NewDefaultViperWithCmd 函数
func TestNewDefaultViperWithCmd(t *testing.T) {
	Convey("测试 NewDefaultViperWithCmd 函数", t, func() {
		Convey("当配置文件不存在时应触发 panic", func() {
			cmd := &cobra.Command{}
			So(func() {
				NewDefaultViperWithCmd(cmd)
			}, ShouldPanic)
		})
	})
}

// TestNewViper_WithConfigFile 测试 NewViper 使用实际配置文件
func TestNewViper_WithConfigFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	configContent := `
[test]
key = "value"
`
	if err := os.WriteFile(tmpDir+"/config.toml", []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	Convey("测试 NewViper 使用实际配置文件", t, func() {
		Convey("当配置文件存在时应成功创建 Viper 实例", func() {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			v := NewViper("test", "config", "toml", tmpDir, flags)
			So(v, ShouldNotBeNil)
			So(v.GetString("test.key"), ShouldEqual, "value")
		})

		Convey("当指定配置类型为 toml 时应正确解析", func() {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			v := NewViper("myapp", "config", "toml", tmpDir, flags)
			So(v, ShouldNotBeNil)
		})
	})
}

// TestNewViper_WithFlags 测试 NewViper 绑定命令行参数
func TestNewViper_WithFlags(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := os.WriteFile(tmpDir+"/config.toml", []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	Convey("测试 NewViper 绑定命令行参数", t, func() {
		Convey("命令行参数应正确绑定到 viper", func() {
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("debug", "false", "debug mode")
			flags.Set("debug", "true")

			v := NewViper("test", "config", "toml", tmpDir, flags)
			So(v.GetBool("debug"), ShouldBeTrue)
		})

		Convey("nil flags 应触发 panic", func() {
			So(func() {
				NewViper("test", "config", "toml", tmpDir, nil)
			}, ShouldPanic)
		})
	})
}

// TestDescribeConfig_EdgeCases 测试 DescribeConfig 边界情况
func TestDescribeConfig_EdgeCases(t *testing.T) {
	Convey("测试 DescribeConfig 边界情况", t, func() {
		Convey("多次调用 DescribeConfig 应返回相同结果", func() {
			descs1 := DescribeConfig[simpleConfig]("app")
			descs2 := DescribeConfig[simpleConfig]("app")
			So(len(descs1), ShouldEqual, len(descs2))
			for i := range descs1 {
				So(descs1[i].Path, ShouldEqual, descs2[i].Path)
				So(descs1[i].Kind, ShouldEqual, descs2[i].Kind)
				So(descs1[i].Default, ShouldEqual, descs2[i].Default)
				So(descs1[i].Comment, ShouldEqual, descs2[i].Comment)
			}
		})

		Convey("不同前缀应生成不同路径", func() {
			descsPrimary := DescribeConfig[simpleConfig]("primary")
			descsSecondary := DescribeConfig[simpleConfig]("secondary")
			So(descsPrimary[0].Path, ShouldEqual, "primary.host")
			So(descsSecondary[0].Path, ShouldEqual, "secondary.host")
		})

		Convey("不同类型应生成不同的 Kind 值", func() {
			descs := DescribeConfig[simpleConfig]("app")
			So(descs[0].Kind, ShouldEqual, "string")
			So(descs[1].Kind, ShouldEqual, "int")
		})
	})
}

// TestExtractConfig_EdgeCases 测试 ExtractConfig 边界情况
func TestExtractConfig_EdgeCases(t *testing.T) {
	validate := validator.New()

	Convey("测试 ExtractConfig 边界情况", t, func() {
		Convey("全部使用默认值应成功", func() {
			v := viper.New()
			v.Set("app.host", "localhost")
			v.Set("app.port", 8080)

			cfg, err := ExtractConfig[simpleConfig](v, validate, "app")
			So(err, ShouldBeNil)
			So(cfg.Host, ShouldEqual, "localhost")
			So(cfg.Port, ShouldEqual, 8080)
		})

		Convey("viper 中仅设置部分字段时其余应使用默认值", func() {
			v := viper.New()
			v.Set("app.host", "custom-host")

			cfg, err := ExtractConfig[simpleConfig](v, validate, "app")
			So(err, ShouldBeNil)
			So(cfg.Host, ShouldEqual, "custom-host")
			So(cfg.Port, ShouldEqual, 8080)
		})

		Convey("空 viper 实例对无验证的结构体应返回前缀不存在的错误", func() {
			v := viper.New()
			cfg, err := ExtractConfig[simpleConfig](v, validate, "")
			So(err, ShouldNotBeNil)
			So(cfg, ShouldBeNil)
		})
	})
}

// TestConfigDescriptions_Integration 测试 ConfigDescriptions 的集成操作
func TestConfigDescriptions_Integration(t *testing.T) {
	Convey("测试 ConfigDescriptions 集成操作", t, func() {
		Convey("DescribeConfig + Join 应正确合并多个模块的配置描述", func() {
			appDescs := DescribeConfig[simpleConfig]("app")
			serverDescs := DescribeConfig[nestedConfig]("server")
			joined := make(ConfigDescriptions, 0)
			joined.Join(appDescs)
			joined.Join(serverDescs)

			So(len(joined), ShouldEqual, len(appDescs)+len(serverDescs))

			So(joined[0].Path, ShouldEqual, "app.host")
			So(joined[len(appDescs)].Path, ShouldEqual, "server.host")
		})

		Convey("合并后的配置描述应能正常输出到表格", func() {
			descs := DescribeConfig[simpleConfig]("app")
			descs.Join(DescribeConfig[nestedConfig]("server"))
			err := descs.Describe(nil)
			So(err, ShouldBeNil)
		})
	})
}
