package llm

import (
	// 标准库
	"encoding/json"
	"fmt"
	"strings"
	"time"

	// 第三方
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// 项目内部

	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	aimodel "zhongkui.com/zhongkui/internal/models/ai"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// McpServerCreateIn 定义注册 MCP 服务器端点的请求输入结构。
// 名称与传输方式、URL 必填；timeoutSeconds 默认 60 秒（经 default 标签声明并在反序列化前由
// defaults.Set 填充）；headers 为明文 JSON 对象文本（承载 Authorization 等凭据），落库为密文。
//
// 校验一律由绑定标签表达（无就地解析/校验方法）：trimmed 只施加于前端会 trim 的单行字段
// （name / transport / url），description 与 headers 不加——前者为空多行文案、后者为 JSON 文本，
// 首尾空白可能具有语义（与 AGENTS.md「文本域豁免 trim」一致）。
type McpServerCreateIn struct {
	// Name 服务器的本地标识，全局唯一，最长 64
	Name string `json:"name" binding:"required,trimmed,max=64"`
	// Transport 远端传输方式，取值空间取自枚举类 mcpTransport（streamable_http | sse）
	Transport string `json:"transport" binding:"required,trimmed,max=32,enum=mcpTransport"`
	// URL MCP 端点地址（http / https），最长 512
	URL string `json:"url" binding:"required,trimmed,max=512,httpUrl"`
	// Headers 请求头的明文 JSON 对象文本（如 {"Authorization":"Bearer xxx"}），留空表示无请求头
	Headers string `json:"headers" binding:"omitempty,jsonObject"`
	// TimeoutSeconds 单次请求超时秒数，须为正数，默认 60
	TimeoutSeconds int `json:"timeoutSeconds" default:"60" binding:"gt=0"`
	// Enabled 是否启用，默认启用；禁用的服务器及其全部工具不参与运行时装配
	Enabled bool `json:"enabled" default:"true"`
	// Description 服务器描述，最长 512
	Description string `json:"description" binding:"max=512"`
}

// McpServerUpdateIn 定义更新 MCP 服务器端点的请求输入结构。
// 传输方式允许修改；headers 留空表示沿用原凭据（避免编辑时强制回填密文）；其余字段按整表单提交。
// 校验标签与 McpServerCreateIn 同口径（见其注释）。
type McpServerUpdateIn struct {
	// Name 服务器的本地标识，全局唯一，最长 64
	Name string `json:"name" binding:"required,trimmed,max=64"`
	// Transport 远端传输方式，取值空间取自枚举类 mcpTransport（streamable_http | sse）
	Transport string `json:"transport" binding:"required,trimmed,max=32,enum=mcpTransport"`
	// URL MCP 端点地址（http / https），最长 512
	URL string `json:"url" binding:"required,trimmed,max=512,httpUrl"`
	// Headers 请求头的明文 JSON 对象文本；留空表示沿用原凭据
	Headers string `json:"headers" binding:"omitempty,jsonObject"`
	// TimeoutSeconds 单次请求超时秒数，须为正数
	TimeoutSeconds int `json:"timeoutSeconds" binding:"gt=0"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Description 服务器描述，最长 512
	Description string `json:"description" binding:"max=512"`
}

// McpServerDetailOut 定义 MCP 服务器端点的详情输出结构。
// 嵌入 BaseItemOut 携带审计字段；字段名与列表字段投影白名单（search.toml selectFields）一一对应；
// headers 以密文形式返回（crypto 序列化器，经 form 标签告知前端该字段已加密），不回显明文。
type McpServerDetailOut struct {
	// utils.BaseItemOut 内嵌明细通用字段（ID 与创建/更新时间）。
	utils.BaseItemOut
	// Name 服务器的本地标识，全局唯一
	Name string `json:"name"`
	// Transport 远端传输方式：streamable_http | sse
	Transport string `json:"transport"`
	// URL MCP 端点地址
	URL string `json:"url"`
	// Headers 请求头的密文；明文不回显，需经明文权限接口查看
	Headers *serializer.Crypto `json:"headers" form:"kind:crypto"`
	// TimeoutSeconds 单次请求超时秒数
	TimeoutSeconds int `json:"timeoutSeconds"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Description 服务器描述
	Description string `json:"description"`
}

// McpServerListOut 定义 MCP 服务器列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type McpServerListOut struct {
	// McpServerDetailOut 内嵌 MCP 服务器详情字段。
	McpServerDetailOut
	// utils.BaseTotalOut 内嵌分页总数统计字段。
	utils.BaseTotalOut
}

