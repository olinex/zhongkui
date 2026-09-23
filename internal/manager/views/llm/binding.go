package llm

import (
	// third-party
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal

	"zhongkui.com/zhongkui/internal/ai"
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	pkgllm "zhongkui.com/zhongkui/internal/pkg/llm"
)

// LlmSlotBindingOut 定义单个档位的绑定摘要与可用性输出。
// 无论档位是否已绑定都保留一条记录，供运维区分「未绑定」「已绑定但模型被禁用」
// 「已绑定但模型类型与档位不符」三种情况。
type LlmSlotBindingOut struct {
	// Slot 档位标识：flash | standard | advanced | embedding
	Slot string `json:"slot"`
	// ModelID 绑定模型的 ID（未绑定时为 0）
	ModelID uint `json:"modelId"`
	// ModelName 绑定模型的本地标识（未绑定时为空）
	ModelName string `json:"modelName"`
	// Kind 绑定模型的类型：chat | embedding（未绑定时为空）
	Kind string `json:"kind"`
	// RequiredKind 该档位要求的模型类型：chat | embedding
	// RequiredKind is the model kind the slot requires: chat | embedding
	RequiredKind string `json:"requiredKind"`
	// ProviderModel 绑定模型的供应商侧模型名（未绑定时为空）
	ProviderModel string `json:"providerModel"`
	// Enabled 绑定模型是否启用
	Enabled bool `json:"enabled"`
	// Dimensions 绑定模型的向量维度（仅 embedding 档位有意义）
	Dimensions int `json:"dimensions"`
	// Bound 该档位是否存在绑定记录
	Bound bool `json:"bound"`
	// Available 该档位当前是否可用（存在绑定记录且模型已启用、类型与档位匹配）
	Available bool `json:"available"`
}

// LlmBindingListOut 定义档位绑定列表的输出结构。
// Slots 固定覆盖四个槽位；Enabled / DisabledReason 报告 AI 能力的整体可用性与不可用原因。
type LlmBindingListOut struct {
	// Slots 四个固定槽位的绑定摘要（按槽位清单顺序）
	Slots []LlmSlotBindingOut `json:"slots"`
	// Enabled AI 能力整体是否可用（standard 与 embedding 档位均可用）
	Enabled bool `json:"enabled"`
	// DisabledReason AI 能力不可用的原因（按请求语言本地化；可用时为空）
	DisabledReason string `json:"disabledReason"`
}

// LlmBindingSetIn 定义绑定档位的请求输入结构。
type LlmBindingSetIn struct {
	// ModelID 待绑定的模型 ID
	ModelID uint `json:"modelId"`
}

// LlmBindingListGet 处理获取档位绑定情况的请求。
// 槽位可用性以 AI 运行时快照为权威（含「模型被禁用」「类型不符」的判定），
// 绑定记录 ID 取自 llm_bindings；整体可用性与不可用原因取快照的判定结果
// （不可用原因按请求语言本地化）。
func LlmBindingListGet(c *gin.Context) {
	snap := middlewares.GetAiRuntime(c).Snapshot()
	infos := snap.Slots()
	db := middlewares.GetCoreDatabase(c)
	var bindings []core.LlmBinding
	if err := db.Find(&bindings).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	modelIDs := make(map[string]uint, len(bindings))
	for i := range bindings {
		modelIDs[bindings[i].Slot] = bindings[i].ModelID
	}
	slots := make([]LlmSlotBindingOut, 0, len(pkgllm.AllSlots))
	for _, slot := range pkgllm.AllSlots {
		out := LlmSlotBindingOut{Slot: string(slot), ModelID: modelIDs[string(slot)]}
		out.RequiredKind = string(pkgllm.SlotKind(slot))
		if info, ok := infos[slot]; ok {
			out.ModelName = info.Name
			out.ProviderModel = info.Model
			out.Kind = string(info.Kind)
			out.Enabled = info.Enabled
			out.Dimensions = info.Dimensions
			out.Bound = true
			out.Available = info.Bound
		}
		slots = append(slots, out)
	}
	response.SimpleOK.AbortWithGinW(c, LlmBindingListOut{
		Slots:          slots,
		Enabled:        snap.Enabled(),
		DisabledReason: snap.DisabledReason(middlewares.GetLanguage(c)),
	})
}

