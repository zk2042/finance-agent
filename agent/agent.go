package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"finance-agent/tools"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

const MaxLoop = 10 // 最大循环次数，防止无限循环

// Agent 财务助手的核心，持有：LLM 客户端、会话历史、工具注册表、以及已经转好的 API 工具参数
type Agent struct {
	Client       openai.Client
	Session      *Session
	Tools        map[string]*tools.ToolDef  // 名字 → 工具实现
	OpenAITools  []responses.ToolUnionParam // 已经转好的 API 格式
	LoadedGroups map[string]bool            // 记录当前 Run 加载了哪些工具组
}

var MetaTool = responses.ToolUnionParam{
	OfFunction: &responses.FunctionToolParam{
		Name:        "load_tool_group",
		Description: openai.String("加载指定工具箱以获取具体工具。请根据用户需求自主选择："),
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"group": map[string]any{
					"type": "string",
					"enum": []string{"finance", "zenmoney", "notify"},
					"description": "工具组名。\n" +
						"- 'finance': 本地日常记账、预算管理、财务规划。\n" +
						"- 'zenmoney': ZenMoney 云端真实银行流水。\n" +
						"- 'notify': 发送通知。",
				},
			},
			"required": []string{"group"},
		},
	},
}

// New 创建一个 Agent
// 自动把系统提示词加入 Session。所以每次新建 Agent 就是一个全新的会话
func New(
	client openai.Client,
	toolMap map[string]*tools.ToolDef,
	openaiTools []responses.ToolUnionParam,
) *Agent {
	session := NewSession()
	session.AddMessage(
		responses.ResponseInputItemParamOfMessage(
			SystemPrompt,
			responses.EasyInputMessageRoleSystem,
		),
	)

	return &Agent{
		Client:       client,
		Session:      session,
		Tools:        toolMap,
		OpenAITools:  openaiTools,
		LoadedGroups: make(map[string]bool),
	}
}

// Reset 清空会话（保留系统提示词），用于"新对话"功能
func (a *Agent) Reset() {
	a.Session = NewSession()
	a.Session.AddMessage(
		responses.ResponseInputItemParamOfMessage(
			SystemPrompt,
			responses.EasyInputMessageRoleSystem,
		),
	)
}

