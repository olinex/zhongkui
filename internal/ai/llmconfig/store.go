// Package llmconfig 从 core 库读取 LLM 接入配置（模型注册表 llm_models、档位绑定 llm_bindings、
// 行为参数 agent_params、工具注册表 llm_tools 与 MCP 服务器 mcp_servers），组装为一份不可变的
// 运行时就绪快照 Snapshot，并把已启用工具连同 MCP 会话句柄一并挂载其上。
//
// 本包只做「读库 + 校验 + 快照」：不持有运行时状态、不做热重载调度（由 internal/ai 的 Runtime
// 负责整体重建）、不执行工具调用。配置非法（参数缺行、取值不可解析、embedding 维度不一致）一律
// 返回英文错误并由装配层中止；MCP 端点故障只按服务器粒度降级为告警，不使装配失败。
package llmconfig

import (
	// 标准库
	"context"
	"maps"
	"strings"
	"time"

	// 第三方
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/config"
	"zhongkuisec.com/zhongkui/internal/ai/llminfra"
	"zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/translate"
)

// ErrSlotNotBound 档位没有可用模型绑定的错误哨兵：该档位无绑定记录，或绑定记录指向的
// 模型未启用/类型与档位不匹配。调用方经 errors.Is 判断后按「未配置」处理。
var ErrSlotNotBound = errors.New("ai: no usable model bound to the requested slot")

// SlotInfo 是单个档位绑定的展示信息（不含凭据），用于管理端展示与状态自检：无论绑定
// 是否可用都保留一条记录，供运维定位「已绑定但被禁用/类型不符」的情况。
type SlotInfo struct {
	// Slot 档位标识
	Slot ai.Slot
	// Name 绑定模型的本地标识（llm_models.name）
	Name string
	// Model 供应商侧的模型名（llm_models.model）
	Model string
	// Kind 模型类型（chat / embedding）
	Kind llm.ModelKind
	// APIMode 对话模型的 API 形态（chat / responses）
	APIMode llm.APIMode
	// Enabled 被绑定模型是否启用
	Enabled bool
	// Dimensions 被绑定模型的向量维度（仅 embedding 有意义）
	Dimensions int
	// Bound 该绑定是否可用（模型已启用且类型与档位匹配）
	Bound bool
}

// Snapshot 是一份不可变的 LLM 配置快照：由数据库在某一时刻的状态组装而成，构造后在
// 运行期内只读，因此可被任意 goroutine 并发读取而无需加锁。
type Snapshot struct {
	slots       map[ai.Slot]llm.ModelConfig // 可用档位 → 模型端点配置
	infos       map[ai.Slot]SlotInfo        // 全部档位绑定（含不可用）的展示信息
	params      *config.Params              // 行为参数读取器（值全部取自 agent_params.value）
	deviated    int                         // 当前值与出厂值不同的参数条数
	enabled     bool                        // AI 能力是否可用（standard 与 embedding 档位均可用）
	defaultSlot ai.Slot                     // 参数 agent.defaultSlot 的生效值（未装配时为空）
	tools       []tool.BaseTool             // 已启用工具（发布前一次性挂载，发布后只读）
	toolInfos   []*schema.ToolInfo          // 与 tools 一一对应的 schema 清单（顺序一致）
	closeTools  func() error                // MCP 会话关闭句柄（本次装配未拨号时为 nil）
}

// EmptySnapshot 返回一份未启用且无任何绑定的空快照，用于运行时尚未完成首次装配时的
// 安全读取（各读取方法均返回零值/错误，不会 panic）。
func EmptySnapshot() *Snapshot {
	return &Snapshot{
		slots:  map[ai.Slot]llm.ModelConfig{},
		infos:  map[ai.Slot]SlotInfo{},
		params: config.NewParams(nil),
	}
}

// ModelConfig 返回档位对应的模型端点配置；档位未绑定或绑定不可用时返回
// ErrSlotNotBound，调用方据此报告「指定档位不可用」而非静默降级。
func (s *Snapshot) ModelConfig(slot ai.Slot) (llm.ModelConfig, error) {
	if s == nil {
		return llm.ModelConfig{}, ErrSlotNotBound
	}
	if cfg, ok := s.slots[slot]; ok {
		return cfg, nil
	}
	return llm.ModelConfig{}, ErrSlotNotBound
}

