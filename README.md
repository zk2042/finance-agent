# 💼 Finance Agent — 终端个人财务助手

一个用 Go 编写的**终端 AI 财务助手**。在 TUI 里用自然语言描述需求，大模型自主决定调用哪个工具（记账、查预算、生成报表、做规划、同步云端流水、发通知），工具读写本地 JSON 或调用外部 MCP Server，模型再用自然语言总结，TUI 展示对话、思考过程、工具调用和图表。

**核心模式：Agent Loop + Tool Calling + MCP 集成（JSON 配置驱动）。**

---

## ✨ 功能特性

- 🗣️ **自然语言交互** —— 直接说"今天午饭花了25块，餐饮"，自动解析并记账
- 🧠 **Agent Loop** —— 多轮工具调用，最多 10 轮，模型自主决策是否继续
- 📊 **纯文本图表** —— 自绘柱状图 / 折线图，中文对齐无乱码
- 🔧 **6 个本地工具** —— 记账、预算、固定支出、报表、规划、时间
- 🔌 **MCP 支持（JSON 配置）** —— 通过 `mcp.json` 声明外部 MCP Server，格式兼容 Claude Desktop / Cursor
- 🎯 **工具组按需加载** —— 首轮强制调用 `load_tool_group` 元工具，减少 token 消耗
- 💾 **本地 JSON 存储** —— 数据完全在你手里，无需数据库
- 🎨 **Bubble Tea TUI** —— 流畅的终端界面，支持鼠标滚轮、思考过程、Markdown 渲染
- 🗜️ **历史自动压缩** —— 超过阈值（60 条）自动让 LLM 做摘要，控制上下文长度

---

## 🏗️ 架构

项目分五层，职责清晰：

```
main.go        入口：读 .env、建 client、从 mcp.json 加载 MCP、注册工具、启动 TUI
mcp/           MCP 层：config.go 读 mcp.json，client.go 连接 Server 并包装成 ToolDef
tools/         工具层：读写 data/*.json，做业务计算
agent/         智能体层：调 LLM、管理会话、执行 Agent Loop、发事件
tui/           界面层：Bubble Tea，展示对话/思考/工具/图表
data/          数据层：finance.json, budget.json, recurring.json
```

### 核心数据流

```
用户输入
  ↓
TUI 捕获回车 → 启动 goroutine 执行 agent.Run
  ↓
Agent 把用户消息加入 Session
  ↓
【首轮】只给元工具 load_tool_group，强制调用（tool_choice = required）
  ↓
Agent 加载对应工具组 → 下一轮带上真实工具
  ↓
LLM 返回：reasoning / function_call / 最终文本
  ↓
若是 function_call：
  执行本地工具或 MCP 工具 → 读写 data/*.json 或调用外部服务
  把结果加入 Session → 进入下一轮
若是最终文本：
  发 EventText / EventChart / EventDone
  ↓
TUI 收到事件，更新界面
```

### MCP 加载流程

```
main.go 启动
  ↓
mcp.LoadConfig("mcp.json")  读取配置文件
  ↓
逐个 Server：
  展开 env 里的 ${VAR} → 跳过 disabled=true 的条目
  15s 超时内连接 → ListTools() 动态拉取工具
  包装成 ToolDef，名字加组前缀（如 zenmoney__list_transactions）
  ↓
失败只打警告，不退出（本地工具照常使用）
  ↓
所有工具并入 allTools，传给 agent
```

### Agent Loop 关键参数

| 参数 | 值 | 说明 |
| --- | --- | --- |
| `MaxLoop` | 10 | 单轮对话最大循环次数，防止无限循环 |
| `CompressThreshold` | 60 | 消息数超过此值触发 LLM 摘要压缩 |
| `MaxSessionMessages` | 80 | 消息硬上限，超过则兜底裁剪（保留最近 3 轮） |
| `KeepRecentRounds` | 5 | 压缩时保留的最近轮次 |

---

## 🚀 快速开始

### 前置要求

- **Go 1.22+**（`go.mod` 声明的是 `go 1.26.5`，若本机版本更低，将其改为你实际安装的版本即可）
- 一个兼容 OpenAI **Responses API** 的 LLM 服务（本项目默认使用**火山方舟**）
- 可选：外部 MCP Server 可执行文件（如 `zenmoney-mcp.exe`、`ntfy-mcp.exe`）

