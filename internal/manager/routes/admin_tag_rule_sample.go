// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含标签规则验证样例管理相关的 API 路由处理器，
// 提供样例的列表查询、详情获取、创建、更新与删除能力。
//
// 一致性设计：
//   - 样例为全局数据（不归属命名空间），所有 handler 不读取请求命名空间；
//   - 样例的 Model 与 Input 创建后不可变，更新仅允许调整 Name 与 Description
//     （DTO 不暴露 Model/Input，落库仅写 name/description 两列，双重保证）；
//   - 创建时校验目标模型已注册（tagengine.IsRegistered），未注册返回 400 + 1107。
package routes

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"zhongkui.com/zhongkui/internal/manager"
	"zhongkui.com/zhongkui/internal/models/tag"
	"zhongkui.com/zhongkui/internal/pkg/s3"
	"zhongkui.com/zhongkui/internal/pkg/serializer"
	"zhongkui.com/zhongkui/internal/pkg/utils"
	"zhongkui.com/zhongkui/internal/tagengine"
)

// TagRuleSampleCreateIn 定义创建标签规则样例的请求输入结构。
// 样例为全局数据（不归属命名空间）；model 必须是已注册的目标模型类型键
// （如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）。
type TagRuleSampleCreateIn struct {
	// Model 目标模型类型键
	Model string `json:"model" binding:"required,max=255"`
	// Name 样例名称，同一模型内唯一，仅允许字母、数字、下划线、连字符，长度 1~128
	Name string `json:"name" binding:"required,min=1,max=128,alphanumspecial=_-"`
	// Description 样例说明，最长 512 字符
	Description string `json:"description" binding:"max=512"`
	// Input 样例输入文档（目标模型实例的 JSON 序列化，反序列化为 map）；
	// 创建时须可反序列化为目标模型实例，否则被拒绝
	Input map[string]any `json:"input" binding:"required"`
}

// TagRuleSampleUpdateIn 定义更新标签规则样例的请求输入结构。
// 样例的 Model 与 Input 创建后不可变，仅允许调整 Name 与 Description。
type TagRuleSampleUpdateIn struct {
	// Name 样例名称，同一模型内唯一，仅允许字母、数字、下划线、连字符，长度 1~128
	Name string `json:"name" binding:"required,min=1,max=128,alphanumspecial=_-"`
	// Description 样例说明，最长 512 字符
	Description string `json:"description" binding:"max=512"`
}

// TagRuleSampleBaseDetailOut 定义标签规则样例的基础详情输出结构。
// 包含样例的标识字段与审计字段，用于列表和详情场景的公共部分。
type TagRuleSampleBaseDetailOut struct {
	utils.BaseItemOut
	// Model 目标模型类型键
	Model string `json:"model"`
	// Name 样例名称
	Name string `json:"name"`
	// Description 样例说明
	Description string `json:"description"`
}

// TagRuleSampleDetailOut 定义标签规则样例的完整详情输出结构。
// 在基础详情之上附加样例输入文档（反序列化后的 map）。
type TagRuleSampleDetailOut struct {
	TagRuleSampleBaseDetailOut
	// Input 样例输入文档
	Input map[string]any `json:"input"`
}

// TagRuleSampleListOut 定义标签规则样例列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计；省略 Input 重负载字段（详情接口返回）。
type TagRuleSampleListOut struct {
	TagRuleSampleBaseDetailOut
	utils.BaseTotalOut
}

// invalidTagRuleSampleInputError 构造样例输入文档无法反序列化为目标模型的错误响应（HTTP 400）。
// 反序列化失败说明字段类型与模型定义不符，样例不可用于规则运行期校验，写入时直接拒绝。
func invalidTagRuleSampleInputError(err error) *manager.CodeError {
	return manager.CreateCodeError(http.StatusBadRequest, manager.CodeTagRuleSampleInputInvalid, err.Error())
}

// tagRuleSampleListGet 处理获取标签规则样例列表的请求。
// 支持分页、排序与过滤（时间/名称/目标模型）；样例为全局数据，不过滤命名空间。
func tagRuleSampleListGet(c *gin.Context) {
	var in TagRuleSampleListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*TagRuleSampleListOut
	db := manager.GetCoreDatabase(c).Model(&tag.TagRuleSample{})
	if query, err := utils.BuildSearchQuery(db, &in); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinW(c, utils.NewSearchOut(items))
}

