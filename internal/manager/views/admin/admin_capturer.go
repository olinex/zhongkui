// Package admin 提供管理域（用户 / 分组 / RBAC / 命名空间 / 采集器 / 服务 / 依赖）的 API 处理器
// 与视图 In/Out 结构体。
//
// 本文件（admin_capturer.go）定义了抓包器管理相关的 API 路由处理器，
// 提供抓包器的列表查询、详情获取、创建、配置更新和删除功能。
// 抓包器配置存储在 etcd 中，元数据存储在数据库中，
// 因此所有写操作均需同时操作数据库和 etcd，以保证数据一致性。
package admin

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"github.com/samber/do/v2"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/provider"
)

// capturerStatusRunning / capturerStatusStopped 是运行状态短键，驱动前端
// model.capturer.option.status.* 的 i18n 标签渲染。
const (
	capturerStatusRunning = "running"
	capturerStatusStopped = "stopped"
)

// CapturerConfigIn 定义抓包器配置的请求输入结构。
// 该结构对应 reporter 进程的实际抓包参数，在创建和更新抓包器时使用。
// 配置数据序列化后存储在 etcd 中，供 reporter 进程读取。
//
// 字段说明：
//   - Device:  网络设备名称（如 eth0），长度限制 2~32 字符
//   - SnapLen:  每个数据包的捕获长度（字节数），决定截断位置
//   - Promisc: 是否启用混杂模式，启用后可捕获非本机的流量
//   - Timeout: 读超时时间，以毫秒为单位；交互输入与 JSON 均支持可读时长字符串
//     （如 "5s"、"1m30s"）或整数毫秒
//   - BpfExpr: BPF 过滤表达式（如 "tcp port 80"），长度限制 3~1024 字符
//   - Enabled: 是否启用该抓包器配置
type CapturerConfigIn struct {
	Device  string                `json:"device" viper:"device" prompt:"请输入网络设备名称（如 eth0）" binding:"required,min=2,max=32"`
	SnapLen uint32                `json:"snapLen" viper:"snap-len" prompt:"请输入抓包长度（字节）" binding:"required"`
	Promisc bool                  `json:"promisc" viper:"promisc" prompt:"是否启用混杂模式？"`
	Timeout serializer.DurationMS `json:"timeout" viper:"read-timeout" prompt:"请输入读超时时间（如 5s、1m30s）" binding:"required"`
	BpfExpr string                `json:"bpfExpr" viper:"bpf-expr" prompt:"请输入 BPF 过滤表达式（如 tcp port 80）" binding:"required,min=3,max=1024"`
	Enabled bool                  `json:"enabled" viper:"enabled" prompt:"是否启用该抓包器？"`
}

// ToGRPC 将 CapturerConfigIn 转换为 gRPC 协议层定义的 Config 结构。
// 用于在调用 core.Capturer 的 CreateWithConfig/UpdateConfig 方法时，
// 将 HTTP 请求的 JSON 配置转换为 etcd 存储所需的 protobuf 格式。
//
// 返回值：
//   - *reporter_v1.Config: 与 CapturerConfigIn 字段一一对应的 gRPC 配置对象
func (cc *CapturerConfigIn) ToGRPC() *reporter_v1.Config {
	return &reporter_v1.Config{
		Device:  cc.Device,
		SnapLen: cc.SnapLen,
		Promisc: cc.Promisc,
		Timeout: cc.Timeout.Milliseconds(),
		BpfExpr: cc.BpfExpr,
		Enabled: cc.Enabled,
	}
}

// CapturerCreateIn 定义创建抓包器的请求输入结构。
// 包含抓包器的标识信息（命名空间、名称、版本）和配置参数。
// 三个标识字段联合组成唯一索引，确保同一命名空间下不存在重复的抓包器。
//
// 字段说明：
//   - Namespace: 所属命名空间名称，长度限制 3~32 字符
//   - Name:      抓包器名称，长度限制 3~32 字符
//   - Version:   抓包器版本号，长度限制 3~32 字符
//   - Config:    抓包器的运行配置
type CapturerCreateIn struct {
	Namespace string           `json:"namespace" viper:"namespace" prompt:"请输入命名空间" binding:"required,min=3,max=32"`
	Name      string           `json:"name" viper:"name" prompt:"请输入抓包器名称" binding:"required,min=3,max=32"`
	Version   string           `json:"version" viper:"version" prompt:"请输入版本号" binding:"required,min=3,max=32"`
	Config    CapturerConfigIn `json:"config"`
}