// Run 执行一次完整的 Agent Loop（支持首轮元工具强制路由）
func (a *Agent) Run(ctx context.Context, userInput string, emit Emitter) {
	//先检查是否需要压缩历史
	a.maybeCompress(ctx, emit)

	// 每次 Run 开始，清空上一轮的工具组状态
	a.LoadedGroups = make(map[string]bool)

	// 1. 把用户输入记入 session
	a.Session.AddMessage(
		responses.ResponseInputItemParamOfMessage(
			userInput,
			responses.EasyInputMessageRoleUser,
		),
	)

	// 缓存本次 Run 里产生的所有图表，等最终回答发完再统一发
	var pendingCharts []*ChartData

	// 2. 开始 Loop
	for round := 1; round <= MaxLoop; round++ {
		emit.Emit(Event{Type: EventRoundStart, Round: round})

		var currentTools []responses.ToolUnionParam
		var toolChoice responses.ResponseNewParamsToolChoiceUnion

		if round == 1 {
			// 首轮：只给元工具，让它自然决定是否调用
			currentTools = []responses.ToolUnionParam{MetaTool}
			toolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			}
		} else {
			// 第 2 轮及以后：根据已加载的组过滤工具
			currentTools = a.filterToolsByLoadedGroups()
			toolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			}
		}

		// 2.1 调 LLM
		resp, err := a.Client.Responses.New(ctx, responses.ResponseNewParams{
			Input: responses.ResponseNewParamsInputUnion{
				OfInputItemList: a.Session.BuildInput(),
			},
			Tools:      currentTools,
			ToolChoice: toolChoice,
			Model:      "deepseek-v4-1-flash-economy-261017",
		})
		if err != nil {
			emit.Emit(Event{
				Type: EventError,
				Err:  normalizeErr(err),
			})
			return
		}

		// 2.2 遍历模型输出，处理每个 item
		hasToolCall := false
		for _, item := range resp.Output {
			switch item.Type {
			case "reasoning":
				// 先看这段思考是不是真的有内容（空 summary 直接跳过）
				hasContent := false
				for _, s := range item.Summary {
					if s.Text != "" {
						hasContent = true
						break
					}
				}
				if !hasContent {
					break
				}
				// 1. 通知 UI：一段思考开始
				emit.Emit(Event{Type: EventReasoningStart, Round: round})

				// 2. 逐个 summary 片段发送
				for _, s := range item.Summary {
					if s.Text != "" {
						emit.Emit(Event{Type: EventReasoning, Round: round, Text: s.Text})
					}
				}

				// 3. 通知 UI：一段思考结束
				emit.Emit(Event{Type: EventReasoningEnd, Round: round})

			case "function_call":
				hasToolCall = true
				fc := item.AsFunctionCall()

				// 拦截元工具（写入 Session，让模型有记忆）
				if fc.Name == "load_tool_group" {
					var groupArgs struct {
						Group string `json:"group"`
					}
					json.Unmarshal([]byte(fc.Arguments), &groupArgs)

					// 记录加载的组
					a.LoadedGroups[groupArgs.Group] = true
					emit.Emit(Event{
						Type: EventToolCall,
						Tool: "load_tool_group",
						Args: fc.Arguments,
					})

					// 1. 写入 function_call
					a.Session.AddMessage(responses.ResponseInputItemUnionParam{
						OfFunctionCall: &responses.ResponseFunctionToolCallParam{
							Arguments: fc.Arguments,
							CallID:    fc.CallID,
							Name:      fc.Name,
						},
					})

					// 2. 检查实际加载到了几个工具（用于反馈给模型）
					groupToolsCount := 0
					if groupArgs.Group == "finance" {
						// 本地组：数所有本地工具
						for name := range a.Tools {
							if isLocalTool(name) {
								groupToolsCount++
							}
						}
					} else {
						// MCP 组：按 group__ 前缀数
						for name := range a.Tools {
							if strings.HasPrefix(name, groupArgs.Group+"__") {
								groupToolsCount++
							}
						}
					}

					var resultMsg string
					if groupToolsCount > 0 {
						resultMsg = fmt.Sprintf("加载成功！工具组 '%s' 包含 %d 个工具，请在下一轮直接调用。", groupArgs.Group, groupToolsCount)
					} else {
						resultMsg = fmt.Sprintf("加载失败：未找到工具组 '%s' 的任何工具（可能是本地环境未配置对应的 MCP Server）。请直接告知用户该功能不可用。", groupArgs.Group)
					}

					// 3. 写入 function_call_output
					a.Session.AddMessage(responses.ResponseInputItemUnionParam{
						OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
							Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
								OfString: openai.String(resultMsg),
							},
							CallID: openai.String(fc.CallID),
						},
					})

					emit.Emit(Event{Type: EventToolResult, Tool: fc.Name, Result: resultMsg})
					continue
				}

				// 广播"要调工具了"
				emit.Emit(Event{
					Type: EventToolCall,
					Tool: fc.Name,
					Args: fc.Arguments,
				})

				// 把 function_call 记入 session，供下一轮 LLM 看到
				a.Session.AddMessage(responses.ResponseInputItemUnionParam{
					OfFunctionCall: &responses.ResponseFunctionToolCallParam{
						Arguments: fc.Arguments,
						CallID:    fc.CallID,
						Name:      fc.Name,
					},
				})

				// 执行工具
				var result string
				if t, ok := a.Tools[fc.Name]; ok {
					r, err := t.Call(ctx, fc.Arguments)
					if err != nil {
						// 工具内部错误不中断整个 Loop，只是把错误当成结果告诉模型
						r = fmt.Sprintf("工具执行失败: %v", err)
					}
					result = r
				} else {
					result = fmt.Sprintf("未知工具: %s", fc.Name)
				}

				// 广播"工具执行完了"
				emit.Emit(Event{
					Type:   EventToolResult,
					Tool:   fc.Name,
					Result: result,
				})

				if chart := buildChart(fc.Name, fc.Arguments, result); chart != nil {
					pendingCharts = append(pendingCharts, chart)
				}

				// 把结果记入 session
				a.Session.AddMessage(responses.ResponseInputItemUnionParam{
					OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{
						Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{
							OfString: openai.String(result),
						},
						CallID: openai.String(fc.CallID),
					},
				})

			}
		}

		// 2.3 这一轮没有工具调用 → 模型给出最终回答，结束
		if !hasToolCall {
			text := resp.OutputText()
			if text != "" {
				// 把最终回答也记入 session，多轮对话时模型能看到自己上次说了什么
				a.Session.AddMessage(
					responses.ResponseInputItemParamOfMessage(
						text,
						responses.EasyInputMessageRoleAssistant,
					),
				)
				emit.Emit(Event{
					Type: EventText,
					Text: text,
				})
			}

			for _, c := range pendingCharts {
				emit.Emit(Event{Type: EventChart, Chart: c})
			}
			pendingCharts = nil

			emit.Emit(Event{Type: EventDone})
			return
		}
	}

	// 3. 超过 MaxLoop 还没结束
	for _, c := range pendingCharts {
		emit.Emit(Event{Type: EventChart, Chart: c})
	}

	emit.Emit(Event{
		Type: EventError,
		Err:  fmt.Errorf("达到最大循环次数 %d，强制结束", MaxLoop),
	})
}

