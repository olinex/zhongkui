package periodicreport

import (
	// 标准库
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	// 项目内部
	authpkg "zhongkuisec.com/zhongkui/internal/pkg/auth"
)

// TestAPIInvoker_SignsRequest 验证调用器发出的请求携带可通过验签的 X-Access-Key，
// 方法、路径、请求体与签名原文一致，2xx 响应体原样返回。
func TestAPIInvoker_SignsRequest(t *testing.T) {
	const (
		accessKeyID = "test-ak"
		secret      = "test-secret"
		path        = "/api/v1/reports/query"
	)
	var (
		gotMethod, gotPath, gotBody string
		signErr                     error
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		raw := r.Header.Get(authpkg.AccessKeyHeaderName)
		token, err := authpkg.ParseAccessKeyToken(raw)
		if err != nil {
			signErr = err
		} else {
			var ok bool
			ok, signErr = authpkg.VerifyAccessKeySignature(secret, token)
			if signErr == nil && !ok {
				signErr = fmt.Errorf("signature does not match")
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	invoker := newAPIInvoker(server.URL, accessKeyID, secret)
	got, err := invoker.Invoke(context.Background(), http.MethodPost, path, json.RawMessage(`{"region":"cn"}`))
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if signErr != nil {
		t.Fatalf("request signature verification error = %v", signErr)
	}
	if gotMethod != http.MethodPost || gotPath != path || gotBody != `{"region":"cn"}` {
		t.Fatalf("request = %s %s body %q, want POST %s body %q", gotMethod, gotPath, gotBody, path, `{"region":"cn"}`)
	}
	if got != `{"ok":true}` {
		t.Fatalf("Invoke result = %q, want the response body", got)
	}
}

// TestAPIInvoker_EmptyBodyUsesEmptyJSON 验证未提供参数时以空 JSON 对象作为请求体。
func TestAPIInvoker_EmptyBodyUsesEmptyJSON(t *testing.T) {
	var gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	invoker := newAPIInvoker(server.URL, "test-ak", "test-secret")
	got, err := invoker.Invoke(context.Background(), http.MethodGet, "/api/v1/empty", nil)
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if gotBody != "{}" {
		t.Fatalf("request body = %q, want {}", gotBody)
	}
	if got != "{}" {
		t.Fatalf("Invoke result = %q, want {}", got)
	}
}

// TestAPIInvoker_Non2xxReturnsStatusAndBody 验证非 2xx 响应转换为包含状态码与响应体的错误。
func TestAPIInvoker_Non2xxReturnsStatusAndBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid query"}`))
	}))
	defer server.Close()

	invoker := newAPIInvoker(server.URL, "test-ak", "test-secret")
	got, err := invoker.Invoke(context.Background(), http.MethodPost, "/api/v1/bad", json.RawMessage(`{}`))
	if err == nil {
		t.Fatalf("Invoke error = nil, want a non-2xx error")
	}
	if got != "" {
		t.Fatalf("Invoke result = %q, want empty on error", got)
	}
	if !strings.Contains(err.Error(), "HTTP 400") || !strings.Contains(err.Error(), "invalid query") {
		t.Fatalf("Invoke error = %q, want status code and response body", err)
	}
}

// TestAPIInvoker_TruncatesOversizedResponse 验证超过 1 MiB 的响应被截断并追加提示，且仍为合法 UTF-8。
func TestAPIInvoker_TruncatesOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("报", apiInvokeMaxResponseBytes)))
	}))
	defer server.Close()

	invoker := newAPIInvoker(server.URL, "test-ak", "test-secret")
	got, err := invoker.Invoke(context.Background(), http.MethodGet, "/api/v1/large", nil)
	if err != nil {
		t.Fatalf("Invoke error = %v", err)
	}
	if !strings.HasSuffix(got, apiInvokeTruncatedNotice) {
		t.Fatalf("Invoke result does not end with the truncation notice")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("truncated result is not valid UTF-8")
	}
	if len(got) > apiInvokeMaxResponseBytes+len(apiInvokeTruncatedNotice)+3 {
		t.Fatalf("truncated result length = %d, want at most %d", len(got), apiInvokeMaxResponseBytes+len(apiInvokeTruncatedNotice)+3)
	}
}

// TestAPIInvoker_DisablesRedirect 验证重定向不会被跟随，避免签名凭据被转发到其他地址。
func TestAPIInvoker_DisablesRedirect(t *testing.T) {
	var followed bool
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		followed = true
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer other.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/api/v1/redirected", http.StatusFound)
	}))
	defer server.Close()

	invoker := newAPIInvoker(server.URL, "test-ak", "test-secret")
	_, err := invoker.Invoke(context.Background(), http.MethodGet, "/api/v1/redirect", nil)
	if err == nil {
		t.Fatalf("Invoke error = nil, want a non-2xx error for the redirect response")
	}
	if followed {
		t.Fatalf("redirect was followed, want it rejected")
	}
}