// Params 返回行为参数读取器（值全部取自库内 agent_params.value；出厂值只由 migrate
// fixture 注入 default_value，代码内不持有任何出厂值）。
func (s *Snapshot) Params() *config.Params {
	if s == nil {
		return config.NewParams(nil)
	}
	return s.params
}

// ParamOverrideCount 返回当前值与出厂值（default_value）不同的参数条数：0 表示全部参数
// 仍是出厂值，供状态接口展示当前生效值偏离出厂值的规模。
func (s *Snapshot) ParamOverrideCount() int {
	if s == nil {
		return 0
	}
	return s.deviated
}

// DefaultSlot 返回参数 agent.defaultSlot 的生效值，供状态接口下发「未显式指定档位且会话未记录
// 档位时使用的默认档位」，使前端无需自持档位字面量；快照尚未完成首次装配时返回空串。
func (s *Snapshot) DefaultSlot() ai.Slot {
	if s == nil {
		return ""
	}
	return s.defaultSlot
}

// Enabled 报告 AI 能力是否可用：standard（默认对话档位）与 embedding 档位均有可用绑定
// 时为 true，任一缺失即整体关闭，避免以残缺能力运行。
func (s *Snapshot) Enabled() bool {
	return s != nil && s.enabled
}

// DisabledReason 返回 AI 能力整体不可用的原因（按请求语言本地化），供状态接口向运维说明
// 「为什么没启用」而不是只给一个 false：逐个检查必需的 standard 档位与 embedding 槽位，并把
// 原因以本地化分隔符连接；AI 可用时返回空串。快照缺失（尚未装配）时同样给出明确原因。
// 文案取自 translate 的 ai.disabled.* 消息，占位符为 [[.Slot]] / [[.Model]] / [[.Want]]。
func (s *Snapshot) DisabledReason(lang *base.Language) string {
	if s == nil {
		return translate.L(lang, "ai.disabled.not-assembled", nil)
	}
	if s.enabled {
		return ""
	}
	issues := make([]string, 0, 2)
	for _, slot := range []ai.Slot{ai.SlotStandard, ai.SlotEmbedding} {
		if reason := s.slotIssue(lang, slot); reason != "" {
			issues = append(issues, reason)
		}
	}
	return strings.Join(issues, translate.L(lang, "ai.disabled.separator", nil))
}

// slotIssue 返回单个必需档位不可用的原因（按请求语言本地化）：区分「无绑定记录」「绑定模型
// 被禁用」「绑定模型类型与档位不符」，档位可用时返回空串。
func (s *Snapshot) slotIssue(lang *base.Language, slot ai.Slot) string {
	if s.has(slot) {
		return ""
	}
	info, ok := s.infos[slot]
	if !ok {
		return translate.L(lang, "ai.disabled.slot-unbound", map[string]any{"Slot": slot})
	}
	if !info.Enabled {
		return translate.L(lang, "ai.disabled.model-disabled", map[string]any{"Slot": slot, "Model": info.Name})
	}
	return translate.L(lang, "ai.disabled.kind-mismatch", map[string]any{
		"Slot": slot, "Model": info.Name, "Want": ai.SlotKind(slot),
	})
}

// EmbeddingDimensions 返回 embedding 档位绑定模型的向量维度（未绑定或不可用时为 0）。
// 该值须与 knowledge_chunks.embedding 列的 pgvector 维度一致。
func (s *Snapshot) EmbeddingDimensions() int {
	if s == nil {
		return 0
	}
	return s.slots[ai.SlotEmbedding].Dimensions
}

// Slots 返回全部档位绑定的展示信息（含不可用绑定），按档位清单顺序。
func (s *Snapshot) Slots() map[ai.Slot]SlotInfo {
	out := make(map[ai.Slot]SlotInfo, len(s.infos))
	if s == nil {
		return out
	}
	maps.Copy(out, s.infos)
	return out
}

