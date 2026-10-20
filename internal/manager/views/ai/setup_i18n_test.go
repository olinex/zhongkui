package ai

import (
	// 标准库
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"

	// 第三方
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	en2 "github.com/go-playground/locales/en"
	ut "github.com/go-playground/universal-translator"
	uv "github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/pelletier/go-toml/v2"
	"go.uber.org/zap"
	"golang.org/x/text/language"

	// 项目内部
	ai "zhongkuisec.com/zhongkui/internal/ai"
	"zhongkuisec.com/zhongkui/internal/manager/middlewares"
	"zhongkuisec.com/zhongkui/internal/manager/response"
	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/models/core"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
	"zhongkuisec.com/zhongkui/internal/pkg/database"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
	"zhongkuisec.com/zhongkui/internal/pkg/logging"
	"zhongkuisec.com/zhongkui/internal/pkg/pipeline"
	"zhongkuisec.com/zhongkui/internal/pkg/s3"
	"zhongkuisec.com/zhongkui/internal/pkg/serializer"
	"zhongkuisec.com/zhongkui/internal/pkg/testdb"
	"zhongkuisec.com/zhongkui/internal/pkg/validator"
)

// knowledgeTestNamespace 知识库视图测试专用命名空间，与迁移种子的 default 隔离。
const knowledgeTestNamespace = "aitest"

// TestMain 初始化全局 KeyDerivation（crypto serializer 落库/读取所需）与 Gin binding
// 自定义验证规则。本测试包自包含（基座 views 包的 TestMain 未被子包继承）。
func TestMain(m *testing.M) {
	pb, err := key.LoadPasswordBook("")
	if err != nil {
		panic(err)
	}
	provider := key.NewConfigFileSecretKeyProvider("this-is-a-32-byte-secret-key!!")
	key.InitGlobalKeyDerivation(provider, pb)

	if v, ok := binding.Validator.Engine().(*uv.Validate); ok {
		for tag, fn := range validator.Funcs() {
			_ = v.RegisterValidation(tag, fn)
		}
	}

	os.Exit(m.Run())
}

// newKnowledgeTestLogger 创建测试用日志记录器（丢弃输出）。
func newKnowledgeTestLogger() *logging.Logger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{}
	zapLogger, _ := config.Build()
	return &logging.Logger{SugaredLogger: zapLogger.Sugar()}
}

// newKnowledgeTestLocalizer 返回测试专用 go-i18n Localizer，内置错误响应渲染所需的最小消息键。
func newKnowledgeTestLocalizer() *i18n.Localizer {
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)
	bundle.MustParseMessageFileBytes([]byte(`
code.1000 = "unknown error"
code.1002 = "bad body"
code.1003 = "bad query"
code.1004 = "bad url param"
code.1009 = "namespace not exist"
code.1120 = "no live run"
code.1121 = "live run expired"
code.1122 = "live run forbidden"
code.1124 = "the session has reached the turn limit"
code.1125 = "the current interaction state does not allow this operation"
code.200 = "ok"
code.404 = "not found"
error.emptyUrlParam = "empty url param: %s"
error.badUrlParam = "bad url param: %s"
error.aiFeatureDisabled = "ai feature is disabled"
error.knowledgeImportedContentReadOnly = "imported document content is read-only; reparse the document to refresh it"
error.knowledgeDocumentParsing = "document is already being parsed; wait for it to finish before reindexing"
error.knowledgeDocumentNotFound = "knowledge document %d not found"
error.knowledgeUploadTooLarge = "request body exceeds the maximum upload size of %d bytes"
error.knowledgeUploadFileRequired = "multipart field \"file\" is required"
error.knowledgeUnsupportedFormat = "unsupported document format %s"
`), "en.toml")
	return i18n.NewLocalizer(bundle, "en")
}

// newKnowledgeTestTranslator 返回测试专用 universal-translator。
func newKnowledgeTestTranslator() ut.Translator {
	uni := ut.New(en2.New())
	trans, _ := uni.GetTranslator("en")
	return trans
}

// setupI18nMiddleware 返回注入 Localizer 与 Translator 的 Gin 中间件。
func setupI18nMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(response.LocalizerCtxName, newKnowledgeTestLocalizer())
		c.Set(response.TranslatorCtxName, newKnowledgeTestTranslator())
		c.Next()
	}
}

// createKnowledgeTestDB 返回共享 PostgreSQL 测试库（testdb.Core 事务隔离）并确保测试专用
// 命名空间存在（满足 knowledge_documents.namespace_name 外键约束）。
func createKnowledgeTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := testdb.Core(t)
	ns := core.Namespace{Name: knowledgeTestNamespace}
	if err := db.Where(core.Namespace{Name: knowledgeTestNamespace}).FirstOrCreate(&ns).Error; err != nil {
		t.Fatalf("seed knowledge test namespace: %v", err)
	}
	return db
}

