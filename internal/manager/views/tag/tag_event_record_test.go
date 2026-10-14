package tag

import (
	// 标准库
	"encoding/json"
	"net/http"
	"testing"
	"time"

	// 第三方
	. "github.com/smartystreets/goconvey/convey"
	"gorm.io/datatypes"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/models/network"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/tagengine"
)

// insertHttpTransportRowMeta 在 raw 库落一条 http_transports 明细行，timestamp 与 created_at 独立指定。
// 仅填充反查断言涉及的字段，其余列由 ClickHouse 以默认值填充。
func insertHttpTransportRowMeta(rawDB *database.DB, id serializer.HexUUID, namespace, method, host, path string, timestamp, createdAt time.Time) {
	clientIP, err := serializer.NewIPAddr("192.168.1.100")
	So(err, ShouldBeNil)
	serverIP, err := serializer.NewIPAddr("10.0.0.8")
	So(err, ShouldBeNil)
	row := &network.HttpTransport{
		RowMeta:       network.RowMeta{ID: id},
		BaseMeta:      network.BaseMeta{Namespace: namespace, Capture: "capture-a", Version: "1.0", SessionID: serializer.NewHexUUIDv4()},
		TimestampMeta: network.TimestampMeta{Timestamp: timestamp, CreatedAt: createdAt},
		TraceMeta:     network.TraceMeta{TraceID: serializer.NewHexUUIDv4()},
		HttpBase: network.HttpBase{
			ClientIP:    *clientIP,
			ServerIP:    *serverIP,
			HttpVersion: "HTTP/1.1",
		},
		HttpURI: network.HttpURI{
			Host: host,
			Path: path,
			// queries 列必须为合法 JSON：NULL/空值扫回后 datatypes.JSON 无法序列化，
			// 与流量列表/详情生产数据（恒为 JSON 文档）保持一致
			Queries: datatypes.JSON("{}"),
		},
		HttpRequestBody: network.HttpRequestBody{
			Method: method,
		},
		AppTailer: network.AppTailer{
			// 四个方向时间戳必须赋有效值：零值会以 0000-00-00 存储，
			// ClickHouse 扫回后 time.Time 年份越界导致 JSON 序列化失败
			ClientStartTimestamp: timestamp,
			ClientEndTimestamp:   timestamp,
			ServerStartTimestamp: timestamp,
			ServerEndTimestamp:   timestamp,
		},
	}
	So(rawDB.Create(row).Error, ShouldBeNil)
}

// insertHttpTransportRowAt 在 raw 库落一条 http_transports 明细行，timestamp 与 created_at 均取 createdAt。
// 用于模拟实时捕获（入库时间 ≈ 流量时间）的普通行。
func insertHttpTransportRowAt(rawDB *database.DB, id serializer.HexUUID, namespace, method, host, path string, createdAt time.Time) {
	insertHttpTransportRowMeta(rawDB, id, namespace, method, host, path, createdAt, createdAt)
}

// insertHttpTransportRow 在 raw 库落一条 http_transports 明细行（入库时间取当前时刻）。
func insertHttpTransportRow(rawDB *database.DB, id serializer.HexUUID, namespace, method, host, path string) {
	insertHttpTransportRowAt(rawDB, id, namespace, method, host, path, time.Now().UTC())
}

// tagEventRecordHitPath 拼接命中条件原始数据反查路由路径（前端透传入参，无需命中明细 ID）。
func tagEventRecordHitPath() string {
	return "/tag-event/record/hit/records"
}

