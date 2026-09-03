// Package routes 提供 HTTP 路由注册和管理功能。
// 本文件包含通知模板（NotificationTemplate）管理相关的 API 路由处理器，
// 提供事件模板枚举（events）、模板列表、创建/更新、恢复默认（restore-default）
// 与渲染预览（render-test）能力。通知模板为命名空间级资源，
// 同一命名空间下（event + channel_type + language）组合唯一。
// 内置默认模板由 internal/notify/template.go 注册表种子生成（source=default），
// 用户覆盖模板在此管理接口维护（source=override）。
package views

import (
	// stdlib
	"net/http"
	"slices"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	message "zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

// NotificationTemplateEventsOut 定义通知模板事件枚举的输出结构。
// 枚举注册表中各事件的默认模板能力，供前端动态渲染模板管理页。
type NotificationTemplateEventsOut struct {
	// Events 已注册事件模板的事件列表
	Events []NotificationTemplateEventItemOut `json:"events"`
}

// NotificationTemplateEventItemOut 定义单条事件模板枚举项。
type NotificationTemplateEventItemOut struct {
	// Event 事件类型键
	Event string `json:"event"`
	// Template 消息模板键
	Template string `json:"template"`
	// Channels 该事件支持的渠道类型键列表（可能含 "*" 通配）
	Channels []string `json:"channels"`
	// Languages 该事件支持的语言键列表（可能含 "*" 通配）
	Languages []string `json:"languages"`
	// DefaultTitle 平台默认语言下的默认标题模板
	DefaultTitle string `json:"defaultTitle"`
	// DefaultContent 平台默认语言下的默认正文模板
	DefaultContent string `json:"defaultContent"`
	// Fields 该事件的可选模板变量名清单（供前端智能补全）
	Fields []string `json:"fields"`
}

// NotificationTemplateListGetSearchIn 定义通知模板列表查询的请求输入结构。
// 支持按事件、来源、语言与渠道类型过滤；分页使用工作区统一的 Pagination 结构。
type NotificationTemplateListGetSearchIn struct {
	Pagination
	// Event 事件类型键，可选，精确过滤
	Event string `json:"event"`
	// Source 模板来源，可选，仅允许 default / override
	Source string `json:"source" binding:"omitempty,oneof=default override"`
	// Language 语言键，可选，精确过滤
	Language string `json:"language"`
	// ChannelType 渠道类型键，可选，精确过滤
	ChannelType string `json:"channelType"`
}

// NotificationTemplateItemOut 定义通知模板列表的输出项结构。
// 字段与核心模型解耦（独立定义），JSON 键与映射来源模型字段保持一致。
type NotificationTemplateItemOut struct {
	// Id 模板 ID
	Id uint `json:"id"`
	// NamespaceName 命名空间名称
	NamespaceName string `json:"namespaceName"`
	// Event 事件类型键
	Event string `json:"event"`
	// ChannelType 渠道类型键
	ChannelType string `json:"channelType"`
	// Language 语言键
	Language string `json:"language"`
	// Title 标题模板
	Title string `json:"title"`
	// Content 正文模板
	Content string `json:"content"`
	// Source 模板来源：default | override
	Source string `json:"source"`
	// Description 模板描述
	Description string `json:"description"`
}

// NotificationTemplateListOut 定义通知模板列表的输出项结构。
// 嵌入 BaseTotalOut 以支持分页总数统计（Total 不参与 JSON 序列化）。
type NotificationTemplateListOut struct {
	NotificationTemplateItemOut
	utils.BaseTotalOut
}

// NotificationTemplateCreateIn 定义创建通知模板的请求输入结构。
type NotificationTemplateCreateIn struct {
	// Event 事件类型键，必填，必须为注册表中已知事件
	Event string `json:"event" binding:"required"`
	// ChannelType 渠道类型键，必填，必须为 notify.SupportedChannels() 之一
	ChannelType string `json:"channelType" binding:"required"`
	// Language 语言键，必填，必须为 notify.SupportedLanguages() 之一
	Language string `json:"language" binding:"required"`
	// Title 标题模板，必填
	Title string `json:"title" binding:"required"`
	// Content 正文模板，必填
	Content string `json:"content" binding:"required"`
	// Description 模板描述（可选）
	Description string `json:"description"`
}

// NotificationTemplateUpdateIn 定义更新通知模板的请求输入结构。
// 仅允许调整标题、正文与描述；事件/渠道/语言组合不可变，更新后强制为 override 来源。
type NotificationTemplateUpdateIn struct {
	// Id 模板 ID，必填
	Id uint `json:"id" binding:"required"`
	// Title 标题模板
	Title string `json:"title"`
	// Content 正文模板
	Content string `json:"content"`
	// Description 模板描述
	Description string `json:"description"`
}

// NotificationTemplateRestoreDefaultIn 定义恢复默认模板的请求输入结构。
// 语义：将整个事件在命名空间下的模板重置为代码内置默认（删除全部该事件行后重新种子化）。
type NotificationTemplateRestoreDefaultIn struct {
	// Event 事件类型键，必填，恢复该事件全部模板为默认
	Event string `json:"event" binding:"required"`
	// Language 语言键，可选（语义按事件整体恢复，忽略语言）
	Language string `json:"language" binding:"required"`
}

// NotificationTemplateRenderTestIn 定义模板渲染预览的请求输入结构。
type NotificationTemplateRenderTestIn struct {
	// Event 事件类型键，必填
	Event string `json:"event" binding:"required"`
	// ChannelType 渠道类型键，缺省使用 "*" 通配
	ChannelType string `json:"channelType"`
	// Language 语言键，缺省使用平台默认语言
	Language string `json:"language"`
	// Params 模板参数，供 Go text/template 渲染
	Params map[string]any `json:"params"`
}

// NotificationTemplateRenderTestOut 定义模板渲染预览的输出结构。
type NotificationTemplateRenderTestOut struct {
	// Event 事件类型键
	Event string `json:"event"`
	// ChannelType 实际使用的渠道类型键
	ChannelType string `json:"channelType"`
	// Language 实际使用的语言键
	Language string `json:"language"`
	// Title 渲染后的标题
	Title string `json:"title"`
	// Content 渲染后的正文
	Content string `json:"content"`
}

// NotificationTemplateValidateIn 定义模板语法校验的请求输入结构。
// 校验编辑器内尚未保存的标题/正文，均按 Go text/template 语法解析。
type NotificationTemplateValidateIn struct {
	// Title 标题模板
	Title string `json:"title"`
	// Content 正文模板
	Content string `json:"content"`
}

// NotificationTemplateValidateOut 定义模板语法校验的输出结构。
// 标题/正文各自返回是否合法与错误信息，校验失败不视为 HTTP 错误，便于编辑器反复调用。
type NotificationTemplateValidateOut struct {
	// TitleValid 标题模板语法是否合法
	TitleValid bool `json:"titleValid"`
	// TitleError 标题模板语法错误信息（合法时为空）
	TitleError string `json:"titleError"`
	// ContentValid 正文模板语法是否合法
	ContentValid bool `json:"contentValid"`
	// ContentError 正文模板语法错误信息（合法时为空）
	ContentError string `json:"contentError"`
}

// NotificationTemplateDryRunIn 定义模板试运行的请求输入结构。
// 使用事件的内置样例参数渲染编辑器内尚未保存的标题/正文，无需手填参数即可预览。
type NotificationTemplateDryRunIn struct {
	// Event 事件类型键，必填，用于选取该事件的样例参数
	Event string `json:"event" binding:"required"`
	// ChannelType 渠道类型键，仅回显，缺省使用 "*" 通配
	ChannelType string `json:"channelType"`
	// Language 语言键，仅回显，缺省使用平台默认语言
	Language string `json:"language"`
	// Title 标题模板
	Title string `json:"title"`
	// Content 正文模板
	Content string `json:"content"`
}

// NotificationTemplateDryRunOut 定义模板试运行的输出结构。
type NotificationTemplateDryRunOut struct {
	// Event 事件类型键
	Event string `json:"event"`
	// ChannelType 实际使用的渠道类型键
	ChannelType string `json:"channelType"`
	// Language 实际使用的语言键
	Language string `json:"language"`
	// Title 渲染后的标题
	Title string `json:"title"`
	// Content 渲染后的正文
	Content string `json:"content"`
}

// notificationTemplateRenderLogger 供 render-test 构造 notify.Service 使用的记录器。
// RenderPreview 不依赖 logger，这里以调试级记录器作占位，避免每次请求重复创建。
var notificationTemplateRenderLogger = func() *logging.Logger {
	logger, err := logging.NewDebugLogger()
	if err != nil {
		return nil
	}
	return logger
}()

// NotificationTemplateEventsGet 处理获取通知模板事件枚举的请求。
// 依据 notify.TemplateSpecs() 注册表返回各事件的支持渠道/语言与默认标题/正文。
func NotificationTemplateEventsGet(c *gin.Context) {
	specs := notify.TemplateSpecs()
	events := make([]NotificationTemplateEventItemOut, 0, len(specs))
	defLang := base.DefaultLanguage.String()
	for _, spec := range specs {
		events = append(events, NotificationTemplateEventItemOut{
			Event:          spec.Event,
			Template:       spec.Template,
			Channels:       slices.Clone(spec.Channels),
			Languages:      slices.Clone(spec.Languages),
			DefaultTitle:   spec.TitleByLang[defLang],
			DefaultContent: spec.ContentByLang[defLang],
			Fields:         notify.EventFields(spec.Event),
		})
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTemplateEventsOut{Events: events})
}

// NotificationTemplateListGet 处理获取通知模板列表的请求。
// 支持按事件/来源/语言/渠道类型精确过滤，显式分页（Count + Offset/Limit）并映射为 Out；
// 仅返回当前命名空间内的模板。
func NotificationTemplateListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTemplateListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c).WithContext(c).Model(&core.NotificationTemplate{}).
		Where("namespace_name = ?", namespace)
	if in.Event != "" {
		db = db.Where("event = ?", in.Event)
	}
	if in.Source != "" {
		db = db.Where("source = ?", in.Source)
	}
	if in.Language != "" {
		db = db.Where("language = ?", in.Language)
	}
	if in.ChannelType != "" {
		db = db.Where("channel_type = ?", in.ChannelType)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	var rows []core.NotificationTemplate
	offset := int((in.Page - 1) * in.PageSize)
	if err := db.Order("created_at DESC").Offset(offset).Limit(int(in.PageSize)).Find(&rows).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	items := make([]*NotificationTemplateListOut, 0, len(rows))
	for i := range rows {
		items = append(items, &NotificationTemplateListOut{
			NotificationTemplateItemOut: newNotificationTemplateItemOut(&rows[i]),
			BaseTotalOut:                utils.BaseTotalOut{Total: uint(total)},
		})
	}
	response.SimpleOK.AbortWithGinW(c, &utils.SearchOut[*NotificationTemplateListOut]{Total: uint(total), Items: items})
}

