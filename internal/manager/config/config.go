// Package config 定义 API 管理层（`internal/manager`）的配置结构体体系。
//
// 该包独立于 `internal/manager` 根包，供根包与 `internal/manager/middlewares`
// 共同引用，避免根包（router.go）→ middlewares → 根包 的导入环。
// 涵盖服务监听地址、命名空间、认证、安全策略等配置项，
// 并提供配置提取、描述生成及 Gin 中间件注入等功能。
package config

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
	"zhongkuisec.com/zhongkui/internal/pkg/config"
	"zhongkuisec.com/zhongkui/internal/pkg/crypto/key"
)

// PREFIX 是 API 管理层配置在 Viper 中的键前缀，
// 所有 API 管理层的配置项均位于 "api" 前缀之下。
// 例如：api.host、api.port、api.auth.secretKey 等。
const (
	Prefix = "api"
	// CryptoKeyOffset 是派生子密钥（CryptoKey）时使用的偏移量。
	CryptoKeyOffset = 9820
	// CryptoKeyLength 是派生子密钥（CryptoKey）的字节长度（对应 AES-256）。
	CryptoKeyLength = 32
)

// ConfigCtxName 是配置对象在 Gin 上下文中存储时使用的键名，
// 用于 ConfigMiddleware 注入和 GetConfig 提取配置。
const ConfigCtxName = "config"

// CookieConfig 定义了 JWT 令牌在 Cookie 中的存储配置。
//
// 该结构体控制浏览器如何存储和发送 JWT 令牌的 Cookie，
// 通过 Secure 和 HttpOnly 字段提供基本的安全防护。
type CookieConfig struct {
	// Name 是 Cookie 的名称，客户端和服务端通过此名称识别 JWT 令牌 Cookie
	Name string `mapstructure:"name" default:"jwtoken" comment:"Cookie 名称，用于识别 JWT 令牌 Cookie"`
	// Domain 是 Cookie 的作用域名，限制 Cookie 只在指定域名下发送
	Domain string `mapstructure:"domain" default:"localhost" comment:"Cookie 作用域名，限制 Cookie 仅在指定域名下发送"`
	// Path 是 Cookie 的作用路径，限制 Cookie 只在指定路径下发送
	Path string `mapstructure:"path" default:"/" comment:"Cookie 作用路径，限制 Cookie 仅在指定路径下发送"`
	// Secure 控制是否仅通过 HTTPS 协议发送 Cookie，
	// 开启后 HTTP 请求不会携带此 Cookie，防止令牌在未加密连接中泄露
	Secure bool `mapstructure:"secure" default:"false" comment:"是否仅通过 HTTPS 发送 Cookie，防止令牌在未加密连接中泄露"`
	// HttpOnly 控制是否禁止 JavaScript 访问此 Cookie，
	// 开启后可有效防止 XSS 攻击窃取令牌
	HttpOnly bool `mapstructure:"httpOnly" default:"false" comment:"是否禁止 JavaScript 访问 Cookie，防止 XSS 攻击窃取令牌"`
}

