package agent

import (
	"encoding/json"
	"fmt"
	"sort"
)

// 负责"从工具返回值里提取图表数据"

// buildChart 根据工具名、调用参数、返回值判断能否生成图表
func buildChart(toolName, argsJSON, result string) *ChartData {
	// 目前只处理 report 工具
	if toolName != "report" {
		return nil
	}

	// 解析调用参数，拿到 action
	var args map[string]any

	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return nil
	}

	action, _ := args["action"].(string)

	// 根据 action 分派到不同的解析函数
	switch action {
	case "monthly":
		return buildMonthlyChart(result)
	case "weekly":
		return buildWeeklyChart(result)
	case "trend":
		return buildTrendChart(result)
	case "top":
		return buildTopChart(result)

	}

	return nil
}

// ---------- 具体解析函数 ----------
// buildMonthlyChart 解析 report.monthly 的返回值
func buildMonthlyChart(result string) *ChartData {
	var data struct {
		Month      string  `json:"month"`
		Total      float64 `json:"total"`
		Categories []struct {
			Name   string  `json:"name"`
			Amount float64 `json:"amount"`
		} `json:"categories"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil
	}
	if len(data.Categories) == 0 {
		return nil
	}

	// 1. 过滤并排序
	var validCats []struct {
		Name   string
		Amount float64
	}
	for _, c := range data.Categories {
		if c.Amount > 0 {
			validCats = append(validCats, struct {
				Name   string
				Amount float64
			}{c.Name, c.Amount})
		}
	}
	sort.Slice(validCats, func(i, j int) bool {
		return validCats[i].Amount > validCats[j].Amount
	})

	// 2. 限制最多显示 8 个分类
	if len(validCats) > 8 {
		validCats = validCats[:8]
	}

	cd := &ChartData{
		Kind:  "bar",
		Title: fmt.Sprintf("%s 支出分类 (Top %d)", data.Month, len(validCats)),
	}

	for _, c := range validCats {
		cd.Labels = append(cd.Labels, c.Name)
		cd.Values = append(cd.Values, c.Amount)
	}
	return cd
}

// buildWeeklyChart 解析 report.weekly 的返回值
func buildWeeklyChart(result string) *ChartData {
	var data struct {
		Start      string             `json:"start"`
		End        string             `json:"end"`
		Categories map[string]float64 `json:"categories"`
	}
	if err := json.Unmarshal([]byte(result), &data); err != nil {
		return nil
	}
	if len(data.Categories) == 0 {
		return nil
	}

	// 1. Map 转 Slice 并排序
	type catItem struct {
		Name   string
		Amount float64
	}
	var validCats []catItem
	for name, amount := range data.Categories {
		if amount > 0 {
			validCats = append(validCats, catItem{name, amount})
		}
	}
	sort.Slice(validCats, func(i, j int) bool {
		return validCats[i].Amount > validCats[j].Amount
	})

	// 2. 限制最多显示 8 个分类
	if len(validCats) > 8 {
		validCats = validCats[:8]
	}

	cd := &ChartData{
		Kind:  "bar",
		Title: fmt.Sprintf("%s ~ %s 支出分类 (Top %d)", data.Start, data.End, len(validCats)),
	}
	for _, c := range validCats {
		cd.Labels = append(cd.Labels, c.Name)
		cd.Values = append(cd.Values, c.Amount)
	}
	return cd
}

// buildTrendChart 解析 report.trend 的返回值
func buildTrendChart(result string) *ChartData {
	var points []struct {
		Month string  `json:"month"`
		Total float64 `json:"total"`
	}
	if err := json.Unmarshal([]byte(result), &points); err != nil {
		return nil
	}
	if len(points) == 0 {
		return nil
	}

	cd := &ChartData{
		Kind:  "line",
		Title: "月度支出趋势",
	}

	for _, p := range points {
		cd.Labels = append(cd.Labels, p.Month)
		cd.Values = append(cd.Values, p.Total)
	}
	return cd
}

// buildTopChart 解析 report.top 的返回值
func buildTopChart(result string) *ChartData {
	var records []struct {
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := json.Unmarshal([]byte(result), &records); err != nil {
		return nil
	}
	if len(records) == 0 {
		return nil
	}

	cd := &ChartData{
		Kind:  "bar",
		Title: "Top 支出",
	}

	for _, r := range records {
		if r.Amount <= 0 {
			continue
		}
		// Note 太长会撑爆图表，截断到 8 个字
		label := r.Note
		if label == "" {
			label = "(无备注)"
		}
		runes := []rune(label)
		if len(runes) > 8 {
			label = string(runes[:8]) + "..."
		}
		cd.Labels = append(cd.Labels, label)
		cd.Values = append(cd.Values, r.Amount)
	}
	if len(cd.Labels) == 0 {
		return nil
	}

	return cd
}
