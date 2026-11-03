package tools

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type ToolDef struct {
	Name        string
	Description string
	Params      openai.FunctionParameters
	Fn          func(ctx context.Context, args map[string]any) (string, error)
}

// ToParam 生成 chat completions 格式的工具定义
func (t ToolDef) ToParam() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionToolUnionParam{
		OfFunction: &openai.ChatCompletionFunctionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        t.Name,
				Description: openai.String(t.Description),
				Parameters:  t.Params,
			},
		},
	}
}

func (t ToolDef) Call(ctx context.Context, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	return t.Fn(ctx, args)
}
