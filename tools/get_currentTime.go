package tools

// 获取当前日期，用于默认日期填充

import (
	"context"
	"time"

	"github.com/openai/openai-go/v3"
)

func NewGetCurrentTime() *ToolDef {
	return &ToolDef{
		Name:        "get_current_time",
		Description: "获取本地当前的日期和时间",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"format": map[string]string{
					"type":        "string",
					"description": "时间格式，可选：datetime（默认）、date、time",
				},
			},
			"required": []string{},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			now := time.Now()
			format, _ := args["format"].(string)
			var layout string
			switch format {
			case "date":
				layout = "2006-01-02"
			case "time":
				layout = "15:04:05"
			default:
				layout = "2006-01-02 15:04:05"
			}
			return now.Format(layout), nil
		},
	}
}