// McpServerBatchIn 批量操作 MCP 服务器的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type McpServerBatchIn struct {
	// Ids 待批量操作的服务器 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
}

// McpServerTestIn 连通性测试请求输入，支持两种用法：
//   - 仅传 id：读取该服务器记录（含解密凭据）后探测，其余字段被忽略；
//   - 不传 id：按本次提交的完整端点配置探测，不落库。
//
// 因「带 id 时其余字段被忽略」，此处不能写无条件的 required：改用 required_if=ID 0 表达
// 「仅在按配置探测时必填」，配合 omitempty 让空值在带 id 的用法下跳过其余校验。
type McpServerTestIn struct {
	// ID 已注册 MCP 服务器的 ID；大于 0 时忽略其余字段
	ID uint64 `json:"id"`
	// Transport 远端传输方式，取值空间取自枚举类 mcpTransport（streamable_http | sse）
	Transport string `json:"transport" binding:"required_if=ID 0,omitempty,trimmed,max=32,enum=mcpTransport"`
	// URL MCP 端点地址（http / https）
	URL string `json:"url" binding:"required_if=ID 0,omitempty,trimmed,max=512,httpUrl"`
	// Headers 请求头的明文 JSON 对象文本
	Headers string `json:"headers" binding:"omitempty,jsonObject"`
	// TimeoutSeconds 探测超时秒数，默认 60
	TimeoutSeconds int `json:"timeoutSeconds" default:"60" binding:"gt=0"`
}

// McpServerTestOut 连通性测试结果输出：Ok 表示端点可达，ToolCount / ToolNames 为远端
// tools/list 发现的工具数量与名称清单。失败时接口返回明确英文错误（不返回本结构）。
type McpServerTestOut struct {
	// Ok 端点是否可达（成功响应恒为 true）
	Ok bool `json:"ok"`
	// ToolCount 远端发现的工具数量
	ToolCount int `json:"toolCount"`
	// ToolNames 远端发现的工具名清单
	ToolNames []string `json:"toolNames"`
}

// McpServerSyncOut 工具清单同步结果输出：Created / Updated / Removed 分别为新增、刷新与
// 清理（远端已不提供）的工具行数。
type McpServerSyncOut struct {
	// Created 新增的工具行数（enabled=false）
	Created int `json:"created"`
	// Updated 已存在并刷新 description / remote_name 的工具行数
	Updated int `json:"updated"`
	// Removed 远端已不提供而被删除的工具行数
	Removed int `json:"removed"`
}

