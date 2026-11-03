package mcp

// 负责从 mcp.json 读取 MCP Server 配置
// 格式与 Claude Desktop / Cursor 的 mcp.json 保持一致
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// mcpConfigFile 是 mcp.json 的顶层结构
type mcpConfigFile struct {
	// 键 = group 名，值 = 该 Server 的启动参数
	MCPServers map[string]serverEntry `json:"mcpServers"`
}

// serverEntry 对应 mcp.json 里单个 Server 的配置
type serverEntry struct {
	Command  string            `json:"command"`
	Args     []string          `json:"args,omitempty"`
	Env      map[string]string `json:"env,omitempty"`
	Disabled bool              `json:"disabled,omitempty"` // 项目扩展字段：为 true 时跳过
}

// LoadConfig 读取 mcp.json，把每个 Server 转成 MCPServerConfig
//
// 处理规则：
//  1. 文件不存在 → 返回 error（调用方自己决定是警告还是忽略）
//  2. env 值里的 ${VAR} / $VAR 会展开成对应环境变量，变量没设则打警告并展开为空
//  3. disabled=true 的条目直接跳过
//  4. command 为空的条目跳过（避免 exec 直接失败）
func LoadConfig(path string) ([]MCPServerConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw mcpConfigFile
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("解析 %s 失败：%w", path, err)
	}
	if len(raw.MCPServers) == 0 {
		return nil, nil
	}

	cfgs := make([]MCPServerConfig, 0, len(raw.MCPServers))
	for group, entry := range raw.MCPServers {
		if entry.Disabled {
			continue
		}
		if strings.TrimSpace(entry.Command) == "" {
			fmt.Printf("⚠️ 警告：mcp.json 中 '%s' 的 command 为空，已跳过\n", group)
			continue
		}

		// 展开 env 里的 ${VAR}
		env := make(map[string]string, len(entry.Env))
		for k, v := range entry.Env {
			env[k] = expandEnv(v)
		}

		cfgs = append(cfgs, MCPServerConfig{
			Group:   group,
			Command: entry.Command,
			Args:    entry.Args,
			Env:     env,
		})
	}
	return cfgs, nil
}

// expandEnv 展开字符串里的 ${VAR} 和 $VAR
func expandEnv(s string) string {
	if !strings.Contains(s, "$") {
		return s
	}
	return os.Expand(s, func(key string) string {
		val, ok := os.LookupEnv(key)
		if !ok {
			fmt.Printf("⚠️ 警告：环境变量 %s 未设置（mcp.json 引用了它），相关 MCP 可能会失败\n", key)
			return ""
		}
		return val
	})
}