### 克隆项目

```bash
git clone https://github.com/<your-name>/finance-agent.git
cd finance-agent
```

### 配置 .env

项目根目录附带了 `.env.example` 模板，复制一份并填入你的值：

```bash
cp .env.example .env
```

`.env` 文件内容：

```env
# 必填：LLM API Key（火山方舟）
ARK_API_KEY=你的Key

# 可选：ZenMoney 云端 token（mcp.json 中引用）
ZENMONEY_TOKEN=你的ZenMoneyToken

# 可选：ntfy 通知主题（mcp.json 中引用）
NTFY_TOPIC=你的ntfy主题
```

> ⚠️ 项目不附带 `.env.example`，请自行创建 `.env`。

### 配置 mcp.json（可选）

项目根目录的 `mcp.json` 声明要连接的外部 MCP Server，**格式与 Claude Desktop / Cursor 完全一致**：

```json
{
  "mcpServers": {
    "zenmoney": {
      "command": "./zenmoney-mcp.exe",
      "args": [],
      "env": {
        "ZENMONEY_TOKEN": "${ZENMONEY_TOKEN}"
      }
    },
    "notify": {
      "command": "D:\\software\\python\\Scripts\\ntfy-mcp.exe",
      "args": [],
      "env": {
        "NTFY_TOPIC": "${NTFY_TOPIC}"
      }
    }
  }
}
```

**配置规则：**

| 字段 | 说明 |
| --- | --- |
| 键名（如 `zenmoney`） | 工具组名，MCP 工具会自动加此前缀，如 `zenmoney__list_transactions` |
| `command` | MCP Server 可执行文件路径（可用相对路径或绝对路径） |
| `args` | 命令参数数组，可选 |
| `env` | 传给子进程的环境变量，`${VAR}` 会展开成 `.env` / 系统环境变量的值 |
| `disabled` | 设为 `true` 则跳过该 Server，可选 |

> ⚠️ **示例中的 `notify.command` 是作者本机路径**（`D:\software\python\Scripts\ntfy-mcp.exe`），请改成你自己机器上 `ntfy-mcp.exe` 的实际路径，或改用相对路径如 `./ntfy-mcp.exe`。`zenmoney.command` 已用相对路径，把 `zenmoney-mcp.exe` 放到项目根目录即可。
>
> 💡 不想用某个 MCP Server？把它的 `command` 留空或加 `"disabled": true` 即可，无需删除。
>
> 💡 想用其它配置文件路径？设置环境变量 `MCP_CONFIG=/path/to/your/mcp.json`。

### 安装依赖

```bash
go mod tidy
```

下载慢可设置代理：

```bash
go env -w GOPROXY=https://goproxy.cn,direct
```

### 运行

```bash
go run .
```

启动时会打印 MCP 连接状态：

```text
✓ 已连接 MCP 组 [zenmoney]，加载 3 个工具
✓ 已连接 MCP 组 [notify]，加载 1 个工具
```

若某个 MCP 连接失败，只会警告，不影响本地工具使用：

```text
⚠️ 警告：notify MCP 连接失败：...
```

启动后界面类似：

```text
💼 个人财务助手
┌──────────────────────────────────────┐
│ 💼 财务助手已启动，输入你的问题。      │
│                                      │
└──────────────────────────────────────┘
> 输入消息，回车发送（exit / quit 退出）
```

### 试试这些指令

```text
今天午饭花了25，餐饮
生成本月月报
检查这个月是否超支
下个月预算怎么规划？（用 carryover 策略）
把固定支出入账
用 zenmoney 查一下我上个月的银行流水
给我发一条通知提醒交房租
```

---

## ⌨️ 键盘快捷键

| 按键 | 作用 |
| --- | --- |
| `Enter` | 发送输入框中的消息 |
| `Esc` | 退出程序 |
| `Ctrl+C` | 强制退出（任何时候） |
| 输入 `exit` / `quit` 后回车 | 退出程序 |
| 鼠标滚轮 / 方向键 | 滚动对话历史（loading 中也可用） |

> Agent 运行中输入框被锁定，只能滚动查看历史。

---

## ⚙️ 配置说明

### 环境变量