// tagRuleSampleCreate 处理创建标签规则样例的请求。
// 创建前校验目标模型已注册（未注册直接拒绝，避免死样例），并将输入文档
// 反序列化为目标模型实例验证（与规则运行期样例校验的反序列化路径一致），
// 失败拒绝落库；通过后输入文档序列化为 JSONB 落库。
func tagRuleSampleCreate(c *gin.Context) {
	var in TagRuleSampleCreateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	// 目标模型未注册的样例引擎永不加载，写入时直接拒绝避免死样例
	if !tagengine.IsRegistered(in.Model) {
		invalidTagRuleModelNotRegisteredError(in.Model).AbortWithGinH(c)
		return
	}
	raw, err := json.Marshal(in.Input)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 输入文档须能反序列化为目标模型实例：字段类型与模型定义不符等
	// 不可解析的样例在写入时直接拒绝，避免运行期样例校验才暴露
	target, err := tagengine.NewTargetInstance(in.Model)
	if err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := json.Unmarshal(raw, target); err != nil {
		invalidTagRuleSampleInputError(err).AbortWithGinH(c)
		return
	}
	// 模型含 S3 body 字段且输入实际引用对象时，将 body 对象复制为样例专属副本（永久保留），
	// 并改写 input 指向副本；源对象已过期（不存在）时拒绝创建，保证样例引用始终有效。
	// 输入无 body 引用（字段缺失/为空）时不触碰 S3。
	var rollback func()
	if keys, extractErr := manager.SampleBodyObjectKeys(in.Model, in.Input); extractErr != nil {
		// S3 body 字段值类型非字符串：输入与模型定义不符，按输入不合法拒绝
		invalidTagRuleSampleInputError(extractErr).AbortWithGinH(c)
		return
	} else if len(keys) > 0 {
		uid := serializer.NewHexUUIDv4().String()
		var copyErr error
		rollback, copyErr = manager.CopySampleS3Objects(c.Request.Context(), manager.GetS3Client(c), in.Model, in.Input, uid)
		if copyErr != nil {
			if rollback != nil {
				rollback()
			}
			if s3.IsNotExists(copyErr) {
				manager.CreateCodeError(http.StatusBadRequest, manager.CodeTagRuleSampleInputInvalid, nil).AbortWithGinH(c)
			} else {
				manager.CreateAccidentError(copyErr).AbortWithGinH(c)
			}
			return
		}
	}
	raw, err = json.Marshal(in.Input)
	if err != nil {
		if rollback != nil {
			rollback()
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	instance := &tag.TagRuleSample{
		Model:       in.Model,
		Name:        in.Name,
		Description: in.Description,
		Input:       datatypes.JSON(raw),
	}
	if err := manager.GetCoreDatabase(c).WithContext(c).Create(instance).Error; err != nil {
		if rollback != nil {
			rollback()
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagRuleSampleDetailGet 处理获取标签规则样例详情的请求。
// 返回样例完整内容，输入文档反序列化为 map 后输出。
func tagRuleSampleDetailGet(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	instance := tag.TagRuleSample{}
	if err := manager.GetCoreDatabase(c).WithContext(c).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	input := map[string]any{}
	if err := json.Unmarshal(instance.Input, &input); err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinW(c, TagRuleSampleDetailOut{
		TagRuleSampleBaseDetailOut: newTagRuleSampleBaseDetailOut(&instance),
		Input:                      input,
	})
}

// tagRuleSampleUpdate 处理更新标签规则样例的请求。
// 样例的 Model 与 Input 创建后不可变，仅允许调整 Name 与 Description；
// 落库仅写 name/description 两列，不触碰 model/input。
func tagRuleSampleUpdate(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	var in TagRuleSampleUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		manager.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	instance := tag.TagRuleSample{}
	db := manager.GetCoreDatabase(c)
	if err := db.WithContext(c).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Model(&tag.TagRuleSample{}).Where("id = ?", instance.ID).Updates(map[string]any{
		"name":        in.Name,
		"description": in.Description,
	}).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// tagRuleSampleDelete 处理删除标签规则样例的请求。
// 先同步删除样例引用的 S3 body 对象，再删除数据库记录；
// S3 删除失败时保留记录返回错误（可重试），避免残留孤儿对象。
// 删除后若该模型后续触发懒供给，缺失的系统样例（default 等）会被重新生成。
func tagRuleSampleDelete(c *gin.Context) {
	id, ok := getUint64Param(c, "id")
	if !ok {
		return
	}
	instance := tag.TagRuleSample{}
	db := manager.GetCoreDatabase(c)
	if err := db.WithContext(c).First(&instance, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			manager.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 模型含 S3 body 字段时，先同步删除样例引用的对象，再删除数据库记录；
	// 删除失败时保留记录返回错误（可重试），避免残留孤儿对象。无 S3 字段的模型跳过。
	if len(manager.SampleS3Fields[instance.Model]) > 0 {
		input := map[string]any{}
		if err := json.Unmarshal(instance.Input, &input); err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
		if err := manager.DeleteSampleS3Objects(c.Request.Context(), manager.GetS3Client(c), instance.Model, input); err != nil {
			manager.CreateAccidentError(err).AbortWithGinH(c)
			return
		}
	}
	if err := db.WithContext(c).Where("id = ?", instance.ID).Delete(&tag.TagRuleSample{}).Error; err != nil {
		manager.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	manager.SimpleOK.AbortWithGinH(c)
}

// newTagRuleSampleBaseDetailOut 将样例模型转换为基础详情输出结构。
func newTagRuleSampleBaseDetailOut(sample *tag.TagRuleSample) TagRuleSampleBaseDetailOut {
	return TagRuleSampleBaseDetailOut{
		BaseItemOut: utils.BaseItemOut{
			ID:        sample.ID,
			CreatedAt: sample.CreatedAt,
			UpdatedAt: sample.UpdatedAt,
		},
		Model:       sample.Model,
		Name:        sample.Name,
		Description: sample.Description,
	}
}