// AuthConfig 定义了 JWT 认证相关的完整配置。
//
// 该结构体涵盖了 JWT 令牌的签发、验证、传输和刷新等所有认证环节的配置，
// 是整个认证体系的核心配置入口。由 NewJwtAuthenticator 使用来创建认证器实例。
type AuthConfig struct {
	// Issuer 是 JWT 令牌的签发者标识（iss 声明），
	// 用于在多服务环境中区分令牌来源，验证时会校验此值
	Issuer string `mapstructure:"issuer" default:"zhongkui" comment:"JWT 签发者标识（iss 声明），用于区分令牌来源"`
	// Subject 是 JWT 令牌的主题标识（sub 声明），
	// 标识令牌的用途，当前默认为 "api" 表示 API 访问令牌
	Subject string `mapstructure:"subject" default:"api" comment:"JWT 主题标识（sub 声明），标识令牌用途"`
	// Audience 是 JWT 令牌的目标受众列表（aud 声明），
	// 限定令牌的接收方，验证时会校验请求方是否在受众列表中
	Audience []string `mapstructure:"audience" default:"[\"web\"]" comment:"JWT 目标受众列表（aud 声明），限定令牌接收方"`
	// TokenPrefix 是令牌在传输时携带的前缀标识，
	// 类似于 HTTP Authorization 头的 "Bearer " 方案，用于从原始令牌值中区分出 JWT
	TokenPrefix string `mapstructure:"tokenPrefix" default:"Bearer:" comment:"令牌传输前缀，类似 Authorization 头的 Bearer 方案"`
	// TokenTimeout 是 JWT 令牌的有效时长，
	// 令牌在此时间后过期（exp 声明），需要重新签发或刷新
	TokenTimeout time.Duration `mapstructure:"tokenTimeout" default:"1h" comment:"JWT 令牌有效时长，过期后需重新签发或刷新"`
	// TokenRefreshTimeout 是 JWT 令牌的刷新超时时间，
	// 令牌在此时间后无法再刷新，必须重新认证登录
	TokenRefreshTimeout time.Duration `mapstructure:"tokenRefreshTimeout" default:"24h" comment:"JWT 令牌刷新超时时间，超时后必须重新登录"`
	// HeaderName 是携带 JWT 令牌的 HTTP 请求头名称，
	// 客户端需要在此请求头中传递令牌以完成身份认证
	HeaderName string `mapstructure:"headerName" default:"X-Jwtoken" comment:"携带 JWT 令牌的 HTTP 请求头名称"`
	// Cookie 是 JWT 令牌的 Cookie 存储配置，
	// 控制令牌如何以 Cookie 形式存储在客户端浏览器中
	Cookie CookieConfig `mapstructure:"cookie" comment:"JWT 令牌的 Cookie 存储配置"`
	// AccessKeyCount 是每个用户允许创建的访问密钥（AccessKey）最大数量，
	// 用于限制用户创建的长期凭证数量
	AccessKeyCount uint `mapstructure:"accessKeyCount" default:"3" comment:"每个用户允许创建的访问密钥（AccessKey）最大数量"`
}

// ExtractCryptoKey 从密钥派生器派生出 API 管理层专属的子密钥，
// 用于对敏感配置（如 JWT 密钥）进行加解密。
func (a *AuthConfig) ExtractCryptoKey(kd *key.KeyDerivation) ([]byte, error) {
	return kd.DeriveChildKey(CryptoKeyOffset, CryptoKeyLength)
}

