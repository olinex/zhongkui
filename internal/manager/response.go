// Package manager 提供 API 管理层的核心功能，包括：
//   - HTTP 服务的配置管理与生命周期管理（config.go, service.go）
//   - JWT 认证与令牌管理（authenticate.go）
//   - RBAC 授权策略管理（authorize.go）
//   - 安全中间件（secure.go）
//   - 统一响应与错误处理（本文件）
//   - 数据库、日志、国际化等基础设施初始化
//
// 本文件（response.go）定义了 API 统一响应与错误处理体系，包括：
//   - 错误码常量定义（基础错误码与业务错误码）
//   - CodeError 结构体及其方法，封装 HTTP 状态码、业务错误码和详情
//   - DetailContext 结构体，支持国际化消息模板的本地化
//   - LocalizedError 接口，用于可本地化的错误详情
//   - 多种错误构造工厂函数（CreateCodeError、CreateBadBodyError 等）
//   - 错误序列化方法（ToGinH、ToGinW），将错误转换为 JSON 响应
//   - 错误中断方法（AbortWithGinH、AbortWithGinW），直接中断请求并返回错误响应
package manager

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// 基础错误码常量
// 使用 iota 从 1000 开始自增，用于标识系统级别的通用错误。
// 错误码同时也作为国际化消息的键名（如 "code.1000"、"code.1001" 等），
// 在翻译文件中通过对应的键来查找本地化的错误摘要信息。
const (
	CodeUnknown                       = 1000 + iota // 1000: 未知错误，用于未预期的内部错误
	CodeUserNotExist                                // 1001: 用户不存在
	CodeBadBody                                     // 1002: 请求体格式错误（如 JSON 解析失败或字段验证不通过）
	CodeBadQuery                                    // 1003: 查询参数错误
	CodeBadUrlParam                                 // 1004: URL 路径参数错误
	CodeBadJwtToken                                 // 1005: JWT 令牌格式错误或无效
	CodeTimeoutJwtToken                             // 1006: JWT 令牌已过期
	CodeAuthorizeBlankFullPath                      // 1007: 授权时请求路径为空
	CodeAuthorizeFailed                             // 1008: 授权失败（权限不足）
	CodeNamespaceNotExist                           // 1009: 命名空间不存在
	CodeCannotDeleteCauseRelateRecord               // 1010: 因存在关联记录而无法删除
)

// 业务错误码常量
// 使用 iota 从 1100 开始自增，用于标识业务逻辑层面的特定错误。
// 与基础错误码区分，便于在日志和前端中快速定位业务问题。
const (
	CodeUsernamePasswordCheckFailed = 1100 + iota // 1100: 用户名或密码校验失败
	CodeOldPasswordCheckFailed                    // 1101: 旧密码校验失败
	CodeUsernameOrEmailAlreadyExist               // 1102: 用户名或邮箱已存在
	CodeTooManyAccessKey                          // 1103: 访问密钥数量超出限制
)

// 预定义的常用错误实例
// 这些实例在包初始化时创建，可在路由处理函数中直接使用，
// 避免重复创建相同的错误对象。
var (
	// SimpleOK 表示请求成功的响应，HTTP 状态码和业务码均为 200
	SimpleOK = CreateCodeError(http.StatusOK, http.StatusOK, nil)
	// ErrDefaultPageNotFound 表示请求的资源不存在（404）
	ErrDefaultPageNotFound = CreateCodeError(http.StatusNotFound, http.StatusNotFound, nil)
	// ErrDefaultNoMethod 表示请求的 HTTP 方法不被允许（405）
	ErrDefaultNoMethod = CreateCodeError(http.StatusMethodNotAllowed, http.StatusMethodNotAllowed, nil)
	// ErrCannotDeleteCauseRelateRecord 表示因存在关联记录而无法执行删除操作
	ErrCannotDeleteCauseRelateRecord = CreateCodeError(http.StatusBadRequest, CodeCannotDeleteCauseRelateRecord, nil)
)

// LocalizedError 定义可本地化错误的接口
// 实现该接口的错误详情能够根据请求的语言环境返回本地化的错误描述。
// 目前 DetailContext 是该接口的唯一实现，用于支持带参数的国际化消息模板。
type LocalizedError interface {
	Localize(localizer *i18n.Localizer) string
}