// McpServerListGet 处理获取 MCP 服务器注册表列表的请求。
// 支持分页、排序与过滤（时间/名称/传输方式/启用状态）；注册表为全局配置，不按命名空间隔离。
// 查询按 gorm 模型扫描（headers 经 crypto 序列化器读取），再映射为接口层 Out 结构并做字段投影；
// headers 出参为密文，不出现在任何投影项的明文位置。
func McpServerListGet(c *gin.Context) {
	var in views.McpServerListGetSearchIn
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
		aimodel.McpServer
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&aimodel.McpServer{})
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*McpServerListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		items = append(items, &McpServerListOut{
			McpServerDetailOut: newMcpServerDetailOut(&rows[i].McpServer),
			BaseTotalOut:       utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **McpServerListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// McpServerCreate 处理注册 MCP 服务器端点的请求。
// 字段合法性（名称非空、传输方式属于 mcpTransport 枚举、URL 为绝对 http/https、超时为正数、
// 请求头为合法 JSON 对象）一律由 McpServerCreateIn 的绑定标签保证；处理器只做名称唯一性预检
// （含软删除记录，避免触发唯一索引冲突）与落库（headers 以密文写入），成功后在提交后置动作中
// 重建运行时并广播变更。
func McpServerCreate(c *gin.Context) {
	var in McpServerCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 名称唯一性预检：含软删除记录（uq_mcp_servers_name 为非部分索引），
	// 以便返回明确的英文校验错误而非唯一约束冲突的 500
	var count int64
	if err := db.Unscoped().Model(&aimodel.McpServer{}).Where("name = ?", in.Name).Count(&count).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count > 0 {
		response.CreateBadBodyError(errors.Newf("mcp server name %q already exists", in.Name)).AbortWithGinH(c)
		return
	}
	// 请求头留空表示无请求头：以 nil 明文入库，而非空串的密文（保持列为空）
	var headers []byte
	if in.Headers != "" {
		headers = []byte(in.Headers)
	}
	instance := &aimodel.McpServer{
		Name:           in.Name,
		Transport:      in.Transport,
		URL:            in.URL,
		Headers:        serializer.NewCrypto(headers),
		TimeoutSeconds: in.TimeoutSeconds,
		Enabled:        in.Enabled,
		Description:    in.Description,
	}
	if err := db.Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindMcp, instance.Name)
	response.SimpleOK.AbortWithGinH(c)
}

// McpServerDetailGet 处理获取 MCP 服务器详情的请求。
// 服务器不存在时返回 404；headers 以密文形式返回。
func McpServerDetailGet(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := aimodel.McpServer{}
	if err := middlewares.GetCoreDatabase(c).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newMcpServerDetailOut(&instance))
}

// McpServerUpdate 处理更新 MCP 服务器端点的请求。
// 传输方式允许修改；headers 留空表示沿用原凭据（Select 不含 headers 列）；其余字段按整表单全量
// 校验后落库，成功后重建运行时并广播变更。
func McpServerUpdate(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in McpServerUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := aimodel.McpServer{}
	if err := db.First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 改名时做唯一性预检（排除自身，含软删除记录）
	var count int64
	if err := db.Unscoped().Model(&aimodel.McpServer{}).Where("name = ? AND id <> ?", in.Name, instance.ID).Count(&count).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count > 0 {
		response.CreateBadBodyError(errors.Newf("mcp server name %q already exists", in.Name)).AbortWithGinH(c)
		return
	}
	columns := []string{"name", "transport", "url", "timeout_seconds", "enabled", "description"}
	update := aimodel.McpServer{
		Name:           in.Name,
		Transport:      in.Transport,
		URL:            in.URL,
		TimeoutSeconds: in.TimeoutSeconds,
		Enabled:        in.Enabled,
		Description:    in.Description,
	}
	if in.Headers != "" {
		// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
		columns = append(columns, "headers")
		update.Headers = serializer.NewCrypto([]byte(in.Headers))
	}
	if err := db.Model(&aimodel.McpServer{}).Where("id = ?", instance.ID).Select(columns).Updates(&update).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindMcp, in.Name)
	response.SimpleOK.AbortWithGinH(c)
}

// McpServerBatchDelete 批量删除 MCP 服务器。
// 每台服务器在一个事务里被软删除，并**显式一并软删其工具行**：GORM 的软删除发出的是 UPDATE，
// 不会触发数据库层的 ON DELETE CASCADE（该外键只在物理 DELETE 时生效），故在此显式级联，
// 保证删除服务器后其同步出的工具行不再出现在工具列表中。服务器不存在视为成功跳过（幂等）。
// 全部条目处理完毕后统一重建运行时并广播变更。
func McpServerBatchDelete(c *gin.Context) {
	var in McpServerBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	deleted := 0
	views.BatchExecute(c, ids,
		func(id uint64) string { return fmt.Sprintf("%d", id) },
		func(id uint64) error {
			instance := aimodel.McpServer{}
			if err := db.First(&instance, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if err := db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Where("mcp_server_id = ?", instance.ID).Delete(&aimodel.LlmTool{}).Error; err != nil {
					return errors.WithStack(err)
				}
				return errors.WithStack(tx.Delete(&aimodel.McpServer{}, instance.ID).Error)
			}); err != nil {
				return err
			}
			deleted++
			return nil
		},
	)
	if deleted > 0 {
		afterConfigWrite(c, ai.ChangeKindMcp, fmt.Sprintf("%d servers deleted", deleted))
	}
}