// CapturerBaseDetailOut 定义抓包器的基础详情输出结构。
// 包含抓包器的标识信息和审计字段，用于列表和详情场景的公共部分。
//
// 嵌入字段说明：
//   - utils.BaseItemOut: 提供 ID、CreatedAt、UpdatedAt 三个公共审计字段
//   - NamespaceName:     所属命名空间名称
//   - Name:              抓包器名称
//   - Version:           抓包器版本号
type CapturerBaseDetailOut struct {
	utils.BaseItemOut
	NamespaceName string `json:"namespace"`
	Name          string `json:"name"`
	Version       string `json:"version"`
}

// CapturerDetailOut 定义抓包器的完整详情输出结构。
// 在基础详情之上增加了从 etcd 获取的运行配置信息。
// 用于 CapturerDetailGet 接口的响应体。
type CapturerDetailOut struct {
	CapturerBaseDetailOut
	Config CapturerConfigIn `json:"config"`
}

// CapturerListOut 定义抓包器列表的输出项结构。
// 在基础详情之上嵌入 BaseTotalOut 以支持分页总数统计。
// 用于 CapturerListGet 接口的响应体列表项。
type CapturerListOut struct {
	CapturerBaseDetailOut
	utils.BaseTotalOut
}

// CapturerListGet 处理获取抓包器列表的请求。
//
// 请求方式：POST（使用 JSON 请求体传递搜索参数）
// 请求体：CapturerListGetSearchIn（分页、排序、过滤参数）
//
// 处理流程：
//  1. 使用 defaults.Set 设置搜索参数的默认值（如分页参数）
//  2. 绑定并验证 JSON 请求体到搜索输入结构
//  3. 构建 GORM 搜索查询（包含过滤、排序、分页），附加总数统计
//  4. 执行查询并将结果映射到 CapturerListOut 列表
//  5. 返回包含总数和列表项的搜索结果
//
// 错误处理：
//   - 默认值设置失败 → 500 意外错误
//   - 请求体绑定/验证失败 → 400 请求体错误
//   - 查询构建或执行失败 → 500 意外错误
func CapturerListGet(c *gin.Context) {
	var in views.CapturerListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*CapturerListOut
	db := middlewares.GetCoreDatabase(c).Model(&core.Capturer{})
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **CapturerListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// CapturerDetailGet 处理获取抓包器详情的请求。
//
// 请求方式：GET
// URL 路径参数：:id — 抓包器的数据库主键 ID（uint64）
//
// 处理流程：
//  1. 从 URL 路径中解析抓包器 ID
//  2. 从数据库中查询抓包器元数据记录
//  3. 从 etcd 中获取抓包器的运行配置
//  4. 组装 CapturerDetailOut（含元数据 + 配置）并返回
//
// 错误处理：
//   - ID 解析失败 → 400 URL 参数错误（由 getUint64Param 处理）
//   - 数据库查询失败 → 500 意外错误
//   - etcd 配置获取失败 → 500 意外错误
func CapturerDetailGet(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.Capturer{}
	if err := db.WithContext(c).First(&instance, id).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	client := middlewares.GetEtcdClient(c)
	if config, err := instance.GetConfig(c, client); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		response.SimpleOK.AbortWithGinW(c, CapturerDetailOut{
			CapturerBaseDetailOut: CapturerBaseDetailOut{
				BaseItemOut: utils.BaseItemOut{
					ID:        instance.ID,
					CreatedAt: instance.CreatedAt,
					UpdatedAt: instance.UpdatedAt,
				},
				NamespaceName: instance.NamespaceName,
				Name:          instance.Name,
				Version:       instance.Version,
			},
			Config: CapturerConfigIn{
				Device:  config.Device,
				SnapLen: config.SnapLen,
				Promisc: config.Promisc,
				Timeout: serializer.DurationMS(config.Timeout),
				BpfExpr: config.BpfExpr,
				Enabled: config.Enabled,
			},
		})
	}
}

