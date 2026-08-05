package routes

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	. "github.com/smartystreets/goconvey/convey"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// createCapturerTestDB 创建用于抓包器路由测试的 SQLite 内存数据库实例
// 并自动迁移所需的表结构（Namespace 和 Capturer）
func createCapturerTestDB() *database.DB {
	cfg := &database.Config{
		Driver: "sqlite",
		DSN:    ":memory:",
	}
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	config.Level = zap.NewAtomicLevelAt(zapcore.DebugLevel)
	zapLogger, _ := config.Build()
	logger := &logging.Logger{SugaredLogger: zapLogger.Sugar()}

	db, err := database.NewDatabase(cfg, logger)
	if err != nil {
		panic("failed to create test database: " + err.Error())
	}
	if err := db.AutoMigrate(&core.Namespace{}, &core.Capturer{}); err != nil {
		panic("failed to migrate test database: " + err.Error())
	}
	return db
}

// createCapturerTestEtcdClient 创建用于抓包器路由测试的 etcd 客户端
// 注意：使用不可达的端点，仅用于类型测试，不会实际连接
func createCapturerTestEtcdClient() *etcd.EtcdClient {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:   []string{"localhost:9999"},
		DialTimeout: 0,
	})
	if err != nil {
		panic("failed to create test etcd client: " + err.Error())
	}
	return client
}

// setupCapturerTestRouter 创建用于抓包器路由测试的 Gin 路由引擎
// 注入 i18n 中间件以支持错误响应的国际化
func setupCapturerTestRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	return r
}

// setupCapturerTestRouterWithDB 创建带有数据库中间件的测试路由引擎
func setupCapturerTestRouterWithDB(db *database.DB) *gin.Engine {
	r := setupCapturerTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(manager.DatabaseCtxName, db)
		c.Next()
	})
	return r
}

// setupCapturerTestRouterWithDBAndEtcd 创建带有数据库和 etcd 中间件的测试路由引擎
func setupCapturerTestRouterWithDBAndEtcd(db *database.DB, etcdClient *etcd.EtcdClient) *gin.Engine {
	r := setupCapturerTestRouter()
	r.Use(func(c *gin.Context) {
		c.Set(manager.DatabaseCtxName, db)
		c.Set(manager.EtcdCtxName, etcdClient)
		c.Next()
	})
	return r
}

// TestCapturerConfigIn_Struct 测试 CapturerConfigIn 结构体
func TestCapturerConfigIn_Struct(t *testing.T) {
	Convey("测试 CapturerConfigIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			cfg := CapturerConfigIn{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}
			So(cfg.Device, ShouldEqual, "eth0")
			So(cfg.SnapLen, ShouldEqual, 65535)
			So(cfg.Promisc, ShouldBeTrue)
			So(cfg.Timeout, ShouldEqual, 30)
			So(cfg.BpfExpr, ShouldEqual, "tcp port 80")
			So(cfg.Enabled, ShouldBeTrue)
		})

		Convey("应正确设置默认零值", func() {
			cfg := CapturerConfigIn{}
			So(cfg.Device, ShouldEqual, "")
			So(cfg.SnapLen, ShouldEqual, 0)
			So(cfg.Promisc, ShouldBeFalse)
			So(cfg.Timeout, ShouldEqual, 0)
			So(cfg.BpfExpr, ShouldEqual, "")
			So(cfg.Enabled, ShouldBeFalse)
		})

		Convey("应正确序列化为 JSON", func() {
			cfg := CapturerConfigIn{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}
			data, err := json.Marshal(cfg)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)
			So(result["device"], ShouldEqual, "eth0")
			So(result["snapLen"], ShouldEqual, float64(65535))
			So(result["promisc"], ShouldBeTrue)
			So(result["timeout"], ShouldEqual, float64(30))
			So(result["bpfExpr"], ShouldEqual, "tcp port 80")
			So(result["enabled"], ShouldBeTrue)
		})

		Convey("应正确从 JSON 反序列化", func() {
			jsonStr := `{"device":"eth1","snapLen":262144,"promisc":false,"timeout":60,"bpfExpr":"udp port 53","enabled":false}`
			var cfg CapturerConfigIn
			err := json.Unmarshal([]byte(jsonStr), &cfg)
			So(err, ShouldBeNil)
			So(cfg.Device, ShouldEqual, "eth1")
			So(cfg.SnapLen, ShouldEqual, 262144)
			So(cfg.Promisc, ShouldBeFalse)
			So(cfg.Timeout, ShouldEqual, 60)
			So(cfg.BpfExpr, ShouldEqual, "udp port 53")
			So(cfg.Enabled, ShouldBeFalse)
		})
	})
}

