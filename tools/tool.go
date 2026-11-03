package tools

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// ToolDef 是所有工具的统一定义。
// 每个工具都有：名字、描述、参数 Schema、执行函数
type ToolDef struct {
	Name        string
	Description string
	Params      openai.FunctionParameters
	Fn          func(ctx context.Context, args map[string]any) (string, error)
}

// ToParam 把 ToolDef 转成 OpenAI API 需要的格式
func (t ToolDef) ToParam() responses.ToolUnionParam {
	return responses.ToolUnionParam{
		OfFunction: &responses.FunctionToolParam{
			Name:        t.Name,
			Description: openai.String(t.Description),
			Parameters:  t.Params,
		},
	}
}

// Call 把 LLM 返回的 JSON 字符串解析成 map，然后调用 Fn
func (t ToolDef) Call(ctx context.Context, argsJSON string) (string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", err
	}
	return t.Fn(ctx, args)
}
