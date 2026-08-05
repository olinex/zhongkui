package logging

import (
	"fmt"
	"strings"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/config"
)

func TestConfigStruct(t *testing.T) {
	convey.Convey("Test Config struct", t, func() {
		convey.Convey("should create Config with all fields", func() {
			cfg := &Config{
				Level:             "debug",
				Development:       true,
				DisableCaller:     false,
				DisableStacktrace: true,
				Encoding:          "console",
				OutputPaths:       []string{"stdout", "/tmp/app.log"},
				ErrorOutputPaths:  []string{"stderr"},
			}

			convey.So(cfg.Level, convey.ShouldEqual, "debug")
			convey.So(cfg.Development, convey.ShouldBeTrue)
			convey.So(cfg.DisableCaller, convey.ShouldBeFalse)
			convey.So(cfg.DisableStacktrace, convey.ShouldBeTrue)
			convey.So(cfg.Encoding, convey.ShouldEqual, "console")
			convey.So(cfg.OutputPaths, convey.ShouldResemble, []string{"stdout", "/tmp/app.log"})
			convey.So(cfg.ErrorOutputPaths, convey.ShouldResemble, []string{"stderr"})
		})

		convey.Convey("should create Config with minimal fields", func() {
			cfg := &Config{
				Level: "info",
			}

			convey.So(cfg.Level, convey.ShouldEqual, "info")
			convey.So(cfg.Development, convey.ShouldBeFalse) // default value
			convey.So(cfg.Encoding, convey.ShouldEqual, "")  // not set
		})
	})
}

func TestParseLevel(t *testing.T) {
	convey.Convey("Test ParseLevel function", t, func() {
		convey.Convey("should parse debug level", func() {
			level := ParseLevel("debug")
			convey.So(level, convey.ShouldEqual, zapcore.DebugLevel)
		})

		convey.Convey("should parse info level", func() {
			level := ParseLevel("info")
			convey.So(level, convey.ShouldEqual, zapcore.InfoLevel)
		})

		convey.Convey("should parse warn level", func() {
			level := ParseLevel("warn")
			convey.So(level, convey.ShouldEqual, zapcore.WarnLevel)
		})

		convey.Convey("should parse error level", func() {
			level := ParseLevel("error")
			convey.So(level, convey.ShouldEqual, zapcore.ErrorLevel)
		})

		convey.Convey("should parse dpanic level", func() {
			level := ParseLevel("dpanic")
			convey.So(level, convey.ShouldEqual, zapcore.DPanicLevel)
		})

		convey.Convey("should parse panic level", func() {
			level := ParseLevel("panic")
			convey.So(level, convey.ShouldEqual, zapcore.PanicLevel)
		})

		convey.Convey("should parse fatal level", func() {
			level := ParseLevel("fatal")
			convey.So(level, convey.ShouldEqual, zapcore.FatalLevel)
		})

		convey.Convey("should panic on invalid level", func() {
			convey.So(func() {
				ParseLevel("invalid")
			}, convey.ShouldPanicWith, "unknown log level: invalid")
		})

		convey.Convey("should be case-sensitive for valid levels", func() {
			convey.So(func() {
				ParseLevel("DEBUG")
			}, convey.ShouldPanicWith, "unknown log level: DEBUG")

			convey.So(func() {
				ParseLevel("Info")
			}, convey.ShouldPanicWith, "unknown log level: Info")
		})
	})
}

