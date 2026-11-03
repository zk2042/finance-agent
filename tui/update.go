package tui

import (
	"context"
	"finance-agent/agent"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Update ：Bubble Tea 的心脏
// 接收任意 Msg，返回"修改后的 Model" + "下一步要执行的 Cmd"
// 整个程序的循环是：Cmd 在 goroutine 里执行 → 返回一个 Msg → Update 处理 → 返回新 Model + 新 Cmd → ...
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	// ---- 终端尺寸变化 ----
	case tea.WindowSizeMsg:
		return m, m.handleResize(msg)

	// ---- 键盘事件 ----
	case tea.KeyMsg:
		return m, m.handleKey(msg)

	// ---- 用户提交了消息（由 handleKey 的 Cmd 发出） ----
	case userSubmitMsg:
		return m, m.startAgentRun(msg.Text)

		// ---- agent 产生了一个事件 ----
	case agentEventMsg:
		return m, m.handleAgentEvent(msg.Event)

		// ---- agent 跑完了 ----
	case agentFinishedMsg:
		m.loading = false
		return m, nil

		// ---- spinner 的定时 tick ----
	case spinner.TickMsg:
		// 只有 loading 时才继续 tick，避免空转
		if m.loading {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

		return m, nil
	}

	// 兜底：其它消息（比如鼠标滚轮）交给 viewport
	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return m, cmd
}

// ---------- 各分支的实现 ----------

// handleResize 终端尺寸变化时重新计算各组件宽高
func (m *Model) handleResize(msg tea.WindowSizeMsg) tea.Cmd {
	m.width = msg.Width
	m.height = msg.Height

	// viewport 宽度：终端宽 - margin(左右各1) - border(左右各1) - padding(左右各1) = -6
	m.vp.Width = msg.Width - 6
	// viewport 高度：终端高 - 标题(2) - 输入框(3) - margin(上下各1) - border(上下各1) = -8
	m.vp.Height = msg.Height - 8

	// 防止终端特别小时宽度/高度为负
	if m.vp.Width < 20 {
		m.vp.Width = 20
	}
	if m.vp.Height < 5 {
		m.vp.Height = 5
	}

	// 输入框占满一行，减去 border + padding
	m.input.Width = msg.Width - 8

	return nil
}

// handleKey 处理键盘
func (m *Model) handleKey(msg tea.KeyMsg) tea.Cmd {
	// 全局退出：无论何时按 ctrl+c 都直接退
	if msg.String() == "ctrl+c" {
		return tea.Quit
	}

	// loading 中：只允许滚动 viewport 看历史，不允许输入
	if m.loading {
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return cmd
	}

	switch msg.String() {
	case "esc":
		return tea.Quit

	case "enter":
		text := strings.TrimSpace(m.input.Value())
		if text == "" {
			return nil
		}
		if text == "exit" || text == "quit" {
			return tea.Quit
		}
		// 清空输入框
		m.input.Reset()

		// 通过返回一个 Cmd 来异步地把"用户提交了"这个消息送回 Update
		return func() tea.Msg {
			return userSubmitMsg{Text: text}
		}
	}

	// 其它按键交给输入框（光标移动、退格、粘贴等）
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return cmd

}

// startAgentRun 触发一次 Agent 执行
func (m *Model) startAgentRun(text string) tea.Cmd {
	// 1. 如果不是第一次对话，先画一条分隔线
	if m.hasUserInput {
		m.appendLine("") // 空行，视觉上更透气
		m.appendLine(styleDim.Render(strings.Repeat("─", m.dialogWidth())))
		m.appendLine("")
	}
	m.hasUserInput = true

	// 2. 把用户消息显示到对话区
	m.appendLine(styleUser.Render("你：") + text)

	// 3. 进入 loading 状态：禁用输入、启动 spinner
	m.loading = true

	// 4. 返回一个 Cmd
	return tea.Batch(
		// Cmd A：跑 agent（阻塞在 goroutine 里）
		func() tea.Msg {
			m.ag.Run(
				context.Background(),
				text,
				agent.EmitterFunc(func(ev agent.Event) {
					m.program.Send(agentEventMsg{Event: ev})
				}),
			)
			// agent.Run 返回后，通知主循环结束 loading
			return agentFinishedMsg{}
		},

		// Cmd B：启动 spinner 心跳
		m.spinner.Tick,
	)
}