// TestCapturerConfigIn_ToGRPC 测试 CapturerConfigIn.ToGRPC 方法
func TestCapturerConfigIn_ToGRPC(t *testing.T) {
	Convey("测试 CapturerConfigIn.ToGRPC 方法", t, func() {
		Convey("应正确转换为 gRPC 配置对象", func() {
			cfg := CapturerConfigIn{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}

			grpcCfg := cfg.ToGRPC()
			So(grpcCfg, ShouldNotBeNil)
			So(grpcCfg.Device, ShouldEqual, "eth0")
			So(grpcCfg.SnapLen, ShouldEqual, 65535)
			So(grpcCfg.Promisc, ShouldBeTrue)
			So(grpcCfg.Timeout, ShouldEqual, 30)
			So(grpcCfg.BpfExpr, ShouldEqual, "tcp port 80")
			So(grpcCfg.Enabled, ShouldBeTrue)
		})

		Convey("零值配置应正确转换", func() {
			cfg := CapturerConfigIn{}
			grpcCfg := cfg.ToGRPC()
			So(grpcCfg, ShouldNotBeNil)
			So(grpcCfg.Device, ShouldEqual, "")
			So(grpcCfg.SnapLen, ShouldEqual, 0)
			So(grpcCfg.Promisc, ShouldBeFalse)
			So(grpcCfg.Timeout, ShouldEqual, 0)
			So(grpcCfg.BpfExpr, ShouldEqual, "")
			So(grpcCfg.Enabled, ShouldBeFalse)
		})

		Convey("转换后修改 gRPC 对象不应影响原始结构体", func() {
			cfg := CapturerConfigIn{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: true,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: true,
			}

			grpcCfg := cfg.ToGRPC()
			grpcCfg.Device = "modified"
			grpcCfg.SnapLen = 99999

			So(cfg.Device, ShouldEqual, "eth0")
			So(cfg.SnapLen, ShouldEqual, 65535)
		})

		Convey("转换结果应为 reporter_v1.Config 指针类型", func() {
			cfg := CapturerConfigIn{
				Device:  "eth0",
				SnapLen: 65535,
				Promisc: false,
				Timeout: 30,
				BpfExpr: "tcp port 80",
				Enabled: false,
			}

			grpcCfg := cfg.ToGRPC()
			So(grpcCfg, ShouldNotBeNil)
			So(grpcCfg.Device, ShouldEqual, "eth0")
		})
	})
}

// TestCapturerCreateIn_Struct 测试 CapturerCreateIn 结构体
func TestCapturerCreateIn_Struct(t *testing.T) {
	Convey("测试 CapturerCreateIn 结构体", t, func() {
		Convey("应正确创建并赋值", func() {
			in := CapturerCreateIn{
				Namespace: "default",
				Name:      "packet-capture",
				Version:   "v1.0.0",
				Config: CapturerConfigIn{
					Device:  "eth0",
					SnapLen: 65535,
					Promisc: true,
					Timeout: 30,
					BpfExpr: "tcp port 80",
					Enabled: true,
				},
			}
			So(in.Namespace, ShouldEqual, "default")
			So(in.Name, ShouldEqual, "packet-capture")
			So(in.Version, ShouldEqual, "v1.0.0")
			So(in.Config.Device, ShouldEqual, "eth0")
			So(in.Config.SnapLen, ShouldEqual, 65535)
		})

		Convey("应正确序列化为 JSON", func() {
			in := CapturerCreateIn{
				Namespace: "production",
				Name:      "sniffer",
				Version:   "v2.1.0",
				Config: CapturerConfigIn{
					Device:  "ens192",
					SnapLen: 262144,
					Promisc: false,
					Timeout: 60,
					BpfExpr: "udp port 53",
					Enabled: true,
				},
			}

			data, err := json.Marshal(in)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)
			So(result["namespace"], ShouldEqual, "production")
			So(result["name"], ShouldEqual, "sniffer")
			So(result["version"], ShouldEqual, "v2.1.0")

			config, ok := result["config"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(config["device"], ShouldEqual, "ens192")
			So(config["bpfExpr"], ShouldEqual, "udp port 53")
		})

		Convey("应正确从 JSON 反序列化", func() {
			jsonStr := `{
				"namespace": "staging",
				"name": "capture-01",
				"version": "v3.0.0",
				"config": {
					"device": "wlan0",
					"snapLen": 128,
					"promisc": true,
					"timeout": 15,
					"bpfExpr": "host 192.168.1.1",
					"enabled": true
				}
			}`
			var in CapturerCreateIn
			err := json.Unmarshal([]byte(jsonStr), &in)
			So(err, ShouldBeNil)
			So(in.Namespace, ShouldEqual, "staging")
			So(in.Name, ShouldEqual, "capture-01")
			So(in.Version, ShouldEqual, "v3.0.0")
			So(in.Config.Device, ShouldEqual, "wlan0")
			So(in.Config.SnapLen, ShouldEqual, 128)
			So(in.Config.Promisc, ShouldBeTrue)
			So(in.Config.Timeout, ShouldEqual, 15)
			So(in.Config.BpfExpr, ShouldEqual, "host 192.168.1.1")
			So(in.Config.Enabled, ShouldBeTrue)
		})
	})
}