// CodeError 是 API 统一错误响应的核心结构体
// 封装了 HTTP 响应所需的全部信息，支持国际化错误摘要和详情。
// 通过 ToGinH / ToGinW 方法将错误信息序列化为 JSON 格式，
// 通过 AbortWithGinH / AbortWithGinW 方法直接中断 Gin 请求上下文并返回错误响应。
//
// 响应 JSON 格式示例：
//
//	{
//	  "code": 1000,           // 业务错误码
//	  "summary": "未知错误",   // 本地化的错误摘要
//	  "details": {...}        // 可选的错误详情
//	}
//
// ToGinW 在上述基础上额外包含 "data" 字段：
//
//	{
//	  "code": 200,
//	  "summary": "成功",
//	  "data": {...}           // 响应数据
//	}
type CodeError struct {
	// statusCode 是 HTTP 响应状态码（如 200、400、401、404、500 等）
	statusCode int
	// code 是业务错误码（如 CodeUnknown=1000、CodeBadBody=1002 等），
	// 用于前端根据错误码展示特定的错误提示
	code int
	// details 是错误详情，支持以下类型：
	//   - nil: 无详情
	//   - validator.ValidationErrors: 请求体字段验证错误，会翻译为字段名到错误消息的映射
	//   - LocalizedError: 可本地化的错误，调用其 Localize 方法获取翻译后的字符串
	//   - 其他类型: 原样输出到响应中
	details any
}

// DetailContext 是 LocalizedError 接口的具体实现
// 用于描述需要国际化的错误详情，支持消息模板和参数替换。
// 消息 ID 会在翻译文件中以 "error.{MsgID}" 的格式查找对应的翻译模板，
// 模板中使用 fmt.Sprintf 的占位符语法（如 %s、%d）来替换参数。
//
// 使用示例：
//
//	// 在翻译文件中定义：error.pageSize = "page size must be less than %d"
//	// 构造错误：
//	DetailContext{MsgID: "pageSize", args: []any{100}}
//	// 本地化后输出："page size must be less than 100"
type DetailContext struct {
	// MsgID 是国际化消息的标识符，对应翻译文件中 "error.{MsgID}" 键
	MsgID string
	// args 是消息模板的格式化参数，用于替换模板中的占位符
	args []any
}

// Localize 根据 Localizer 将 DetailContext 本地化为翻译后的字符串
// 先从翻译文件中查找 "error.{MsgID}" 对应的消息模板，再使用 args 参数格式化模板。
// 如果翻译文件中不存在对应的消息 ID，MustLocalizeMessage 会 panic。
func (d *DetailContext) Localize(localizer *i18n.Localizer) string {
	template := localizer.MustLocalizeMessage(&i18n.Message{
		ID: fmt.Sprintf("error.%s", d.MsgID),
	})
	return fmt.Sprintf(template, d.args...)
}

// summaryLocalize 根据 Localizer 将错误码本地化为翻译后的摘要字符串
// 从翻译文件中查找 "code.{code}" 键对应的翻译消息。
// 例如错误码 1000 对应翻译文件中的 "code.1000" 键。
// 如果翻译文件中不存在对应的键，MustLocalizeMessage 会 panic。
func summaryLocalize(localizer *i18n.Localizer, code int) string {
	return localizer.MustLocalizeMessage(&i18n.Message{
		ID: fmt.Sprintf("code.%d", code),
	})
}

// validationErrorsTranslate 将 validator.ValidationErrors 翻译为字段名到错误消息的映射
// 遍历所有验证错误，使用给定的翻译器将每个错误翻译为目标语言的错误消息。
// 返回的 map 中，键为验证失败的字段名，值为翻译后的错误消息。
//
// 参数：
//   - translator: 验证器翻译器，由 go-playground/universal-translator 提供
//   - errs: 请求体验证失败的错误集合
//
// 返回值：
//   - map[string]string: 字段名到翻译后错误消息的映射，例如 {"Name": "Name为必填字段", "Age": "Age必须大于18"}
func validationErrorsTranslate(translator ut.Translator, errs validator.ValidationErrors) map[string]string {
	results := make(map[string]string, len(errs))
	for _, err := range errs {
		results[err.Field()] = err.Translate(translator)
	}
	return results
}