// newDisabledAiRuntime 构造未启用的 AI 运行时（模型注册表为空），用于覆盖「AI 未启用」分支。
func newDisabledAiRuntime(t *testing.T, db *database.DB) *ai.Runtime {
	t.Helper()
	rt, err := ai.NewRuntime(context.Background(), db, nil, newKnowledgeTestLogger())
	if err != nil {
		t.Fatalf("build disabled ai runtime: %v", err)
	}
	return rt
}

// newEnabledAiRuntime 通过种子化两个模型（chat + embedding）与两条档位绑定把运行时装配为
// 启用态，使 Knowledge 组件可用（用于覆盖「导入文档重新索引的进行中守卫」）。端点地址指向
// 不可达地址：装配阶段只构建客户端、不联网，解析任务也不在本测试中执行。
func newEnabledAiRuntime(t *testing.T, db *database.DB) *ai.Runtime {
	t.Helper()
	chat := &modelsai.LlmModel{
		Name: "chat-test", Kind: llm.ModelKindChat,
		BaseURL: "http://127.0.0.1:1/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "gpt-test", Enabled: true,
	}
	if err := db.Create(chat).Error; err != nil {
		t.Fatalf("seed chat model: %v", err)
	}
	embed := &modelsai.LlmModel{
		Name: "embed-test", Kind: llm.ModelKindEmbedding,
		BaseURL: "http://127.0.0.1:1/v1", APIKey: serializer.NewCrypto([]byte("sk-test")),
		ModelName: "emb-test", Enabled: true,
	}
	if err := db.Create(embed).Error; err != nil {
		t.Fatalf("seed embedding model: %v", err)
	}
	// 省略关联的模型实体：ModelID 已指向已播种的行，否则零值关联会被 GORM 执行 upsert。
	for _, binding := range []modelsai.LlmBinding{
		{Slot: modelsai.SlotStandard, ModelID: chat.ID},
		{Slot: modelsai.SlotEmbedding, ModelID: embed.ID},
	} {
		if err := db.Omit("Model").Create(&binding).Error; err != nil {
			t.Fatalf("seed slot binding %q: %v", binding.Slot, err)
		}
	}
	rt, err := ai.NewRuntime(context.Background(), db, nil, newKnowledgeTestLogger())
	if err != nil {
		t.Fatalf("build enabled ai runtime: %v", err)
	}
	return rt
}

// newKnowledgeTestRouter 创建知识库文档测试路由引擎，注入 i18n、日志、核心库、命名空间、
// AI 运行时、对象缓存与管道投递客户端。对象缓存与管道客户端以类型化 nil 注入：被测分支均在
// 触达它们之前返回，故无需真实 S3/asynq。
func newKnowledgeTestRouter(db *database.DB, namespace string, rt *ai.Runtime) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(setupI18nMiddleware())
	r.Use(middlewares.ZapLoggerMiddleware(newKnowledgeTestLogger()))
	r.Use(func(c *gin.Context) {
		c.Set(middlewares.CoreDatabaseCtxName, db)
		c.Set(middlewares.NamespaceCtxName, namespace)
		c.Set(middlewares.AiRuntimeCtxName, rt)
		c.Set(middlewares.ObjectCacheCtxName, (*s3.ObjectCache)(nil))
		c.Set(middlewares.PipelineClientCtxName, (*pipeline.Client)(nil))
		c.Next()
	})
	group := r.Group("/ai/doc")
	group.POST("/upload", KnowledgeDocumentUpload)
	group.POST("/:id/update", KnowledgeDocumentUpdate)
	group.POST("/:id/reindex", KnowledgeDocumentReindex)
	return r
}

// doKnowledgeUploadRequest 构造一次 multipart/form-data 上传请求（title 可选、file 最后），
// 并可按需覆盖 ContentLength 以触发体积早拒分支。
func doKnowledgeUploadRequest(r *gin.Engine, path, filename, title string, content []byte, contentLength int64) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if title != "" {
		_ = mw.WriteField("title", title)
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		panic("failed to create multipart file field: " + err.Error())
	}
	if _, err := fw.Write(content); err != nil {
		panic("failed to write multipart file body: " + err.Error())
	}
	_ = mw.Close()

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// doKnowledgeJSONRequest 执行一次 JSON 请求并返回响应记录器。
func doKnowledgeJSONRequest(r *gin.Engine, method, path string, body any) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			panic("failed to marshal test request body: " + err.Error())
		}
		reader = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// knowledgeDocPath 拼接知识库文档子操作路径。
func knowledgeDocPath(id uint, action string) string {
	return "/ai/doc/" + strconv.FormatUint(uint64(id), 10) + action
}