// NotificationTemplateCreate 处理创建通知模板的请求。
// 校验事件为注册表已知事件、渠道类型/语言在支持范围内（否则 400），
// 同一命名空间下 (event, channel_type, language) 已存在时返回 400；
// 新建模板来源强制为 override。
func NotificationTemplateCreate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTemplateCreateIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if _, ok := notify.EventSpecOf(in.Event); !ok {
		notificationTemplateBadInput(c, errors.Newf("notification template: unknown event %q", in.Event))
		return
	}
	if !slices.Contains(notify.SupportedChannels(), in.ChannelType) {
		notificationTemplateBadInput(c, errors.Newf("notification template: unsupported channel type %q", in.ChannelType))
		return
	}
	if !slices.Contains(notify.SupportedLanguages(), in.Language) {
		notificationTemplateBadInput(c, errors.Newf("notification template: unsupported language %q", in.Language))
		return
	}
	if err := notify.ValidateTemplateSyntax("title", in.Title); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template: invalid title: %v", err))
		return
	}
	if err := notify.ValidateTemplateSyntax("content", in.Content); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template: invalid content: %v", err))
		return
	}
	db := middlewares.GetCoreDatabase(c)
	var conflict int64
	if err := db.WithContext(c).Model(&core.NotificationTemplate{}).
		Where("namespace_name = ? AND event = ? AND channel_type = ? AND language = ?",
			namespace, in.Event, in.ChannelType, in.Language).
		Count(&conflict).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if conflict > 0 {
		notificationTemplateBadInput(c, errors.Newf(
			"notification template for event %q, channel %q, language %q already exists",
			in.Event, in.ChannelType, in.Language))
		return
	}
	instance := &core.NotificationTemplate{
		NamespaceName: namespace,
		Event:         in.Event,
		ChannelType:   in.ChannelType,
		Language:      in.Language,
		Title:         in.Title,
		Content:       in.Content,
		Source:        core.TemplateSourceOverride,
		Description:   in.Description,
	}
	if err := db.WithContext(c).Create(instance).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationTemplateUpdate 处理更新通知模板的请求。