// ToJwtRegisterClaims 将认证配置转换为 JWT 标准注册声明（Registered Claims）。
//
// 该方法基于当前认证配置生成符合 JWT RFC 7519 规范的标准声明集合，
// 包括签发者、主题、受众、过期时间、生效时间和签发时间。
// 过期时间基于 TokenTimeout 计算，生效时间和签发时间均设为当前时刻。
//
// 返回值：
//   - jwt.RegisteredClaims: 可直接用于 jwt.Token 签名的标准声明
func (a *AuthConfig) ToJwtRegisterClaims() jwt.RegisteredClaims {
	now := time.Now()
	return jwt.RegisteredClaims{
		Issuer:    a.Issuer,
		Subject:   a.Subject,
		Audience:  a.Audience,
		ExpiresAt: jwt.NewNumericDate(now.Add(a.TokenTimeout)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
	}
}

// NamespaceConfig 定义了多命名空间（多租户）的识别配置。
//
// 在多租户架构中，请求需要携带命名空间标识以区分不同的租户环境。
// 该结构体定义了从 HTTP 请求头、查询参数和 Cookie 三种途径获取命名空间标识的键名，
// 中间件会按优先级依次检查这些途径来解析当前请求所属的命名空间。
type NamespaceConfig struct {
	// HeaderName 是携带命名空间标识的 HTTP 请求头名称
	HeaderName string `mapstructure:"headerName" default:"X-Namespace" comment:"携带命名空间标识的 HTTP 请求头名称"`
	// QueryName 是携带命名空间标识的 URL 查询参数名称
	QueryName string `mapstructure:"queryName" default:"namespace" comment:"携带命名空间标识的 URL 查询参数名称"`
	// CookieName 是携带命名空间标识的 Cookie 名称
	CookieName string `mapstructure:"namespace" default:"namespace" comment:"携带命名空间标识的 Cookie 名称"`
}

// CORSConfig 定义了跨域资源共享（CORS）策略配置。
//
// CORS 策略控制浏览器如何处理跨源请求，
// 是前后端分离架构中不可或缺的安全配置项。
type CORSConfig struct {
	// AllowOrigins 是允许的跨域请求来源地址列表，
	// 只有来自列表内地址的跨域请求才会被接受，支持使用 * 允许所有来源（不推荐生产环境使用）
	AllowOrigins []string `mapstructure:"allowOrigins" default:"[\"http://127.0.0.1:8000\", \"http://localhost:8000\"]" comment:"允许的跨域请求来源地址列表，* 表示允许所有来源（不推荐生产使用）"`
	// AllowCredentials 控制是否允许跨域请求携带凭证（Cookie、HTTP 认证等），
	// 开启后浏览器会在跨域请求中包含认证信息
	AllowCredentials bool `mapstructure:"allowCredentials" default:"true" comment:"是否允许跨域请求携带凭证（Cookie、HTTP 认证等）"`
	// AllowHeaders 是允许的跨域请求头列表，
	// 客户端在预检请求（OPTIONS）中声明的自定义请求头必须在此列表中
	AllowHeaders []string `mapstructure:"allowHeaders" default:"[\"Content-Type\", \"Content-Length\", \"Accept\", \"Accept-Encoding\", \"X-CSRF-Token\", \"Authorization\", \"Cache-Control\", \"X-Jwtoken\", \"X-Namespace\"]" comment:"允许的跨域请求头列表，需在预检请求中声明"`
	// AllowMethods 是允许的跨域 HTTP 方法列表，
	// 不在列表中的方法将被浏览器拒绝
	AllowMethods []string `mapstructure:"allowMethods" default:"[\"GET\", \"POST\", \"PUT\", \"DELETE\", \"OPTIONS\"]" comment:"允许的跨域 HTTP 方法列表"`
}

// SecureConfig 定义了 HTTP 安全响应头的完整配置。
//
// 该结构体涵盖了 OWASP 推荐的所有重要安全响应头，
// 通过设置这些响应头可以有效防御点击劫持、XSS、代码注入等常见 Web 攻击。
// 每个字段对应一个 HTTP 安全响应头，在生产环境中应根据实际需求调整。
type SecureConfig struct {
	// XFrameOptions 控制页面是否允许被嵌入到 iframe 中，
	// "DENY" 表示完全禁止嵌入，"SAMEORIGIN" 表示仅允许同源嵌入
	XFrameOptions string `mapstructure:"frameOptions" default:"DENY" comment:"是否允许页面嵌入 iframe，DENY 禁止，SAMEORIGIN 仅允许同源"`
	// ContentSecurityPolicy 定义内容安全策略（CSP），
	// 控制页面可以加载哪些来源的资源（脚本、样式、图片、字体等），
	// 是防御 XSS 和代码注入攻击的最有效手段之一
	ContentSecurityPolicy string `mapstructure:"contentSecurityPolicy" default:"default-src 'self'; connect-src *; font-src *; script-src-elem * 'unsafe-inline'; img-src * data:; style-src * 'unsafe-inline';" comment:"内容安全策略（CSP），控制可加载的资源来源，防御 XSS 与代码注入"`
	// XXSSProtection 控制浏览器内置的 XSS 过滤器行为，
	// "1; mode=block" 表示启用过滤并在检测到攻击时阻止页面渲染
	XXSSProtection string `mapstructure:"xssProtection" default:"1; mode=block" comment:"浏览器内置 XSS 过滤器行为，1; mode=block 表示检测到攻击时阻止渲染"`
	// StrictTransportSecurity 强制浏览器在指定时间内仅通过 HTTPS 访问此站点，
	// "max-age=31536000; includeSubDomains; preload" 表示缓存一年、包含子域名、加入 HSTS 预加载列表
	StrictTransportSecurity string `mapstructure:"strictTransportSecurity" default:"max-age=31536000; includeSubDomains; preload" comment:"强制浏览器仅通过 HTTPS 访问，HSTS 策略"`
	// ReferrerPolicy 控制 HTTP Referer 请求头的发送策略，
	// "strict-origin" 表示仅在跨域请求中发送来源的域名部分（不含路径和查询参数）
	ReferrerPolicy string `mapstructure:"referrerPolicy" default:"strict-origin" comment:"HTTP Referer 请求头的发送策略"`
	// XContentTypeOptions 防止浏览器对响应内容进行 MIME 类型嗅探，
	// "nosniff" 强制浏览器使用服务端声明的 Content-Type，防止将非执行文件误判为可执行文件
	XContentTypeOptions string `mapstructure:"contentTypeOptions" default:"nosniff" comment:"防止 MIME 类型嗅探，nosniff 强制使用声明的 Content-Type"`
	// PermissionsPolicy 控制浏览器功能（地理位置、摄像头、麦克风等）的使用权限，
	// 空括号 "()" 表示完全禁用该功能，"(self)" 表示仅允许同源使用
	PermissionsPolicy string `mapstructure:"permissionsPolicy" default:"geolocation=(),midi=(),sync-xhr=(),microphone=(),camera=(),magnetometer=(),gyroscope=(),fullscreen=(self),payment=()" comment:"控制浏览器功能权限（地理位置、摄像头、麦克风等）"`
	// CORS 是跨域资源共享策略配置，控制浏览器的跨域请求行为
	CORS CORSConfig `mapstructure:"cors" comment:"跨域资源共享（CORS）策略配置"`
}

// Config 是 API 管理层的顶层配置结构体，聚合了所有子模块的配置。
//
// 该结构体是 API 管理层配置体系的根入口，通过 Viper 从配置文件中
// 以 "api" 为前缀提取所有配置项。各子配置通过嵌套结构体组织，
// 支持配置文件的层级化结构。
type Config struct {
	// Host 是 HTTP 服务的监听地址，"0.0.0.0" 表示监听所有网络接口
	Host string `mapstructure:"host" default:"0.0.0.0" comment:"HTTP 服务监听地址，0.0.0.0 表示监听所有网络接口"`
	// Port 是 HTTP 服务的监听端口号
	Port int `mapstructure:"port" default:"8080" comment:"HTTP 服务监听端口号"`
	// FrontendBaseURL 是前端站点的基础地址（含协议与端口），项目级配置，
	// 用于构造外部可访问的前端链接（如密码重置链接）
	FrontendBaseURL string `mapstructure:"frontendBaseURL" default:"http://127.0.0.1:8000" comment:"前端站点基础地址（含协议与端口），项目级配置"`
	// Namespace 是多命名空间（多租户）识别配置
	Namespace NamespaceConfig `mapstructure:"namespace" comment:"多命名空间（多租户）识别配置"`
	// Auth 是 JWT 认证相关配置
	Auth AuthConfig `mapstructure:"auth" comment:"JWT 认证相关配置"`
	// Secure 是 HTTP 安全响应头配置
	Secure SecureConfig `mapstructure:"secure" comment:"HTTP 安全响应头配置"`
	// PasswordReset 是用户强制重置密码相关配置
	PasswordReset PasswordResetConfig `mapstructure:"passwordReset" comment:"密码重置配置"`
}

// PasswordResetConfig 定义了用户强制重置密码的配置（密码重置专属项）。
type PasswordResetConfig struct {
	// TokenTTL 是重置令牌的有效时长，过期后用户需重新发起申请
	TokenTTL time.Duration `mapstructure:"tokenTTL" default:"30m" comment:"重置密码令牌有效时长，过期后需重新申请"`
}

// ConfigMiddleware 创建一个将配置注入 Gin 上下文的中间件。
//
// 该中间件在每次请求处理前将 Config 对象存入 Gin 上下文中，
// 键名为 ConfigCtxName（"config"），后续处理器可通过 GetConfig 提取使用。
//
// 参数：
//   - cfg: 需要注入到上下文的配置对象指针
//
// 返回值：
//   - gin.HandlerFunc: 可直接注册到 Gin 路由的中间件函数
func ConfigMiddleware(cfg *Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ConfigCtxName, cfg)
		c.Next()
	}
}