// handleAgentEvent 把 agent 事件转成界面上的文字
func (m *Model) handleAgentEvent(ev agent.Event) tea.Cmd {
	switch ev.Type {
	case agent.EventRoundStart:
	case agent.EventReasoningStart:
		label := fmt.Sprintf(" 🤔 第 %d 轮思考 ", ev.Round)
		m.appendLine("")
		m.appendLine(styleThinkStart.Render("─────" + label + "─────"))

	case agent.EventReasoning:
		if ev.Text == "" {
			break
		}

		if strings.HasPrefix(ev.Text, "📦") || strings.HasPrefix(ev.Text, "✓") {
			m.appendLine(styleChartTitle.Render(ev.Text))
		} else {

			m.appendLine(styleThink.Render("  │ " + ev.Text))
		}

	case agent.EventReasoningEnd:
		m.appendLine(styleThinkEnd.Render("───── 思考结束 ─────"))

	case agent.EventToolCall:
		m.appendLine(styleTool.Render("⚙ " + ev.Tool + " " + ev.Args))

	case agent.EventToolResult:
		m.appendLine(styleTool.Render("  ↳ " + truncate(ev.Result, 120)))

	case agent.EventText:
		if ev.Text != "" {
			// 计算渲染宽度：viewport 宽度减去左右边框和缩进
			w := m.vp.Width - 4
			rendered := renderMarkdown(ev.Text, w)

			lines := strings.Split(rendered, "\n")
			var buf strings.Builder
			for i, line := range lines {
				if i == 0 {
					buf.WriteString(styleAssistant.Render("助手："))
				} else {
					buf.WriteString("     ")
				}
				buf.WriteString(line)
				buf.WriteString("\n")

			}

			m.appendLine(strings.TrimRight(buf.String(), "\n"))
		}

	case agent.EventError:
		m.appendLine(styleError.Render("✗ " + ev.Err.Error()))
		m.loading = false

	case agent.EventDone:
		m.loading = false

	case agent.EventChart:
		if ev.Chart != nil {
			m.appendChart(ev.Chart)
		}

	}

	return nil
}

// ---------- 辅助函数 ----------

// appendLine 把一行文字追加到对话区
func (m *Model) appendLine(line string) {
	m.history.WriteString(line)
	m.history.WriteString("\n")
	m.vp.SetContent(m.history.String())
	m.vp.GotoBottom() // 自动滚到底部
}

// truncate 截断字符串。用 []rune 而不是 []byte，避免切坏中文
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}

	return string(r[:n]) + "..."
}

// dialogWidth 返回分隔线的宽度
func (m *Model) dialogWidth() int {
	w := m.vp.Width - 2
	if w < 20 {
		w = 20
	}
	return w
}

// appendChart 把图表渲染成 ANSI 字符串，直接追加到对话历史
func (m *Model) appendChart(cd *agent.ChartData) {
	if cd == nil || len(cd.Labels) == 0 {
		return
	}

	w := m.vp.Width - 4
	if w < 30 {
		w = 30
	}
	h := 10

	var chartStr string
	switch cd.Kind {
	case "line":
		chartStr = renderLineChart(cd, w, h)
	default:
		chartStr = renderBarChart(cd, w, h)
	}

	// 空行 + 标题 + 图表，作为一个整体追加到对话流
	m.appendLine("")
	m.appendLine(styleChartTitle.Render("📊 " + cd.Title))
	m.appendLine(chartStr)
}

