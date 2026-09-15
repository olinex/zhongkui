package datasource

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/crypto/key"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/etcd"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkui.com/zhongkui/internal/pkg/testdb"
)

// datasourceTestNamespace 数据源热同步测试专用命名空间，与迁移种子的 default 隔离。
const datasourceTestNamespace = "dstest"

var initDatasourceTestKeyOnce sync.Once

// initDatasourceTestKeyDerivation 初始化全局密钥派生：数据源配置经 crypto 序列化器
// 加解密存储，测试须与生产一致地派生密钥，否则写库会报 KeyDerivation not initialized。
func initDatasourceTestKeyDerivation() {
	initDatasourceTestKeyOnce.Do(func() {
		provider := key.NewConfigFileSecretKeyProvider("test-secret-key-for-unit-test")
		book, err := key.LoadPasswordBook("")
		if err != nil {
			panic("failed to load password book: " + err.Error())
		}
		key.InitGlobalKeyDerivation(provider, book)
	})
}

// setupWatcher 创建共享已迁移的 PostgreSQL 测试库（testdb.Core）与本地 vtable 池持有者。
// 进程内先确保测试专用命名空间存在（满足 datasources 的 namespace_name 外键约束）。
func setupWatcher(t *testing.T) (*Watcher, *vtab.Pools, *database.DB) {
	t.Helper()
	initDatasourceTestKeyDerivation()
	logger, err := logging.NewDefaultLogger()
	if err != nil {
		t.Fatal(err)
	}
	db := testdb.Core(t)
	ns := core.Namespace{Name: datasourceTestNamespace}
	if err := db.Where(core.Namespace{Name: datasourceTestNamespace}).FirstOrCreate(&ns).Error; err != nil {
		t.Fatalf("seed datasource test namespace: %v", err)
	}
	pools := vtab.NewPools(nil)
	t.Cleanup(func() { pools.Close(context.Background()) })
	return NewWatcher(db, pools, nil, logger), pools, db
}

// TestDatasourceChangeKey 测试数据源变更广播键的全局前缀。
func TestDatasourceChangeKey(t *testing.T) {
	Convey("Test DatasourceChangeKey", t, func() {
		Convey("数据源变更键应落在全局前缀下", func() {
			So(datasourceChangeKey, ShouldEqual, "zki/datasource/change")
		})
	})
}

// TestBroadcastChangeNoEtcd 未绑定 etcd 客户端时广播快速失败（写路径告警降级）。
func TestBroadcastChangeNoEtcd(t *testing.T) {
	Convey("Test BroadcastChange without etcd", t, func() {
		err := BroadcastChange(context.Background(), nil, "default", "ds")
		So(err, ShouldNotBeNil)
		So(err, ShouldEqual, etcd.ErrChangeUnavailable)
	})
}

// TestWatcherStartNoEtcd 未绑定 etcd 客户端时 Start 直接返回（无监听能力）。
func TestWatcherStartNoEtcd(t *testing.T) {
	Convey("Test Watcher Start without etcd", t, func() {
		watcher, _, _ := setupWatcher(t)
		So(watcher.Start(context.Background()), ShouldBeNil)
	})
}

// TestWatcherHandleResponse 测试数据源变更处理器按 (Namespace, Name) 精准同步本地池。
func TestWatcherHandleResponse(t *testing.T) {
	Convey("Test Watcher HandleResponse", t, func() {
		watcher, pools, db := setupWatcher(t)
		handler := &datasourceChangeHandler{db: db, pools: pools, logger: watcher.logger}
		insertDatasource(t, db, datasourceTestNamespace, "dsA", true)
		insertDatasource(t, db, datasourceTestNamespace, "dsB", false)

		Convey("启用数据源事件注册裸名 vtable", func() {
			So(handler.HandleChange(context.Background(), changePayloadOf(&DatasourceChangeEvent{Namespace: datasourceTestNamespace, Name: "dsA"})), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldContain, "dsA")
		})

		Convey("停用数据源事件注销 vtable", func() {
			// 预热：先注册 dsA 再停用，验证事件驱动注销
			So(handler.HandleChange(context.Background(), changePayloadOf(&DatasourceChangeEvent{Namespace: datasourceTestNamespace, Name: "dsA"})), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldContain, "dsA")
			if err := db.Model(&core.Datasource{}).
				Where("namespace_name = ? AND name = ?", datasourceTestNamespace, "dsA").
				Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			So(handler.HandleChange(context.Background(), changePayloadOf(&DatasourceChangeEvent{Namespace: datasourceTestNamespace, Name: "dsA"})), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldNotContain, "dsA")
		})

		Convey("已停用数据源事件保持未注册", func() {
			So(handler.HandleChange(context.Background(), changePayloadOf(&DatasourceChangeEvent{Namespace: datasourceTestNamespace, Name: "dsB"})), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldNotContain, "dsB")
		})

		Convey("负载无法解析时回退全量重载", func() {
			// 模拟 dsA 注册后从池中注销的陈旧状态
			So(handler.HandleChange(context.Background(), changePayloadOf(&DatasourceChangeEvent{Namespace: datasourceTestNamespace, Name: "dsA"})), ShouldBeNil)
			So(pools.Unregister(context.Background(), datasourceTestNamespace, "dsA"), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldNotContain, "dsA")

			// 发送不可解析负载（非 JSON 且无身份信息），回退全量重载重新注册启用数据源
			So(handler.HandleChange(context.Background(), []byte("not-json")), ShouldBeNil)
			So(pools.Pool(datasourceTestNamespace).Datasources(), ShouldContain, "dsA")
		})
	})
}

// insertDatasource 插入一条 redis（struct=string，惰性不联网）数据源记录。
func insertDatasource(t *testing.T, db *database.DB, namespace, name string, enabled bool) {
	t.Helper()
	if err := db.Create(&core.Datasource{
		NamespaceName: namespace,
		Name:          name,
		Type:          vtab.DatasourceTypeRedis,
		Enabled:       enabled,
		Config:        serializer.NewCrypto([]byte(`{"addr":"127.0.0.1","port":6379,"db":0,"struct":"string"}`)),
	}).Error; err != nil {
		t.Fatal(err)
	}
}

// changePayloadOf 构造携带指定数据源变更事件负载的 JSON 字节。
func changePayloadOf(event *DatasourceChangeEvent) []byte {
	payload, err := json.Marshal(event)
	if err != nil {
		panic("failed to marshal datasource change event: " + err.Error())
	}
	return payload
}