// 按 (id, namespace) 定位行，不存在返回 404；标题/正文为空返回 400；
// 更新标题/正文/描述并强制 Source=override（覆盖内置默认）。
func NotificationTemplateUpdate(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTemplateUpdateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	if in.Title == "" {
		notificationTemplateBadInput(c, errors.New("notification template: title is required"))
		return
	}
	if in.Content == "" {
		notificationTemplateBadInput(c, errors.New("notification template: content is required"))
		return
	}
	if err := notify.ValidateTemplateSyntax("title", in.Title); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template: invalid title: %v", err))
		return
	}
	if err := notify.ValidateTemplateSyntax("content", in.Content); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template: invalid content: %v", err))
		return
	}
	db := middlewares.GetCoreDatabase(c)
	instance := core.NotificationTemplate{}
	if err := db.WithContext(c).Where("id = ? AND namespace_name = ?", in.Id, namespace).First(&instance).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.ErrDefaultPageNotFound.AbortWithGinH(c)
			return
		}
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := db.WithContext(c).Model(&core.NotificationTemplate{}).
		Where("id = ? AND namespace_name = ?", instance.ID, namespace).
		Updates(map[string]any{
			"title":       in.Title,
			"content":     in.Content,
			"description": in.Description,
			"source":      core.TemplateSourceOverride,
		}).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationTemplateRestoreDefault 恢复整个事件在该命名空间下的模板为内置默认。
