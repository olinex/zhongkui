package skill

import (
	// 标准库
	"context"
	"encoding/json"
	"os"
	"testing"

	// 项目内部
	"zhongkuisec.com/zhongkui/internal/ai/aitest"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// 测试专用命名空间，与迁移种子的 default 隔离。
const (
	// skillTestNamespace 技能存储测试的主命名空间。
	skillTestNamespace = "skilltest"
	// skillOtherNamespace 跨命名空间隔离用例的另一个命名空间。
	skillOtherNamespace = "skilltest2"
)

// TestMain 初始化技能测试的前置条件：技能正文经字段级加密落库，故先初始化全局密钥派生。
func TestMain(m *testing.M) {
	aitest.InitKeys()
	os.Exit(m.Run())
}

// newSkillTestStore 返回共享 PostgreSQL 测试库上的技能存储，并在测试事务内确保测试专用
// 命名空间存在（满足 agent_skills.namespace_name 外键约束）。
func newSkillTestStore(t *testing.T) *Store {
	t.Helper()
	db := testdb.Core(t)
	for _, name := range []string{skillTestNamespace, skillOtherNamespace} {
		if err := db.Exec(
			`INSERT INTO namespaces (name, description) VALUES (?, ?) ON CONFLICT (name) DO NOTHING`,
			name, "ai skill store test namespace",
		).Error; err != nil {
			t.Fatalf("seed namespace %q: %v", name, err)
		}
	}
	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("NewStore error = %v", err)
	}
	return store
}

// mustCreateSkill 创建一条技能，失败即终止当前用例。
func mustCreateSkill(t *testing.T, store *Store, namespace string, in CreateInput) *modelsai.AgentSkill {
	t.Helper()
	created, err := store.Create(context.Background(), namespace, in)
	if err != nil {
		t.Fatalf("Create(%+v) error = %v", in, err)
	}
	return created
}

// TestNewStore_NilDB 测试 db 为空时返回英文错误。
func TestNewStore_NilDB(t *testing.T) {
	if _, err := NewStore(nil); err == nil {
		t.Fatal("NewStore(nil) = nil error, want an error")
	}
}

// TestCreateGet_ReadsBackContent 测试创建后可按 ID 读回解密正文与启用状态。
func TestCreateGet_ReadsBackContent(t *testing.T) {
	store := newSkillTestStore(t)
	created := mustCreateSkill(t, store, skillTestNamespace, CreateInput{
		Name:        "greet",
		Description: "打招呼技能",
		Content:     "# 技能正文\n按用户语言问候。",
	})
	if created.ID == 0 || !created.Enabled {
		t.Fatalf("Create() = %+v, want a non-zero ID and enabled skill", created)
	}
	got, err := store.Get(context.Background(), skillTestNamespace, created.ID)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if got.Name != "greet" || got.Description != "打招呼技能" || got.NamespaceName != skillTestNamespace {
		t.Errorf("Get() = %+v, want the created fields", got)
	}
	if string(got.Content.Value()) != "# 技能正文\n按用户语言问候。" {
		t.Errorf("Get().Content = %q, want the decrypted content", string(got.Content.Value()))
	}
}

