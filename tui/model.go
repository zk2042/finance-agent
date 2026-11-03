package tui

import (
	"strings"

	"finance-agent/agent"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Model 是 Bubble Tea 要求的"应用状态容器"
// 所有会被 Update 修改的东西，都必须作为字段放在这里
type Model struct {
	// ---- 外部依赖 ----
	ag      *agent.Agent // 财务助手核心逻辑，由 main.go 注入
	program *tea.Program // 用于从 goroutine 里发消息回主循环

	// ---- Bubble Tea 子组件 ----
	vp      viewport.Model  // 可滚动的对话区
	input   textinput.Model // 底部单行输入框
	spinner spinner.Model   // 加载动画

	// ---- 界面状态 ----
	width   int  // 终端宽度，WindowSizeMsg 里来的
	height  int  // 终端高度
	loading bool // agent 是否正在跑（跑的时候禁用输入、显示 spinner）

	// ---- 内容缓存 ----
	history strings.Builder

	hasUserInput bool // 是否已经有过至少一次用户输入

}

// New 创建初始 Model, tui 包的唯一入口
func New(ag *agent.Agent) *Model {
	// 对话区
	vp := viewport.New(80, 20)
	vp.SetContent(styleWelcome.Render("💼 财务助手已启动，输入你的问题。"))

	// 输入框
	ti := textinput.New()
	ti.Placeholder = "输入消息，回车发送（exit / quit 退出）"
	ti.CharLimit = 500
	ti.Width = 80
	ti.Prompt = "> "
	ti.Focus() // 默认聚焦

	// 加载动画
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styleDim

	return &Model{
		ag:      ag,
		vp:      vp,
		input:   ti,
		spinner: sp,
	}
}

// SetProgram ：让 Model 拿到 *tea.Program，从而在 goroutine 里能 p.Send(...)
func (m *Model) SetProgram(p *tea.Program) {
	m.program = p
}

// Init 在程序启动时执行一次，返回一个初始 Cmd
// 这里启动 textinput.Blink，让光标闪烁
func (m *Model) Init() tea.Cmd {
	return textinput.Blink
}
