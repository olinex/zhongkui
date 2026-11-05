package etcd

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
)

// TestWithPrefix_ValidKey 测试合法键的 WithPrefix 函数
func TestWithPrefix_ValidKey(t *testing.T) {
	Convey("测试合法键的 WithPrefix 函数", t, func() {
		Convey("当键为 myapp/ 时应返回 zki/myapp/", func() {
			result := WithPrefix("myapp/")
			So(result, ShouldEqual, "zki/myapp/")
		})

		Convey("当键为 config/ 时应返回 zki/config/", func() {
			result := WithPrefix("config/")
			So(result, ShouldEqual, "zki/config/")
		})

		Convey("当键为 a/ 时应返回 zki/a/", func() {
			result := WithPrefix("a/")
			So(result, ShouldEqual, "zki/a/")
		})

		Convey("当键包含多级路径时应正确拼接", func() {
			result := WithPrefix("app/submodule/")
			So(result, ShouldEqual, "zki/app/submodule/")
		})
	})
}

// TestWithPrefix_KeyStartsWithSlash 测试以斜杠开头的键应触发 panic
func TestWithPrefix_KeyStartsWithSlash(t *testing.T) {
	Convey("测试以斜杠开头的键应触发 panic", t, func() {
		Convey("当键以 / 开头时应 panic", func() {
			So(func() {
				WithPrefix("/myapp/")
			}, ShouldPanicWith, "Etcd key /myapp/ cannot startswith slash '/'")
		})

		Convey("当键仅以 / 开头时也应 panic", func() {
			So(func() {
				WithPrefix("/")
			}, ShouldPanicWith, "Etcd key / cannot startswith slash '/'")
		})
	})
}

// TestWithPrefix_KeyNotEndsWithSlash 测试不以斜杠结尾的键应触发 panic
func TestWithPrefix_KeyNotEndsWithSlash(t *testing.T) {
	Convey("测试不以斜杠结尾的键应触发 panic", t, func() {
		Convey("当键不以 / 结尾时应 panic", func() {
			So(func() {
				WithPrefix("myapp")
			}, ShouldPanicWith, "Etcd key myapp must endswith slash '/'")
		})

		Convey("当键既不以 / 开头也不以 / 结尾时应 panic（先触发开头校验）", func() {
			So(func() {
				WithPrefix("/myapp")
			}, ShouldPanicWith, "Etcd key /myapp cannot startswith slash '/'")
		})

		Convey("当键为空字符串时应 panic", func() {
			So(func() {
				WithPrefix("")
			}, ShouldPanicWith, "Etcd key  must endswith slash '/'")
		})
	})
}

// TestNewEtcdClient_NilConfig 测试配置为 nil 时 NewEtcdClient 的行为
func TestNewEtcdClient_NilConfig(t *testing.T) {
	Convey("测试配置为 nil 时 NewEtcdClient 的行为", t, func() {
		Convey("当 Config 为 nil 时应返回 no available endpoints 错误", func() {
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			var cfg *Config
			_, err = NewEtcdClient(ctx, cfg, logger)
			So(err, ShouldNotBeNil)
		})
	})
}

// TestNewEtcdClient_EmptyEndpoints 测试端点列表为空时 NewEtcdClient 的行为
func TestNewEtcdClient_EmptyEndpoints(t *testing.T) {
	Convey("测试端点列表为空时 NewEtcdClient 的行为", t, func() {
		Convey("当 Endpoints 为空时应返回 no available endpoints 错误", func() {
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			cfg := &Config{
				Endpoints: []string{},
			}
			_, err = NewEtcdClient(ctx, cfg, logger)
			So(err, ShouldNotBeNil)
		})
	})
}

// TestNewEtcdClient_InvalidEndpoints 测试无效端点时 NewEtcdClient 的行为
func TestNewEtcdClient_InvalidEndpoints(t *testing.T) {
	Convey("测试无效端点时 NewEtcdClient 的行为", t, func() {
		Convey("当端点地址不可达时 clientv3.New 仍可创建客户端（延迟连接）", func() {
			ctx := context.Background()
			logger, err := logging.NewDefaultLogger()
			So(err, ShouldBeNil)
			cfg := &Config{
				Endpoints:   []string{"localhost:9999"},
				DialTimeout: 1 * time.Second,
			}
			client, err := NewEtcdClient(ctx, cfg, logger)
			So(err, ShouldBeNil)
			So(client, ShouldNotBeNil)
			client.Close()
		})
	})
}