// 先删除该事件在该命名空间的全部模板行，再调用 notify.SeedDefaultTemplates 重新种子化。
func NotificationTemplateRestoreDefault(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTemplateRestoreDefaultIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	db := middlewares.GetCoreDatabase(c)
	if err := db.WithContext(c).Where("namespace_name = ? AND event = ?", namespace, in.Event).
		Delete(&core.NotificationTemplate{}).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := notify.SeedDefaultTemplates(db.WithContext(c), namespace); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	response.SimpleOK.AbortWithGinH(c)
}

// NotificationTemplateRenderTest 渲染模板预览（render-test）。
// 按事件在命名空间下解析持久化模板并渲染标题/正文；缺省渠道使用 "*"、缺省语言使用
// 平台默认语言。渲染失败（未知事件、模板非法等）以 400 + 具体原因返回。
func NotificationTemplateRenderTest(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in NotificationTemplateRenderTestIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	channelType := in.ChannelType
	if channelType == "" {
		channelType = "*"
	}
	lang := in.Language
	if lang == "" {
		lang = base.DefaultLanguage.String()
	}
	service := notify.NewService(middlewares.GetCoreDatabase(c), notificationTemplateRenderLogger)
	rendered, err := service.RenderPreview(c, namespace, in.Event, channelType, lang, in.Params)
	if err != nil {
		// 渲染失败（未知事件、模板非法）作为正常业务错误返回，details 携带具体原因
		notificationTestSendFailed(c, err)
		return
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTemplateRenderTestOut{
		Event:       in.Event,
		ChannelType: channelType,
		Language:    lang,
		Title:       rendered.Title,
		Content:     rendered.Content,
	})
}