// TestCreateUpdate_PersistsDefinitionFields 测试用户填写的周期能力、输入字段与固定参数在创建与
// 更新时整体落库，并保留固定参数各键的原始 JSON 类型。
func TestCreateUpdate_PersistsDefinitionFields(t *testing.T) {
	store := newSkillTestStore(t)
	created := mustCreateSkill(t, store, skillTestNamespace, CreateInput{
		Name:            "report",
		Content:         "周报正文，固定区域与阈值",
		PeriodicCapable: true,
		InputFields: []modelsai.SkillInputField{
			{Name: modelsai.SkillInputFieldNameNowTime, Type: modelsai.SkillInputFieldTypeDate, Required: true, Description: "运行时注入当前时间"},
		},
		FixedFields: map[string]json.RawMessage{
			modelsai.SkillFixedFieldTimeOffset: json.RawMessage(`"-24h"`),
			modelsai.SkillFixedFieldTimeWindow: json.RawMessage(`"168h"`),
			"region":                           json.RawMessage(`"cn"`),
			"threshold":                        json.RawMessage(`0.75`),
			"onlyHigh":                         json.RawMessage(`true`),
		},
	})
	if !created.PeriodicCapable {
		t.Errorf("Create().PeriodicCapable = false, want true")
	}
	fields, err := modelsai.ParseSkillInputFields(created.InputFields)
	if err != nil {
		t.Fatalf("ParseSkillInputFields error = %v", err)
	}
	if len(fields) != 1 || fields[0].Name != modelsai.SkillInputFieldNameNowTime ||
		fields[0].Type != modelsai.SkillInputFieldTypeDate {
		t.Fatalf("Create().InputFields = %+v, want one nowTime date field", fields)
	}
	fixedFields, err := modelsai.ParseSkillFixedFields(created.FixedFields)
	if err != nil {
		t.Fatalf("ParseSkillFixedFields error = %v", err)
	}
	if len(fixedFields) != 5 {
		t.Fatalf("fixed fields = %+v, want 5 entries", fixedFields)
	}
	assertFixedFieldRaw(t, fixedFields, "threshold", "number", "0.75")
	assertFixedFieldRaw(t, fixedFields, "onlyHigh", "boolean", "true")
	assertFixedFieldRaw(t, fixedFields, "region", "string", `"cn"`)

	// 更新按用户输入整体覆盖：关闭周期能力并清空输入字段与固定参数。
	updated, err := store.Update(context.Background(), skillTestNamespace, created.ID, UpdateInput{
		Name:            "report",
		Description:     "改为单次报告",
		Content:         "单次报告正文",
		PeriodicCapable: false,
		InputFields:     nil,
		FixedFields:     nil,
	})
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if updated.PeriodicCapable {
		t.Errorf("Update().PeriodicCapable = true, want false")
	}
	got, err := store.Get(context.Background(), skillTestNamespace, created.ID)
	if err != nil {
		t.Fatalf("Get after Update error = %v", err)
	}
	gotFields, err := modelsai.ParseSkillInputFields(got.InputFields)
	if err != nil {
		t.Fatalf("ParseSkillInputFields after Update error = %v", err)
	}
	if len(gotFields) != 0 {
		t.Errorf("Get().InputFields = %+v, want empty", gotFields)
	}
	gotFixed, err := modelsai.ParseSkillFixedFields(got.FixedFields)
	if err != nil {
		t.Fatalf("ParseSkillFixedFields after Update error = %v", err)
	}
	if len(gotFixed) != 0 {
		t.Errorf("Get().FixedFields = %+v, want empty", gotFixed)
	}
	if string(got.Content.Value()) != "单次报告正文" {
		t.Errorf("Get().Content = %q, want the updated content", string(got.Content.Value()))
	}
}

// assertFixedFieldRaw 断言固定参数的原始 JSON 类型与原文。
func assertFixedFieldRaw(t *testing.T, fields map[string]json.RawMessage, key, wantKind, wantRaw string) {
	t.Helper()
	raw, ok := fields[key]
	if !ok {
		t.Fatalf("fixed field %q is missing", key)
	}
	kind, err := validator.JSONValueKind(raw)
	if err != nil {
		t.Fatalf("JSONValueKind(%q) error = %v", key, err)
	}
	if string(kind) != wantKind {
		t.Errorf("fixed field %q kind = %q, want %q", key, kind, wantKind)
	}
	if string(raw) != wantRaw {
		t.Errorf("fixed field %q raw = %q, want %q", key, string(raw), wantRaw)
	}
}

// TestBuiltinSkills_EmptyFixedFields 测试内置技能注册行的固定参数恒为空对象。
func TestBuiltinSkills_EmptyFixedFields(t *testing.T) {
	store := newSkillTestStore(t)
	rows, err := store.ListEnabledBuiltinSkills(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledBuiltinSkills error = %v", err)
	}
	for _, row := range rows {
		fields, err := modelsai.ParseSkillFixedFields(row.FixedFields)
		if err != nil {
			t.Fatalf("ParseSkillFixedFields(%q) error = %v", row.Name, err)
		}
		if len(fields) != 0 {
			t.Errorf("builtin skill %q fixed fields = %+v, want empty", row.Name, fields)
		}
	}
}

// TestCreate_DuplicateNameRejected 测试同命名空间内同名技能被拒绝。
func TestCreate_DuplicateNameRejected(t *testing.T) {
	store := newSkillTestStore(t)
	mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "dup", Content: "first"})
	if _, err := store.Create(context.Background(), skillTestNamespace, CreateInput{Name: "dup", Content: "second"}); !errors.Is(err, ErrSkillNameExists) {
		t.Errorf("Create(duplicate) error = %v, want ErrSkillNameExists", err)
	}
}

// TestGet_NotFound 测试查询不存在的技能返回 ErrSkillNotFound。
func TestGet_NotFound(t *testing.T) {
	store := newSkillTestStore(t)
	if _, err := store.Get(context.Background(), skillTestNamespace, 999999); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("Get(missing) error = %v, want ErrSkillNotFound", err)
	}
}