// GetConfig 从 Gin 上下文中提取 API 管理层配置。
//
// 该函数从 Gin 上下文中读取由 ConfigMiddleware 注入的配置对象。
// 如果上下文中不存在配置对象（通常意味着缺少 ConfigMiddleware 注册），
// 将触发 panic，因为这属于程序初始化阶段的严重错误。
//
// 参数：
//   - c: Gin 请求上下文
//
// 返回值：
//   - *Config: 上下文中的配置对象指针
func GetConfig(c *gin.Context) *Config {
	if cfg, ok := c.Get(ConfigCtxName); ok {
		return cfg.(*Config)
	}
	panic("config does not exists")
}

// ExtractConfig 从 Viper 实例中提取并校验 API 管理层的配置。
//
// 该函数执行以下步骤：
//  1. 应用结构体默认值（default 标签）
//  2. 从 Viper 中提取 "api" 前缀下的子配置树
//  3. 将子配置树反序列化到 Config 结构体
//  4. 对 Config 结构体执行结构化校验
//
// 参数：
//   - v: 已加载配置的 Viper 实例
//
// 返回值：
//   - *Config: 成功时返回填充并校验通过的配置结构体指针
//   - error: 任一步骤失败时返回的错误
func ExtractConfig(v *viper.Viper, validate *validator.Validate) (*Config, error) {
	return config.ExtractConfig[Config](v, validate, Prefix)
}

// DescribeConfig 返回 API 管理层配置项的元描述列表。
func DescribeConfig() config.ConfigDescriptions {
	return config.DescribeConfig[Config](Prefix)
}
