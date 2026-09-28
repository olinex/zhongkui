package message

import (
	// 标准库
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"time"

	// 项目内部
	"zhongkui.com/zhongkui/internal/pkg/errors"
	"zhongkui.com/zhongkui/internal/pkg/validator"
)

// webhookHTTPClient Webhook 渠道使用的 HTTP 客户端，带整体超时避免拖垮任务执行。
// 其 Transport 配置了连接池（MaxIdleConns/MaxIdleConnsPerHost/IdleConnTimeout），
// 使 HTTP 连接由标准库自动复用并在闲置超时后回收——即 webhook 渠道底层外部资源的
// 生命周期管理，由统一资源池之外的 net/http 运输层负责。webhook 通道自身无长期
// 服务端连接，因此不实现 resourcepool.Lifespan，按纯对象复用处理。
var webhookHTTPClient = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	},
	Timeout: 10 * time.Second,
}

// webhookChannel Webhook 渠道实现：以 HTTP POST 将消息 JSON 投递到目标配置的 URL。
// 渠道级配置（notification_channels.config）当前为空对象 {}（Webhook 地址在目标级配置中）；
// 目标级配置（notification_targets.config）结构：
//
//	{"url": "https://example.com/hook", "headers": {"X-Token": "secret"}}
type webhookChannel struct{}

// WebhookChannelConfig Webhook 渠道级配置：当前为空对象（Webhook 地址在目标级配置中）。
// 反射产出空字段列表，供前端渲染空渠道级配置表单。
type WebhookChannelConfig struct{}

// WebhookTarget Webhook 目标级配置。
type WebhookTarget struct {
	// URL 接收 Webhook 的地址，必填
	URL string `json:"url" binding:"required" form:"kind=string"`
	// Headers 附加请求头，可选
	Headers map[string]string `json:"headers" form:"kind=object"`
}

// Type 返回渠道类型键。
func (c *webhookChannel) Type() string { return TypeWebhook }

// Send 将消息以 JSON 形式 POST 到目标 Webhook URL。
// 2xx 视为成功；目标配置缺失 URL、非法 JSON 或非 2xx 响应均返回错误。
func (c *webhookChannel) Send(ctx context.Context, target *Target, msg *RenderedMessage) (string, error) {
	var cfg WebhookTarget
	if err := json.Unmarshal(target.Config, &cfg); err != nil {
		return "", errors.WithStack(err)
	}
	if err := validator.Validate.Struct(&cfg); err != nil {
		return "", errors.WithStack(err)
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", errors.WithStack(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(payload))
	if err != nil {
		return "", errors.WithStack(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range cfg.Headers {
		req.Header.Set(key, value)
	}
	resp, err := webhookHTTPClient.Do(req)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", errors.Newf("webhook: unexpected status %d: %s", resp.StatusCode, string(body))
	}
	// Webhook 渠道不提供消息 ID
	return "", nil
}

// init 注册 Webhook 渠道工厂并注册其配置结构体类型供表单元信息抽取；
// 渠道级配置暂不解析（空对象）。
func init() {
	RegisterChannel(
		TypeWebhook,
		func(_ json.RawMessage) (Channel, error) {
			return &webhookChannel{}, nil
		},
		reflect.TypeFor[WebhookChannelConfig](),
		reflect.TypeFor[WebhookTarget](),
	)
}