// TestCapturerBaseDetailOut_Struct 测试 CapturerBaseDetailOut 结构体
func TestCapturerBaseDetailOut_Struct(t *testing.T) {
	Convey("测试 CapturerBaseDetailOut 结构体", t, func() {
		Convey("应正确嵌入 BaseItemOut 并赋值", func() {
			now := time.Now()
			out := CapturerBaseDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        1,
					CreatedAt: now,
					UpdatedAt: now,
				},
				NamespaceName: "default",
				Name:          "capture-01",
				Version:       "v1.0.0",
			}
			So(out.ID, ShouldEqual, 1)
			So(out.CreatedAt, ShouldEqual, now)
			So(out.UpdatedAt, ShouldEqual, now)
			So(out.NamespaceName, ShouldEqual, "default")
			So(out.Name, ShouldEqual, "capture-01")
			So(out.Version, ShouldEqual, "v1.0.0")
		})

		Convey("应正确序列化为 JSON", func() {
			now := time.Now()
			out := CapturerBaseDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        42,
					CreatedAt: now,
					UpdatedAt: now,
				},
				NamespaceName: "production",
				Name:          "sniffer",
				Version:       "v2.0.0",
			}
			data, err := json.Marshal(out)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)
			So(result["id"], ShouldEqual, float64(42))
			So(result["namespace"], ShouldEqual, "production")
			So(result["name"], ShouldEqual, "sniffer")
			So(result["version"], ShouldEqual, "v2.0.0")
		})
	})
}

// TestCapturerDetailOut_Struct 测试 CapturerDetailOut 结构体
func TestCapturerDetailOut_Struct(t *testing.T) {
	Convey("测试 CapturerDetailOut 结构体", t, func() {
		Convey("应正确嵌套基础详情和配置", func() {
			now := time.Now()
			out := CapturerDetailOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					NamespaceName: "default",
					Name:          "capture-01",
					Version:       "v1.0.0",
				},
				Config: CapturerConfigIn{
					Device:  "eth0",
					SnapLen: 65535,
					Promisc: true,
					Timeout: 30,
					BpfExpr: "tcp port 80",
					Enabled: true,
				},
			}

			So(out.ID, ShouldEqual, 1)
			So(out.NamespaceName, ShouldEqual, "default")
			So(out.Name, ShouldEqual, "capture-01")
			So(out.Version, ShouldEqual, "v1.0.0")
			So(out.Config.Device, ShouldEqual, "eth0")
			So(out.Config.SnapLen, ShouldEqual, 65535)
			So(out.Config.BpfExpr, ShouldEqual, "tcp port 80")
		})

		Convey("应正确序列化为 JSON", func() {
			now := time.Now()
			out := CapturerDetailOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        10,
						CreatedAt: now,
						UpdatedAt: now,
					},
					NamespaceName: "test",
					Name:          "detail-test",
					Version:       "v1.0.0",
				},
				Config: CapturerConfigIn{
					Device:  "eth0",
					SnapLen: 65535,
					Promisc: false,
					Timeout: 30,
					BpfExpr: "tcp port 443",
					Enabled: true,
				},
			}

			data, err := json.Marshal(out)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)
			So(result["id"], ShouldEqual, float64(10))
			So(result["namespace"], ShouldEqual, "test")
			So(result["name"], ShouldEqual, "detail-test")

			config, ok := result["config"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(config["device"], ShouldEqual, "eth0")
			So(config["bpfExpr"], ShouldEqual, "tcp port 443")
		})
	})
}