// CreateCodeError 创建一个新的 CodeError 实例
// 这是所有错误构造的基础工厂函数，其他 Create* 函数均基于此实现。
//
// 参数：
//   - statusCode: HTTP 响应状态码（如 http.StatusBadRequest、http.StatusUnauthorized 等）
//   - code: 业务错误码（如 CodeUnknown、CodeBadBody 等）
//   - details: 错误详情，可以为 nil；支持 validator.ValidationErrors、LocalizedError 或其他任意类型
//
// 返回值：
//   - *CodeError: 新创建的错误实例
func CreateCodeError(statusCode, code int, details any) *CodeError {
	return &CodeError{
		statusCode: statusCode,
		code:       code,
		details:    details,
	}
}

// CreateBadBodyError 创建一个请求体验证失败的错误
// 仅接受 validator.ValidationErrors 类型的错误，用于处理请求体 JSON 绑定后的字段验证失败场景。
// 如果传入的 error 不是 validator.ValidationErrors 类型，会触发 panic。
// 该函数通常在 ShouldBindBodyWithJSON 返回错误后调用。
//
// 参数：
//   - err: 验证错误，必须为 validator.ValidationErrors 类型
//
// 返回值：
//   - *CodeError: 包含验证错误详情的 CodeError，HTTP 状态码为 400，业务码为 CodeBadBody
func CreateBadBodyError(err error) *CodeError {
	if vErr, ok := err.(validator.ValidationErrors); ok {
		return CreateCodeError(http.StatusBadRequest, CodeBadBody, vErr)
	} else {
		panic("error is not validator.ValidationErrors type")
	}
}

// CreateBadQueryError 创建一个查询参数错误的 CodeError
// 用于处理 URL 查询参数（?key=value）的验证失败场景。
// 错误详情使用 DetailContext 包装，支持国际化消息模板。
//
// 参数：
//   - msgID: 国际化消息 ID，对应翻译文件中 "error.{msgID}" 键
//   - args: 消息模板的格式化参数
//
// 返回值：
//   - *CodeError: HTTP 状态码为 400，业务码为 CodeBadQuery 的错误实例
func CreateBadQueryError(msgID string, args ...any) *CodeError {
	return CreateCodeError(http.StatusBadRequest, CodeBadQuery, &DetailContext{
		MsgID: msgID,
		args:  args,
	})
}

// CreateBadUrlParamError 创建一个 URL 路径参数错误的 CodeError
// 用于处理 URL 路径参数（如 /api/user/:id 中的 :id）的验证失败场景。
// 错误详情使用 DetailContext 包装，支持国际化消息模板。
//
// 参数：
//   - msgID: 国际化消息 ID，对应翻译文件中 "error.{msgID}" 键
//   - args: 消息模板的格式化参数
//
// 返回值：
//   - *CodeError: HTTP 状态码为 400，业务码为 CodeBadUrlParam 的错误实例
func CreateBadUrlParamError(msgID string, args ...any) *CodeError {
	return CreateCodeError(http.StatusBadRequest, CodeBadUrlParam, &DetailContext{
		MsgID: msgID,
		args:  args,
	})
}

// CreateAccidentError 创建一个意外内部错误的 CodeError
// 用于处理不可预期的运行时错误（如数据库连接失败、文件读写异常等）。
// 错误的 HTTP 状态码固定为 500，业务码固定为 CodeUnknown，详情为原始 error 对象。
//
// 参数：
//   - err: 原始运行时错误
//
// 返回值：
//   - *CodeError: HTTP 状态码为 500，业务码为 CodeUnknown 的错误实例
func CreateAccidentError(err error) *CodeError {
	return CreateCodeError(http.StatusInternalServerError, CodeUnknown, err)
}

// Summary 返回错误码对应的本地化摘要信息
// 通过 i18n Localizer 从翻译文件中查找 "code.{code}" 键对应的翻译消息。
//
// 参数：
//   - localizer: i18n 本地化器，由请求上下文提供
//
// 返回值：
//   - string: 翻译后的错误摘要文本
func (c *CodeError) Summary(localizer *i18n.Localizer) string {
	return summaryLocalize(localizer, c.code)
}

