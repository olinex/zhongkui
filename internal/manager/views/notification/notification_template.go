package notification

import (
	// stdlib
	"net/http"
	"slices"
	"sort"

	// third-party
	"github.com/creasty/defaults"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	// internal
	"zhongkui.com/zhongkui/internal/manager/middlewares"
	"zhongkui.com/zhongkui/internal/manager/response"
	"zhongkui.com/zhongkui/internal/manager/views"
	"zhongkui.com/zhongkui/internal/models/core"
	"zhongkui.com/zhongkui/internal/notify"
	"zhongkui.com/zhongkui/internal/pkg/base"
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	message "zhongkui.com/zhongkui/internal/pkg/message"
	"zhongkui.com/zhongkui/internal/pkg/utils"
)

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
	// Language 语言（base.Language），可为 "*" 通配
	Language base.Language `json:"language"`
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
	// Language 语言（base.Language），必填，必须为 notify.SupportedLanguages() 之一（可为 "*"）
	Language base.Language `json:"language" binding:"required"`
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
	// Language 语言（base.Language），必填（语义按事件整体恢复，忽略语言）
	Language base.Language `json:"language" binding:"required"`
}

// NotificationTemplateRenderTestIn 定义模板渲染预览的请求输入结构。
type NotificationTemplateRenderTestIn struct {
	// Event 事件类型键，必填
	Event string `json:"event" binding:"required"`
	// ChannelType 渠道类型键，缺省使用 "*" 通配
	ChannelType string `json:"channelType"`
	// Language 语言（base.Language），缺省使用平台默认语言
	Language base.Language `json:"language"`
	// Params 模板参数，供 Go text/template 渲染
	Params map[string]any `json:"params"`
}

// NotificationTemplateRenderTestOut 定义模板渲染预览的输出结构。
type NotificationTemplateRenderTestOut struct {
	// Event 事件类型键
	Event string `json:"event"`
	// ChannelType 实际使用的渠道类型键
	ChannelType string `json:"channelType"`
	// Language 实际使用的语言（base.Language）
	Language base.Language `json:"language"`
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
	// Language 语言（base.Language），仅回显，缺省使用平台默认语言
	Language base.Language `json:"language"`
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
	// Language 实际使用的语言（base.Language）
	Language base.Language `json:"language"`
	// Title 渲染后的标题
	Title string `json:"title"`
	// Content 渲染后的正文
	Content string `json:"content"`
}

// NotificationTemplateEventMetaItemOut 定义单条通知事件的精简元数据输出项。
// 供模板编辑器生成语言下拉提示与模板变量智能补全，不承载渠道/模板键等已由
// 统一枚举接口（/admin/enum）覆盖的枚举信息。
// NotificationTemplateEventMetaItemOut defines the concise metadata item of a single
// notification event, feeding the template editor's language dropdown hint and
// template-variable smart completion. It omits channel/template-key enum info that is
// already covered by the unified enum API (/admin/enum).
type NotificationTemplateEventMetaItemOut struct {
	// Event 事件类型键
	// Event the event type key
	Event string `json:"event"`
	// Languages 该事件支持的语言（BCP-47 标签）
	// Languages the languages supported by this event (BCP-47 tags)
	Languages []string `json:"languages"`
	// Fields 该事件可用的模板变量名清单（升序）
	// Fields the available template variable names of this event (ascending)
	Fields []string `json:"fields"`
}