// CapturerCreate 处理创建抓包器的请求。
//
// 请求方式：POST
// 请求体：CapturerCreateIn（命名空间、名称、版本和配置参数）
//
// 处理流程：
//  1. 设置默认值并绑定/验证 JSON 请求体
//  2. 构造 core.Capturer 实例（含命名空间、名称、版本）
//  3. 调用 CreateWithConfig 在数据库事务中同时创建数据库记录和 etcd 配置
//  4. 成功后返回 200 OK
//
// 注意：
//   - CreateWithConfig 内部使用数据库事务，但 etcd 写入不在事务范围内，
//     存在潜在的跨系统一致性问题（详见 core.Capturer.CreateWithConfig 注释）
//
// 错误处理：
//   - 默认值设置失败 → 500 意外错误
//   - 请求体绑定/验证失败 → 400 请求体错误
//   - 创建失败（如唯一索引冲突） → 500 意外错误
func CapturerCreate(c *gin.Context) {
	var in CapturerCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	client := middlewares.GetEtcdClient(c)
	instance := core.Capturer{
		NamespaceName: in.Namespace,
		Name:          in.Name,
		Version:       in.Version,
	}
	if err := instance.CreateWithConfig(db, client, in.Config.ToGRPC()); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// CapturerUpdate 处理更新抓包器配置的请求。
//
// 请求方式：PUT/PATCH
// URL 路径参数：:id — 抓包器的数据库主键 ID（uint64）
// 请求体：CapturerConfigIn（新的抓包器运行配置）
//
// 处理流程：
//  1. 从 URL 路径中解析抓包器 ID
//  2. 绑定并验证 JSON 请求体到配置结构
//  3. 从数据库中查询抓包器记录（确保存在）
//  4. 调用 UpdateConfig 更新 etcd 中的配置
//  5. 成功后返回 200 OK
//
// 注意：
//   - 该接口仅更新 etcd 中的配置，不修改数据库记录
//   - 如需修改抓包器的标识信息（命名空间/名称/版本），需删除后重新创建
//
// 错误处理：
//   - ID 解析失败 → 400 URL 参数错误
//   - 请求体绑定/验证失败 → 400 请求体错误
//   - 数据库查询失败 → 500 意外错误
//   - etcd 配置更新失败 → 500 意外错误
func CapturerUpdate(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in CapturerConfigIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.Capturer{}
	if err := db.First(&instance, id).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	client := middlewares.GetEtcdClient(c)
	if err := instance.UpdateConfig(c, client, in.ToGRPC()); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// CapturerBatchDeleteIn 批量删除抓包器的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type CapturerBatchDeleteIn struct {
	Ids views.FlexList[uint64] `json:"ids"`
}

// CapturerBatchDelete 批量删除抓包器。
// 逐条复用原 capturerDelete 的 DeleteWithConfig 语义（数据库事务删除记录 + etcd 配置同步）；
// 抓包器不存在视为成功跳过（幂等）。
//
// 请求方式：POST
// 请求体：CapturerBatchDeleteIn（批量抓包器 ID）
//
// 错误处理：
//   - 请求体绑定失败 → 400 请求体错误
//   - 批量长度非法 → 400 请求体错误
//   - 单条删除失败（数据库或 etcd 错误）→ 记入失败明细，不中断其他条目
func CapturerBatchDelete(c *gin.Context) {
	var in CapturerBatchDeleteIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	client := middlewares.GetEtcdClient(c)
	views.BatchExecute(c, ids,
		func(id uint64) string { return strconv.FormatUint(id, 10) },
		func(id uint64) error {
			var instance core.Capturer
			if err := db.First(&instance, "id = ?", id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// 不存在视为成功跳过
					return nil
				}
				return errors.WithStack(err)
			}
			return instance.DeleteWithConfig(db, client)
		},
	)
}

// capturerStatusItem 内部聚合载体：一个采集器资源的最新在线会话。
type capturerStatusItem struct {
	namespace     string
	name          string
	version       string
	sessionID     string
	statusKey     string
	configVersion int64
	globalVersion int64
	lastSeen      time.Time
}

// CapturerStatusItemOut 单个采集器资源运行状态输出项。
// 字段为基本类型，逐字段映射自 etcd 中解析出的会话状态，不引用模型层结构体。
type CapturerStatusItemOut struct {
	// Namespace 命名空间
	Namespace string `json:"namespace"`
	// Name 采集器名称
	Name string `json:"name"`
	// Version 版本号
	Version string `json:"version"`
	// SessionID 最新在线会话 ID
	SessionID string `json:"sessionId"`
	// Status 运行状态短键（running / stopped）
	Status string `json:"status"`
	// ConfigVersion 配置版本
	ConfigVersion int64 `json:"configVersion"`
	// GlobalVersion 全局版本
	GlobalVersion int64 `json:"globalVersion"`
	// LastSeen 最后上报时刻
	LastSeen time.Time `json:"lastSeen"`
}

// CapturerStatusListOut 采集器运行状态列表输出。
type CapturerStatusListOut struct {
	// Items 在线采集器资源运行状态项
	Items []CapturerStatusItemOut `json:"items"`
}