| 变量 | 必填 | 说明 |
| --- | --- | --- |
| `ARK_API_KEY` | ✅ | 火山方舟 API Key |
| `ZENMONEY_TOKEN` | ❌ | ZenMoney 云端 token（在 mcp.json 中通过 `${ZENMONEY_TOKEN}` 引用） |
| `NTFY_TOPIC` | ❌ | ntfy 通知主题（在 mcp.json 中通过 `${NTFY_TOPIC}` 引用） |
| `MCP_CONFIG` | ❌ | 自定义 mcp.json 路径，默认是项目根目录的 `mcp.json` |

### 修改模型 / BaseURL

- **BaseURL**：[main.go](file:///d:/GoProjects/agent/finance-agent/main.go#L116) 中的 `option.WithBaseURL("https://ark.cn-beijing.volces.com/api/v3")`
- **模型名**：[agent/agent.go](file:///d:/GoProjects/agent/finance-agent/agent/agent.go) 中的 `Model: "deepseek-v4-1-flash-economy-261017"`

如需切换 OpenAI 官方：

```go
// main.go
option.WithBaseURL("https://api.openai.com/v1")
// agent/agent.go
Model: "gpt-4o"
```

> 注意：摘要压缩和主对话共用同一个模型名（写死在 `agent.go` 的 `summarize` / `mergeSummaries` 中），换模型时记得一并修改。

### mcp.json 完整示例

```json
{
  "mcpServers": {
    "zenmoney": {
      "command": "./zenmoney-mcp.exe",
      "args": [],
      "env": {
        "ZENMONEY_TOKEN": "${ZENMONEY_TOKEN}"
      }
    },
    "notify": {
      "command": "D:\\software\\python\\Scripts\\ntfy-mcp.exe",
      "args": [],
      "env": {
        "NTFY_TOPIC": "${NTFY_TOPIC}"
      },
      "disabled": false
    }
  }
}
```

**环境变量展开**：`env` 中的 `${ZENMONEY_TOKEN}` 会在加载时替换成 `.env` 里 `ZENMONEY_TOKEN` 的值。如果变量未设置，会打印警告并展开为空字符串，对应 MCP 可能连接失败。

---

## 🔧 工具说明

### 元工具：`load_tool_group`

每次对话首轮，Agent 只暴露这一个元工具，强制模型选择工具组。加载成功后下一轮才会带上该组的真实工具。

| 组名 | 说明 | 包含工具 |
| --- | --- | --- |
| `finance` | 本地日常记账、预算、报表、规划（默认启用） | finance, budget, recurring, report, plan, get_current_time |
| `zenmoney` | ZenMoney 云端真实银行流水 | zenmoney\_\_\*（需在 mcp.json 配置且 ZENMONEY_TOKEN 已设置） |
| `notify` | 发送通知到手机 | notify\_\_\*（需在 mcp.json 配置且 NTFY_TOPIC 已设置） |

MCP 工具名前会带上组前缀，例如 `zenmoney__list_transactions`。**MCP 工具组无需在代码里注册**，只要在 `mcp.json` 里声明即可自动加载。

### 本地工具（tools/）

#### `finance` — 记账管理

| action | 说明 | 参数 |
| --- | --- | --- |
| `add` | 添加一笔账目 | amount, category, note, date(默认今天) |
| `list` | 列出记录 | limit(默认10), month(可选 2026-10) |
| `delete` | 删除记录 | id |
| `get` | 查询单条 | id |

#### `budget` — 预算管理

| action | 说明 | 参数 |
| --- | --- | --- |
| `set` | 设置总预算或分类预算 | month(默认本月), category(可选), amount |
| `get` | 查询某月预算 | month |
| `check` | 检查是否超支（对比 finance.json） | month |
| `adjust` | 调整预算（增量，可为负） | month, category(可选), delta |

#### `recurring` — 固定支出

| action | 说明 | 参数 |
| --- | --- | --- |
| `add` | 添加固定支出项 | name, amount, day(1-28), category |
| `list` | 列出所有固定支出 | — |
| `remove` | 删除固定支出项 | id |
| `apply` | 把固定支出计入当月账目（自动去重） | month(默认本月) |

#### `report` — 财务报告

| action | 说明 | 参数 | 图表 |
| --- | --- | --- | --- |
| `monthly` | 月报（含环比） | month(默认本月) | 柱状图 |
| `weekly` | 周报（本周一~周日） | date(默认今天) | 柱状图 |
| `trend` | 月度支出趋势 | months(默认6) | 折线图 |
| `top` | Top 支出 | month(默认本月), n(默认5) | 柱状图 |

#### `plan` — 预算规划

| action | 说明 | 参数 |
| --- | --- | --- |
| `monthly` | 生成下月预算草案 | month(默认本月), strategy |
| `weekly` | 生成下周预算草案（按日均×7） | — |
| `forecast` | 趋势预测（近 N 月均值） | months(默认3) |
| `apply` | 把规划草案写入 budget.json | month(默认下月), strategy |

**strategy 策略：**

| 策略 | 说明 |
| --- | --- |
| `penalty` | 超支扣下月预算，节余不结转（默认） |
| `carryover` | 超支扣，节余结转下月 |
| `average` | 按下月预算 = 本月实际支出重分配 |
| `fixed` | 不变 |

#### `get_current_time` — 当前时间

| format | 说明 |
| --- | --- |
| `datetime` | `2026-11-02 15:04:05`（默认） |
| `date` | `2026-11-02` |
| `time` | `15:04:05` |

---

## 📁 项目结构

```text
finance-agent/
├── go.mod
├── main.go                      # 入口：组装依赖、从 mcp.json 加载 MCP
├── mcp.json                     # MCP Server 配置（Claude Desktop 兼容格式）
├── .gitignore
├── README.md
├── agent/
│   ├── agent.go                 # Agent Loop 核心（Run、元工具、工具过滤）
│   ├── session.go               # 会话历史与压缩
│   ├── event.go                 # 事件定义与 Emitter 接口
│   ├── prompt.go                # 系统提示词
│   └── chart.go                 # 从工具返回值提取 ChartData
├── tools/
│   ├── tool.go                  # ToolDef 抽象与 Call
│   ├── finance.go
│   ├── budget.go
│   ├── recurring.go
│   ├── report.go
│   ├── plan.go
│   └── get_currentTime.go
├── mcp/
│   ├── config.go                # 读取 mcp.json，展开 ${VAR}，跳过 disabled
│   └── client.go                # 连接 MCP Server、动态拉取工具、包装成 ToolDef
├── tui/
│   ├── model.go                 # Bubble Tea Model
│   ├── update.go                # 事件处理、图表渲染（renderBarChart/renderLineChart）
│   ├── view.go                  # 界面布局
│   ├── styles.go                # 样式定义
│   ├── markdown.go              # Markdown 渲染（glamour）
│   └── messages.go              # 对话消息辅助
└── data/                        # 数据文件（首次运行自动创建，gitignore）
    ├── finance.json
    ├── budget.json
    └── recurring.json
```

---

## 📦 数据文件格式

### finance.json（账目明细）

```json
[
  {
    "id": "1",
    "amount": 25,
    "category": "餐饮",
    "note": "午饭",
    "date": "2026-11-02"
  }
]
```

### budget.json（月度预算）

```json
{
  "2026-11": {
    "total": 5000,
    "categories": {
      "餐饮": 500,
      "住房": 2000
    }
  }
}
```

### recurring.json（固定支出）

```json
[
  {
    "id": "1",
    "name": "房租",
    "amount": 2000,
    "day": 1,
    "category": "住房"
  }
]
```

> `data/` 目录已加入 `.gitignore`，首次运行工具时会自动创建对应文件。

---

## 🧩 如何扩展

### 新增一个本地工具

1. 在 `tools/` 下新建文件，例如 `tools/xxx.go`
2. 定义 `NewXxx() *ToolDef`，实现 `Fn` 函数（参照已有工具的写法）
3. 在 [main.go](file:///d:/GoProjects/agent/finance-agent/main.go#L32) 的 `getLlmTools()` 里注册：

   ```go
   m["xxx"] = tools.NewXxx()
   ```

4. 在 [agent/agent.go](file:///d:/GoProjects/agent/finance-agent/agent/agent.go#L356) 的 `isLocalTool()` 里加上工具名（否则会被当作 MCP 工具过滤掉）

   ```go
   localNames := []string{"finance", "budget", "recurring", "report", "plan", "get_current_time", "xxx"}
   ```

5. 可选：在 [agent/prompt.go](file:///d:/GoProjects/agent/finance-agent/agent/prompt.go) 的系统提示词里补充工具用途说明

### 新增一个外部 MCP 工具组

**无需改代码**，只需编辑 `mcp.json`：

```json
{
  "mcpServers": {
    "mygroup": {
      "command": "/path/to/my-mcp-server",
      "args": ["--port", "8080"],
      "env": {
        "MY_TOKEN": "${MY_TOKEN}"
      }
    }
  }
}
```

1. 在 `.env` 里加上 `MY_TOKEN=xxx`
2. 重启程序，启动日志会显示 `✓ 已连接 MCP 组 [mygroup]，加载 N 个工具`
3. 工具名自动带前缀，如 `mygroup__do_something`

### 如何让工具生成图表

图表目前只对 `report` 工具生效。如需让新工具也出图，在 [agent/chart.go](file:///d:/GoProjects/agent/finance-agent/agent/chart.go#L12) 的 `buildChart()` 里添加对应的 `case`，解析工具返回的 JSON 并返回 `*ChartData`。TUI 的 `renderBarChart` / `renderLineChart` 会自动渲染。

---

## ❓ 常见问题

**Q：启动报错"环境变量 ARK_API_KEY 未设置"？**
A：检查项目根目录是否有 `.env` 文件，且内容为 `ARK_API_KEY=你的Key`。注意 `.env` 不会被 git 提交。

**Q：模型返回 404 / 模型不可用？**
A：可能是火山方舟账号下没有该模型 endpoint，或模型不支持 Responses API。把 `agent/agent.go` 里的 `Model` 换成你账号下可用的模型 ID。

**Q：为什么首轮一定要调 load_tool_group？**
A：工具太多会浪费 token，先让模型决策"这次任务需要哪一组工具"，再带上下文调具体工具，更经济。本地工具组始终可用，MCP 工具组按前缀过滤。

**Q：mcp.json 里的 `${ZENMONEY_TOKEN}` 是什么？**
A：环境变量占位符。加载时会自动替换成 `.env`（或系统环境变量）里 `ZENMONEY_TOKEN` 的值。这样密钥不写进 mcp.json，安全且可切换。

**Q：MCP 连接失败怎么办？**
A：启动时会打印 `⚠️ 警告：xxx MCP 连接失败`。检查：1) mcp.json 里的 `command` 路径是否正确；2) 对应环境变量是否在 `.env` 里设置；3) MCP Server 是否能独立运行。连接失败不影响本地工具使用，也可以在 mcp.json 里加 `"disabled": true` 临时跳过。

**Q：如何临时禁用某个 MCP Server？**
A：在 mcp.json 对应条目里加 `"disabled": true`，程序会自动跳过，无需删除或改路径。

**Q：图表不显示？**
A：只有 `report` 工具会生成图表，且要求其返回的 JSON 符合 `agent/chart.go` 里的解析格式。空数据不会出图。

**Q：中文字符截断乱码？**
A：项目用 `[]rune` 截断 + `lipgloss.Width` 计算显示宽度，不会切坏多字节字符。

**Q：如何取消正在跑的 Agent？**
A：目前需 `Ctrl+C` 退出程序。可扩展为用 `context.WithCancel`，在按 `Esc` 时 cancel。

**Q：数据存哪？如何备份？**
A：业务数据全部在 `data/` 下的三个 JSON 文件里。备份就是拷贝这三个文件。它们已在 `.gitignore` 中。

**Q：历史对话会无限增长吗？**
A：不会。消息数超过 60 条时自动用 LLM 做摘要压缩；硬上限 80 条，超过则兜底裁剪最近 3 轮。

---

## 🛠️ 技术栈

| 类别 | 技术 |
| --- | --- |
| 语言 | Go 1.22+ |
| TUI | [Bubble Tea](https://github.com/charmbracelet/bubbletea) + Bubbles + Lipgloss |
| Markdown | [Glamour](https://github.com/charmbracelet/glamour) |
| LLM SDK | [openai-go/v3](https://github.com/openai/openai-go)（Responses API） |
| MCP | [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) |
| MCP 配置 | JSON 文件（Claude Desktop / Cursor 兼容格式） |
| 配置 | [godotenv](https://github.com/joho/godotenv) |
| 存储 | 本地 JSON（无数据库） |