// 过滤工具
func (a *Agent) filterToolsByLoadedGroups() []responses.ToolUnionParam {
	var filtered []responses.ToolUnionParam

	// 始终保留元工具，避免模型失忆
	filtered = append(filtered, MetaTool)

	for _, t := range a.OpenAITools {
		name := t.OfFunction.Name

		// 1. 本地工具永远保留
		if isLocalTool(name) {
			filtered = append(filtered, t)
			continue
		}

		// 2. 匹配已加载的 MCP 组
		for group := range a.LoadedGroups {
			if strings.HasPrefix(name, group+"__") {
				filtered = append(filtered, t)
				break
			}
		}
	}
	return filtered
}

// 判断是不是项目自带的本地工具
func isLocalTool(name string) bool {
	localNames := []string{"finance", "budget", "recurring", "report", "plan", "get_current_time"}
	for _, n := range localNames {
		if name == n {
			return true
		}
	}
	return false
}

// normalizeErr 把 openai-go 的 API 错误转成更可读的形式
func normalizeErr(err error) error {
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf("LLM API 错误 [HTTP %d]: %s", apiErr.StatusCode, apiErr.RawJSON())
	}

	return err
}

// 历史压缩相关

// maybeCompress 检查是否需要压缩历史，需要就调用 LLM 做摘要
func (a *Agent) maybeCompress(ctx context.Context, emit Emitter) {
	if !a.Session.NeedsCompress() {
		return
	}

	// 通知 UI：正在压缩
	emit.Emit(Event{
		Type: EventReasoning,
		Text: "📦 正在压缩历史对话（约需几秒）...",
	})

	// 1. 先"看"要切哪些消息（不修改 Messages）
	toSummarize, cutIdx, ok := a.Session.PrepareCompress()
	if !ok {
		return
	}

	// 2. 调 LLM 做摘要
	newSummary, err := a.summarize(ctx, toSummarize)
	if err != nil {
		// 摘要失败：不修改 Messages，让滑动窗口慢慢兜底
		emit.Emit(Event{
			Type: EventReasoning,
			Text: fmt.Sprintf("⚠ 摘要失败（%v），继续使用原始历史", err),
		})
		return
	}

	// 3. 摘要成功：真正切掉 Messages 里的旧消息
	a.Session.ApplyCompress(cutIdx)

	// 4. 合并摘要（首次直接写，之后合并）
	if a.Session.Summary == "" {
		a.Session.SetSummary(newSummary)

	} else {
		merged, err := a.mergeSummaries(ctx, a.Session.Summary, newSummary)
		if err != nil {
			// 合并失败：直接拼接
			a.Session.SetSummary(a.Session.Summary + "\n---\n" + newSummary)
		} else {
			a.Session.SetSummary(merged)
		}
	}

	emit.Emit(Event{
		Type: EventReasoning,
		Text: fmt.Sprintf("✓ 已压缩 %d 条旧消息", len(toSummarize)),
	})
}

// summarize 把一批消息压缩成一段中文摘要
func (a *Agent) summarize(ctx context.Context, messages responses.ResponseInputParam) (string, error) {
	input := make(responses.ResponseInputParam, 0, len(messages)+1)
	input = append(input, responses.ResponseInputItemParamOfMessage(
		`请把以下对话历史压缩成一段简洁的中文摘要。

要求：
1. 保留所有关键事实：金额、日期、分类、用户偏好、约定事项
2. 保留用户明确表达过的需求（如"下个月预算 5000"、"用 penalty 策略"）
3. 忽略工具返回的原始 JSON 数据，只保留结论性信息（如"9月支出 5299 元"）
4. 按时间顺序组织，但不要逐条罗列
5. 不要添加评论、建议或"以上是摘要"之类的元话语
6. 直接输出摘要正文，不要用 Markdown 标题或代码块

只输出摘要内容，不要任何前后缀。`,
		responses.EasyInputMessageRoleSystem,
	))
	input = append(input, messages...)

	resp, err := a.Client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Model: "deepseek-v4-1-flash-economy-261017",
	})
	if err != nil {
		return "", err
	}

	text := resp.OutputText()
	if text == "" {
		return "", fmt.Errorf("摘要返回为空")
	}

	return text, nil
}

// mergeSummaries 把旧摘要和新摘要合并成一段
func (a *Agent) mergeSummaries(ctx context.Context, oldSummary, newSummary string) (string, error) {
	prompt := `把下面两段对话摘要合并成一段。

要求：
1. 保留所有关键事实：金额、日期、分类、用户偏好、约定
2. 如果两段有矛盾，以【新摘要】为准（时间更新）
3. 不要重复信息
4. 按时间顺序组织
5. 直接输出合并后的正文，不要 Markdown 标题

【旧摘要】
` + oldSummary + `

【新摘要】
` + newSummary

	input := responses.ResponseInputParam{
		responses.ResponseInputItemParamOfMessage(prompt, responses.EasyInputMessageRoleSystem),
	}

	resp, err := a.Client.Responses.New(ctx, responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		},
		Model: "deepseek-v4-1-flash-economy-261017",
	})
	if err != nil {
		return "", err
	}

	text := resp.OutputText()
	if text == "" {
		return "", fmt.Errorf("合并摘要返回为空")
	}
	return text, nil
}
