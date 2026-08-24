package tag

import (
	"gorm.io/datatypes"

	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/pkg/database"
)

// TagRuleSample 表示标签识别规则的验证样例数据。
// 存储目标模型实例的 JSON 序列化文档（可反序列化回目标类型），
// 供规则运行期合法性校验（第二层）使用：将规则组作用于样例输入，
// 任一样例评估报错即判定规则运行期不合法。
// 样例归属命名空间（NamespaceName），与标签规则同租户隔离，按 (命名空间, 模型, 名称) 唯一索引。
type TagRuleSample struct {
	database.BaseModel
	// Namespace 所属命名空间的关联模型，用于多租户隔离
	Namespace core.Namespace `gorm:"foreignKey:NamespaceName;references:Name"`
	// NamespaceName 所属命名空间名称，同一命名空间内按 (模型, 名称) 唯一
	NamespaceName string `gorm:"type:VARCHAR(32);uniqueIndex:uq_tag_rule_samples_namespace_name_model_name"`
	// Model 目标模型类型键（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport）
	Model string `gorm:"type:VARCHAR(255);uniqueIndex:uq_tag_rule_samples_namespace_name_model_name"`
	// Name 样例名称，同一命名空间同一模型内唯一
	Name string `gorm:"type:VARCHAR(128);uniqueIndex:uq_tag_rule_samples_namespace_name_model_name"`
	// Input 样例输入文档（目标模型实例的 JSON 序列化）
	Input datatypes.JSON `gorm:"type:JSONB"`
	// Description 样例说明
	Description string `gorm:"type:VARCHAR(512)"`
}
