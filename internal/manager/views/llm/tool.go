package llm

import (
	// 标准库
	"fmt"

	// 第三方

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// 项目内部

	"zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	"zhongkuisec.com/zhongkui/internal/manager/views"
	aimodel "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/utils"
)

// LlmToolListOut 定义工具注册表列表的输出项结构。
// 嵌入 BaseItemOut 携带审计字段、BaseTotalOut 以支持分页总数统计；字段名与列表字段投影白名单
// （search.toml selectFields）一一对应。McpServerName 由处理器按 mcp_server_id 批量补齐可读的
// 服务器名，非库内列。
type LlmToolListOut struct {
	utils.BaseItemOut
	// Name Agent 面向的工具名，全局唯一
	Name string `json:"name"`
	// Source 工具来源：builtin | mcp
	Source aimodel.ToolSource `json:"source"`
	// McpServerName 所属 MCP 服务器的本地标识；内置工具为空
	McpServerName string `json:"mcpServerName"`
	// RemoteName MCP 服务器侧的工具名；内置工具为空
	RemoteName string `json:"remoteName"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Description 展示用描述缓存；内置工具为空（其文案取自 i18n）
	Description string `json:"description"`
	utils.BaseTotalOut
}

// LlmToolBatchIn 批量操作工具的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type LlmToolBatchIn struct {
	// Ids 待批量操作的工具 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
}

// LlmToolSetEnabledIn 批量启用/停用工具的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）；Enabled 为目标状态，整批统一写入。
type LlmToolSetEnabledIn struct {
	// Ids 待批量操作的工具 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
	// Enabled 目标启用状态
	Enabled bool `json:"enabled"`
}

// LlmToolListGet 处理获取工具注册表列表的请求。
// 支持分页、排序与过滤（时间/名称/来源/启用状态）；工具注册表为全局配置，不按命名空间隔离。
// 查询按 gorm 模型扫描（工具行只存 mcp_server_id），再由处理器批量补齐所属服务器名后投影字段。
func LlmToolListGet(c *gin.Context) {
	var in views.LlmToolListGetSearchIn
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
		aimodel.LlmTool
		utils.BaseTotalOut
	}
	base := middlewares.GetCoreDatabase(c)
	db := base.Model(&aimodel.LlmTool{})
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 批量补齐所属 MCP 服务器名：工具行只持有 mcp_server_id，列表需展示可读的服务器标识
	serverIDs := make([]uint, 0, len(rows))
	seen := make(map[uint]struct{}, len(rows))
	for i := range rows {
		id := rows[i].McpServerID
		if id == nil {
			continue
		}
		if _, ok := seen[*id]; ok {
			continue
		}
		seen[*id] = struct{}{}
		serverIDs = append(serverIDs, *id)
	}
	serverNames, err := mcpServerNamesOf(base, serverIDs)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*LlmToolListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		row := &rows[i].LlmTool
		name := ""
		if row.McpServerID != nil {
			name = serverNames[*row.McpServerID]
		}
		items = append(items, &LlmToolListOut{
			BaseItemOut: utils.BaseItemOut{
				ID:        row.ID,
				CreatedAt: row.CreatedAt,
				UpdatedAt: row.UpdatedAt,
			},
			Name:          row.Name,
			Source:        row.Source,
			McpServerName: name,
			RemoteName:    row.RemoteName,
			Enabled:       row.Enabled,
			Description:   row.Description,
			BaseTotalOut:  utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **LlmToolListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// LlmToolSetEnabled 处理批量启用/停用工具的请求。
// 逐条读取后写入目标状态（已处于目标状态幂等短路），工具不存在视为成功跳过（幂等）。全部条目处理
// 完毕后，若有实际变更则重建运行时并广播变更。
func LlmToolSetEnabled(c *gin.Context) {
	var in LlmToolSetEnabledIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	changed := 0
	views.BatchExecute(c, ids,
		func(id uint64) string { return fmt.Sprintf("%d", id) },
		func(id uint64) error {
			instance := aimodel.LlmTool{}
			if err := db.First(&instance, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if instance.Enabled == in.Enabled {
				return nil // 幂等短路
			}
			if err := db.Model(&aimodel.LlmTool{}).Where("id = ?", instance.ID).
				Update("enabled", in.Enabled).Error; err != nil {
				return errors.WithStack(err)
			}
			changed++
			return nil
		},
	)
	if changed > 0 {
		afterConfigWrite(c, ai.ChangeKindTool, fmt.Sprintf("%d tools updated", changed))
	}
}

// LlmToolBatchDelete 批量删除工具。
// 内置工具（source='builtin'）由代码内注册表声明、生命周期由代码掌握，接口不得删除：只要本批
// 任一 ID 命中内置工具，就**整批拒绝**并返回明确的英文错误（不做部分删除）。校验通过后逐条软删除，
// 工具不存在视为成功跳过（幂等）；全部条目处理完毕后统一重建运行时并广播变更。
func LlmToolBatchDelete(c *gin.Context) {
	var in LlmToolBatchIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	ids := []uint64(in.Ids)
	if !views.ValidFlexLen(c, ids) {
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 整批预检：批内存在内置工具即整批拒绝，避免出现「部分删除」的不可预期结果
	var builtin int64
	if err := db.Model(&aimodel.LlmTool{}).
		Where("id IN ? AND source = ?", ids, string(aimodel.ToolSourceBuiltin)).Count(&builtin).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if builtin > 0 {
		response.CreateBadBodyError(errors.Newf(
			"the batch contains %d builtin tool(s), which cannot be deleted", builtin)).AbortWithGinH(c)
		return
	}
	deleted := 0
	views.BatchExecute(c, ids,
		func(id uint64) string { return fmt.Sprintf("%d", id) },
		func(id uint64) error {
			instance := aimodel.LlmTool{}
			if err := db.First(&instance, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			if err := db.Delete(&aimodel.LlmTool{}, instance.ID).Error; err != nil {
				return errors.WithStack(err)
			}
			deleted++
			return nil
		},
	)
	if deleted > 0 {
		afterConfigWrite(c, ai.ChangeKindTool, fmt.Sprintf("%d tools deleted", deleted))
	}
}

// mcpServerNamesOf 按工具行持有的 mcp_server_id 清单批量查回服务器名，返回 id → name 映射；
// 清单为空时不执行查询并返回空映射。
func mcpServerNamesOf(db *gorm.DB, ids []uint) (map[uint]string, error) {
	names := make(map[uint]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var servers []struct {
		ID   uint
		Name string
	}
	if err := db.Model(&aimodel.McpServer{}).Select("id", "name").Where("id IN ?", ids).
		Find(&servers).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	for _, server := range servers {
		names[server.ID] = server.Name
	}
	return names, nil
}