// McpServerTest 处理 MCP 服务器连通性测试请求。
// 不落库、不产生任何副作用；支持「已注册服务器 id」（读取解密凭据）与「完整端点配置」两种用法。
// 探测经 ai.ProbeMCPServer 执行 MCP initialize 握手 + tools/list，返回远端发现的工具数量与名称
// 清单；失败（不可达、握手失败等）返回明确的英文错误。本接口耗时较长，服务端不设额外上限，
// 超时由 timeoutSeconds / 请求上下文约束。
func McpServerTest(c *gin.Context) {
	var in McpServerTestIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var (
		transport string
		endpoint  string
		headers   map[string]string
		timeout   time.Duration
	)
	if in.ID > 0 {
		// 已注册服务器：以库内记录（含解密凭据）为准，请求体其余字段被忽略
		instance := aimodel.McpServer{}
		if err := middlewares.GetCoreDatabase(c).First(&instance, in.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				response.ErrDefaultPageNotFound.AbortWithGinH(c)
				return
			}
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		transport, endpoint = instance.Transport, instance.URL
		timeout = time.Duration(instance.TimeoutSeconds) * time.Second
		decoded, err := decodeMcpHeaders(instance.HeadersData())
		if err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		headers = decoded
	} else {
		// 按本次提交的配置探测：字段合法性已由绑定标签（required_if/enum/httpUrl/jsonObject）保证
		decoded, err := decodeMcpHeaders([]byte(in.Headers))
		if err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		transport, endpoint, headers = in.Transport, in.URL, decoded
		timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}
	tools, err := ai.ProbeMCPServer(c, transport, endpoint, headers, timeout)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	response.SimpleOK.AbortWithGinW(c, McpServerTestOut{Ok: true, ToolCount: len(tools), ToolNames: names})
}

// McpServerSync 处理把远端工具清单同步到工具注册表的请求。
// 经 ai.ProbeMCPServer 取回远端 tools/list 后与 llm_tools 中该服务器的行（source='mcp'、
// mcp_server_id=:id）对账：新工具以 enabled=false 新增，已存在工具保留 enabled 并刷新
// description / remote_name，远端已不存在的工具被删除。全程在一个事务里完成，一致后重建运行时
// 并广播变更。
func McpServerSync(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := aimodel.McpServer{}
	if err := db.First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	headers, err := decodeMcpHeaders(instance.HeadersData())
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	tools, err := ai.ProbeMCPServer(c, instance.Transport, instance.URL, headers,
		time.Duration(instance.TimeoutSeconds)*time.Second)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	out, err := reconcileMCPTools(db, &instance, tools)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindTool, instance.Name)
	response.SimpleOK.AbortWithGinW(c, out)
}