// TestTagEventRecordHitRecords 标签事件记录命中条件原始数据反查全链路。
// 入参由前端从详情透传（model/ids/createdAtFrom/createdAtTo），后端不再回查命中明细归属，
// 直接按模型、ID 列表与 created_at 范围查询 raw 库；时间范围贴近检测窗口 [now-5m, now]，
// 上界由后端 +1 分钟余量补偿分钟桶边界。
func TestTagEventRecordHitRecords(t *testing.T) {
	Convey("测试标签事件记录命中条件原始数据反查", t, func() {
		db := createTagEventRuleTestDB(t)
		// goconvey 会为每个叶子重新执行外层 Convey 体，导致每次 new 一个未提交事务；
		// 若不在此处及时回滚，多个叶子事务会在连接池上重叠持有行锁，引发 PostgreSQL 锁等待（单测挂死）。
		Reset(func() {
			So(db.Rollback().Error, ShouldBeNil)
		})
		rawDB := createTagEventRuleRawDB(t)
		r := setupTagEventRuleTestRouter(db, rawDB)

		// 确定性 UUID：两条存在、一条不存在，用于验证命中与部分命中
		v1 := serializer.MustHexUUIDv4FromString("11111111-2222-3333-4444-555555555555")
		v2 := serializer.MustHexUUIDv4FromString("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
		v3 := serializer.MustHexUUIDv4FromString("bbbbbbbb-cccc-dddd-eeee-ffffffffffff")

		// 检测时间基准；入参时间范围为 [now-5m, now]（贴近检测窗口，验证预筛不丢命中）
		now := time.Now().UTC()
		reqIn := func(ids serializer.HexUUIDs) *TagEventRecordHitRecordsIn {
			return &TagEventRecordHitRecordsIn{
				Model:         tagengine.MustTypeKey(&network.HttpTransport{}),
				IDs:           ids,
				CreatedAtFrom: now.Add(-5 * time.Minute),
				CreatedAtTo:   now,
			}
		}

		Convey("正常反查：按命中 IDs 与时间范围返回对应 http_transports 明细", func() {
			insertHttpTransportRow(rawDB, v1, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/users")
			insertHttpTransportRow(rawDB, v2, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/login")

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), reqIn(serializer.HexUUIDs{v1, v2}))
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 2)
			got := make(map[string]bool, len(resp.Data.Items))
			for _, item := range resp.Data.Items {
				So(item.Namespace, ShouldEqual, middlewares.DefaultNamespace)
				got[item.ID.String()] = true
			}
			So(got, ShouldResemble, map[string]bool{v1.String(): true, v2.String(): true})
		})

		Convey("空 IDs 返回空 items", func() {
			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), reqIn(nil))
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 0)
		})

		Convey("未知模型返回空 items（200）", func() {
			in := reqIn(serializer.HexUUIDs{v1})
			in.Model = "example.com/UnregisteredModel"

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), in)
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 0)
		})

		Convey("timestamp 范围外的不返回：仅返回检测窗口内的命中行", func() {
			insertHttpTransportRow(rawDB, v1, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/users")
			// 旧流量时间（timestamp 超出 [now-5m, now] 下界）的记录被精确过滤排除
			insertHttpTransportRowAt(rawDB, v3, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/old", now.Add(-2*time.Hour))

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), reqIn(serializer.HexUUIDs{v1, v3}))
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].ID.String(), ShouldEqual, v1.String())
		})

		Convey("精确过滤基于 timestamp：created_at 晚于窗口的 pcap 回放行仍返回", func() {
			// 模拟 pcap 回放/历史导入：流量时间在窗口内（timestamp=now-1min），
			// 入库时间远晚于窗口（created_at=now+2h），旧实现按 created_at 上界过滤会误排
			insertHttpTransportRowMeta(rawDB, v1, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/users", now.Add(-time.Minute), now.Add(2*time.Hour))

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), reqIn(serializer.HexUUIDs{v1}))
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].ID.String(), ShouldEqual, v1.String())
		})

		Convey("IDs 含不存在的 id 时仅返回命中的行", func() {
			insertHttpTransportRow(rawDB, v1, middlewares.DefaultNamespace, "GET", "api.example.com", "/admin/users")

			w := doTagEventRuleRequest(r, http.MethodPost, tagEventRecordHitPath(), reqIn(serializer.HexUUIDs{v1, v3}))
			So(w.Code, ShouldEqual, http.StatusOK)
			var resp struct {
				Code int                          `json:"code"`
				Data *TagEventRecordHitRecordsOut `json:"data"`
			}
			So(json.Unmarshal(w.Body.Bytes(), &resp), ShouldBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)
			So(resp.Data, ShouldNotBeNil)
			So(resp.Data.Items, ShouldHaveLength, 1)
			So(resp.Data.Items[0].ID.String(), ShouldEqual, v1.String())
		})
	})
}
