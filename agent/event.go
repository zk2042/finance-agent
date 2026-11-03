package agent

// 定义 Agent 向 UI 层发送的事件

// EventType 是事件的类型枚举
type EventType int

const (
	// EventRoundStart 每一轮 Loop 开始时触发
	EventRoundStart EventType = iota

	// EventReasoning 模型输出的思考内容
	// 只有支持 reasoning 的模型（如 deepseek-v3）才会发这个
	EventReasoning

	// EventToolCall 模型决定调用某个工具
	EventToolCall

	// EventToolResult 工具执行完毕，返回结果
	// 此时 Tool 和 Result 有值。如果执行失败，Result 里是错误信息
	EventToolResult

	// EventText 模型给出的最终自然语言回答，Agent 收到此事件后通常立即跟一个 EventDone
	EventText

	// EventError Agent 执行过程中发生不可恢复的错误
	// 收到此事件后，本次 Run 立刻结束，不会再发其它事件
	EventError

	// EventDone 本次 Run 正常结束
	EventDone

	// EventChart Agent 主动发出的"图表数据"事件
	EventChart

	// 一段思考的开始 / 结束
	EventReasoningStart
	EventReasoningEnd
)

// Event 是 Agent 向外广播的统一事件
type Event struct {
	Type   EventType
	Round  int // 当前是第几轮 Agent Loop（从 1 开始）
	Text   string
	Tool   string
	Args   string // 模型请求调用工具时给的参数（JSON 字符串）
	Result string // 工具返回值（JSON 字符串或错误信息）
	Err    error
	Chart  *ChartData
}

// ChartData 是跨层的"图表数据"契约
// Agent 负责生成它（解析工具返回的 JSON，提取数值），TUI 负责渲染它
type ChartData struct {
	Kind   string
	Title  string
	Labels []string
	Values []float64
}

// Emitter 是 Agent 向外部发送事件的抽象接口，TUI 层通过实现它，把 agent.Event 转成 tea.Msg 发回主循环
type Emitter interface {
	Emit(Event)
}

// EmitterFunc 把普通函数适配成 Emitter 接口
type EmitterFunc func(Event)

func (emit EmitterFunc) Emit(e Event) {
	emit(e)
}