// ToGinH 将 CodeError 转换为 Gin 响应的 gin.H（即 map[string]any）格式
// 根据错误详情的类型，生成不同结构的 JSON 响应：
//   - details 为 nil: 返回 {"code": ..., "summary": "..."}
//   - details 为 validator.ValidationErrors: 返回 {"code": ..., "summary": "...", "details": {"Field": "翻译后的消息"}}
//   - details 为 LocalizedError: 返回 {"code": ..., "summary": "...", "details": "本地化后的字符串"}
//   - details 为其他类型: 返回 {"code": ..., "summary": "...", "details": 原始值}
//
// 参数：
//   - localizer: i18n 本地化器，用于翻译错误摘要和 LocalizedError 详情
//   - translator: 验证器翻译器，用于翻译 ValidationErrors 详情
//
// 返回值：
//   - gin.H: 可直接作为 JSON 响应的 map
func (c *CodeError) ToGinH(localizer *i18n.Localizer, translator ut.Translator) gin.H {
	summary := c.Summary(localizer)
	if c.details == nil {
		return gin.H{
			"code":    c.code,
			"summary": summary,
		}
	}
	switch v := c.details.(type) {
	case validator.ValidationErrors:
		return gin.H{
			"code":    c.code,
			"summary": summary,
			"details": validationErrorsTranslate(translator, v),
		}
	case LocalizedError:
		return gin.H{
			"code":    c.code,
			"summary": summary,
			"details": v.Localize(localizer),
		}
	default:
		return gin.H{
			"code":    c.code,
			"summary": summary,
			"details": v,
		}
	}
}

// ToGinW 将 CodeError 转换为包含数据字段的 Gin 响应格式
// 在 ToGinH 的基础上额外添加 "data" 字段，通常用于成功响应时携带业务数据。
// "W" 代表 "Wrapper"，即对基础响应的包装。
//
// 响应格式示例：
//
//	{
//	  "code": 200,
//	  "summary": "成功",
//	  "data": {"id": 1, "name": "test"}
//	}
//
// 参数：
//   - localizer: i18n 本地化器
//   - translator: 验证器翻译器
//   - data: 需要包含在响应中的业务数据
//
// 返回值：
//   - gin.H: 包含 code、summary 和 data 字段的 map
func (c *CodeError) ToGinW(localizer *i18n.Localizer, translator ut.Translator, data any) gin.H {
	wrapper := c.ToGinH(localizer, translator)
	wrapper["data"] = data
	return wrapper
}

// AbortWithGinH 中断当前请求并返回错误 JSON 响应
// 调用 Gin 的 AbortWithStatusJSON 方法，设置 HTTP 状态码并写入 JSON 响应体，
// 同时阻止后续的处理函数执行。
// 自动从 Gin 上下文中提取 Localizer 和 Translator 来完成国际化翻译。
//
// 典型用法：
//
//	if err != nil {
//	    CreateAccidentError(err).AbortWithGinH(c)
//	    return
//	}
//
// 参数：
//   - ctx: Gin 请求上下文，从中获取 Localizer 和 Translator
func (c *CodeError) AbortWithGinH(ctx *gin.Context) {
	ctx.AbortWithStatusJSON(
		c.statusCode,
		c.ToGinH(GetLocalizer(ctx), GetTranslator(ctx)),
	)
}

// AbortWithGinW 中断当前请求并返回包含数据的 JSON 响应
// 与 AbortWithGinH 类似，但响应中额外包含 "data" 字段。
// 通常用于成功响应场景（如 SimpleOK），在返回成功状态的同时携带业务数据。
//
// 典型用法：
//
//	SimpleOK.AbortWithGinW(c, userData)
//
// 参数：
//   - ctx: Gin 请求上下文，从中获取 Localizer 和 Translator
//   - data: 需要包含在响应中的业务数据
func (c *CodeError) AbortWithGinW(ctx *gin.Context, data any) {
	ctx.AbortWithStatusJSON(
		c.statusCode,
		c.ToGinW(GetLocalizer(ctx), GetTranslator(ctx), data),
	)
}
