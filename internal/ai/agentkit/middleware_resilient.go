package agentkit

import (
	"context"
	"io"
	"strings"
	"time"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"zhongkuisec.com/zhongkui/internal/pkg/errors"
)

const (
	// modelRetryAttempts 瞬时模型错误（限流）的最大重试次数
	modelRetryAttempts = 3
	// modelRetryBaseDelay 首次重试的等待时间，之后按指数退避
	modelRetryBaseDelay = 200 * time.Millisecond
)

// resilientMiddleware 为模型调用补上 eino ADK 对 agentic 模型尚未提供的两项能力：
//   - ctx 取消感知的流读取：取消后向消费者返回 ctx 错误，而不是静默挂起；
//   - 瞬时错误重试：429 / Too Many Requests / qpm limit 等限流错误按指数退避重试。
//
// chat 模型另有 eino 原生的 ModelRetryConfig，故本中间件对 chat 只做取消感知包装，避免两层
// 重试叠加；agentic 模型由本中间件承担重试。
type resilientMiddleware[M adk.MessageType] struct {
	*adk.TypedBaseChatModelAgentMiddleware[M]
	// retry 是否对瞬时错误重试
	retry bool
}

// newResilientMiddleware 构造韧性中间件；retry 为 true 时对瞬时错误重试。
func newResilientMiddleware[M adk.MessageType](retry bool) adk.TypedChatModelAgentMiddleware[M] {
	return &resilientMiddleware[M]{
		TypedBaseChatModelAgentMiddleware: &adk.TypedBaseChatModelAgentMiddleware[M]{},
		retry:                             retry,
	}
}

// WrapModel 用韧性包装替换底层模型。
func (m *resilientMiddleware[M]) WrapModel(_ context.Context, base model.BaseModel[M], _ *adk.TypedModelContext[M]) (model.BaseModel[M], error) {
	if base == nil {
		return nil, errors.New("agentkit: nil model in resilient middleware")
	}
	return &resilientModel[M]{inner: base, retry: m.retry}, nil
}

// resilientModel 是带取消感知与瞬时重试的模型包装。
type resilientModel[M adk.MessageType] struct {
	inner model.BaseModel[M]
	retry bool
}

// Generate 调用底层模型，瞬时错误按退避重试。
func (m *resilientModel[M]) Generate(ctx context.Context, input []M, opts ...model.Option) (M, error) {
	var zero M
	for attempt := 0; ; attempt++ {
		output, err := m.inner.Generate(ctx, input, opts...)
		if err == nil {
			return output, nil
		}
		if !m.shouldRetry(ctx, err, attempt) {
			return zero, errors.WithStack(err)
		}
		if err := sleepWithContext(ctx, modelRetryBackoff(attempt)); err != nil {
			return zero, errors.WithStack(err)
		}
	}
}

// Stream 调用底层模型的流式接口，瞬时错误在创建流阶段按退避重试；成功创建的流经取消感知包装。
func (m *resilientModel[M]) Stream(ctx context.Context, input []M, opts ...model.Option) (*schema.StreamReader[M], error) {
	for attempt := 0; ; attempt++ {
		stream, err := m.inner.Stream(ctx, input, opts...)
		if err == nil {
			return ctxAwareStream(ctx, stream), nil
		}
		if !m.shouldRetry(ctx, err, attempt) {
			return nil, errors.WithStack(err)
		}
		if err := sleepWithContext(ctx, modelRetryBackoff(attempt)); err != nil {
			return nil, errors.WithStack(err)
		}
	}
}

// shouldRetry 判断一次失败是否值得重试：仅限瞬时错误、未取消、且仍有重试额度。
func (m *resilientModel[M]) shouldRetry(ctx context.Context, err error, attempt int) bool {
	if !m.retry || attempt >= modelRetryAttempts {
		return false
	}
	if ctx.Err() != nil || errors.Is(err, adk.ErrStreamCanceled) {
		return false
	}
	if _, ok := compose.ExtractInterruptInfo(err); ok {
		return false
	}
	return isTransientModelError(err)
}

// isTransientModelError 识别限流类瞬时错误。
func isTransientModelError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "429") ||
		strings.Contains(message, "Too Many Requests") ||
		strings.Contains(message, "qpm limit")
}

// modelRetryBackoff 返回指数退避时长。
func modelRetryBackoff(attempt int) time.Duration {
	return modelRetryBaseDelay << uint(attempt)
}

// sleepWithContext 等待给定时间，ctx 取消时立即返回其错误。
func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// ctxAwareStream 包装底层模型流：每次读取前后检查 ctx，取消后向消费者返回 ctx 错误并关闭底层流。
func ctxAwareStream[M adk.MessageType](ctx context.Context, stream *schema.StreamReader[M]) *schema.StreamReader[M] {
	reader, writer := schema.Pipe[M](16)
	go func() {
		defer writer.Close()
		defer stream.Close()
		for {
			if err := ctx.Err(); err != nil {
				var zero M
				writer.Send(zero, err)
				return
			}
			chunk, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				return
			}
			if err != nil {
				var zero M
				writer.Send(zero, err)
				return
			}
			select {
			case <-ctx.Done():
				var zero M
				writer.Send(zero, ctx.Err())
				return
			default:
			}
			if closed := writer.Send(chunk, nil); closed {
				return
			}
		}
	}()
	return reader
}
