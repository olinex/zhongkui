package ai

import (
	// stdlib
	"context"
	"strings"
	"time"

	// third-party
	"gorm.io/gorm"

	// internal
	coremodel "zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/database"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/llm"
	"zhongkui.com/zhongkui/internal/pkg/translate"
)

// ErrSlotNotBound 档位没有可用模型绑定的错误哨兵：该档位无绑定记录，或绑定记录指向的
// 模型未启用/类型与档位不匹配。调用方经 errors.Is 判断后按「未配置」处理。
// ErrSlotNotBound is the error sentinel of a slot without a usable model binding: either the
// slot has no binding record, or the bound model is disabled or has a kind that does not
// match the slot. Callers check it via errors.Is and treat it as "not configured".
var ErrSlotNotBound = errors.New("ai: no usable model bound to the requested slot")

// SlotInfo 是单个档位绑定的展示信息（不含凭据），用于管理端展示与状态自检：无论绑定
// 是否可用都保留一条记录，供运维定位「已绑定但被禁用/类型不符」的情况。
// SlotInfo is the display information of one slot binding (credentials excluded), used by
// the admin view and status self-checks: a record is kept whether or not the binding is
// usable, so operators can spot "bound but disabled / kind mismatch" cases.
type SlotInfo struct {
	// Slot 档位标识
	// Slot the slot identifier
	Slot llm.Slot
	// Name 绑定模型的本地标识（llm_models.name）
	// Name the local identity of the bound model (llm_models.name)
	Name string
	// Model 供应商侧的模型名（llm_models.model）
	// Model the vendor-side model name (llm_models.model)
	Model string
	// Kind 模型类型（chat / embedding）
	// Kind the model kind (chat / embedding)
	Kind llm.ModelKind
	// Enabled 被绑定模型是否启用
	// Enabled whether the bound model is enabled
	Enabled bool
	// Dimensions 被绑定模型的向量维度（仅 embedding 有意义）
	// Dimensions the vector dimensions of the bound model (meaningful for embedding only)
	Dimensions int
	// Bound 该绑定是否可用（模型已启用且类型与档位匹配）
	// Bound whether the binding is usable (the model is enabled and its kind matches the slot)
	Bound bool
}

// Snapshot 是一份不可变的 LLM 配置快照：由数据库在某一时刻的状态组装而成，构造后在
// 运行期内只读，因此可被任意 goroutine 并发读取而无需加锁。
// Snapshot is one immutable LLM config snapshot: it is assembled from the database state at a
// point in time and read-only for the rest of its life, so any goroutine may read it
// concurrently without locking.
type Snapshot struct {
	slots       map[llm.Slot]llm.ModelConfig // 可用档位 → 模型端点配置
	infos       map[llm.Slot]SlotInfo        // 全部档位绑定（含不可用）的展示信息
	params      *llm.Params                  // 行为参数读取器（值全部取自 agent_params.value）
	deviated    int                          // 当前值与出厂值不同的参数条数
	enabled     bool                         // AI 能力是否可用（standard 与 embedding 档位均可用）
	defaultSlot llm.Slot                     // 参数 agent.defaultSlot 的生效值（未装配时为空）
}

// emptySnapshot 返回一份未启用且无任何绑定的空快照，用于运行时尚未完成首次装配时的
// 安全读取（各读取方法均返回零值/错误，不会 panic）。
// emptySnapshot returns an empty, disabled snapshot with no bindings, used for safe reads
// before the runtime completes its first assembly (every read returns a zero value or an
// error and never panics).
func emptySnapshot() *Snapshot {
	return &Snapshot{
		slots:  map[llm.Slot]llm.ModelConfig{},
		infos:  map[llm.Slot]SlotInfo{},
		params: llm.NewParams(nil),
	}
}

