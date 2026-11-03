package tui

import "finance-agent/agent"

// ---- TUI 内部消息（tea.Msg 的载体） ----
// Bubble Tea 的 Update 靠"类型断言"分发消息

// agentEventMsg 把 agent.Event 从 goroutine 送进主循环
type agentEventMsg struct {
	Event agent.Event
}

// agentFinishedMsg 表示一次 agent.Run 已经返回
type agentFinishedMsg struct{}

// userSubmitMsg 用户按回车提交了一条非空文本
type userSubmitMsg struct {
	Text string
}