// TestUpdate_RefreshesUpdatedAtAndRejectsDuplicate 测试更新刷新 updated_at、可由详情读回新正文，
// 且改成同命名空间下其他技能的名称被拒绝、保留自身名称则允许。
func TestUpdate_RefreshesUpdatedAtAndRejectsDuplicate(t *testing.T) {
	store := newSkillTestStore(t)
	first := mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "first", Content: "first content"})
	second := mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "second", Content: "second content"})

	before := second.UpdatedAt
	updated, err := store.Update(context.Background(), skillTestNamespace, second.ID, UpdateInput{
		Name:        "second-renamed",
		Description: "更新后的描述",
		Content:     "updated content",
	})
	if err != nil {
		t.Fatalf("Update error = %v", err)
	}
	if !updated.UpdatedAt.After(before) {
		t.Errorf("Update().UpdatedAt = %v, want later than %v", updated.UpdatedAt, before)
	}
	got, err := store.Get(context.Background(), skillTestNamespace, second.ID)
	if err != nil {
		t.Fatalf("Get after Update error = %v", err)
	}
	if got.Name != "second-renamed" || got.Description != "更新后的描述" || string(got.Content.Value()) != "updated content" {
		t.Errorf("Get after Update = %+v, want the updated fields", got)
	}
	if _, err := store.Update(context.Background(), skillTestNamespace, second.ID, UpdateInput{Name: first.Name, Content: "x"}); !errors.Is(err, ErrSkillNameExists) {
		t.Errorf("Update(duplicate name) error = %v, want ErrSkillNameExists", err)
	}
	if _, err := store.Update(context.Background(), skillTestNamespace, second.ID, UpdateInput{Name: "second-renamed", Content: "x"}); err != nil {
		t.Errorf("Update(keeping own name) error = %v, want nil", err)
	}
}

// TestSetEnabled 测试启停生效，以及不存在的技能返回 ErrSkillNotFound。
func TestSetEnabled(t *testing.T) {
	store := newSkillTestStore(t)
	skill := mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "toggle", Content: "toggle content"})

	if err := store.SetEnabled(context.Background(), skillTestNamespace, skill.ID, false); err != nil {
		t.Fatalf("SetEnabled(false) error = %v", err)
	}
	got, err := store.Get(context.Background(), skillTestNamespace, skill.ID)
	if err != nil {
		t.Fatalf("Get error = %v", err)
	}
	if got.Enabled {
		t.Errorf("Get().Enabled = true, want false")
	}
	if err := store.SetEnabled(context.Background(), skillTestNamespace, 999999, true); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("SetEnabled(missing) error = %v, want ErrSkillNotFound", err)
	}
}

// TestDelete_Batch 测试批量软删除生效，以及空 ID 列表返回 ErrEmptySkillIDs。
func TestDelete_Batch(t *testing.T) {
	store := newSkillTestStore(t)
	items := []*modelsai.AgentSkill{
		mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "del-1", Content: "one"}),
		mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "del-2", Content: "two"}),
		mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "del-3", Content: "three"}),
	}
	if err := store.Delete(context.Background(), skillTestNamespace, []uint{items[0].ID, items[1].ID}); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	for _, removed := range items[:2] {
		if _, err := store.Get(context.Background(), skillTestNamespace, removed.ID); !errors.Is(err, ErrSkillNotFound) {
			t.Errorf("Get(deleted %d) error = %v, want ErrSkillNotFound", removed.ID, err)
		}
	}
	if _, err := store.Get(context.Background(), skillTestNamespace, items[2].ID); err != nil {
		t.Errorf("Get(remaining) error = %v, want nil", err)
	}
	if err := store.Delete(context.Background(), skillTestNamespace, nil); !errors.Is(err, ErrEmptySkillIDs) {
		t.Errorf("Delete(nil) error = %v, want ErrEmptySkillIDs", err)
	}
}

// TestCrossNamespaceInvisible 测试其他命名空间读不到、也改不了本命名空间的技能。
func TestCrossNamespaceInvisible(t *testing.T) {
	store := newSkillTestStore(t)
	created := mustCreateSkill(t, store, skillTestNamespace, CreateInput{Name: "isolated", Content: "content"})

	if _, err := store.Get(context.Background(), skillOtherNamespace, created.ID); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("Get(other namespace) error = %v, want ErrSkillNotFound", err)
	}
	if _, err := store.Update(context.Background(), skillOtherNamespace, created.ID, UpdateInput{Name: "hijack", Content: "x"}); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("Update(other namespace) error = %v, want ErrSkillNotFound", err)
	}
	if err := store.SetEnabled(context.Background(), skillOtherNamespace, created.ID, false); !errors.Is(err, ErrSkillNotFound) {
		t.Errorf("SetEnabled(other namespace) error = %v, want ErrSkillNotFound", err)
	}
}
