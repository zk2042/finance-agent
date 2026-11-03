package tui

// 所有 Lip Gloss 样式集中在这里，改配色只改这一个文件

import "github.com/charmbracelet/lipgloss"

var (
	// 标题
	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500")).
			MarginBottom(1)

	// 用户说的话
	styleUser = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#04B575")).
			Bold(true)

	// 助手最终回答
	styleAssistant = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4"))

	// 模型的思考过程
	styleThink = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888888")).
			Italic(true)

	// 工具调用与工具返回
	styleTool = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3C9AE8"))

	// 错误
	styleError = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F5F")).
			Bold(true)

	// 弱化的辅助信息（分隔线、加载提示）
	styleDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))

	// 对话主区域的外框
	styleBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3C3C3C")).
			Padding(0, 1)

	// 输入框外框
	styleInputBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7D56F4")).
			Padding(0, 1)

	// 启动欢迎语
	styleWelcome = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7D56F4")).
			Bold(true)

	// 图表标题
	styleChartTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFA500")).
			MarginBottom(1)

	// 思考块的开始分隔
	styleThinkStart = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#BB88FF")).
			Bold(true)

	// 思考块的结束分隔
	styleThinkEnd = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#666666"))
)