// newCapturerStatusItemOut 将内部聚合项逐字段转换为视图层独立输出项。
func newCapturerStatusItemOut(item capturerStatusItem) CapturerStatusItemOut {
	return CapturerStatusItemOut{
		Namespace:     item.namespace,
		Name:          item.name,
		Version:       item.version,
		SessionID:     item.sessionID,
		Status:        item.statusKey,
		ConfigVersion: item.configVersion,
		GlobalVersion: item.globalVersion,
		LastSeen:      item.lastSeen,
	}
}

// statusKeyOf 将 proto 状态枚举映射为前端展示短键。
func statusKeyOf(st reporter_v1.StatusType) string {
	switch st {
	case reporter_v1.StatusType_STOPPED:
		return capturerStatusStopped
	default:
		return capturerStatusRunning
	}
}

// CapturerStatusListGet 处理采集器运行状态列表请求：读取 etcd 中 reporter 前缀下的
// 在线会话键，按资源聚合最新会话并返回列表。由于需从注入器取 etcd 客户端，返回
// gin.HandlerFunc 闭包。
//
// 键格式：{GlobalPrefix}/reporter/{namespace}/{name}/{version}/sts/{sessionID}，
// 值为 proto 序列化的 reporter_v1.FeedbackRequest（带租约，离线即过期消失）。
// 仅保留每个资源（namespace/name/version）中 timestamp 最大的一条会话。
func CapturerStatusListGet(i do.Injector) gin.HandlerFunc {
	return func(c *gin.Context) {
		client := provider.MustInvokeEtcdClient(i)

		ctx, cancel := context.WithTimeout(c, 5*time.Second)
		defer cancel()
		resp, err := client.Get(ctx, core.ReporterEtcdPrefix, clientv3.WithPrefix())
		if err != nil {
			response.CreateAccidentError(errors.WithStack(err)).AbortWithGinH(c)
			return
		}

		// latest 以资源标识为键，保存该资源最新（timestamp 最大）的在线会话。
		latest := make(map[string]capturerStatusItem)
		for _, kv := range resp.Kvs {
			item, ok := parseReporterStatusKey(string(kv.Key), kv.Value)
			if !ok {
				continue
			}
			key := item.namespace + "/" + item.name + "/" + item.version
			if prev, exists := latest[key]; !exists || item.lastSeen.After(prev.lastSeen) {
				latest[key] = item
			}
		}

		items := make([]CapturerStatusItemOut, 0, len(latest))
		for _, item := range latest {
			items = append(items, newCapturerStatusItemOut(item))
		}
		sortCapturerStatusItems(items)
		response.SimpleOK.AbortWithGinW(c, CapturerStatusListOut{Items: items})
	}
}

// parseReporterStatusKey 解析单个状态键并反序列化状态值。
// 键去掉 core.ReporterEtcdPrefix 后须为 {namespace}/{name}/{version}/sts/{sessionID}
// 五段结构（跳过 cfg 配置键），值按 reporter_v1.FeedbackRequest 解析以取得
// timestamp / session_id / status。
func parseReporterStatusKey(key string, value []byte) (capturerStatusItem, bool) {
	rest, ok := strings.CutPrefix(key, core.ReporterEtcdPrefix)
	if !ok {
		return capturerStatusItem{}, false
	}
	segments := strings.Split(rest, "/")
	if len(segments) != 5 || segments[3] != "sts" {
		return capturerStatusItem{}, false
	}
	var req reporter_v1.FeedbackRequest
	if err := proto.Unmarshal(value, &req); err != nil {
		return capturerStatusItem{}, false
	}
	return capturerStatusItem{
		namespace:     segments[0],
		name:          segments[1],
		version:       segments[2],
		sessionID:     segments[4],
		statusKey:     statusKeyOf(req.GetStatus().GetType()),
		configVersion: req.GetStatus().GetConfigVersion(),
		globalVersion: req.GetStatus().GetGlobalVersion(),
		lastSeen:      time.UnixMilli(req.GetTimestamp()),
	}, true
}

// sortCapturerStatusItems 按 namespace → name → version 稳定排序列表。
func sortCapturerStatusItems(items []CapturerStatusItemOut) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && capturerStatusLess(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

// capturerStatusLess 比较两个输出项的字典序（namespace → name → version）。
func capturerStatusLess(a, b CapturerStatusItemOut) bool {
	if a.Namespace != b.Namespace {
		return a.Namespace < b.Namespace
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Version < b.Version
}
