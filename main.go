package main

// 入口文件：只负责组装依赖，不含任何业务逻辑

// 组装流程：
//   1. 读 .env
//   2. 建 openai.Client
//   3. 注册本地工具 + 从 mcp.json 加载 MCP 工具
//   4. 建 agent
//   5. 启动 tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"finance-agent/agent"
	"finance-agent/mcp"
	"finance-agent/tools"
	"finance-agent/tui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

// getLlmTools 注册所有工具
func getLlmTools() map[string]*tools.ToolDef {
	m := make(map[string]*tools.ToolDef)
	m["finance"] = tools.NewFinance()
	m["budget"] = tools.NewBudget()
	m["recurring"] = tools.NewRecurring()
	m["report"] = tools.NewReport()
	m["plan"] = tools.NewPlan()
	m["get_current_time"] = tools.NewGetCurrentTime()
	return m
}

// buildOpenaiTools 把 ToolDef 转成 API 格式
func buildOpenaiTools(toolMap map[string]*tools.ToolDef) []responses.ToolUnionParam {
	result := make([]responses.ToolUnionParam, 0, len(toolMap))
	for _, t := range toolMap {
		result = append(result, t.ToParam())
	}
	return result
}

// loadMCPTools 从 mcp.json 读取 MCP Server 配置，逐个连接并把工具并入 allTools。
//
// 设计要点：
//  1. 每个 Server 单独 15s 超时，一个卡住不拖累其它
//  2. 单个 Server 连接失败只打警告，不退出（本地工具还能用）
//  3. 返回的 sessions 由调用方在程序结束时统一 Close
func loadMCPTools(allTools map[string]*tools.ToolDef) []io.Closer {
	// 支持用 MCP_CONFIG 环境变量指定其它路径；不设则用项目根目录的 mcp.json
	configPath := os.Getenv("MCP_CONFIG")
	if configPath == "" {
		configPath = "mcp.json"
	}

	cfgs, err := mcp.LoadConfig(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("提示：未找到 %s，将只使用本地工具\n", configPath)
		} else {
			fmt.Printf("⚠️ 警告：加载 %s 失败：%v\n", configPath, err)
		}
		return nil
	}
	if len(cfgs) == 0 {
		fmt.Println("提示：mcp.json 中没有任何启用的 MCP Server")
		return nil
	}

	var sessions []io.Closer
	for _, cfg := range cfgs {
		// 每个 Server 独立超时：避免前一个卡死拖累后面
		serverCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		mcpTools, session, err := mcp.ConnectMCP(serverCtx, cfg)
		cancel()

		if err != nil {
			fmt.Printf("⚠️ 警告：%s MCP 连接失败：%v\n", cfg.Group, err)
			continue
		}

		for name, t := range mcpTools {
			allTools[name] = t
		}
		sessions = append(sessions, session)
		fmt.Printf("✓ 已连接 MCP 组 [%s]，加载 %d 个工具\n", cfg.Group, len(mcpTools))
	}
	return sessions
}

func main() {
	// 1. 读取 .env 文件
	if err := godotenv.Load(); err != nil {
		fmt.Println("提示：未找到 .env 文件，将使用系统环境变量")
	}
	// 从环境变量读取 API Key
	apiKey := os.Getenv("ARK_API_KEY")
	if apiKey == "" {
		fmt.Println("错误：环境变量 ARK_API_KEY 未设置")
		fmt.Println("请创建 .env 文件并写入：ARK_API_KEY=你的Key")
		os.Exit(1)
	}

	// 2. 建客户端
	client := openai.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL("https://ark.cn-beijing.volces.com/api/v3"),
		option.WithRequestTimeout(60*time.Second),
		// option.WithJSONSet("thinking.type", "disabled"),
	)

	// 3. 注册工具（本地 + MCP）
	// 3.1 本地工具永远可用
	allTools := getLlmTools()

	// 3.2 从 mcp.json 加载外部 MCP 工具
	mcpSessions := loadMCPTools(allTools)
	defer func() {
		for _, s := range mcpSessions {
			_ = s.Close()
		}
	}()

	// 3.3 转成 OpenAI 参数格式
	openaiTools := buildOpenaiTools(allTools)

	// 4. 建 agent（逻辑层），把合并后的工具传进去
	ag := agent.New(client, allTools, openaiTools)

	// 5. 启动 TUI
	m := tui.New(ag)
	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// 把 Program 注入 Model
	m.SetProgram(p)

	if _, err := p.Run(); err != nil {
		fmt.Println("启动失败:", err)
		os.Exit(1)
	}

}
