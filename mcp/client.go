package mcp

// 负责连接外部 MCP Server 并包装工具，同时按组进行命名与分类
import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"finance-agent/tools"

	mcpSDK "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServerConfig 定义外部 MCP Server 的启动参数
type MCPServerConfig struct {
	Group   string            // 所属工具组（"finance" / "notify"）
	Command string            // 命令（"zenmoney-mcp" / "npx"）
	Args    []string          // 参数
	Env     map[string]string // 环境变量
}

// ConnectMCP 连接外部 MCP Server，拉取并包装其工具
func ConnectMCP(ctx context.Context, cfg MCPServerConfig) (map[string]*tools.ToolDef, *mcpSDK.ClientSession, error) {
	client := mcpSDK.NewClient(&mcpSDK.Implementation{Name: "finance-agent", Version: "v1.0.0"}, nil)

	cmd := exec.Command(cfg.Command, cfg.Args...)

	cmd.Env = os.Environ()

	for k, v := range cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	transport := &mcpSDK.CommandTransport{Command: cmd}

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, nil, err
	}

	list, err := session.ListTools(ctx, &mcpSDK.ListToolsParams{})
	if err != nil {
		session.Close()
		return nil, nil, err
	}

	toolMap := make(map[string]*tools.ToolDef, len(list.Tools))
	for _, t := range list.Tools {
		wrapped := wrapMCPTool(t, session, cfg.Group)
		toolMap[wrapped.Name] = wrapped
	}
	return toolMap, session, nil
}

// wrapMCPTool 把 MCP 工具包装为本地 ToolDef，并自动带上命名空间前缀
func wrapMCPTool(mcpTool *mcpSDK.Tool, session *mcpSDK.ClientSession, group string) *tools.ToolDef {
	return &tools.ToolDef{
		Name:        fmt.Sprintf("%s__%s", group, mcpTool.Name), // 加上组名前缀，如 zen__analyze_spending
		Description: mcpTool.Description,
		Params:      mcpTool.InputSchema.(map[string]any),
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			res, err := session.CallTool(ctx, &mcpSDK.CallToolParams{
				Name:      mcpTool.Name,
				Arguments: args,
			})
			if err != nil {
				return "", err
			}
			var out string
			for _, c := range res.Content {
				if text, ok := c.(*mcpSDK.TextContent); ok {
					out += text.Text
				}
			}
			return out, nil
		},
	}
}