// Bindings 返回「档位 → 绑定模型本地标识」的映射，供管理端展示绑定关系。
func (s *Snapshot) Bindings() map[ai.Slot]string {
	out := make(map[ai.Slot]string, len(s.infos))
	if s == nil {
		return out
	}
	for slot, info := range s.infos {
		out[slot] = info.Name
	}
	return out
}

// SlotContextLimit 返回档位绑定模型的上下文窗口容量（llm_models.context_tokens，token 数）：
// agent 使用该档位模型时据此计算上下文占用比例并判定是否自动压缩。档位未绑定、绑定不可用或
// 非对话档位（embedding）时返回 0，表示该档位关闭占用展示与自动压缩。
func (s *Snapshot) SlotContextLimit(slot ai.Slot) int {
	if s == nil {
		return 0
	}
	return s.slots[slot].ContextTokens
}

// Tools 返回已启用工具的副本（调用方修改返回值不会影响快照内部状态）；未装配或 AI 未启用
// 时返回空切片（非 nil）。视图层与 Agent 由它取得可消费的 eino 工具清单。
func (s *Snapshot) Tools() []tool.BaseTool {
	if s == nil {
		return []tool.BaseTool{}
	}
	out := make([]tool.BaseTool, len(s.tools))
	copy(out, s.tools)
	return out
}

// ToolInfos 返回已启用工具的 schema 清单副本，顺序与 Tools 一一对应；未装配或 AI 未启用时
// 返回空切片（非 nil）。节点 4 以它作为 model.WithTools 的入参。
func (s *Snapshot) ToolInfos() []*schema.ToolInfo {
	if s == nil {
		return []*schema.ToolInfo{}
	}
	out := make([]*schema.ToolInfo, len(s.toolInfos))
	copy(out, s.toolInfos)
	return out
}

// CloseTools 关闭本快照持有的 MCP 会话（幂等）；nil 接收者或本次装配未拨号时返回 nil。快照在
// 发布后只读，会话句柄由快照持有：仅供 Runtime 在替换快照后（或关闭时）关闭旧快照的连接时
// 调用，调用方不得自行关闭，否则会与他人的读取竞争。
func (s *Snapshot) CloseTools() error {
	if s == nil || s.closeTools == nil {
		return nil
	}
	return s.closeTools()
}

// MountTools 把一次工具装配的产出挂到快照上：已构建工具、与之一一对应的 schema 清单以及 MCP
// 会话关闭句柄。仅供 Runtime.build 在快照发布前调用一次（快照发布后即只读，不得再写入）。
func (s *Snapshot) MountTools(set *ToolSet) {
	if s == nil || set == nil {
		return
	}
	s.tools = set.Tools
	s.toolInfos = set.Infos
	s.closeTools = set.Close
}

// Store 从 core 库读取 LLM 配置：模型注册表（llm_models）、档位绑定（llm_bindings，含
// 关联的模型实体）与行为参数（agent_params，出厂值与当前值）。模型绑定携带明文凭据，
// 故 Store 只应在进程内使用，不得序列化输出。
type Store struct {
	// db core 库连接（模型注册表、档位绑定与行为参数读取）。
	db     *database.DB
	dialer mcpDialer // MCP 连接器（生产实现委托 internal/ai/tools/mcp，测试可替换为假实现）
}

// NewStore 构造 Store；db 为空时返回英文错误。MCP 连接器默认取生产实现（本包不引用 MCP SDK）。
func NewStore(db *database.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("ai store: nil database")
	}
	return &Store{db: db, dialer: realMCPDialer{}}, nil
}

