package mail

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	"zhongkui.com/zhongkui/internal/pkg/logging"
	"zhongkui.com/zhongkui/internal/pkg/message"
)

// newTestLogger 构造一个无输出的 logger（Nop），避免测试打印日志噪音。
func newTestLogger() *logging.Logger {
	return &logging.Logger{SugaredLogger: zap.NewNop().Sugar()}
}

// TestSender_Send_Disabled 测试未启用时 Send 返回"disabled"错误。
func TestSender_Send_Disabled(t *testing.T) {
	s := NewSender(&Config{Enabled: false}, newTestLogger(), message.NewPool())
	err := s.Send(context.Background(), "ops@example.com", message.Message{Template: "test"}, "")
	if err == nil {
		t.Fatalf("Send() error = nil, want a disabled error")
	}
	if !strings.Contains(err.Error(), "disabled") {
		t.Errorf("Send() error = %v, want containing %q", err, "disabled")
	}
}

// TestSender_Send_NotConfigured 测试启用但缺少 host/from 时 Send 返回"not configured"错误。
func TestSender_Send_NotConfigured(t *testing.T) {
	tests := []struct {
		name string
		cfg  *Config
	}{
		{name: "missing host and from", cfg: &Config{Enabled: true}},
		{name: "missing host", cfg: &Config{Enabled: true, From: "alert@example.com"}},
		{name: "missing from", cfg: &Config{Enabled: true, Host: "smtp.example.com"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewSender(tt.cfg, newTestLogger(), message.NewPool())
			err := s.Send(context.Background(), "ops@example.com", message.Message{Template: "test"}, "")
			if err == nil {
				t.Fatalf("Send() error = nil, want a not-configured error")
			}
			if !strings.Contains(err.Error(), "not configured") {
				t.Errorf("Send() error = %v, want containing %q", err, "not configured")
			}
		})
	}
}
