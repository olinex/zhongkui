package datasource

import (
	// 标准库

	"io"
	"strconv"
	"strings"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/datasource"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/sqlitevtab/vtab"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// DatasourceCreateIn 定义创建第三方数据源的请求输入结构。
// 命名空间取自请求上下文，数据源名称在同一命名空间内唯一（本地身份）；类型必须为
// vtab 支持的类型（redis / mysql / postgres / clickhouse / sqlite_file / etcd / mongo）
// 之一；Config 为连接配置的 JSON 文本，结构随类型不同而不同。
type DatasourceCreateIn struct {
	// Name 数据源名称，必填，长度 1~128；注册为该命名空间池内的裸名 vtable
	Name string `json:"name" binding:"required,min=1,max=128"`
	// Type 数据源类型键，必填，长度 1~16，必须为支持的类型之一
	Type string `json:"type" binding:"required,max=16"`
	// Enabled 是否启用，默认为启用
	Enabled bool `json:"enabled" default:"true"`
	// Config 连接配置（JSON 文本），必填，结构随数据源类型不同而不同
	Config string `json:"config" binding:"required"`
	// Description 数据源描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// DatasourceUpdateIn 定义更新第三方数据源的请求输入结构。
// 数据源名称与类型不可变（不接受二者），仅允许调整启用状态、配置与描述。
type DatasourceUpdateIn struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Config 连接配置（JSON 文本）
	Config string `json:"config"`
	// Description 数据源描述，最长 256 字符
	Description string `json:"description" binding:"max=256"`
}

// DatasourceDetailOut 定义第三方数据源的详情输出结构。
// 嵌入 BaseItemOut 携带审计字段，业务字段与模型字段一一对应；
// Name 同时是该数据源注册为 vtable 后 rego 可引用的裸名。
type DatasourceDetailOut struct {
	utils.BaseItemOut
	// Name 数据源名称（命名空间内本地身份），即 rego 引用的裸名 vtable
	Name string `json:"name"`
	// Type 数据源类型键（redis 或 mysql）
	Type string `json:"type"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Config 连接配置的密文。密文由 gorm 模型经 crypto 序列化器落库/读取，
	// 本 Out 结构直接复用模型 *serializer.Crypto，经其 MarshalJSON 输出 base64 密文；
	// 字段经 form 标签标记为 crypto，由 meta options 告知前端该字段已加密
	Config *serializer.Crypto `json:"config" form:"kind:crypto"`
	// Description 数据源描述
	Description string `json:"description"`
}

// DatasourceListOut 定义第三方数据源列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type DatasourceListOut struct {
	DatasourceDetailOut
	utils.BaseTotalOut
}

// DatasourceBatchIn 批量操作第三方数据源的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type DatasourceBatchIn struct {
	// Ids 待批量操作的数据源 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
}

// DatasourceTestIn 测试连接请求输入：传入数据源类型与连接配置 JSON。
type DatasourceTestIn struct {
	// Type 数据源类型键（redis / mysql / postgres / clickhouse / sqlite_file / etcd / mongo）
	Type string `json:"type" binding:"required,max=16"`
	// Config 连接配置（JSON 文本），结构与数据源类型对应
	Config string `json:"config" binding:"required"`
}

// DatasourceTestOut 测试连接结果输出：Success 表示连通性，Message 为失败原因（成功可为空）。
type DatasourceTestOut struct {
	// Success 是否连通
	Success bool `json:"success"`
	// Message 失败原因（Success=true 时通常为空）
	Message string `json:"message"`
}

// DatasourceIntrospectIn 自省表结构请求输入。
type DatasourceIntrospectIn struct {
	// Type 数据源类型键
	Type string `json:"type" binding:"required,max=16"`
	// Config 连接配置（JSON 文本）
	Config string `json:"config" binding:"required"`
}

// DatasourceIntrospectOut 自省表结构结果输出：Success 表示自省成功，Message 为失败原因，
// Columns 为解析出的列定义（供前端继续调整）。
type DatasourceIntrospectOut struct {
	// Success 是否自省成功
	Success bool `json:"success"`
	// Message 失败原因（Success=true 时通常为空）
	Message string `json:"message"`
	// Columns 解析出的列定义（列名 + 类型键）
	Columns []vtab.DatasourceColumn `json:"columns"`
}

// DatasourceTypeOut 描述单个第三方数据源类型的枚举项。
type DatasourceTypeOut struct {
	// Type 数据源类型键（redis / mysql）
	Type string `json:"type"`
	// Name 数据源类型显示名称（当前为类型键本身）
	Name string `json:"name"`
	// Config 该类型连接配置的字段 schema，供前端动态渲染创建/编辑表单
	Config []*views.FieldMeta `json:"config"`
}

// DatasourceTypesOut 定义第三方数据源类型枚举的输出结构。
type DatasourceTypesOut struct {
	// Types 第三方数据源类型列表
	Types []DatasourceTypeOut `json:"types"`
}

// DatasourceTypeConfigGet 处理获取第三方数据源类型枚举的请求。
// 返回全部已注册数据源类型及其配置字段 schema，供前端动态渲染创建/编辑表单。schema
// 依据对应 vtab 前端配置结构体的 binding/default 标签经 views.getStructFields 反射生成
// （与 OPTIONS 元信息同一事实来源）。
func DatasourceTypeConfigGet(c *gin.Context) {
	plugins := vtab.DatasourceTypes()
	results := make([]DatasourceTypeOut, 0, len(plugins))
	for _, p := range plugins {
		results = append(results, DatasourceTypeOut{
			Type:   p.Key(),
			Name:   p.Key(),
			Config: views.GetStructFields(p.ConfigType()),
		})
	}
	response.SimpleOK.AbortWithGinW(c, DatasourceTypesOut{Types: results})
}

// DatasourceListGet 处理获取第三方数据源列表的请求。
// 支持分页、排序与过滤（时间/名称/类型/启用状态），仅返回当前命名空间内的数据源。
// 查询按 gorm 模型扫描（config 经 crypto 序列化器读取），再映射为接口层 Out 结构。
func DatasourceListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in views.DatasourceListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 以局部扫描行承载 gorm 模型与注入的 total 列（用于分页总数），随后映射为 Out 结构
	var rows []struct {
		core.Datasource
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&core.Datasource{}).Where("namespace_name = ?", namespace)
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*DatasourceListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		detail := newDatasourceDetailOut(&rows[i].Datasource)
		items = append(items, &DatasourceListOut{
			DatasourceDetailOut: detail,
			BaseTotalOut:        utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **DatasourceListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// DatasourceCreate 处理创建第三方数据源的请求。
// 创建前先经 pools.Register 校验配置并在当前命名空间池注册为裸名 vtable（MySQL 注册期间
// 自省一次表结构），成功后再落库；落库失败则回滚注销刚注册的 vtable，保证内存池与 DB 一致。
// sqlite_file 类型以 multipart/form-data 提交：.db 文件先经 S3 加密上传（Store），随后
// Register 依赖 Materialize 从 S3 拉取解密建表；任一环节失败都清理已上传对象与已注册
// vtable，保证「S3 对象 ↔ 配置」共存亡、不泄露无归属文件。
// 数据源归属当前请求命名空间。
func DatasourceCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in DatasourceCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// sqlite_file 前端以 multipart/form-data 提交（含 .db 文件流），其余类型仍为 JSON 体。
	// 仅 sqlite 需要 file 字段；views.ReadMultipartFile 流式返回 file 部分，避免整文件缓冲。
	var file io.Reader
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		var err error
		if file, err = views.ReadMultipartFile(c, func(name string, body []byte) {
			switch name {
			case "name":
				in.Name = string(body)
			case "type":
				in.Type = string(body)
			case "enabled":
				in.Enabled, _ = strconv.ParseBool(string(body))
			case "config":
				in.Config = string(body)
			case "description":
				in.Description = string(body)
			}
		}); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		if closer, ok := file.(io.ReadCloser); ok {
			defer closer.Close()
		}
	} else if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := vtab.ValidateLocalName(in.Name); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if !isSupportedDatasourceType(in.Type) {
		response.CreateBadBodyError(nil).AbortWithGinH(c)
		return
	}
	pools := middlewares.GetSQLiteVTabPools(c)
	// sqlite_file：先把 .db 文件加密上传到 S3，再由 Register 的 Materialize 拉取建表；
	// 后续注册/落库任一失败都要清理该对象（对象键由命名空间+数据源名派生，随配置共存亡）。
	objectKey := ""
	if in.Type == vtab.DatasourceTypeSQLiteFile {
		if file == nil {
			response.CreateBadBodyError(errors.New("creating a sqlite datasource requires an uploaded .db file")).AbortWithGinH(c)
			return
		}
		objectKey = vtab.SQLiteObjectKey(namespace, in.Name)
		if err := pools.StoreSQLiteObject(c, namespace, in.Name, file); err != nil {
			if errors.Is(err, vtab.ErrSQLiteStoreNotInjected) {
				response.CreateBadBodyError(errors.New("creating a sqlite datasource requires S3 object storage to be enabled")).AbortWithGinH(c)
			} else {
				response.CreateAccidentError(err).AbortWithGinH(c)
			}
			return
		}
	}
	// 先注册（即校验）：配置非法或建数据源自省失败时在此报错，不落库
	if err := pools.Register(c, namespace, in.Name, in.Type, []byte(in.Config)); err != nil {
		if objectKey != "" {
			_ = pools.RemoveSQLiteObject(c, namespace, in.Name)
		}
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := &core.Datasource{
		NamespaceName: namespace,
		Name:          in.Name,
		Type:          in.Type,
		Enabled:       in.Enabled,
		Config:        serializer.NewCrypto([]byte(in.Config)),
		Description:   in.Description,
	}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).Create(instance).Error; err != nil {
		// 落库失败时回滚已注册的 vtable 与已上传的 S3 对象，保持内存池/对象与 DB 一致
		_ = pools.Unregister(c, namespace, in.Name)
		if objectKey != "" {
			_ = pools.RemoveSQLiteObject(c, namespace, in.Name)
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 停用创建的数据源不常驻 vtable：注册仅为校验配置，落库成功后按启用状态收敛注册器
	if !in.Enabled {
		_ = pools.Unregister(c, namespace, in.Name)
	}
	// 广播变更事件，通知 worker 等消费进程同步本地 vtable 池（best-effort）
	broadcastDatasourceChange(c, namespace, in.Name)
	response.SimpleOK.AbortWithGinH(c)
}

// DatasourceDetailGet 处理获取第三方数据源详情的请求。
// 仅允许访问当前命名空间内的数据源，不存在时返回 404。
func DatasourceDetailGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := core.Datasource{}
	if err := middlewares.GetCoreDatabase(c).WithContext(c).
		Where("id = ? AND namespace_name = ?", id, namespace).
		First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newDatasourceDetailOut(&instance))
}

// DatasourceUpdate 处理更新第三方数据源信息的请求。
// 数据源名称与类型不可变，仅更新启用状态、配置与描述；仅允许更新当前命名空间内
// 的数据源。更新启用状态下先经 pools.Register 校验并覆盖注册裸名 vtable（DB 失败
// 时用原配置恢复注册）；停用状态下直接落库后注销 vtable（best-effort）。
func DatasourceUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.Datasource{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var in DatasourceUpdateIn
	// sqlite_file 类型可经 multipart/form-data 携带新的 .db 文件（覆盖同一 S3 对象）；
	// 其余字段与类型仍为 JSON 体。
	var file io.Reader
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		var err error
		if file, err = views.ReadMultipartFile(c, func(name string, body []byte) {
			switch name {
			case "enabled":
				in.Enabled, _ = strconv.ParseBool(string(body))
			case "config":
				in.Config = string(body)
			case "description":
				in.Description = string(body)
			}
		}); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		if closer, ok := file.(io.ReadCloser); ok {
			defer closer.Close()
		}
	} else if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	pools := middlewares.GetSQLiteVTabPools(c)
	// sqlite_file 且提供了新的 .db 文件：先加密上传覆盖同一对象键，再走 Register 的
	// Materialize 以新文件重注册；上传失败即中止（对象键稳定，未造成无归属残留）。
	if instance.Type == vtab.DatasourceTypeSQLiteFile && file != nil {
		if err := pools.StoreSQLiteObject(c, namespace, instance.Name, file); err != nil {
			if errors.Is(err, vtab.ErrSQLiteStoreNotInjected) {
				response.CreateBadBodyError(errors.New("updating a sqlite datasource requires S3 object storage to be enabled")).AbortWithGinH(c)
			} else {
				response.CreateAccidentError(err).AbortWithGinH(c)
			}
			return
		}
	}
	if in.Enabled {
		// 启用/保持启用：先注册新配置（即校验），成功后落库
		if err := pools.Register(c, namespace, instance.Name, instance.Type, []byte(in.Config)); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
	} else if err := vtab.ValidateConfigJSON(instance.Type, []byte(in.Config)); err != nil {
		// 停用：不建立连接，但仍校验配置 JSON 与该类型的结构一致性，避免非法配置落库
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
	update := core.Datasource{
		Enabled:     in.Enabled,
		Config:      serializer.NewCrypto([]byte(in.Config)),
		Description: in.Description,
	}
	if err := db.Model(&core.Datasource{}).
		Where("id = ? AND namespace_name = ?", instance.ID, namespace).
		Select("enabled", "config", "description").
		Updates(&update).Error; err != nil {
		// 落库失败：恢复注册状态（启用用原配置覆盖注册，停用则注销）
		if in.Enabled {
			if instance.Enabled {
				_ = pools.Register(c, namespace, instance.Name, instance.Type, instance.ConfigData())
			} else {
				_ = pools.Unregister(c, namespace, instance.Name)
			}
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 落库成功后，按目标启用状态收敛注册器（启用已注册/停用已注销）
	if !in.Enabled {
		_ = pools.Unregister(c, namespace, instance.Name)
	}
	// 广播变更事件，通知 worker 等消费进程同步本地 vtable 池（best-effort）
	broadcastDatasourceChange(c, namespace, instance.Name)
	response.SimpleOK.AbortWithGinH(c)
}

// DatasourceBatchDelete 批量删除第三方数据源。
// 逐条先删库再注销裸名 vtable（best-effort，忽略未注册）；数据源不存在
// （或不属于当前命名空间）视为成功跳过（幂等）。
func DatasourceBatchDelete(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in DatasourceBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	pools := middlewares.GetSQLiteVTabPools(c)
	views.BatchExecute(c, ids,
		func(id uint64) string { return strconv.FormatUint(id, 10) },
		func(id uint64) error {
			instance := core.Datasource{}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", instance.ID, namespace).Delete(&core.Datasource{}).Error; err != nil {
				return errors.WithStack(err)
			}
			// 先删库再注销 vtable（best-effort，忽略未注册）
			if err := pools.Unregister(c, namespace, instance.Name); err != nil {
				return errors.WithStack(err)
			}
			// sqlite_file：注销后清理其 S3 加密对象，避免无归属文件泄露（对象与配置共存亡）
			if instance.Type == vtab.DatasourceTypeSQLiteFile {
				if err := pools.RemoveSQLiteObject(c, namespace, instance.Name); err != nil &&
					!errors.Is(err, vtab.ErrSQLiteStoreNotInjected) {
					return errors.WithStack(err)
				}
			}
			// 广播变更事件，通知 worker 等消费进程同步本地 vtable 池（best-effort）
			broadcastDatasourceChange(c, namespace, instance.Name)
			return nil
		},
	)
}

// DatasourceBatchSetEnabled 批量启用/停用第三方数据源。
// 启用：先 pools.Register（即校验）落库前注册 vtable，DB 失败则回滚注销；
// 停用：先落库再注销 vtable（best-effort）。逐条幂等（已处于目标状态直接成功）；
// 数据源不存在（或不属于当前命名空间）视为成功跳过。
func DatasourceBatchSetEnabled(enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		namespace, ok := middlewares.RequireNamespace(c)
		if !ok {
			return
		}
		var in DatasourceBatchIn
		if err := c.ShouldBindBodyWithJSON(&in); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		ids := []uint64(in.Ids)
		if !views.ValidFlexLen(c, ids) {
			return
		}
		db := middlewares.GetCoreDatabase(c)
		pools := middlewares.GetSQLiteVTabPools(c)
		views.BatchExecute(c, ids,
			func(id uint64) string { return strconv.FormatUint(id, 10) },
			func(id uint64) error {
				instance := core.Datasource{}
				if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", id, namespace).First(&instance).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return nil // 不存在视为成功跳过
					}
					return errors.WithStack(err)
				}
				if instance.Enabled == enabled {
					return nil // 幂等短路
				}
				if enabled {
					// 先注册（即校验），DB 更新失败则回滚注销
					if err := pools.Register(c, namespace, instance.Name, instance.Type, instance.ConfigData()); err != nil {
						return errors.WithStack(err)
					}
					if err := db.WithContext(c).Model(&core.Datasource{}).
						Where("id = ? AND namespace_name = ?", instance.ID, namespace).
						Update("enabled", true).Error; err != nil {
						_ = pools.Unregister(c, namespace, instance.Name)
						return errors.WithStack(err)
					}
					// 广播变更事件，通知 worker 等消费进程同步本地 vtable 池（best-effort）
					broadcastDatasourceChange(c, namespace, instance.Name)
					return nil
				}
				// 停用：先落库再注销 vtable（best-effort，忽略未注册）
				if err := db.WithContext(c).Model(&core.Datasource{}).
					Where("id = ? AND namespace_name = ?", instance.ID, namespace).
					Update("enabled", false).Error; err != nil {
					return errors.WithStack(err)
				}
				if err := pools.Unregister(c, namespace, instance.Name); err != nil {
					return errors.WithStack(err)
				}
				// 广播变更事件，通知 worker 等消费进程同步本地 vtable 池（best-effort）
				broadcastDatasourceChange(c, namespace, instance.Name)
				return nil
			},
		)
	}
}

// broadcastDatasourceChange 写路径后置动作：广播数据源变更事件通知其他进程。
// 本进程的本地池已在写路径收敛，广播仅服务跨进程热同步（worker 等消费进程的
// Watcher 按 (Namespace, Name) 从 DB 重读并同步各自本地 vtable 池）；
// 广播失败仅记录告警，不阻断已提交的数据源变更。
func broadcastDatasourceChange(c *gin.Context, namespace, name string) {
	if err := datasource.BroadcastChange(c, middlewares.GetEtcdClient(c), namespace, name); err != nil {
		middlewares.GetLogger(c).Warnf(c, nil, "broadcast datasource change failed, name=%s, err=%v", name, err)
	}
}

// isSupportedDatasourceType 判断数据源类型是否为 vtab 支持的类型。
// 判断类型键是否已有插件注册。
func isSupportedDatasourceType(typ string) bool {
	return vtab.IsDatasourceTypeRegistered(typ)
}

// newDatasourceDetailOut 将 Datasource 转换为基础上的详情输出结构。
func newDatasourceDetailOut(ds *core.Datasource) DatasourceDetailOut {
	return DatasourceDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        ds.ID,
			CreatedAt: ds.CreatedAt,
			UpdatedAt: ds.UpdatedAt,
		},
		Name:        ds.Name,
		Type:        ds.Type,
		Enabled:     ds.Enabled,
		Config:      ds.Config,
		Description: ds.Description,
	}
}

// DatasourceTest 处理测试第三方数据源连接的请求。
// 不落库、不注册 vtable，仅按类型建立连接并校验连通性；失败原因放在 body 的 Message
// 字段返回（仍走 HTTP 200，不抛 5xx）。sqlite_file 无网络连接，恒返回失败。
func DatasourceTest(c *gin.Context) {
	var in DatasourceTestIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	out := DatasourceTestOut{Success: true}
	if err := vtab.TestConn(c, in.Type, []byte(in.Config)); err != nil {
		out = DatasourceTestOut{Success: false, Message: err.Error()}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// DatasourceIntrospect 处理自省第三方数据源表结构的请求。
// 不落库、不注册 vtable，仅建立连接后解析目标表结构并映射为列定义（列名 + 类型键），
// 供前端在创建数据源时展示。sqlite_file 类型走 multipart/form-data 分支，复用
// views.ReadMultipartFile 流式拿到上传的 .db 文件后一并传给 IntrospectSchema（其内部按
// 类型分发，sqlite_file 分支做 PRAGMA 自省）；其余类型仍为 JSON 体，file 传 nil。
// 失败原因放在 body 的 Message 字段返回（仍走 HTTP 200，不抛 5xx）。
func DatasourceIntrospect(c *gin.Context) {
	var in DatasourceIntrospectIn
	// sqlite_file 前端以 multipart/form-data 提交（含 .db 文件流），其余类型仍为 JSON 体。
	// 仅 sqlite 需要 file 字段；views.ReadMultipartFile 流式返回 file 部分，避免整文件缓冲。
	var file io.Reader
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		var err error
		if file, err = views.ReadMultipartFile(c, func(name string, body []byte) {
			switch name {
			case "type":
				in.Type = string(body)
			case "config":
				in.Config = string(body)
			}
		}); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		if closer, ok := file.(io.ReadCloser); ok {
			defer closer.Close()
		}
	} else if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}

	var out DatasourceIntrospectOut
	cols, err := vtab.IntrospectSchema(c, in.Type, []byte(in.Config), file)
	if err == nil {
		out = DatasourceIntrospectOut{Success: true, Columns: cols}
	} else {
		out = DatasourceIntrospectOut{Success: false, Message: err.Error()}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// init 注册第三方数据源列表出参（DatasourceListOut.type）的数据源类型枚举类绑定，
// 枚举类由 vtab 包经 enum.RegisterSource 注册（类名 datasourceType，见 internal/pkg/sqlitevtab/vtab/enum.go）。
func init() {
	views.AddEnumBinding("DatasourceListOut", "type", "datasourceType")
}