// reconcileMCPTools 把远端工具清单与 llm_tools 中该服务器的行对账，返回新增/刷新/清理的数量。
// 全程在一个事务里完成；发现任何违反一致性约束（source='mcp' ⟺ mcp_server_id IS NOT NULL）的
// 行、远端工具名为空、或新增工具名与既有工具重名时返回英文错误并回滚。
func reconcileMCPTools(db *gorm.DB, server *aimodel.McpServer, remote []ai.MCPRemoteTool) (McpServerSyncOut, error) {
	out := McpServerSyncOut{}
	err := db.Transaction(func(tx *gorm.DB) error {
		// 该服务器的既有工具行：按 remote_name 索引
		var owned []aimodel.LlmTool
		if err := tx.Where("mcp_server_id = ?", server.ID).Find(&owned).Error; err != nil {
			return errors.WithStack(err)
		}
		byRemote := make(map[string]aimodel.LlmTool, len(owned))
		for i := range owned {
			row := owned[i]
			byRemote[row.RemoteName] = row
		}
		// 全量工具行的来源与归属：用于新增前的重名预检，并校验一致性约束
		// （source='mcp' 当且仅当 mcp_server_id IS NOT NULL）的两个方向
		var all []struct {
			ID          uint
			Name        string
			Source      aimodel.ToolSource
			McpServerID *uint
		}
		if err := tx.Model(&aimodel.LlmTool{}).
			Select("id", "name", "source", "mcp_server_id").Find(&all).Error; err != nil {
			return errors.WithStack(err)
		}
		nameOwner := make(map[string]uint, len(all))
		for i := range all {
			row := all[i]
			if (row.Source == aimodel.ToolSourceMCP) != (row.McpServerID != nil) {
				return errors.Newf(
					"tool %q (#%d) violates the consistency invariant: source %q if and only if mcp_server_id is not null",
					row.Name, row.ID, string(aimodel.ToolSourceMCP))
			}
			nameOwner[row.Name] = row.ID
		}
		seen := make(map[string]struct{}, len(remote))
		for _, remoteTool := range remote {
			remoteName := strings.TrimSpace(remoteTool.Name)
			if remoteName == "" {
				return errors.New("a tool returned by the remote has an empty name and cannot be synchronized")
			}
			if _, dup := seen[remoteName]; dup {
				continue // 远端重复项只取第一条
			}
			seen[remoteName] = struct{}{}
			if row, exists := byRemote[remoteName]; exists {
				// 已存在：保留 enabled 不变，刷新 description 与 remote_name
				refresh := aimodel.LlmTool{RemoteName: remoteName, Description: remoteTool.Description}
				if err := tx.Model(&aimodel.LlmTool{}).Where("id = ?", row.ID).
					Select("remote_name", "description").Updates(&refresh).Error; err != nil {
					return errors.WithStack(err)
				}
				out.Updated++
				continue
			}
			if owner, exists := nameOwner[remoteName]; exists {
				return errors.Newf("tool name %q is already taken by tool #%d and cannot be synchronized from MCP server %q",
					remoteName, owner, server.Name)
			}
			serverID := server.ID
			created := aimodel.LlmTool{
				Name:        remoteName,
				Source:      aimodel.ToolSourceMCP,
				McpServerID: &serverID,
				RemoteName:  remoteName,
				Description: remoteTool.Description,
				Enabled:     false,
			}
			if err := tx.Create(&created).Error; err != nil {
				return errors.WithStack(err)
			}
			nameOwner[remoteName] = created.ID
			out.Created++
		}
		// 远端已不存在的工具：删除其行
		for remoteName, row := range byRemote {
			if _, stillRemote := seen[remoteName]; stillRemote {
				continue
			}
			if err := tx.Delete(&aimodel.LlmTool{}, row.ID).Error; err != nil {
				return errors.WithStack(err)
			}
			out.Removed++
		}
		return nil
	})
	if err != nil {
		return McpServerSyncOut{}, err
	}
	return out, nil
}

// newMcpServerDetailOut 将 MCP 服务器注册表记录映射为详情输出结构（headers 保持密文指针不动）。
func newMcpServerDetailOut(m *aimodel.McpServer) McpServerDetailOut {
	return McpServerDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        m.ID,
			CreatedAt: m.CreatedAt,
			UpdatedAt: m.UpdatedAt,
		},
		Name:           m.Name,
		Transport:      m.Transport,
		URL:            m.URL,
		Headers:        m.Headers,
		TimeoutSeconds: m.TimeoutSeconds,
		Enabled:        m.Enabled,
		Description:    m.Description,
	}
}

// decodeMcpHeaders 把密文列解出的明文请求头解析为 map[string]string；为空返回 nil（表示无请求头），
// JSON 非法返回固定的英文错误——错误文本不携带原始明文，避免凭据经错误信息泄漏。
func decodeMcpHeaders(raw []byte) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var headers map[string]string
	if err := json.Unmarshal(raw, &headers); err != nil {
		return nil, errors.New("the request headers are not a valid JSON object")
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}