// ModelConfig 返回档位对应的模型端点配置；档位未绑定或绑定不可用时返回
// ErrSlotNotBound，调用方据此报告「指定档位不可用」而非静默降级。
// ModelConfig returns the model endpoint config of the slot; an unbound or unusable slot
// yields ErrSlotNotBound, so callers report "the requested slot is unavailable" instead of
// degrading silently.
func (s *Snapshot) ModelConfig(slot llm.Slot) (llm.ModelConfig, error) {
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
// Params returns the behavior-parameter reader (every value comes from agent_params.value in
// the database; factory values are injected into default_value by the migrate fixtures and no
// factory value is held in code).
func (s *Snapshot) Params() *llm.Params {
	if s == nil {
		return llm.NewParams(nil)
	}
	return s.params
}

// ParamOverrideCount 返回当前值与出厂值（default_value）不同的参数条数：0 表示全部参数
// 仍是出厂值，供状态接口展示当前生效值偏离出厂值的规模。
// ParamOverrideCount returns the number of parameters whose effective value differs from the
// factory value (default_value): 0 means every parameter still carries its factory value. It
// lets the status API show how far the effective values deviate from the factory values.
func (s *Snapshot) ParamOverrideCount() int {
	if s == nil {
		return 0
	}
	return s.deviated
}

// DefaultSlot 返回参数 agent.defaultSlot 的生效值，供状态接口下发「未显式指定档位且会话未记录
// 档位时使用的默认档位」，使前端无需自持档位字面量；快照尚未完成首次装配时返回空串。
// DefaultSlot returns the effective value of the agent.defaultSlot parameter, letting the status
// API publish the slot used when no slot is given explicitly and the session records none, so the
// frontend needs no slot literal of its own; it returns an empty string before the runtime
// completes its first assembly.
func (s *Snapshot) DefaultSlot() llm.Slot {
	if s == nil {
		return ""
	}
	return s.defaultSlot
}

// Enabled 报告 AI 能力是否可用：standard（默认对话档位）与 embedding 档位均有可用绑定
// 时为 true，任一缺失即整体关闭，避免以残缺能力运行。
// Enabled reports whether the AI capability is available: true only when both the standard
// (default chat) slot and the embedding slot have usable bindings; a missing one turns the
// whole capability off, so it never runs crippled.
func (s *Snapshot) Enabled() bool {
	return s != nil && s.enabled
}

// DisabledReason 返回 AI 能力整体不可用的原因（按请求语言本地化），供状态接口向运维说明
// 「为什么没启用」而不是只给一个 false：逐个检查必需的 standard 档位与 embedding 槽位，并把
// 原因以本地化分隔符连接；AI 可用时返回空串。快照缺失（尚未装配）时同样给出明确原因。
// 文案取自 translate 的 ai.disabled.* 消息，占位符为 [[.Slot]] / [[.Model]] / [[.Want]]。
// DisabledReason returns the reason why the AI capability is unavailable, localized to the
// request language, letting the status API tell operators why rather than only reporting false:
// it inspects the required standard slot and embedding slot in turn and joins the reasons with a
// localized separator, returning an empty string when the capability is available. A missing
// snapshot (not yet assembled) also yields an explicit reason. The copy comes from the translate
// messages ai.disabled.* with the placeholders [[.Slot]] / [[.Model]] / [[.Want]].
func (s *Snapshot) DisabledReason(lang *base.Language) string {
	if s == nil {
		return translate.L(lang, "ai.disabled.not-assembled", nil)
	}
	if s.enabled {
		return ""
	}
	issues := make([]string, 0, 2)
	for _, slot := range []llm.Slot{llm.SlotStandard, llm.SlotEmbedding} {
		if reason := s.slotIssue(lang, slot); reason != "" {
			issues = append(issues, reason)
		}
	}
	return strings.Join(issues, translate.L(lang, "ai.disabled.separator", nil))
}

// slotIssue 返回单个必需档位不可用的原因（按请求语言本地化）：区分「无绑定记录」「绑定模型
// 被禁用」「绑定模型类型与档位不符」，档位可用时返回空串。
// slotIssue returns the localized reason why one required slot is unavailable, distinguishing
// "no binding record", "the bound model is disabled" and "the bound model kind does not match
// the slot"; it returns an empty string when the slot is usable.
func (s *Snapshot) slotIssue(lang *base.Language, slot llm.Slot) string {
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
		"Slot": slot, "Model": info.Name, "Want": llm.SlotKind(slot),
	})
}

// EmbeddingDimensions 返回 embedding 档位绑定模型的向量维度（未绑定或不可用时为 0）。
// 该值须与 knowledge_chunks.embedding 列的 pgvector 维度一致。
// EmbeddingDimensions returns the vector dimensions of the model bound to the embedding
// slot (0 when unbound or unusable). It must match the pgvector dimension of the
// knowledge_chunks.embedding column.
func (s *Snapshot) EmbeddingDimensions() int {
	if s == nil {
		return 0
	}
	return s.slots[llm.SlotEmbedding].Dimensions
}

// Slots 返回全部档位绑定的展示信息（含不可用绑定），按档位清单顺序。
// Slots returns the display information of every slot binding (including unusable ones) in
// the order of the slot list.
func (s *Snapshot) Slots() map[llm.Slot]SlotInfo {
	out := make(map[llm.Slot]SlotInfo, len(s.infos))
	if s == nil {
		return out
	}
	for slot, info := range s.infos {
		out[slot] = info
	}
	return out
}