// NotificationTemplateValidate 处理模板语法校验请求。
// 分别按 Go text/template 语法解析编辑器内尚未保存的标题/正文，标题与正文各自返回
// 是否合法及错误信息；语法校验失败不视为 HTTP 错误（恒 200），便于编辑器反复调用。
func NotificationTemplateValidate(c *gin.Context) {
	var in NotificationTemplateValidateIn
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	out := NotificationTemplateValidateOut{
		TitleValid:   true,
		ContentValid: true,
	}
	if err := notify.ValidateTemplateSyntax("title", in.Title); err != nil {
		out.TitleValid = false
		out.TitleError = err.Error()
	}
	if err := notify.ValidateTemplateSyntax("content", in.Content); err != nil {
		out.ContentValid = false
		out.ContentError = err.Error()
	}
	response.SimpleOK.AbortWithGinW(c, out)
}

// NotificationTemplateDryRun 处理模板试运行请求。
// 使用事件的内置样例参数渲染编辑器内尚未保存的标题/正文，无需手填参数即可预览；
// 任一模板语法非法则 400 + 具体字段错误，阻止以非法模板试运行。
func NotificationTemplateDryRun(c *gin.Context) {
	var in NotificationTemplateDryRunIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	channelType := in.ChannelType
	if channelType == "" {
		channelType = "*"
	}
	lang := in.Language
	if lang == "" {
		lang = base.DefaultLanguage.String()
	}
	if err := notify.ValidateTemplateSyntax("title", in.Title); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template title invalid: %v", err))
		return
	}
	if err := notify.ValidateTemplateSyntax("content", in.Content); err != nil {
		notificationTemplateBadInput(c, errors.Newf("notification template content invalid: %v", err))
		return
	}
	// 以事件内置样例参数渲染；未注册样例参数时回退为空 map（missingkey=zero 下缺失键渲染为空）
	params, _ := notify.EventSampleParams(in.Event)
	if params == nil {
		params = make(map[string]any)
	}
	renderedTitle, err := message.RenderTemplateString(in.Title, params)
	if err != nil {
		notificationTemplateBadInput(c, errors.WithStack(err))
		return
	}
	renderedContent, err := message.RenderTemplateString(in.Content, params)
	if err != nil {
		notificationTemplateBadInput(c, errors.WithStack(err))
		return
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTemplateDryRunOut{
		Event:       in.Event,
		ChannelType: channelType,
		Language:    lang,
		Title:       renderedTitle,
		Content:     renderedContent,
	})
}

// notificationTemplateBadInput 以 400 + CodeBadBody 响应通知模板业务校验失败。
// details 为具体原因，供前端直接向用户展示。
func notificationTemplateBadInput(c *gin.Context, err error) {
	response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, err).AbortWithGinH(c)
}

// newNotificationTemplateItemOut 将 NotificationTemplate 转换为列表输出项结构。
func newNotificationTemplateItemOut(template *core.NotificationTemplate) NotificationTemplateItemOut {
	return NotificationTemplateItemOut{
		Id:            template.ID,
		NamespaceName: template.NamespaceName,
		Event:         template.Event,
		ChannelType:   template.ChannelType,
		Language:      template.Language,
		Title:         template.Title,
		Content:       template.Content,
		Source:        template.Source,
		Description:   template.Description,
	}
}
