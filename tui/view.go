package tui

import "github.com/charmbracelet/lipgloss"

// View 根据当前 Model 渲染整个界面，Bubble Tea 每次 Update 后都会调用一次 View

func (m *Model) View() string {
	// 1. 顶部标题
	header := styleTitle.Render("💼 个人财务助手")

	// 2. 中间对话区（用外框包住 viewport）
	body := styleBox.Width(m.vp.Width + 2).Render(m.vp.View())

	// 3. 底部输入框：loading 时显示 spinner，否则显示输入框
	var footer string
	if m.loading {
		footer = m.spinner.View() + styleDim.Render(" 思考中...")
	} else {
		footer = styleInputBox.Render(m.input.View())
	}

	return lipgloss.NewStyle().
		Margin(1, 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, header, body, footer))
}
