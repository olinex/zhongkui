package database

import (
	"fmt"
	"reflect"
	"sync"

	"zhongkui.com/zhongkui/internal/pkg/errors"
)

var (
	// ErrUnsupportedModel 目标不是（可解引用的）结构体时返回
	ErrUnsupportedModel = errors.New("unsupported model, expected a struct or pointer to struct")
)

// ModelKey 计算目标模型实例的类型键。
// 类型键格式为 pkgpath.TypeName（如 zhongkui.com/zhongkui/internal/models/network.HttpTransport），
// 作为数据库规则的模型分组标识（TagRule.Model 等模型字段的取值）。
// 指针自动解引用。可打标模型须在全局注册表（见 registry.go）中登记。
func ModelKey(target any) string {
	t := reflect.TypeOf(target)
	if t == nil {
		panic(ErrUnsupportedModel)
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		panic(ErrUnsupportedModel)
	}
	return fmt.Sprintf(`%s.%s`, t.PkgPath(), t.Name())
}

type SchemaManager struct {
	once    sync.Once
	models  []any
	schemas map[string]any
}

func NewSchemaManager() *SchemaManager {
	return &SchemaManager{schemas: make(map[string]any)}
}

func (s *SchemaManager) Sync(db *DB) {
	s.once.Do(func() {
		for _, model := range s.models {
			s.schemas[ModelKey(model)] = model
		}
	})
}

func (s *SchemaManager) Register(model any) {
	s.models = append(s.models, model)
}

func (s *SchemaManager) Models() []any {
	return s.models
}

func (s *SchemaManager) Schema(model any) any {
	return s.schemas[ModelKey(model)]
}