// LlmBindingSet 处理绑定档位的请求。
// 校验档位取值合法、模型存在、模型已启用且类型与档位匹配（embedding 槽位要求向量模型，
// 其余档位要求对话模型）后写入绑定记录（同槽位重复绑定时覆盖为最新模型）；
// embedding 槽位额外做一次真实维度兼容性校验（探测端点实际输出维度与
// knowledge_chunks.embedding 列维度是否兼容，不兼容即 400 拒绝、不写绑定），通过后把
// 已验证维度写回模型记录（后端管理的只读缓存）；成功后在提交后置动作中重建运行时并广播变更。
// LlmBindingSet handles a slot-binding request. After the slot value is validated as legal, the
// model is confirmed to exist, to be enabled and to have a kind matching the slot (the embedding
// slot requires an embedding model and every other slot requires a chat model), the binding
// record is written (rebinding the same slot overwrites it with the latest model). The embedding
// slot additionally performs a real dimension-compatibility check (it probes the actual output
// dimension of the endpoint against the knowledge_chunks.embedding column dimension; an
// incompatible pair is rejected with 400 and no binding is written); on success the verified
// dimension is written back to the model record (a read-only cache managed by the backend).
// Once done, the post-commit action rebuilds the runtime and broadcasts the change.
func LlmBindingSet(c *gin.Context) {
	raw, ok := views.GetRawParam(c, "slot")
	if !ok {
		return
	}
	slot, err := pkgllm.ParseSlot(raw)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var in LlmBindingSetIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if in.ModelID == 0 {
		response.CreateBadBodyError(errors.New("modelId must be a positive integer")).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.LlmModel{}
	if err := db.First(&instance, in.ModelID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if !instance.Enabled {
		response.CreateBadBodyError(errors.Newf("model %q is disabled and cannot be bound to a slot", instance.Name)).AbortWithGinH(c)
		return
	}
	if want := pkgllm.SlotKind(slot); pkgllm.ModelKind(instance.Kind) != want {
		response.CreateBadBodyError(errors.Newf(
			"model %q is a %s model and cannot be bound to the %q slot (which requires a %s model)",
			instance.Name, instance.Kind, slot, want)).AbortWithGinH(c)
		return
	}
	if slot == pkgllm.SlotEmbedding {
		verified, err := verifyEmbeddingCompat(c, modelConfigOf(&instance))
		if err != nil {
			response.CreateBadBodyError(err).AbortWithGinH(c)
			return
		}
		// 把已验证维度写回模型记录（后端管理的只读缓存）
		if err := db.Model(&core.LlmModel{}).Where("id = ?", instance.ID).Update("dimensions", verified).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}
	binding := core.LlmBinding{}
	if err := db.Where("slot = ?", string(slot)).First(&binding).Error; err == nil {
		if err := db.Model(&core.LlmBinding{}).Where("id = ?", binding.ID).Update("model_id", instance.ID).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) {
		if err := db.Create(&core.LlmBinding{Slot: string(slot), ModelID: instance.ID}).Error; err != nil {
			response.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	} else {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindBinding, string(slot))
	response.SimpleOK.AbortWithGinH(c)
}

// LlmBindingUnbind 处理解绑档位的请求。
// 删除该槽位的绑定记录（不存在时视为已解绑，幂等）；成功后在提交后置动作中重建运行时
// 并广播变更。
func LlmBindingUnbind(c *gin.Context) {
	raw, ok := views.GetRawParam(c, "slot")
	if !ok {
		return
	}
	slot, err := pkgllm.ParseSlot(raw)
	if err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if err := middlewares.GetCoreDatabase(c).Where("slot = ?", string(slot)).Delete(&core.LlmBinding{}).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	afterConfigWrite(c, ai.ChangeKindBinding, string(slot))
	response.SimpleOK.AbortWithGinH(c)
}
