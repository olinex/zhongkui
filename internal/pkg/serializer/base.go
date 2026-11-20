// 本文件提供 serializer 包内多个序列化器共用的基础校验逻辑。
package serializer

import (
	"reflect"

	"gorm.io/gorm/schema"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

// validateFieldBinding 校验结构体标签与字段序列化类型一一对应：
// 字段（指针自动解引用）类型必须等于 want，且必须声明 tagKey 标签。
// 标签与类型绑定可防止 hash 字段误挂 crypto 标签（或反之）时静默派生错误密钥。
func validateFieldBinding(field *schema.Field, tagKey string, want reflect.Type) error {
	typ := field.FieldType
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ != want {
		return errors.Newf("%s: field %s must be %s, got %s", tagKey, field.Name, want, field.FieldType)
	}
	if _, ok := field.StructField.Tag.Lookup(tagKey); !ok {
		return errors.Newf("%s: field %s must carry a %q tag", tagKey, field.Name, tagKey)
	}
	return nil
}