// Bindings 返回「档位 → 绑定模型本地标识」的映射，供管理端展示绑定关系。
// Bindings returns the "slot → bound model local identity" mapping for the admin view to
// display the binding relations.
func (s *Snapshot) Bindings() map[llm.Slot]string {
	out := make(map[llm.Slot]string, len(s.infos))
	if s == nil {
		return out
	}
	for slot, info := range s.infos {
		out[slot] = info.Name
	}
	return out
}

// Store 从 core 库读取 LLM 配置：模型注册表（llm_models）、档位绑定（llm_bindings，含
// 关联的模型实体）与行为参数（agent_params，出厂值与当前值）。模型绑定携带明文凭据，
// 故 Store 只应在进程内使用，不得序列化输出。
// Store reads the LLM config from the core database: the model registry (llm_models), the
// slot bindings (llm_bindings, with the associated model entity) and the behavior parameters
// (agent_params, factory and effective values). The model bindings carry plaintext
// credentials, so a Store must stay inside the process and never be serialized outward.
type Store struct {
	db *database.DB
}

// NewStore 构造 Store；db 为空时返回英文错误。
// NewStore constructs a Store; a nil db yields an English error.
func NewStore(db *database.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("ai store: nil database")
	}
	return &Store{db: db}, nil
}

// Load 组装一份配置快照：读取全部档位绑定与行为参数，计算可用性，并在 embedding 档位
// 可用且声明了向量维度时强校验 knowledge_chunks.embedding 列的维度（不一致返回错误，
// 由启动路径中止启动并提示用户执行 migrate 子命令校准）。行为参数同样做启动期强校验：
// 注册表声明的每个键都必须有库内记录且取值可按类型解析，缺失或非法即返回英文错误
// （提示执行 migrate up 种子化出厂值）。首次装配时若模型注册表为空，返回未启用的空
// 快照而非错误。
// Load assembles a config snapshot: it reads every slot binding and the behavior parameters,
// derives the availability, and, when the embedding slot is usable and declares vector
// dimensions, strictly verifies the dimension of the knowledge_chunks.embedding column (a
// mismatch returns an error so the startup path aborts and tells the user to reconcile the
// column through the migrate subcommand). The behavior parameters are strictly checked at
// startup as well: every key declared by the registry must have a database record whose value
// parses as its kind, otherwise an English error is returned (telling the user to run
// `migrate up` to seed the factory values). When the model registry is empty on a fresh
// install it returns a disabled snapshot instead of an error.
func (s *Store) Load(ctx context.Context) (*Snapshot, error) {
	bindings, err := s.loadBindings(ctx)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	snap := emptySnapshot()
	for i := range bindings {
		binding := &bindings[i]
		slot, err := llm.ParseSlot(binding.Slot)
		if err != nil {
			continue // 未知档位（历史遗留脏数据）忽略，不阻断服务
		}
		cfg, usable := modelConfigOf(binding)
		snap.infos[slot] = SlotInfo{
			Slot:       slot,
			Name:       binding.Model.Name,
			Model:      binding.Model.ModelName,
			Kind:       llm.ModelKind(binding.Model.Kind),
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
	snap.defaultSlot = llm.Slot(params.String("agent.defaultSlot"))
	snap.enabled = snap.has(llm.SlotStandard) && snap.has(llm.SlotEmbedding)
	if dims := snap.EmbeddingDimensions(); dims > 0 {
		if err := llm.CheckEmbeddingDimension(ctx, s.db, dims); err != nil {
			return nil, errors.WithStack(err)
		}
	}
	return snap, nil
}

// ModelConfig 读取单个档位的模型端点配置；未绑定或绑定不可用时返回 ErrSlotNotBound。
// 供不持有运行时快照的场景（如依赖状态探测）按档位单独取值。
// ModelConfig reads the model endpoint config of one slot; an unbound or unusable slot yields
// ErrSlotNotBound. It serves scenarios without a runtime snapshot (such as the dependency
// status probes) that resolve a single slot on demand.
func (s *Store) ModelConfig(ctx context.Context, slot llm.Slot) (llm.ModelConfig, error) {
	binding, err := s.findBinding(ctx, slot)
	if err != nil {
		return llm.ModelConfig{}, err
	}
	cfg, usable := modelConfigOf(binding)
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
// EmbeddingDimensions returns the local identity of the model bound to the embedding slot, its
// vector dimensions and whether that slot is bound, serving the binding-conflict check performed
// before an operator reconciles the dimension of the knowledge_chunks.embedding vector column:
// while a binding exists, reconciling the dimension conflicts with the bound model and requires
// an unbind first (the conflict message has to report the model name). An unbound slot yields
// ("", 0, false, nil); a bound slot yields (name, dimensions, true, nil), where a dimensions of 0
// means "bound but the dimension is still unknown" (the model has not been probed yet), a state
// distinct from "unbound" that callers must handle separately; a failed database read yields
// ("", 0, false, err). The dimensions are returned whenever a binding exists (an enabled model is
// not required: disabling a model never changes the dimension).
func (s *Store) EmbeddingDimensions(ctx context.Context) (string, int, bool, error) {
	binding, err := s.findBinding(ctx, llm.SlotEmbedding)
	if err != nil {
		if errors.Is(err, ErrSlotNotBound) {
			return "", 0, false, nil
		}
		return "", 0, false, err
	}
	return binding.Model.Name, binding.Model.Dimensions, true, nil
}

// loadBindings 读取全部档位绑定并预加载关联的模型实体。
// loadBindings reads every slot binding and preloads the associated model entity.
func (s *Store) loadBindings(ctx context.Context) ([]coremodel.LlmBinding, error) {
	var bindings []coremodel.LlmBinding
	if err := s.db.WithContext(ctx).Preload("Model").Find(&bindings).Error; err != nil {
		return nil, errors.WithStack(err)
	}
	return bindings, nil
}

// findBinding 读取指定档位的绑定（含模型实体）；无绑定时返回 ErrSlotNotBound。
// findBinding reads the binding of the given slot (with its model entity); an unbound slot
// yields ErrSlotNotBound.
func (s *Store) findBinding(ctx context.Context, slot llm.Slot) (*coremodel.LlmBinding, error) {
	var binding coremodel.LlmBinding
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
// 类型解析；任一缺失或非法即返回英文错误（提示执行 migrate up 种子化出厂值），由启动路径
// 中止启动。第二个返回值是当前值与出厂值不同的参数条数。
// loadParams reads agent_params and constructs the parameter reader while performing the
// startup check: every key declared by the registry must have a database record, and both the
// factory value (default_value) and the effective value (value) must be non-empty and parse as
// the kind declared for that key; any missing or invalid one returns an English error (telling
// the user to run `migrate up` to seed the factory values) and aborts the startup path. The
// second result is the number of parameters whose effective value differs from the factory
// value.
func (s *Store) loadParams(ctx context.Context) (*llm.Params, int, error) {
	var rows []coremodel.AgentParam
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, 0, errors.WithStack(err)
	}
	byKey := make(map[string]coremodel.AgentParam, len(rows))
	for i := range rows {
		byKey[rows[i].Key] = rows[i]
	}
	values := make(map[string]string, len(llm.Specs))
	missing := make([]string, 0, len(llm.Specs))
	deviated := 0
	for _, spec := range llm.Specs {
		row, ok := byKey[spec.Key]
		if !ok || row.Value == "" {
			missing = append(missing, spec.Key)
			continue
		}
		if row.DefaultValue == "" {
			return nil, 0, errors.Newf("ai: agent parameter %q has no factory value (default_value) in the database; run `migrate up` to seed the agent parameter defaults", spec.Key)
		}
		if err := llm.ValidateValue(spec, row.DefaultValue); err != nil {
			return nil, 0, errors.WithStack(err)
		}
		if err := llm.ValidateValue(spec, row.Value); err != nil {
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
	return llm.NewParams(values), deviated, nil
}

// has 报告档位是否已有可用绑定。
// has reports whether the slot already has a usable binding.
func (s *Snapshot) has(slot llm.Slot) bool {
	_, ok := s.slots[slot]
	return ok
}

// modelConfigOf 把一条档位绑定映射为模型端点配置，并报告该绑定是否可用：模型须已启用
// 且类型与档位匹配，否则视为不可用（返回 ok=false），避免以错误类型的客户端调用端点。
// modelConfigOf maps a slot binding into a model endpoint config and reports whether the
// binding is usable: the model must be enabled and its kind must match the slot, otherwise it
// counts as unusable (ok=false), preventing a client of the wrong kind from calling the
// endpoint.
func modelConfigOf(binding *coremodel.LlmBinding) (llm.ModelConfig, bool) {
	slot := llm.Slot(binding.Slot)
	model := &binding.Model
	if !model.Enabled || llm.ModelKind(model.Kind) != llm.SlotKind(slot) {
		return llm.ModelConfig{}, false
	}
	return llm.ModelConfig{
		Name:        model.Name,
		Kind:        llm.ModelKind(model.Kind),
		BaseURL:     model.BaseURL,
		APIKey:      string(model.APIKeyData()),
		Model:       model.ModelName,
		Timeout:     time.Duration(model.TimeoutSeconds) * time.Second,
		Temperature: model.Temperature,
		MaxTokens:   model.MaxTokens,
		Dimensions:  model.Dimensions,
	}, true
}