// NotificationTemplateEventMetaOut 定义通知事件精简元数据接口的输出结构。
// NotificationTemplateEventMetaOut defines the output of the concise notification
// event metadata API.
type NotificationTemplateEventMetaOut struct {
	// Events 事件元数据项列表
	// Events the list of event metadata items
	Events []NotificationTemplateEventMetaItemOut `json:"events"`
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

// NotificationTemplateListGet 处理获取通知模板列表的请求。
// 基于统一搜索基座 BuildSearchQuery 支持分页、排序与多字段过滤（事件/来源/语言/渠道类型/
// 创建/更新时间），仅返回当前命名空间内的模板。
func NotificationTemplateListGet(c *gin.Context) {
	namespace, ok := middlewares.RequireNamespace(c)
	if !ok {
		return
	}
	var in views.NotificationTemplateListGetSearchIn
	if err := defaults.Set(&in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	if err := c.ShouldBindBodyWithJSON(&in); err != nil {
		response.CreateBadBodyError(err).AbortWithGinH(c)
		return
	}
	var items []*NotificationTemplateListOut
	if query, err := utils.BuildSearchCountQuery(
		middlewares.GetCoreDatabase(c).Model(&core.NotificationTemplate{}).
			Where("namespace_name = ?", namespace), &in); err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	} else if err := query.Find(&items).Error; err != nil {
		response.CreateAccidentError(err).AbortWithGinH(c)
		return
	}
	// 字段投影：按请求 fields 投影，未指定时回退全部白名单字段（封装于 utils.ProjectSearchItems）
	var total uint
	if len(items) > 0 {
		total = items[0].GetTotal()
	}
	projected := utils.ProjectSearchItems(items, &in, func(o **NotificationTemplateListOut) any {
		return *o
	})
	response.SimpleOK.AbortWithGinW(c, utils.NewSearchProjectedOut(total, projected))
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
	if !slices.Contains(notify.SupportedLanguages(), in.Language.String()) {
		notificationTemplateBadInput(c, errors.Newf("notification template: unsupported language %q", in.Language.String()))
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
			in.Event, in.ChannelType, in.Language.String()))
		return
	}
	instance := &core.NotificationTemplate{
		NamespaceName: namespace,
		Event:         in.Event,
		ChannelType:   in.ChannelType,
		Language:      in.Language.String(),
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
// 删除该事件在该命名空间的全部持久化模板行即可：默认标题/正文不再落库，渲染时若该
// 命名空间无对应模板行，将自动回退到翻译包 i18n 键 notify.<template>.title/.content。
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
	if lang.String() == "" {
		lang = *base.DefaultLanguage
	}
	service := notify.NewService(middlewares.GetCoreDatabase(c), notificationTemplateRenderLogger, middlewares.GetMessagePool(c))
	rendered, err := service.RenderPreview(c, namespace, in.Event, channelType, lang.String(), in.Params)
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
	if lang.String() == "" {
		lang = *base.DefaultLanguage
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

// NotificationTemplateEventsMetaGet 处理获取通知事件精简元数据的请求（/events/meta）。
// 遍历 notify.TemplateSpecs() 事件注册表，为每个事件组装支持语言（spec.Languages）与
// 可用模板变量清单（notify.EventFields），供模板编辑器语言下拉提示与智能补全使用；
// 事件键升序返回保证输出稳定。
// NotificationTemplateEventsMetaGet handles the concise notification event metadata
// request (/events/meta). It walks the notify.TemplateSpecs() registry and assembles
// the supported languages (spec.Languages) and available template variable list
// (notify.EventFields) per event for the template editor's language dropdown hint and
// smart completion; event keys are returned ascending for stable output.
func NotificationTemplateEventsMetaGet(c *gin.Context) {
	specs := notify.TemplateSpecs()
	events := make([]string, 0, len(specs))
	for event := range specs {
		events = append(events, event)
	}
	sort.Strings(events)
	items := make([]NotificationTemplateEventMetaItemOut, 0, len(events))
	for _, event := range events {
		spec := specs[event]
		languages := make([]string, 0, len(spec.Languages))
		for _, lang := range spec.Languages {
			languages = append(languages, lang.String())
		}
		items = append(items, NotificationTemplateEventMetaItemOut{
			Event:     event,
			Languages: languages,
			Fields:    notify.EventFields(event),
		})
	}
	response.SimpleOK.AbortWithGinW(c, NotificationTemplateEventMetaOut{Events: items})
}

// notificationTemplateBadInput 以 400 + CodeBadBody 响应通知模板业务校验失败。
// details 为具体原因，供前端直接向用户展示。
func notificationTemplateBadInput(c *gin.Context, err error) {
	response.CreateCodeError(http.StatusBadRequest, response.CodeBadBody, err).AbortWithGinH(c)
}

// init 注册通知模板相关枚举字段的枚举类绑定：
//   - NotificationTemplateCreateIn.event 对应 notificationEvent 类（notify.TemplateSpecs()）
//   - NotificationTemplateListOut.channelType 对应 channelType 类（message.Types()）
func init() {
	views.AddEnumBinding("NotificationTemplateCreateIn", "event", "notificationEvent")
	views.AddEnumBinding("NotificationTemplateListOut", "channelType", "channelType")
}
