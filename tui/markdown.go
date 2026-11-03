package tui

import (
    "strings"

    "github.com/charmbracelet/glamour"
)

// renderMarkdown 把 Markdown 字符串渲染成终端能显示的 ANSI 文本
func renderMarkdown(md string, width int) string {
	if width < 20 {
        width = 20
    }

	r,err:=glamour.NewTermRenderer(
        // 使用暗色主题
        glamour.WithStylePath("dark"),
        // 按指定宽度折行
        glamour.WithWordWrap(width),
    )

	if err != nil {
        return md
    }

	out,err:=r.Render(md)
	if err!=nil {
		return md
	}

	// glamour 会在渲染结果的首尾加空行，去掉让排版更紧凑
    return strings.TrimSpace(out)
}