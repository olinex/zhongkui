// Package routes 提供 API 路由层的通用基础设施，包括：
//   - 抓包器（Capturer）的 CRUD 路由处理器（本文件）
//   - 列表选项的元数据描述与查询参数解析（base.go）
//   - 搜索与分页的通用查询构建（search.go）
//   - 其他资源（用户、组、命名空间等）的路由处理器
//
// 本文件（admin_capturer.go）定义了抓包器管理相关的 API 路由处理器，
// 提供抓包器的列表查询、详情获取、创建、配置更新和删除功能。
// 抓包器配置存储在 etcd 中，元数据存储在数据库中，
// 因此所有写操作均需同时操作数据库和 etcd，以保证数据一致性。
package routes

import (
	"errors"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/core"
	reporter_v1 "zhongkui.com/zhongkui/internal/pkg/protobuf/reporter/v1"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// CapturerConfigIn 定义抓包器配置的请求输入结构。
// 该结构对应 reporter 进程的实际抓包参数，在创建和更新抓包器时使用。
// 配置数据序列化后存储在 etcd 中，供 reporter 进程读取。
//
// 字段说明：
//   - Device:  网络设备名称（如 eth0），长度限制 2~32 字符
//   - SnapLen:  每个数据包的捕获长度（字节数），决定截断位置
//   - Promisc: 是否启用混杂模式，启用后可捕获非本机的流量
//   - Timeout: 读超时时间（毫秒），超时后返回已缓冲的数据
//   - BpfExpr: BPF 过滤表达式（如 "tcp port 80"），长度限制 3~1024 字符
//   - Enabled: 是否启用该抓包器配置
type CapturerConfigIn struct {
	Device  string `json:"device" viper:"device" prompt:"请输入网络设备名称（如 eth0）" binding:"required,min=2,max=32"`
	SnapLen uint32 `json:"snapLen" viper:"snap-len" prompt:"请输入抓包长度（字节）" binding:"required"`
	Promisc bool   `json:"promisc" viper:"promisc" prompt:"是否启用混杂模式？"`
	Timeout int32  `json:"timeout" viper:"read-timeout" prompt:"请输入读超时时间（毫秒）" binding:"required"`
	BpfExpr string `json:"bpfExpr" viper:"bpf-expr" prompt:"请输入 BPF 过滤表达式（如 tcp port 80）" binding:"required,min=3,max=1024"`
	Enabled bool   `json:"enabled" viper:"enabled" prompt:"是否启用该抓包器？"`
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
		Timeout: cc.Timeout,
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
// 用于 capturerDetailGet 接口的响应体。
type CapturerDetailOut struct {
	CapturerBaseDetailOut
	Config CapturerConfigIn `json:"config"`
}

// CapturerListOut 定义抓包器列表的输出项结构。
// 在基础详情之上嵌入 BaseTotalOut 以支持分页总数统计。
// 用于 capturerListGet 接口的响应体列表项。
type CapturerListOut struct {
	CapturerBaseDetailOut
	utils.BaseTotalOut
}

// capturerListGet 处理获取抓包器列表的请求。
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
func capturerListGet(c *gin.Context) {
	var in CapturerListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*CapturerListOut
	db := manager.GetDatabase(c).Model(&core.Capturer{})
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	out := utils.NewSearchOut(items)
	manager.SimpleOK.AbortWithGinW(c, out)
}

// capturerDetailGet 处理获取抓包器详情的请求。
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
func capturerDetailGet(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetDatabase(c)
	instance := core.Capturer{}
	if err := db.WithContext(c).First(&instance, id).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	client := manager.GetEtcdClient(c)
	if config, err := instance.GetConfig(c, client); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else {
		manager.SimpleOK.AbortWithGinW(c, CapturerDetailOut{
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
				Timeout: config.Timeout,
				BpfExpr: config.BpfExpr,
				Enabled: config.Enabled,
			},
		})
	}
}

// capturerCreate 处理创建抓包器的请求。
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
func capturerCreate(c *gin.Context) {
	var in CapturerCreateIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := manager.GetDatabase(c)
	client := manager.GetEtcdClient(c)
	instance := core.Capturer{
		NamespaceName: in.Namespace,
		Name:          in.Name,
		Version:       in.Version,
	}
	if err := instance.CreateWithConfig(db, client, in.Config.ToGRPC()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// capturerUpdate 处理更新抓包器配置的请求。
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
func capturerUpdate(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	var in CapturerConfigIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := manager.GetDatabase(c)
	instance := core.Capturer{}
	if err := db.First(&instance, id).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	client := manager.GetEtcdClient(c)
	if err := instance.UpdateConfig(c, client, in.ToGRPC()); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// capturerDelete 处理删除抓包器的请求。
//
// 请求方式：DELETE
// URL 路径参数：:id — 抓包器的数据库主键 ID（uint64）
//
// 处理流程：
//  1. 从 URL 路径中解析抓包器 ID
//  2. 从数据库中查询抓包器记录（确认存在）
//  3. 调用 DeleteWithConfig 在数据库事务中同时删除数据库记录和 etcd 配置
//  4. 成功后返回 200 OK
//
// 错误处理：
//   - ID 解析失败 → 400 URL 参数错误
//   - 抓包器不存在（ErrRecordNotFound） → 400 关联记录错误
//   - 删除过程中记录不存在 → 400 关联记录错误
//   - 其他数据库或 etcd 错误 → 500 意外错误
func capturerDelete(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	db := manager.GetDatabase(c)
	client := manager.GetEtcdClient(c)
	var instance core.Capturer
	if err := db.First(&instance, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
			return
		} else {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	} else if err := instance.DeleteWithConfig(db, client); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrCannotDeleteCauseRelateRecord.AbortWithGinH(c)
			return
		} else {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}
	manager.SimpleOK.AbortWithGinH(c)
}
