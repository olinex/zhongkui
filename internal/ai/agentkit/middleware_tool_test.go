package agentkit

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	modelsai "zhongkuisec.com/zhongkui/internal/models/ai"
	"zhongkuisec.com/zhongkui/internal/pkg/errors"
	"zhongkuisec.com/zhongkui/internal/pkg/llm"
)

func TestToolExecMiddlewareConvertsErrorsAndTruncates(t *testing.T) {
	middleware := newToolExecMiddleware[*schema.Message](10)
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(),
		func(_ context.Context, _ string, _ ...tool.Option) (string, error) {
			return "abcdefghijklmnop", nil
		}, nil)
	if err != nil {
		t.Fatalf("wrap error = %v", err)
	}
	result, err := wrapped(context.Background(), "{}")
	if err != nil {
		t.Fatalf("wrapped call error = %v", err)
	}
	if result != "abcdefghij..." {
		t.Fatalf("truncated result = %q", result)
	}

	failing, err := middleware.WrapInvokableToolCall(context.Background(),
		func(_ context.Context, _ string, _ ...tool.Option) (string, error) {
			return "", errors.New("boom")
		}, nil)
	if err != nil {
		t.Fatalf("wrap error = %v", err)
	}
	result, err = failing(context.Background(), "{}")
	if err != nil {
		t.Fatalf("error should be converted to text, got %v", err)
	}
	if result != "tool execution failed: boom" {
		t.Fatalf("error text = %q", result)
	}
}

func TestToolExecMiddlewarePassesInterruptThrough(t *testing.T) {
	middleware := newToolExecMiddleware[*schema.Message](0)
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(),
		func(ctx context.Context, _ string, _ ...tool.Option) (string, error) {
			return "", tool.StatefulInterrupt(ctx, "needs approval", "state")
		}, nil)
	if err != nil {
		t.Fatalf("wrap error = %v", err)
	}
	_, err = wrapped(context.Background(), "{}")
	if err == nil {
		t.Fatal("interrupt must not be converted into a text result")
	}
	if _, ok := compose.IsInterruptRerunError(err); !ok {
		t.Fatalf("error is not an interrupt: %v", err)
	}
}

// loopingChat 始终返回一次工具调用，用于验证 RunOptions 的工具轮数上界。
type loopingChat struct{}

// Generate 是 loopingChat 的测试桩，始终返回一次工具调用。
func (loopingChat) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return loopingMessage(), nil
}

// Stream 是 loopingChat 的测试桩，以单元素流返回同一工具调用。
func (loopingChat) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{loopingMessage()}), nil
}

func loopingMessage() *schema.Message {
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:       "loop",
			Function: schema.FunctionCall{Name: "add", Arguments: `{"a":1,"b":2}`},
		}},
	}
}

// levelChat 是审核评估用模型：始终把调用判为 none，使 MaxIterations 测试不被审批中断。
type levelChat struct{}

// Generate 是 levelChat 的测试桩，始终返回 none 级审核结果。
func (levelChat) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: `{"levels":[{"callID":"loop","level":"none"}]}`}, nil
}

// Stream 是 levelChat 的测试桩，以单元素流返回同一审核结果。
func (levelChat) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{mustLevelMessage()}), nil
}

func mustLevelMessage() *schema.Message {
	return &schema.Message{Role: schema.Assistant, Content: `{"levels":[{"callID":"loop","level":"none"}]}`}
}

func TestRuntimeEnforcesMaxToolRounds(t *testing.T) {
	rt, err := New(context.Background(), Deps{
		Models: ModelSet{
			Chat: map[modelsai.Slot]model.BaseModel[*schema.Message]{
				modelsai.SlotStandard: loopingChat{},
			},
			Kinds: map[modelsai.Slot]llm.APIMode{
				modelsai.SlotStandard: llm.APIModeChat,
			},
		},
		Params:       testParams(map[string]string{"agent.maxToolRounds": "1"}),
		Tools:        []tool.BaseTool{addTool{}},
		ChatResolver: func(modelsai.Slot) (model.BaseChatModel, error) { return levelChat{}, nil },
		Checkpoints:  newMemCheckpointStore(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	handle, ok := rt.ChatHandle(modelsai.SlotStandard)
	if !ok {
		t.Fatal("standard chat handle is missing")
	}
	iterator := handle.Runner.Run(context.Background(), []*schema.Message{schema.UserMessage("loop")}, rt.RunOptions()...)
	var runErr error
	for {
		event, ok := iterator.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			runErr = event.Err
		}
	}
	if !errors.Is(runErr, adk.ErrExceedMaxIterations) {
		t.Fatalf("run error = %v, want ErrExceedMaxIterations", runErr)
	}
}
