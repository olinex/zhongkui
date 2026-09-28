package llm

import (
	// 标准库
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
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// LlmModelCreateIn 定义创建 LLM 模型端点的请求输入结构。
// 端点三要素（baseURL / apiKey / model）必填；kind 为 chat 或 embedding 且创建后不可更改；
// 数值型字段的默认值经 default 标签声明，并在反序列化前由 defaults.Set 填充。
type LlmModelCreateIn struct {
	// Name 模型本地标识，全局唯一，最长 64
	Name string `json:"name" binding:"max=64"`
	// Kind 模型类型：chat（对话）| embedding（向量化），创建后不可更改
	Kind pkgllm.ModelKind `json:"kind" binding:"max=16"`
	// BaseURL 模型端点的 OpenAI 兼容接口基地址，最长 512
	BaseURL string `json:"baseURL" binding:"max=512"`
	// APIKey 访问端点的凭据（明文提交，落库为密文）
	APIKey string `json:"apiKey"`
	// Model 供应商侧模型名（如 gpt-4o-mini、text-embedding-3-small），最长 128
	Model string `json:"model" binding:"max=128"`
	// TimeoutSeconds 单次请求超时秒数，默认 60
	TimeoutSeconds int `json:"timeoutSeconds" default:"60"`
	// Temperature 采样温度，取值 [0, 2]，默认 0.2（仅 chat 模型有意义）
	Temperature float64 `json:"temperature" default:"0.2"`
	// MaxTokens 单次生成的最大 token 数，默认 4096（仅 chat 模型有意义）
	MaxTokens int `json:"maxTokens" default:"4096"`
	// Enabled 是否启用，默认启用；禁用的模型不可绑定到档位
	Enabled bool `json:"enabled" default:"true"`
	// Description 模型描述，最长 512
	Description string `json:"description" binding:"max=512"`
}

// LlmModelUpdateIn 定义更新 LLM 模型端点的请求输入结构。
// 模型类型不可变（不接受 Kind），其余字段按整表单提交并全量校验；
// APIKey 留空表示保持原凭据不变，避免编辑时强制回填密文。
type LlmModelUpdateIn struct {
	// Name 模型本地标识，全局唯一，最长 64
	Name string `json:"name" binding:"max=64"`
	// BaseURL 模型端点的 OpenAI 兼容接口基地址，最长 512
	BaseURL string `json:"baseURL" binding:"max=512"`
	// APIKey 访问端点的凭据；留空表示沿用原凭据
	APIKey string `json:"apiKey"`
	// Model 供应商侧模型名，最长 128
	Model string `json:"model" binding:"max=128"`
	// TimeoutSeconds 单次请求超时秒数，须为正数
	TimeoutSeconds int `json:"timeoutSeconds"`
	// Temperature 采样温度，取值 [0, 2]（仅 chat 模型有意义）
	Temperature float64 `json:"temperature"`
	// MaxTokens 单次生成的最大 token 数（仅 chat 模型有意义）
	MaxTokens int `json:"maxTokens"`
	// Enabled 是否启用；禁用的模型不可绑定到档位
	Enabled bool `json:"enabled"`
	// Description 模型描述，最长 512
	Description string `json:"description" binding:"max=512"`
}

// LlmModelDetailOut 定义 LLM 模型端点的详情输出结构。
// 嵌入 BaseItemOut 携带审计字段；字段名与列表字段投影白名单（search.toml selectFields）
// 一一对应；apiKey 以密文形式返回（crypto 序列化器，经 form 标签告知前端该字段已加密）。
type LlmModelDetailOut struct {
	utils.BaseItemOut
	// Name 模型本地标识，全局唯一
	Name string `json:"name"`
	// Kind 模型类型：chat | embedding
	Kind pkgllm.ModelKind `json:"kind"`
	// BaseURL 模型端点的 OpenAI 兼容接口基地址
	BaseURL string `json:"baseURL"`
	// APIKey 访问凭据的密文；明文不回显，需经明文权限接口查看
	APIKey *serializer.Crypto `json:"apiKey" form:"kind:crypto"`
	// Model 供应商侧模型名
	Model string `json:"model"`
	// TimeoutSeconds 单次请求超时秒数
	TimeoutSeconds int `json:"timeoutSeconds"`
	// Temperature 采样温度
	Temperature float64 `json:"temperature"`
	// MaxTokens 单次生成的最大 token 数
	MaxTokens int `json:"maxTokens"`
	// Dimensions 已验证向量维度：后端在绑定 embedding 档位时经真实探测写入的只读缓存
	// （仅 embedding 模型有意义），前端只读、不可提交
	Dimensions int `json:"dimensions"`
	// Enabled 是否启用
	Enabled bool `json:"enabled"`
	// Description 模型描述
	Description string `json:"description"`
}

// LlmModelListOut 定义 LLM 模型列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计。
type LlmModelListOut struct {
	LlmModelDetailOut
	utils.BaseTotalOut
}

// LlmModelBatchIn 批量操作 LLM 模型的请求输入结构。
// Ids 接受标量（单个 ID）或数组（多个 ID）。
type LlmModelBatchIn struct {
	// Ids 待批量操作的模型 ID 列表
	Ids views.FlexList[uint64] `json:"ids"`
}

// LlmModelTestIn 连通性测试请求输入，支持两种用法：
//   - 仅传 id：读取该模型记录（含解密凭据）后探活，其余字段被忽略；
//   - 不传 id：按本次提交的完整端点配置探活，不落库。
type LlmModelTestIn struct {
	// ID 已注册模型的 ID；大于 0 时忽略其余字段
	ID uint64 `json:"id"`
	// Kind 模型类型：chat | embedding
	Kind pkgllm.ModelKind `json:"kind" binding:"max=16"`
	// BaseURL 模型端点的 OpenAI 兼容接口基地址
	BaseURL string `json:"baseURL" binding:"max=512"`
	// APIKey 访问端点的明文凭据
	APIKey string `json:"apiKey"`
	// Model 供应商侧模型名
	Model string `json:"model" binding:"max=128"`
	// TimeoutSeconds 探活超时秒数，默认 60
	TimeoutSeconds int `json:"timeoutSeconds" default:"60"`
	// Temperature 采样温度（仅 chat 模型有意义，探活不参与请求）
	Temperature float64 `json:"temperature" default:"0.2"`
	// MaxTokens 单次生成的最大 token 数（探活不参与请求）
	MaxTokens int `json:"maxTokens" default:"4096"`
}

// LlmModelTestOut 连通性测试结果输出：Success 表示端点可达，Message 为失败原因。
type LlmModelTestOut struct {
	// Success 端点是否可达
	Success bool `json:"success"`
	// Message 失败原因（Success=true 时为空）
	Message string `json:"message"`
}

// LlmModelListGet 处理获取 LLM 模型注册表列表的请求。
// 支持分页、排序与过滤（时间/名称/类型/启用状态）；模型注册表为全局配置，不按命名空间
// 隔离。查询按 gorm 模型扫描（api_key 经 crypto 序列化器读取），再映射为接口层 Out 结构；
// apiKey 在投影白名单内，凭据以密文（base64）出参，仅供列表单元格展示加密标识与直传
// /plaintext/crypto 解密，明文须经明文权限接口查看。
func LlmModelListGet(c *gin.Context) {
	var in views.LlmModelListGetSearchIn
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
		core.LlmModel
		utils.BaseTotalOut
	}
	db := middlewares.GetCoreDatabase(c).Model(&core.LlmModel{})
	if query, err := utils.BuildSearchCountQuery(db, &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*LlmModelListOut, 0, len(rows))
	var total uint
	for i := range rows {
		if i == 0 {
			total = rows[i].Total
		}
		items = append(items, &LlmModelListOut{
			LlmModelDetailOut: newLlmModelDetailOut(&rows[i].LlmModel),
			BaseTotalOut:      utils.BaseTotalOut{Total: total},
		})
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	projected := utils.ProjectSearchItems(items, &in, func(o **LlmModelListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
}

// LlmModelCreate 处理注册 LLM 模型端点的请求。
// 校验名称非空且未被占用（含软删除记录，避免触发唯一索引冲突）、类型合法、端点字段合法
// 后落库；成功后在提交后置动作中重建运行时并广播变更。
func LlmModelCreate(c *gin.Context) {
	var in LlmModelCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	kind, err := pkgllm.ParseModelKind(in.Kind)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	name, baseURL, modelName := strings.TrimSpace(in.Name), strings.TrimSpace(in.BaseURL), strings.TrimSpace(in.Model)
	if name == "" {
		response.CreateBadBodyError(errors.New("name must be non-empty")).AbortWithGinH(c)
		return
	}
	// 端点字段不变量由 pkgllm.ModelConfig.Validate 校验（Kind 与 Name 已在上方单独处理）
	endpoint := pkgllm.ModelConfig{
		Kind:        kind,
		BaseURL:     baseURL,
		APIKey:      in.APIKey,
		Model:       modelName,
		Timeout:     time.Duration(in.TimeoutSeconds) * time.Second,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
	}
	if err := endpoint.Validate(); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	// 名称唯一性预检：含软删除记录（uq_llm_models_name 为非部分索引），
	// 以便返回明确的英文校验错误而非唯一约束冲突的 500
	var count int64
	if err := db.Unscoped().Model(&core.LlmModel{}).Where("name = ?", name).Count(&count).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count > 0 {
		response.CreateBadBodyError(errors.Newf("model name %q already exists", name)).AbortWithGinH(c)
		return
	}
	instance := &core.LlmModel{
		Name:           name,
		Kind:           kind,
		BaseURL:        baseURL,
		APIKey:         serializer.NewCrypto([]byte(in.APIKey)),
		ModelName:      modelName,
		TimeoutSeconds: in.TimeoutSeconds,
		Temperature:    in.Temperature,
		MaxTokens:      in.MaxTokens,
		Enabled:        in.Enabled,
		Description:    in.Description,
	}
	if err := db.Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindModel, instance.Name)
	response.SimpleOK.AbortWithGinH(c)
}

// LlmModelDetailGet 处理获取 LLM 模型详情的请求。
// 模型不存在时返回 404；apiKey 以密文形式返回。
func LlmModelDetailGet(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	instance := core.LlmModel{}
	if err := middlewares.GetCoreDatabase(c).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinW(c, newLlmModelDetailOut(&instance))
}

// LlmModelUpdate 处理更新 LLM 模型端点的请求。
// 模型类型不可变（沿用库内记录的类型做类型相关校验）；apiKey 留空表示沿用原凭据
// （Select 不含 api_key 列）；其余字段按整表单全量校验后落库，成功后重建运行时并广播变更。
// 若该模型当前正绑定在 embedding 槽位，则写库前须用本次提交的新端点做一次真实维度探测，
// 校验其对 knowledge_chunks.embedding 列维度的兼容性（不兼容即 400 拒绝、不落库），并把
// 探测到的已验证维度写回模型记录；未绑定 embedding 槽位时不写 dimensions 列。
func LlmModelUpdate(c *gin.Context) {
	id, ok := views.GetUint64Param(c, "id")
	if !ok {
		return
	}
	var in LlmModelUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.LlmModel{}
	if err := db.First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	kind, err := pkgllm.ParseModelKind(instance.Kind)
	if err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	name, baseURL, modelName := strings.TrimSpace(in.Name), strings.TrimSpace(in.BaseURL), strings.TrimSpace(in.Model)
	if name == "" {
		response.CreateBadBodyError(errors.New("name must be non-empty")).AbortWithGinH(c)
		return
	}
	// 凭据留空即沿用原值：解密读回后参与完整性校验，但不出现在更新列中
	apiKey := in.APIKey
	if strings.TrimSpace(apiKey) == "" {
		apiKey = string(instance.APIKeyData())
	}
	// 本次提交的端点配置：既用于字段不变量校验，也用于 embedding 档位的维度兼容性探测
	endpoint := pkgllm.ModelConfig{
		Name:        name,
		Kind:        kind,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		Model:       modelName,
		Timeout:     time.Duration(in.TimeoutSeconds) * time.Second,
		Temperature: in.Temperature,
		MaxTokens:   in.MaxTokens,
	}
	if err := endpoint.Validate(); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 改名时做唯一性预检（排除自身，含软删除记录）
	var count int64
	if err := db.Unscoped().Model(&core.LlmModel{}).Where("name = ? AND id <> ?", name, instance.ID).Count(&count).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if count > 0 {
		response.CreateBadBodyError(errors.Newf("model name %q already exists", name)).AbortWithGinH(c)
		return
	}
	// 该模型是否正绑定在 embedding 槽位：是则写库前以新端点探测维度兼容性，
	// 并把已验证维度一并写入（dimensions 是后端管理的只读缓存，前端不提交）
	var embeddingBound int64
	if err := db.Model(&core.LlmBinding{}).
		Where("slot = ? AND model_id = ?", string(pkgllm.SlotEmbedding), instance.ID).
		Count(&embeddingBound).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var verifiedDimensions int
	if embeddingBound > 0 {
		// 以本次提交的端点配置探测维度兼容性（apiKey 留空时已沿用解密后的原凭据）
		if verifiedDimensions, err = verifyEmbeddingCompat(c, endpoint); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
	}
	// 凭据留空时更新列中不含 api_key，保持原密文不变；未绑定 embedding 槽位时不写 dimensions
	columns := []string{"name", "base_url", "model", "timeout_seconds", "temperature", "max_tokens", "enabled", "description"}
	update := core.LlmModel{
		Name:           name,
		BaseURL:        baseURL,
		ModelName:      modelName,
		TimeoutSeconds: in.TimeoutSeconds,
		Temperature:    in.Temperature,
		MaxTokens:      in.MaxTokens,
		Enabled:        in.Enabled,
		Description:    in.Description,
	}
	if embeddingBound > 0 {
		columns = append(columns, "dimensions")
		update.Dimensions = verifiedDimensions
	}
	if strings.TrimSpace(in.APIKey) != "" {
		// 采用结构体 Select 更新以触发 crypto 序列化器加密存储（map 更新不会走序列化器）
		columns = append(columns, "api_key")
		update.APIKey = serializer.NewCrypto([]byte(in.APIKey))
	}
	if err := db.Model(&core.LlmModel{}).Where("id = ?", instance.ID).Select(columns).Updates(&update).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindModel, name)
	response.SimpleOK.AbortWithGinH(c)
}

// LlmModelBatchDelete 批量删除 LLM 模型。
// 逐条先校验是否仍被档位绑定（被绑定时返回明确英文错误，避免触发外键 RESTRICT 的 500），
// 再软删除；模型不存在视为成功跳过（幂等）。全部条目处理完毕后统一重建运行时并广播变更。
func LlmModelBatchDelete(c *gin.Context) {
	var in LlmModelBatchIn
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
			instance := core.LlmModel{}
			if err := db.First(&instance, id).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil // 不存在视为成功跳过
				}
				return errors.WithStack(err)
			}
			var bound int64
			if err := db.Model(&core.LlmBinding{}).Where("model_id = ?", instance.ID).Count(&bound).Error; err != nil {
				return errors.WithStack(err)
			}
			if bound > 0 {
				return errors.Newf("model %q is bound to a slot; unbind it before deleting", instance.Name)
			}
			if err := db.Delete(&core.LlmModel{}, instance.ID).Error; err != nil {
				return errors.WithStack(err)
			}
			deleted++
			return nil
		},
	)
	if deleted > 0 {
		afterConfigWrite(c, ai.ChangeKindModel, fmt.Sprintf("%d models deleted", deleted))
	}
}

