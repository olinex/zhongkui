package ai

import (
	// 标准库
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	skillstore "zhongkuisec.com/zhongkui/internal/ai/skill"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// TestNewChatInputPassesSkillID 校验请求中的技能 ID 透传到轮次输入；技能名称/说明/正文由
// AgentChat 在库校验通过后覆盖为库中值，故此处仅断言 ID 透传与其余字段零值。
func TestNewChatInputPassesSkillID(t *testing.T) {
	session := &modelsai.AgentSession{}
	session.ID = 3
	session.UserID = 7
	session.NamespaceName = knowledgeTestNamespace

	in := newChatInput(session, AgentChatIn{Query: "q", SkillID: 42})
	if in.SkillID != 42 {
		t.Fatalf("SkillID = %d, want 42", in.SkillID)
	}
	if in.SkillName != "" || in.SkillDescription != "" || in.SkillContent != "" {
		t.Fatalf("skill fields should stay empty before the DB lookup: %+v", in)
	}
	if in.SessionID != 3 || in.UserID != 7 || in.NamespaceName != knowledgeTestNamespace || in.Query != "q" {
		t.Fatalf("chat input identity = %+v", in)
	}
}

// TestRequireChatSkillRejectsUnavailable 校验聊天技能的三种不可用场景（不存在 / 已停用 / 存储缺失）
// 一律以 400 中断，启用态技能则原样返回库中内容。
func TestRequireChatSkillRejectsUnavailable(t *testing.T) {
	db := createKnowledgeTestDB(t)
	store, err := skillstore.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	comps := &ai.Components{Skills: store}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	var selected *chatSkillSelection
	r.GET("/:id", func(c *gin.Context) {
		id, convErr := strconv.ParseUint(c.Param("id"), 10, 64)
		if convErr != nil {
			t.Fatalf("bad test id: %v", convErr)
		}
		var ok bool
		selected, ok = resolveChatSkill(c, comps, knowledgeTestNamespace, AgentChatIn{SkillID: uint(id)})
		if ok && selected != nil {
			c.Status(http.StatusOK)
		}
	})

	call := func(id uint) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/"+strconv.FormatUint(uint64(id), 10), nil)
		r.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := call(999999); code != http.StatusBadRequest {
		t.Fatalf("missing skill status = %d, want 400", code)
	}

	created, err := store.Create(context.Background(), knowledgeTestNamespace, skillstore.CreateInput{
		Name: "chat-skill", Description: "说明", Content: "正文",
	})
	if err != nil {
		t.Fatalf("Create skill: %v", err)
	}
	if code := call(created.ID); code != http.StatusOK {
		t.Fatalf("enabled skill status = %d, want 200", code)
	}
	if selected == nil || selected.Name != "chat-skill" || selected.Content != "正文" {
		t.Fatalf("selected skill = %+v", selected)
	}

	if err := store.SetEnabled(context.Background(), knowledgeTestNamespace, created.ID, false); err != nil {
		t.Fatalf("SetEnabled: %v", err)
	}
	if code := call(created.ID); code != http.StatusBadRequest {
		t.Fatalf("disabled skill status = %d, want 400", code)
	}

	// 技能存储缺失（组件未装配）同样拒绝
	empty := &ai.Components{}
	selection, ok := resolveChatSkill(newGinContext(t), empty, knowledgeTestNamespace, AgentChatIn{SkillID: created.ID})
	if ok || selection != nil {
		t.Fatal("resolveChatSkill returned a custom skill without a store")
	}
}

// TestResolveChatSkillBuiltin 校验内置技能以数据库注册行 ID 为准、正文取自代码注册表：
// 临时登记一条已启用的注册行与对应代码正文后，按行 ID 解析出库中的名称 / 描述与代码正文。
func TestResolveChatSkillBuiltin(t *testing.T) {
	db := createKnowledgeTestDB(t)
	store, err := skillstore.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	row := modelsai.AgentSkill{
		NamespaceName: base.SystemNamespace,
		Source:        modelsai.SkillSourceBuiltin,
		Name:          "chat_builtin_test",
		Description:   "用于验证内置技能解析",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed builtin skill row: %v", err)
	}

	original := append([]skillstore.BuiltinSkill(nil), skillstore.BuiltinSkills...)
	defer func() { skillstore.BuiltinSkills = original }()
	skillstore.BuiltinSkills = append(skillstore.BuiltinSkills, skillstore.BuiltinSkill{
		Name:    "chat_builtin_test",
		Content: "## 指引",
	})

	selection, ok := resolveChatSkill(
		newGinContext(t), &ai.Components{Skills: store}, knowledgeTestNamespace,
		AgentChatIn{SkillID: row.ID},
	)
	if !ok || selection == nil {
		t.Fatal("resolveChatSkill rejected an enabled builtin skill")
	}
	if selection.ID != row.ID || selection.Name != "chat_builtin_test" || selection.Description != "用于验证内置技能解析" || selection.Content != "## 指引" {
		t.Fatalf("builtin selection = %+v", selection)
	}
}

// TestResolveChatSkillRejectsUnavailableID 校验不存在、已停用与代码正文缺失的内置技能一律拒绝。
func TestResolveChatSkillRejectsUnavailableID(t *testing.T) {
	db := createKnowledgeTestDB(t)
	store, err := skillstore.NewStore(db)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	disabled := modelsai.AgentSkill{
		NamespaceName: base.SystemNamespace,
		Source:        modelsai.SkillSourceBuiltin,
		Name:          "chat_builtin_disabled",
		Description:   "停用内置技能",
	}
	if err := db.Create(&disabled).Error; err != nil {
		t.Fatalf("seed disabled builtin skill row: %v", err)
	}
	// enabled 的 gorm default:true 会在 Create 时替换零值，故停用状态单独更新落库。
	if err := db.Model(&modelsai.AgentSkill{}).
		Where("id = ?", disabled.ID).
		Update("enabled", false).Error; err != nil {
		t.Fatalf("disable builtin skill row: %v", err)
	}
	noContent := modelsai.AgentSkill{
		NamespaceName: base.SystemNamespace,
		Source:        modelsai.SkillSourceBuiltin,
		Name:          "chat_builtin_no_content",
		Description:   "缺少代码正文",
	}
	if err := db.Create(&noContent).Error; err != nil {
		t.Fatalf("seed builtin skill row without content: %v", err)
	}
	original := append([]skillstore.BuiltinSkill(nil), skillstore.BuiltinSkills...)
	defer func() { skillstore.BuiltinSkills = original }()
	skillstore.BuiltinSkills = append(skillstore.BuiltinSkills,
		skillstore.BuiltinSkill{Name: "chat_builtin_disabled", Content: "## 指引"},
	)

	for _, id := range []uint{999999, disabled.ID, noContent.ID} {
		selection, ok := resolveChatSkill(
			newGinContext(t), &ai.Components{Skills: store}, knowledgeTestNamespace,
			AgentChatIn{SkillID: id},
		)
		if ok || selection != nil {
			t.Fatalf("resolveChatSkill(id=%d) = %+v, ok = %v, want rejection", id, selection, ok)
		}
	}
}

// newGinContext 返回注入了 i18n 中间件、带请求的测试 Gin 上下文。
func newGinContext(t *testing.T) *gin.Context {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
	setupI18nMiddleware()(c)
	return c
}

func TestParseAfterIndexRejectsNegative(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.GET("/", func(c *gin.Context) {
		var out int
		_ = parseAfterIndex(c, "-1", &out)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestAgentInteractionReopenInConditionalRequired 校验「callIDs 与 interruptIDs 至少提供其一」
// 由结构体标签表达：两者都空拒绝，任一非空放行，元素空白拒绝。
func TestAgentInteractionReopenInConditionalRequired(t *testing.T) {
	cases := []struct {
		name    string
		in      AgentInteractionReopenIn
		wantErr bool
	}{
		{name: "both empty", in: AgentInteractionReopenIn{}, wantErr: true},
		{name: "call ids", in: AgentInteractionReopenIn{CallIDs: []string{"call-1"}}, wantErr: false},
		{name: "interrupt ids", in: AgentInteractionReopenIn{InterruptIDs: []string{"int-1"}}, wantErr: false},
		{name: "blank call id", in: AgentInteractionReopenIn{CallIDs: []string{"  "}}, wantErr: true},
		{name: "blank interrupt id", in: AgentInteractionReopenIn{InterruptIDs: []string{""}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validator.Check(&tc.in)
			if tc.wantErr && err == nil {
				t.Fatal("validator.Check() = nil, want error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validator.Check() = %v, want nil", err)
			}
		})
	}
}