func TestToZapConfig(t *testing.T) {
	convey.Convey("Test ToZapConfig method", t, func() {
		convey.Convey("should convert production config correctly", func() {
			cfg := &Config{
				Level:             "info",
				Development:       false,
				DisableCaller:     false,
				DisableStacktrace: false,
				Encoding:          "json",
				OutputPaths:       []string{"stdout"},
				ErrorOutputPaths:  []string{"stderr"},
			}

			zapCfg := cfg.ToZapConfig()

			convey.So(zapCfg.Level, convey.ShouldResemble, zap.NewAtomicLevelAt(zapcore.InfoLevel))
			convey.So(zapCfg.Development, convey.ShouldBeFalse)
			convey.So(zapCfg.DisableCaller, convey.ShouldBeFalse)
			convey.So(zapCfg.DisableStacktrace, convey.ShouldBeFalse)
			convey.So(zapCfg.Encoding, convey.ShouldEqual, "json")
			convey.So(zapCfg.OutputPaths, convey.ShouldResemble, []string{"stdout"})
			convey.So(zapCfg.ErrorOutputPaths, convey.ShouldResemble, []string{"stderr"})
			convey.So(zapCfg.EncoderConfig, convey.ShouldNotBeNil)
		})

		convey.Convey("should convert development config correctly", func() {
			cfg := &Config{
				Level:             "debug",
				Development:       true,
				DisableCaller:     true,
				DisableStacktrace: true,
				Encoding:          "console",
				OutputPaths:       []string{"stdout", "stderr"},
				ErrorOutputPaths:  []string{"stderr"},
			}

			zapCfg := cfg.ToZapConfig()

			convey.So(zapCfg.Level, convey.ShouldResemble, zap.NewAtomicLevelAt(zapcore.DebugLevel))
			convey.So(zapCfg.Development, convey.ShouldBeTrue)
			convey.So(zapCfg.DisableCaller, convey.ShouldBeTrue)
			convey.So(zapCfg.DisableStacktrace, convey.ShouldBeTrue)
			convey.So(zapCfg.Encoding, convey.ShouldEqual, "console")
			convey.So(zapCfg.OutputPaths, convey.ShouldResemble, []string{"stdout", "stderr"})
			convey.So(zapCfg.ErrorOutputPaths, convey.ShouldResemble, []string{"stderr"})
			convey.So(zapCfg.EncoderConfig, convey.ShouldNotBeNil)
		})

		convey.Convey("should handle multiple output paths", func() {
			cfg := &Config{
				Level:            "warn",
				Development:      false,
				Encoding:         "json",
				OutputPaths:      []string{"stdout", "/var/log/app.log", "/tmp/debug.log"},
				ErrorOutputPaths: []string{"stderr", "/var/log/error.log"},
			}

			zapCfg := cfg.ToZapConfig()

			convey.So(zapCfg.OutputPaths, convey.ShouldHaveLength, 3)
			convey.So(zapCfg.OutputPaths, convey.ShouldContain, "stdout")
			convey.So(zapCfg.OutputPaths, convey.ShouldContain, "/var/log/app.log")
			convey.So(zapCfg.OutputPaths, convey.ShouldContain, "/tmp/debug.log")
			convey.So(zapCfg.ErrorOutputPaths, convey.ShouldHaveLength, 2)
			convey.So(zapCfg.ErrorOutputPaths, convey.ShouldContain, "stderr")
			convey.So(zapCfg.ErrorOutputPaths, convey.ShouldContain, "/var/log/error.log")
		})

		convey.Convey("should use production encoder config for non-development", func() {
			cfg := &Config{
				Level:       "info",
				Development: false,
			}

			zapCfg := cfg.ToZapConfig()
			convey.So(zapCfg.EncoderConfig, convey.ShouldNotBeNil)
		})

		convey.Convey("should use development encoder config for development", func() {
			cfg := &Config{
				Level:       "info",
				Development: true,
			}

			zapCfg := cfg.ToZapConfig()
			convey.So(zapCfg.EncoderConfig, convey.ShouldNotBeNil)
		})
	})
}

func TestDescribeConfig(t *testing.T) {
	convey.Convey("Test DescribeConfig function", t, func() {
		convey.Convey("should describe all configurations when no names provided", func() {
			descriptions := DescribeConfig()
			convey.So(descriptions, convey.ShouldNotBeNil)
			convey.So(descriptions, convey.ShouldNotBeEmpty)
		})

		convey.Convey("should describe specific configurations", func() {
			descriptions := DescribeConfig("level", "encoding")
			convey.So(descriptions, convey.ShouldNotBeNil)
			convey.So(descriptions, convey.ShouldNotBeEmpty)
		})

		convey.Convey("should return ConfigDescriptions type", func() {
			descriptions := DescribeConfig("development")
			convey.So(descriptions, convey.ShouldHaveSameTypeAs, config.ConfigDescriptions{})

			if len(descriptions) > 0 {
				convey.So(descriptions[0].Path, convey.ShouldNotBeEmpty)
				convey.So(descriptions[0].Kind, convey.ShouldNotBeEmpty)
				convey.So(descriptions[0].Default, convey.ShouldNotBeEmpty)
				convey.So(descriptions[0].Comment, convey.ShouldNotBeEmpty)
			}
		})
	})
}

