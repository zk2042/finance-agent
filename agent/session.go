package agent

// Session 保存对话历史

import (
	"github.com/openai/openai-go/v3/responses"
)

const (
	MaxSessionMessages = 80 // Messages 的硬上限，超过就强制裁剪
	CompressThreshold  = 60 // 触发摘要压缩的消息数阈值
	KeepRecentRounds   = 5  // 压缩后保留的最近轮次数
	TrimKeepRounds     = 3  //兜底裁剪时保留的最近轮次数
)

// Session 保存对话历史
type Session struct {
	Messages responses.ResponseInputParam // 消息列表
	Len      int
	Summary  string
}

func NewSession() *Session {
	return &Session{
		Messages: responses.ResponseInputParam{},
		Len:      0,
		Summary:  "",
	}
}

// GetMessages 返回原始消息列表（不含摘要）
func (s *Session) GetMessages() responses.ResponseInputParam {
	return s.Messages
}

// GetSummary 返回当前历史摘要
func (s *Session) GetSummary() string {
	return s.Summary
}

// GetLen 返回累计加入过的消息数
func (s *Session) GetLen() int {
	return s.Len
}

// AddMessage 追加消息，并在超过硬上限时自动兜底裁剪
func (s *Session) AddMessage(item ...responses.ResponseInputItemUnionParam) {
	s.Messages = append(s.Messages, item...)
	s.Len++

	if len(s.Messages) > MaxSessionMessages {
		s.trimOldest(TrimKeepRounds)
	}
}

// BuildInput 构建真正发给 LLM 的消息列表
func (s *Session) BuildInput() responses.ResponseInputParam {
	result := responses.ResponseInputParam{}

	// 1. 系统提示词
	if len(s.Messages) > 0 {
		result = append(result, s.Messages[0])
	}

	// 2. 历史摘要
	if s.Summary != "" {
		result = append(result, responses.ResponseInputItemParamOfMessage(
			"[以下是之前对话的历史摘要，供你参考]\n"+s.Summary,
			responses.EasyInputMessageRoleSystem,
		))
	}

	// 3. 剩余消息
	if len(s.Messages) > 1 {
		result = append(result, s.Messages[1:]...)
	}

	return result
}

// NeedsCompress 判断是否需要压缩
func (s *Session) NeedsCompress() bool {
	return len(s.Messages) > CompressThreshold
}

// PrepareCompress 切出需要摘要的消息，但不修改 Messages
func (s *Session) PrepareCompress() (responses.ResponseInputParam, int, bool) {
	if len(s.Messages) <= 1 {
		return nil, 0, false
	}

	cutIdx := findCutIndex(s.Messages, KeepRecentRounds)
	if cutIdx <= 1 {
		return nil, 0, false
	}

	toSummarize := make(responses.ResponseInputParam, cutIdx-1)
	copy(toSummarize, s.Messages[1:cutIdx])

	return toSummarize, cutIdx, true
}

// ApplyCompress 摘要成功后调用，真正从 Messages 中移除旧消息
func (s *Session) ApplyCompress(cutIdx int) {
	if cutIdx <= 1 || cutIdx > len(s.Messages) {
		return
	}
	s.Messages = append(s.Messages[:1], s.Messages[cutIdx:]...)
}

// SetSummary 更新历史摘要
func (s *Session) SetSummary(summary string) {
	s.Summary = summary
}

// trimOldest 兜底裁剪：保留 system prompt + 最近 keepRounds 轮
func (s *Session) trimOldest(keepRounds int) {
	cutIdx := findCutIndex(s.Messages, keepRounds)
	if cutIdx <= 1 {
		return
	}
	s.Messages = append(s.Messages[:1], s.Messages[cutIdx:]...)
}

// findCutIndex 找到"保留最近 keepRounds 轮"应该从哪里开始切
func findCutIndex(messages responses.ResponseInputParam, keepRounds int) int {
	userCount := 0
	for i := len(messages) - 1; i >= 1; i-- { // 从 1 开始，跳过 system prompt
		if isUserMessage(messages[i]) {
			userCount++
			if userCount == keepRounds {
				return i
			}
		}
	}
	return 1
}

// isUserMessage 判断一条消息是否是 user 消息
func isUserMessage(m responses.ResponseInputItemUnionParam) bool {
	if m.OfMessage == nil {
		return false
	}

	return string(m.OfMessage.Role) == string(responses.EasyInputMessageRoleUser)
}