// LlmModelTest 处理模型端点连通性测试请求。
// 不落库、不产生任何副作用；支持「已存在模型 id」（读取解密凭据）与「完整端点配置」
// 两种用法。探活一律复用 ai.TestModel 的轻量探测（chat 走 GET {baseURL}/models，
// embedding 走一次最小向量化请求），失败原因放在 body 的 message 字段返回（仍走 HTTP 200）。
func LlmModelTest(c *gin.Context) {
	var in LlmModelTestIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var cfg pkgllm.ModelConfig
	if in.ID > 0 {
		// 已注册模型：以库内记录（含解密凭据）为准，请求体其余字段被忽略
		instance := core.LlmModel{}
		if err := middlewares.GetCoreDatabase(c).First(&instance, in.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				response.ErrDefaultPageNotFound.AbortWithGinH(c)
				return
			}
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		cfg = modelConfigOf(&instance)
	} else {
		kind, err := pkgllm.ParseModelKind(in.Kind)
		if err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		cfg = pkgllm.ModelConfig{
			Kind:        kind,
			BaseURL:     strings.TrimSpace(in.BaseURL),
			APIKey:      in.APIKey,
			Model:       strings.TrimSpace(in.Model),
			Timeout:     time.Duration(in.TimeoutSeconds) * time.Second,
			Temperature: in.Temperature,
			MaxTokens:   in.MaxTokens,
		}
		if err := cfg.Validate(); err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
	}
	out := LlmModelTestOut{Success: true}
	if err := middlewares.GetAiRuntime(c).TestModel(c.Request.Context(), cfg); err != nil {
		out = LlmModelTestOut{Success: false, Message: err.Error()}
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// newLlmModelDetailOut 将模型注册表记录映射为详情输出结构。
func newLlmModelDetailOut(m *core.LlmModel) LlmModelDetailOut {
	return LlmModelDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        m.ID,
			CreatedAt: m.CreatedAt,
			UpdatedAt: m.UpdatedAt,
		},
		Name:           m.Name,
		Kind:           m.Kind,
		BaseURL:        m.BaseURL,
		APIKey:         m.APIKey,
		Model:          m.ModelName,
		TimeoutSeconds: m.TimeoutSeconds,
		Temperature:    m.Temperature,
		MaxTokens:      m.MaxTokens,
		Dimensions:     m.Dimensions,
		Enabled:        m.Enabled,
		Description:    m.Description,
	}
}