func TestExtractConfig(t *testing.T) {
	convey.Convey("Test ExtractConfig function", t, func() {
		convey.Convey("should extract config from viper", func() {
			v := viper.New()
			v.Set("zap.default.level", "debug")
			v.Set("zap.default.development", true)
			v.Set("zap.default.encoding", "console")
			v.Set("zap.default.outputPaths", []string{"stdout"})
			v.Set("zap.default.errorOutputPaths", []string{"stderr"})

			cfg, err := ExtractConfig(v, "default")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Level, convey.ShouldEqual, "debug")
			convey.So(cfg.Development, convey.ShouldBeTrue)
			convey.So(cfg.Encoding, convey.ShouldEqual, "console")
			convey.So(cfg.OutputPaths, convey.ShouldResemble, []string{"stdout"})
			convey.So(cfg.ErrorOutputPaths, convey.ShouldResemble, []string{"stderr"})
		})

		convey.Convey("should apply default values when fields missing", func() {
			v := viper.New()
			v.Set("zap.custom.level", "warn")

			cfg, err := ExtractConfig(v, "custom")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Level, convey.ShouldEqual, "warn")
			convey.So(cfg.Development, convey.ShouldBeFalse)                           // default from struct tag
			convey.So(cfg.Encoding, convey.ShouldEqual, "json")                        // default from struct tag
			convey.So(cfg.OutputPaths, convey.ShouldResemble, []string{"stdout"})      // default from struct tag
			convey.So(cfg.ErrorOutputPaths, convey.ShouldResemble, []string{"stderr"}) // default from struct tag
		})

		convey.Convey("should return error for non-existent config", func() {
			v := viper.New()

			cfg, err := ExtractConfig(v, "non-existent")
			convey.So(err, convey.ShouldNotBeNil)
			convey.So(cfg, convey.ShouldBeNil)
		})

		convey.Convey("should handle array configurations correctly", func() {
			v := viper.New()
			v.Set("zap.app.level", "error")
			v.Set("zap.app.outputPaths", []string{"stdout", "/var/log/app.log", "/tmp/app.debug.log"})
			v.Set("zap.app.errorOutputPaths", []string{"stderr", "/var/log/app.error.log"})

			cfg, err := ExtractConfig(v, "app")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.OutputPaths, convey.ShouldHaveLength, 3)
			convey.So(cfg.OutputPaths, convey.ShouldContain, "stdout")
			convey.So(cfg.OutputPaths, convey.ShouldContain, "/var/log/app.log")
			convey.So(cfg.OutputPaths, convey.ShouldContain, "/tmp/app.debug.log")
			convey.So(cfg.ErrorOutputPaths, convey.ShouldHaveLength, 2)
			convey.So(cfg.ErrorOutputPaths, convey.ShouldContain, "stderr")
			convey.So(cfg.ErrorOutputPaths, convey.ShouldContain, "/var/log/app.error.log")
		})

		convey.Convey("should handle boolean configurations correctly", func() {
			v := viper.New()
			v.Set("zap.test.level", "info")
			v.Set("zap.test.development", false)
			v.Set("zap.test.disableCaller", true)
			v.Set("zap.test.disableStacktrace", true)

			cfg, err := ExtractConfig(v, "test")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Development, convey.ShouldBeFalse)
			convey.So(cfg.DisableCaller, convey.ShouldBeTrue)
			convey.So(cfg.DisableStacktrace, convey.ShouldBeTrue)
		})
	})
}

func TestIntegration(t *testing.T) {
	convey.Convey("Test integration between config functions", t, func() {
		convey.Convey("should parse level and create zap config", func() {
			levelStr := "error"
			level := ParseLevel(levelStr)

			cfg := &Config{
				Level:       levelStr,
				Development: false,
				Encoding:    "json",
			}

			zapCfg := cfg.ToZapConfig()
			convey.So(zapCfg.Level, convey.ShouldResemble, zap.NewAtomicLevelAt(level))
		})

		convey.Convey("should extract config and convert to zap config", func() {
			v := viper.New()
			v.Set("zap.integration.level", "debug")
			v.Set("zap.integration.development", true)
			v.Set("zap.integration.encoding", "console")
			v.Set("zap.integration.outputPaths", []string{"stdout"})

			cfg, err := ExtractConfig(v, "integration")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)

			zapCfg := cfg.ToZapConfig()
			convey.So(zapCfg.Level, convey.ShouldResemble, zap.NewAtomicLevelAt(zapcore.DebugLevel))
			convey.So(zapCfg.Development, convey.ShouldBeTrue)
			convey.So(zapCfg.Encoding, convey.ShouldEqual, "console")
			convey.So(zapCfg.OutputPaths, convey.ShouldResemble, []string{"stdout"})
		})

		convey.Convey("should handle full configuration lifecycle", func() {
			v := viper.New()
			yamlConfig := `
zap:
  production:
    level: info
    development: false
    disableCaller: false
    disableStacktrace: false
    encoding: json
    outputPaths:
      - stdout
      - /var/log/production.log
    errorOutputPaths:
      - stderr
`
			v.SetConfigType("yaml")
			err := v.ReadConfig(strings.NewReader(yamlConfig))
			convey.So(err, convey.ShouldBeNil)

			cfg, err := ExtractConfig(v, "production")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)

			zapCfg := cfg.ToZapConfig()
			convey.So(zapCfg.Level, convey.ShouldResemble, zap.NewAtomicLevelAt(zapcore.InfoLevel))
			convey.So(zapCfg.Development, convey.ShouldBeFalse)
			convey.So(zapCfg.Encoding, convey.ShouldEqual, "json")
			convey.So(zapCfg.OutputPaths, convey.ShouldHaveLength, 2)
			convey.So(zapCfg.OutputPaths, convey.ShouldContain, "stdout")
			convey.So(zapCfg.OutputPaths, convey.ShouldContain, "/var/log/production.log")
		})
	})
}

func TestConstants(t *testing.T) {
	convey.Convey("Test constants", t, func() {
		convey.Convey("should have correct PREFIX constant", func() {
			convey.So(Prefix, convey.ShouldEqual, "zap")
		})

		convey.Convey("should use PREFIX in configuration paths", func() {
			v := viper.New()
			configPath := fmt.Sprintf("%s.test", Prefix)
			v.Set(configPath+".level", "debug")

			cfg, err := ExtractConfig(v, "test")
			convey.So(err, convey.ShouldBeNil)
			convey.So(cfg, convey.ShouldNotBeNil)
			convey.So(cfg.Level, convey.ShouldEqual, "debug")
		})
	})
}