// TestCapturerListOut_Struct 测试 CapturerListOut 结构体
func TestCapturerListOut_Struct(t *testing.T) {
	Convey("测试 CapturerListOut 结构体", t, func() {
		Convey("应正确嵌入基础详情和总数", func() {
			now := time.Now()
			out := CapturerListOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					NamespaceName: "default",
					Name:          "capture-01",
					Version:       "v1.0.0",
				},
				BaseTotalOut: utils.BaseTotalOut{
					Total: 100,
				},
			}

			So(out.ID, ShouldEqual, 1)
			So(out.NamespaceName, ShouldEqual, "default")
			So(out.Name, ShouldEqual, "capture-01")
			So(out.Version, ShouldEqual, "v1.0.0")
			So(out.Total, ShouldEqual, 100)
			So(out.GetTotal(), ShouldEqual, 100)
		})

		Convey("BaseTotalOut 的 JSON 标签应为忽略（不序列化到 JSON）", func() {
			out := CapturerListOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID: 1,
					},
					NamespaceName: "default",
					Name:          "test",
					Version:       "v1.0.0",
				},
				BaseTotalOut: utils.BaseTotalOut{
					Total: 50,
				},
			}

			data, err := json.Marshal(out)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)

			_, hasTotal := result["total"]
			So(hasTotal, ShouldBeFalse)
		})
	})
}

// TestCapturerListGet_Handler 测试 capturerListGet 处理器的参数校验
func TestCapturerListGet_Handler(t *testing.T) {
	Convey("测试 capturerListGet 处理器", t, func() {
		db := createCapturerTestDB()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
		}()

		Convey("无请求体时应触发 panic（JSON 解析失败不是 ValidationErrors）", func() {
			r := setupCapturerTestRouterWithDB(db)
			r.POST("/api/v1/admin/capturer/list", capturerListGet)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/list", nil)
			So(func() {
				r.ServeHTTP(w, req)
			}, ShouldPanic)
		})

		Convey("空 JSON 对象应成功返回空列表", func() {
			r := setupCapturerTestRouterWithDB(db)
			r.POST("/api/v1/admin/capturer/list", capturerListGet)

			body := `{}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/list", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)
			So(resp["code"], ShouldEqual, float64(200))
		})

		Convey("带分页参数的请求应成功返回结果", func() {
			r := setupCapturerTestRouterWithDB(db)
			r.POST("/api/v1/admin/capturer/list", capturerListGet)

			body := `{"page":1,"pageSize":10}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/list", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusOK)

			var resp map[string]any
			err := json.Unmarshal(w.Body.Bytes(), &resp)
			So(err, ShouldBeNil)
			So(resp["code"], ShouldEqual, float64(200))

			data, ok := resp["data"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(data["total"], ShouldEqual, float64(0))
			So(data["items"], ShouldNotBeNil)
		})
	})
}