// renderBarChart 把 ChartData 渲染成柱状图字符串
func renderBarChart(cd *agent.ChartData, w, h int) string {
	if len(cd.Labels) == 0 || len(cd.Values) == 0 {
		return ""
	}

	// 1. 最大值
	maxV := 0.0
	for _, v := range cd.Values {
		if v > maxV {
			maxV = v
		}
	}
	if maxV == 0 {
		maxV = 1
	}

	cols := len(cd.Labels)
	if cols > len(cd.Values) {
		cols = len(cd.Values)
	}

	// 2. 均匀列宽
	const outerPad = 2
	const colGap = 1
	innerW := w - 2*outerPad
	colWidth := (innerW - (cols-1)*colGap) / cols
	if colWidth < 3 {
		colWidth = 3
	}

	// 3. 柱体高度：总高减去标签行(1)+顶部留白(1)
	chartHeight := h - 2
	if chartHeight < 3 {
		chartHeight = 3
	}

	// 4. 逐行画柱体（从上到下）
	var rows []string
	for row := 0; row < chartHeight; row++ {
		threshold := float64(chartHeight-row-1) / float64(chartHeight) * maxV
		var line strings.Builder
		line.WriteString(strings.Repeat(" ", outerPad))
		for i := 0; i < cols; i++ {
			if i > 0 {
				line.WriteString(strings.Repeat(" ", colGap))
			}
			if cd.Values[i] > threshold {
				line.WriteString(strings.Repeat("█", colWidth))
			} else {
				line.WriteString(strings.Repeat(" ", colWidth))
			}
		}
		rows = append(rows, line.String())
	}

	// 5. 标签行：每个标签居中于 colWidth
	var labelLine strings.Builder
	labelLine.WriteString(strings.Repeat(" ", outerPad))
	for i := 0; i < cols; i++ {
		if i > 0 {
			labelLine.WriteString(strings.Repeat(" ", colGap))
		}
		labelLine.WriteString(centerPad(cd.Labels[i], colWidth))
	}
	rows = append(rows, labelLine.String())

	return strings.Join(rows, "\n")
}

// renderLineChart 把 ChartData 渲染成折线图字符串
func renderLineChart(cd *agent.ChartData, w, h int) string {
	if len(cd.Values) == 0 || len(cd.Labels) == 0 {
		return ""
	}

	// 1. 找最大值，用于按比例画条
	maxV := 0.0
	for _, v := range cd.Values {
		if v > maxV {
			maxV = v
		}
	}
	if maxV == 0 {
		maxV = 1
	}

	// 2. 计算标签列宽度
	labelWidth := 0
	for _, l := range cd.Labels {
		if n := len([]rune(l)); n > labelWidth {
			labelWidth = n
		}
	}

	// 3. 计算条形区宽度：给"标签 + 空格 + 数值"预留
	barWidth := w - labelWidth - 12
	if barWidth < 10 {
		barWidth = 10
	}

	// 4. 逐行渲染
	var sb strings.Builder
	for i, v := range cd.Values {
		if i >= len(cd.Labels) {
			break
		}
		if i > 0 {
			sb.WriteString("\n")
		}

		label := cd.Labels[i]

		// 左对齐标签
		pad := labelWidth - len([]rune(label))
		sb.WriteString(strings.Repeat(" ", pad))
		sb.WriteString(label)
		sb.WriteString("  ")

		// 画实心条 + 空心条
		filled := int(float64(barWidth) * v / maxV)
		if filled < 1 && v > 0 {
			filled = 1
		}
		sb.WriteString(strings.Repeat("█", filled))
		sb.WriteString(strings.Repeat("░", barWidth-filled))

		// 数值（保留整数，财务够用）
		sb.WriteString(fmt.Sprintf("  %.0f", v))
		sb.WriteString("\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}

// centerPad 把 s 居中显示在 width 显示宽度内，超出则截断
func centerPad(s string, width int) string {
	sw := lipgloss.Width(s)
	if sw >= width {
		// 逐 rune 截断
		runes := []rune(s)
		for len(runes) > 0 {
			if lipgloss.Width(string(runes)) <= width {
				break
			}
			runes = runes[:len(runes)-1]
		}
		out := string(runes)
		return out + strings.Repeat(" ", width-lipgloss.Width(out))
	}
	left := (width - sw) / 2
	right := width - sw - left
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
}