// Load 组装一份配置快照：读取全部档位绑定与行为参数，计算可用性，并在 embedding 档位
// 可用且声明了向量维度时强校验 knowledge_chunks.embedding 列的维度（不一致返回错误，
// 由启动路径中止启动并提示用户执行 migrate 子命令校准）。行为参数同样做启动期强校验：
// 注册表声明的每个键都必须有库内记录且取值可按类型解析，缺失或非法即返回英文错误
// （提示执行 migrate up 种子化出厂值）。首次装配时若模型注册表为空，返回未启用的空
// 快照而非错误。
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	bindings, err := s.loadBindings(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	snap := EmptySnapshot()
	for i := range bindings {
		binding := &bindings[i]
		slot, err := ai.ParseSlot(binding.Slot)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		cfg, usable, err := modelConfigOf(binding)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		snap.infos[slot] = SlotInfo{
			Slot:       slot,
			Name:       binding.Model.Name,
			Model:      binding.Model.ModelName,
			Kind:       llm.ModelKind(binding.Model.Kind),
			APIMode:    binding.Model.APIMode,
			Enabled:    binding.Model.Enabled,
			Dimensions: binding.Model.Dimensions,
			Bound:      usable,
		}
		if usable {
			snap.slots[slot] = cfg
		}
	}
	params, deviated, err := s.loadParams(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	snap.params = params
	snap.deviated = deviated
	// 默认档位随参数一并快照：装配前已校验全部注册表键均有可解析的值，此处读取不会 panic
	snap.defaultSlot = ai.Slot(params.String("agent.defaultSlot"))
	snap.enabled = snap.has(ai.SlotStandard) && snap.has(ai.SlotEmbedding)
	if dims := snap.EmbeddingDimensions(); dims > 0 {
		if err := llminfra.CheckEmbeddingDimension(ctx, s.db, dims); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return snap, nil
}

// ModelConfig 读取单个档位的模型端点配置；未绑定或绑定不可用时返回 ErrSlotNotBound。
// 供不持有运行时快照的场景（如依赖状态探测）按档位单独取值。
func (s *Store) ModelConfig(ctx context.Context, slot ai.Slot) (llm.ModelConfig, error) {
	binding, err := s.findBinding(ctx, slot)
	if err != nil {
		return llm.ModelConfig{}, errors.WithStack(err)
	}
	cfg, usable, err := modelConfigOf(binding)
	if err != nil {
		return llm.ModelConfig{}, errors.WithStack(err)
	}
	if !usable {
		return llm.ModelConfig{}, ErrSlotNotBound
	}
	return cfg, nil
}

// EmbeddingDimensions 返回 embedding 槽位绑定模型的本地标识、向量维度以及该槽位是否已绑定，
// 服务于「运维调整 knowledge_chunks.embedding 向量列维度前的绑定冲突检查」：已绑定时
// 调整维度会与绑定模型冲突，须先解绑（冲突提示需要报出模型名）。未绑定返回
// ("", 0, false, nil)；已绑定返回 (name, dimensions, true, nil)，其中 dimensions 为 0 表示
// 「已绑定但维度未知」（模型尚未经探测写入维度），与「未绑定」是两种不同状态，调用方须分开
// 处理；读库失败返回 ("", 0, false, err)。只要存在绑定记录即返回其维度（不要求模型已启用：
// 禁用模型不改变维度）。
func (s *Store) EmbeddingDimensions(ctx context.Context) (string, int, bool, error) {
	binding, err := s.findBinding(ctx, ai.SlotEmbedding)
	if err != nil {
		if errors.Is(err, ErrSlotNotBound) {
			return "", 0, false, nil
		}
		return "", 0, false, errors.WithStack(err)
	}
	return binding.Model.Name, binding.Model.Dimensions, true, nil
}

// loadBindings 读取全部档位绑定并预加载关联的模型实体。
func (s *Store) loadBindings(ctx context.Context) ([]ai.LlmBinding, error) {
	var bindings []ai.LlmBinding
	if err := s.db.WithContext(ctx).Preload("Model").Find(&bindings).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return bindings, nil
}

// findBinding 读取指定档位的绑定（含模型实体）；无绑定时返回 ErrSlotNotBound。
func (s *Store) findBinding(ctx context.Context, slot ai.Slot) (*ai.LlmBinding, error) {
	var binding ai.LlmBinding
	err := s.db.WithContext(ctx).Preload("Model").Where("slot = ?", string(slot)).First(&binding).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrSlotNotBound
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &binding, nil
}

// loadParams 读取 agent_params 并构造参数读取器，同时做启动期强校验：注册表声明的每个键
// 都必须有库内记录，出厂值（default_value）与当前值（value）都必须非空且可按该键声明的
// 类型解析；跨键关系（融合权重、工具结果预算与整篇展开预算）由 config.ValidateRelations
// 校验。任一缺失、非法或不满足关系即返回英文错误（提示执行 migrate up 种子化出厂值），
// 由启动路径中止启动。第二个返回值是当前值与出厂值不同的参数条数。
func (s *Store) loadParams(ctx context.Context) (*config.Params, int, error) {
	var rows []ai.AgentParam
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	byKey := make(map[string]ai.AgentParam, len(rows))
	for i := range rows {
		byKey[rows[i].Key] = rows[i]
	}
	values := make(map[string]string, len(config.Specs))
	missing := make([]string, 0, len(config.Specs))
	deviated := 0
	for _, spec := range config.Specs {
		row, ok := byKey[spec.Key]
		if !ok || row.Value == "" {
			missing = append(missing, spec.Key)
			continue
		}
		if row.DefaultValue == "" {
			return nil, 0, errors.Newf("ai: agent parameter %q has no factory value (default_value) in the database; run `migrate up` to seed the agent parameter defaults", spec.Key)
		}
		if err := config.ValidateValue(spec, row.DefaultValue); err != nil {
			return nil, 0, errors.WithStack(err)
		}
		if err := config.ValidateValue(spec, row.Value); err != nil {
			return nil, 0, errors.WithStack(err)
		}
		values[spec.Key] = row.Value
		if row.Value != row.DefaultValue {
			deviated++
		}
	}
	if len(missing) > 0 {
		return nil, 0, errors.Newf("ai: agent parameters are not seeded in the database: %s; run `migrate up` to seed the agent parameter defaults", strings.Join(missing, ", "))
	}
	if err := config.ValidateRelations(values); err != nil {
		return nil, 0, errors.WithStack(err)
	}
	return config.NewParams(values), deviated, nil
}

// has 报告档位是否已有可用绑定。
func (s *Snapshot) has(slot ai.Slot) bool {
	_, ok := s.slots[slot]
	return ok
}

// modelConfigOf 把一条档位绑定映射为模型端点配置，并报告该绑定是否可用：模型须已启用
// 且类型与档位匹配，否则视为不可用（返回 ok=false），避免以错误类型的客户端调用端点。
// 高级请求配置（extra_body）与思考开关的开/关取值须为合法 JSON，解析失败即返回英文错误
// （不静默降级），由调用方中止装配。
func modelConfigOf(binding *ai.LlmBinding) (llm.ModelConfig, bool, error) {
	slot := ai.Slot(binding.Slot)
	model := &binding.Model
	if !model.Enabled || llm.ModelKind(model.Kind) != ai.SlotKind(slot) {
		return llm.ModelConfig{}, false, nil
	}
	extraBody, err := llm.ParseExtraBody(model.ExtraBody)
	if err != nil {
		return llm.ModelConfig{}, false, errors.WithStack(err)
	}
	thinkingOnValue, err := llm.ParseJSONValue(model.ThinkingOnValue)
	if err != nil {
		return llm.ModelConfig{}, false, errors.WithStack(err)
	}
	thinkingOffValue, err := llm.ParseJSONValue(model.ThinkingOffValue)
	if err != nil {
		return llm.ModelConfig{}, false, errors.WithStack(err)
	}
	return llm.ModelConfig{
		Name:             model.Name,
		Kind:             llm.ModelKind(model.Kind),
		APIMode:          model.APIMode,
		BaseURL:          model.BaseURL,
		APIKey:           string(model.APIKeyData()),
		Model:            model.ModelName,
		Timeout:          time.Duration(model.TimeoutSeconds) * time.Second,
		Temperature:      model.Temperature,
		MaxTokens:        model.MaxTokens,
		ContextTokens:    model.ContextTokens,
		Dimensions:       model.Dimensions,
		ExtraBody:        extraBody,
		ThinkingField:    model.ThinkingField,
		ThinkingOnValue:  thinkingOnValue,
		ThinkingOffValue: thinkingOffValue,
	}, true, nil
}
