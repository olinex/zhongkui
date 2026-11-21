package periodicreport

import (
	// 标准库
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	// 项目内部
	authpkg "zhongkuisec.com/zhongkui/internal/pkg/auth"
	"zhongkuisec.com/zhongkui/internal/pkg/base"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// apiInvokeTimeout 是单次内部 API 调用的整体超时。
	apiInvokeTimeout = 60 * time.Second
	// apiInvokeMaxResponseBytes 是单次调用读取响应体的上限（1 MiB），超出部分截断。
	apiInvokeMaxResponseBytes = 1 << 20
	// apiInvokeTruncatedNotice 是响应体被截断时追加的提示。
	apiInvokeTruncatedNotice = "\n…（结果已截断）"
)

// apiInvoker 代表周期报告运行中的属主身份，向 Web API 发起带访问密钥签名的无人值守调用。
// 密钥只保存在内存中，不写入日志或数据库；每次调用都按当前时间重新签名，避免令牌过期。
type apiInvoker struct {
	// baseURL 是 Web API 的内部基地址（含协议与端口）。
	baseURL string
	// accessKeyID 是签名使用的访问密钥 ID。
	accessKeyID string
	// secret 是签名使用的访问密钥明文，只在内存中持有。
	secret string
	// client 是内部 API 调用使用的 HTTP 客户端，超时且禁用重定向。
	client *http.Client
}

// newAPIInvoker 创建内部 API 调用器。客户端禁用重定向，防止签名请求头随重定向泄露到其他地址。
func newAPIInvoker(baseURL, accessKeyID, secret string) *apiInvoker {
	return &apiInvoker{
		baseURL:     strings.TrimRight(baseURL, "/"),
		accessKeyID: accessKeyID,
		secret:      secret,
		client: &http.Client{
			Timeout: apiInvokeTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Invoke 执行一次内部 API 调用并返回响应体。method、path、params 均为能力解析后的规范值；
// path 形如 /api/v1/...，签名与实际请求使用同一路径。非 2xx 响应转换为包含状态码与响应体的错误。
func (a *apiInvoker) Invoke(ctx context.Context, method, path string, params json.RawMessage) (string, error) {
	body := []byte(params)
	if len(body) == 0 {
		body = []byte("{}")
	}
	timestampMs := time.Now().UnixMilli()
	token, err := authpkg.SignHTTPAccessKey(a.secret, a.accessKeyID, method, path, timestampMs)
	if err != nil {
		return "", errors.WithStack(err)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return "", errors.WithStack(err)
	}
	req.Header.Set("Content-Type", base.MediaTypeJSON)
	req.Header.Set(authpkg.AccessKeyHeaderName, token)
	resp, err := a.client.Do(req)
	if err != nil {
		return "", errors.WithStack(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	text, readErr := readAPIResponse(resp.Body)
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if readErr != nil {
			return "", errors.Newf("HTTP %d", resp.StatusCode)
		}
		return "", errors.Newf("HTTP %d %s", resp.StatusCode, text)
	}
	if readErr != nil {
		return "", errors.WithStack(readErr)
	}
	if text == "" {
		return "{}", nil
	}
	return text, nil
}

// readAPIResponse 读取响应体并限制在 1 MiB 内；超限时按 UTF-8 边界截断并追加截断提示。
func readAPIResponse(body io.Reader) (string, error) {
	data, err := io.ReadAll(io.LimitReader(body, apiInvokeMaxResponseBytes+1))
	if err != nil {
		return "", errors.WithStack(err)
	}
	if len(data) > apiInvokeMaxResponseBytes {
		return truncateUTF8Bytes(string(data), apiInvokeMaxResponseBytes) + apiInvokeTruncatedNotice, nil
	}
	return string(data), nil
}