// TestCapturerDetailGet_Handler 测试 capturerDetailGet 处理器
func TestCapturerDetailGet_Handler(t *testing.T) {
	Convey("测试 capturerDetailGet 处理器", t, func() {
		db := createCapturerTestDB()
		etcdClient := createCapturerTestEtcdClient()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
			etcdClient.Close()
		}()

		Convey("无效的 ID 参数应返回 400 错误", func() {
			r := setupCapturerTestRouterWithDBAndEtcd(db, etcdClient)
			r.GET("/api/v1/admin/capturer/:id/get", capturerDetailGet)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/capturer/abc/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("不存在的 ID 应返回 500 错误（数据库查询失败）", func() {
			r := setupCapturerTestRouterWithDBAndEtcd(db, etcdClient)
			r.GET("/api/v1/admin/capturer/:id/get", capturerDetailGet)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/capturer/99999/get", nil)
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})
	})
}

// TestCapturerUpdate_Handler 测试 capturerUpdate 处理器
func TestCapturerUpdate_Handler(t *testing.T) {
	Convey("测试 capturerUpdate 处理器", t, func() {
		db := createCapturerTestDB()
		etcdClient := createCapturerTestEtcdClient()
		defer func() {
			sqlDB, _ := db.DB()
			if sqlDB != nil {
				sqlDB.Close()
			}
			etcdClient.Close()
		}()

		Convey("无效的 ID 参数应返回 400 错误", func() {
			r := setupCapturerTestRouterWithDBAndEtcd(db, etcdClient)
			r.POST("/api/v1/admin/capturer/:id/update", capturerUpdate)

			body := `{"device":"eth0","snapLen":65535,"timeout":30,"bpfExpr":"tcp port 80"}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/abc/update", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusBadRequest)
		})

		Convey("不存在的 ID 应返回 500 错误（数据库查询失败）", func() {
			r := setupCapturerTestRouterWithDBAndEtcd(db, etcdClient)
			r.POST("/api/v1/admin/capturer/:id/update", capturerUpdate)

			body := `{"device":"eth0","snapLen":65535,"timeout":30,"bpfExpr":"tcp port 80"}`
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/99999/update", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			So(w.Code, ShouldEqual, http.StatusInternalServerError)
		})

		Convey("无请求体时应触发 panic（JSON 解析失败不是 ValidationErrors）", func() {
			r := setupCapturerTestRouterWithDBAndEtcd(db, etcdClient)
			r.POST("/api/v1/admin/capturer/:id/update", capturerUpdate)

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/capturer/1/update", nil)
			So(func() {
				r.ServeHTTP(w, req)
			}, ShouldPanic)
		})
	})
}

// TestCapturerConfigIn_Validation 测试 CapturerConfigIn 的绑定验证
func TestCapturerConfigIn_Validation(t *testing.T) {
	Convey("测试 CapturerConfigIn 字段验证规则", t, func() {
		Convey("Device 长度应限制在 2~32 字符", func() {
			shortDevice := CapturerConfigIn{Device: "a", SnapLen: 65535, Timeout: 30, BpfExpr: "tcp"}
			So(len(shortDevice.Device), ShouldBeLessThan, 2)

			validDevice := CapturerConfigIn{Device: "eth0", SnapLen: 65535, Timeout: 30, BpfExpr: "tcp"}
			So(len(validDevice.Device), ShouldBeGreaterThan, 1)
			So(len(validDevice.Device), ShouldBeLessThanOrEqualTo, 32)

			longDevice := CapturerConfigIn{Device: "a_device_name_that_is_way_too_long_for_validation", SnapLen: 65535, Timeout: 30, BpfExpr: "tcp"}
			So(len(longDevice.Device), ShouldBeGreaterThan, 32)
		})

		Convey("BpfExpr 长度应限制在 3~1024 字符", func() {
			shortBpf := CapturerConfigIn{Device: "eth0", SnapLen: 65535, Timeout: 30, BpfExpr: "ab"}
			So(len(shortBpf.BpfExpr), ShouldBeLessThan, 3)

			validBpf := CapturerConfigIn{Device: "eth0", SnapLen: 65535, Timeout: 30, BpfExpr: "tcp port 80"}
			So(len(validBpf.BpfExpr), ShouldBeGreaterThanOrEqualTo, 3)
			So(len(validBpf.BpfExpr), ShouldBeLessThanOrEqualTo, 1024)
		})
	})
}

// TestCapturerCreateIn_Validation 测试 CapturerCreateIn 的绑定验证
func TestCapturerCreateIn_Validation(t *testing.T) {
	Convey("测试 CapturerCreateIn 字段验证规则", t, func() {
		Convey("Namespace 长度应限制在 3~32 字符", func() {
			shortNs := CapturerCreateIn{Namespace: "ab", Name: "test", Version: "v1"}
			So(len(shortNs.Namespace), ShouldBeLessThan, 3)

			validNs := CapturerCreateIn{Namespace: "default", Name: "test", Version: "v1"}
			So(len(validNs.Namespace), ShouldBeGreaterThanOrEqualTo, 3)
			So(len(validNs.Namespace), ShouldBeLessThanOrEqualTo, 32)
		})

		Convey("Name 长度应限制在 3~32 字符", func() {
			shortName := CapturerCreateIn{Namespace: "test", Name: "ab", Version: "v1"}
			So(len(shortName.Name), ShouldBeLessThan, 3)

			validName := CapturerCreateIn{Namespace: "test", Name: "capture-01", Version: "v1"}
			So(len(validName.Name), ShouldBeGreaterThanOrEqualTo, 3)
			So(len(validName.Name), ShouldBeLessThanOrEqualTo, 32)
		})

		Convey("Version 长度应限制在 3~32 字符", func() {
			shortVer := CapturerCreateIn{Namespace: "test", Name: "test", Version: "v1"}
			So(len(shortVer.Version), ShouldBeLessThan, 3)

			validVer := CapturerCreateIn{Namespace: "test", Name: "test", Version: "v1.0.0"}
			So(len(validVer.Version), ShouldBeGreaterThanOrEqualTo, 3)
			So(len(validVer.Version), ShouldBeLessThanOrEqualTo, 32)
		})
	})
}

// TestCapturerDetailOut_Composition 测试 CapturerDetailOut 的组合结构
func TestCapturerDetailOut_Composition(t *testing.T) {
	Convey("测试 CapturerDetailOut 的嵌套组合", t, func() {
		Convey("应能通过嵌入直接访问 BaseItemOut 字段", func() {
			now := time.Now()
			out := CapturerDetailOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        1,
						CreatedAt: now,
						UpdatedAt: now,
					},
					NamespaceName: "ns",
					Name:          "name",
					Version:       "ver",
				},
				Config: CapturerConfigIn{
					Device:  "eth0",
					SnapLen: 65535,
					Timeout: 30,
					BpfExpr: "tcp port 80",
					Enabled: true,
				},
			}

			So(out.ID, ShouldEqual, 1)
			So(out.CreatedAt, ShouldEqual, now)
			So(out.NamespaceName, ShouldEqual, "ns")
			So(out.Config.Device, ShouldEqual, "eth0")
		})

		Convey("JSON 序列化应扁平化嵌入字段", func() {
			now := time.Now()
			out := CapturerDetailOut{
				CapturerBaseDetailOut: CapturerBaseDetailOut{
					BaseItemOut: utils.BaseItemOut{
						ID:        5,
						CreatedAt: now,
						UpdatedAt: now,
					},
					NamespaceName: "flat-ns",
					Name:          "flat-name",
					Version:       "flat-ver",
				},
				Config: CapturerConfigIn{
					Device:  "eth1",
					SnapLen: 128,
					Timeout: 10,
					BpfExpr: "udp",
					Enabled: false,
				},
			}

			data, err := json.Marshal(out)
			So(err, ShouldBeNil)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			So(err, ShouldBeNil)

			So(result["id"], ShouldEqual, float64(5))
			So(result["namespace"], ShouldEqual, "flat-ns")
			So(result["name"], ShouldEqual, "flat-name")
			So(result["version"], ShouldEqual, "flat-ver")

			config, ok := result["config"].(map[string]any)
			So(ok, ShouldBeTrue)
			So(config["device"], ShouldEqual, "eth1")
			So(config["snapLen"], ShouldEqual, float64(128))
		})
	})
}

// TestCapturerListGetSearchIn_Interface 测试 CapturerListGetSearchIn 实现了 SearcherIn 接口
func TestCapturerListGetSearchIn_Interface(t *testing.T) {
	Convey("测试 CapturerListGetSearchIn 接口实现", t, func() {
		Convey("应实现 utils.SearcherIn 接口", func() {
			var _ utils.SearcherIn = &CapturerListGetSearchIn{}
		})

		Convey("默认值应正确设置", func() {
			var in CapturerListGetSearchIn
			So(in.GetPage(), ShouldEqual, 0)
			So(in.GetPageSize(), ShouldEqual, 0)
			So(in.GetGreedy(), ShouldBeFalse)
		})
	})
}

// TestCapturerListOut_ImplementsTotalOut 测试 CapturerListOut 实现了 TotalOut 接口
func TestCapturerListOut_ImplementsTotalOut(t *testing.T) {
	Convey("测试 CapturerListOut 实现 TotalOut 接口", t, func() {
		Convey("应实现 utils.TotalOut 接口", func() {
			var _ utils.TotalOut = &CapturerListOut{}
		})

		Convey("GetTotal 应返回正确的总数", func() {
			out := &CapturerListOut{
				BaseTotalOut: utils.BaseTotalOut{
					Total: 42,
				},
			}
			So(out.GetTotal(), ShouldEqual, 42)
		})

		Convey("nil 接收者应返回零值", func() {
			var totalOut *utils.BaseTotalOut
			So(totalOut.GetTotal(), ShouldEqual, 0)
		})
	})
}
