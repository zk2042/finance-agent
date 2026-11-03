package tools

// 做数据加工，返回结构化报告。

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/openai/openai-go/v3"
)

func NewReport() *ToolDef {
	return &ToolDef{
		Name:        "report",
		Description: "生成财务报告。支持 monthly（月报）、weekly（周报）、trend（趋势）、top（Top 支出）。",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{
					"type":        "string",
					"description": "报告类型：monthly、weekly、trend、top",
				},
				"month": map[string]string{
					"type":        "string",
					"description": "月份，如 2026-10，默认本月",
				},
				"date": map[string]string{
					"type":        "string",
					"description": "周报的基准日期，默认今天",
				},
				"months": map[string]string{
					"type":        "integer",
					"description": "trend 时统计近 N 个月，默认 6",
				},
				"n": map[string]string{
					"type":        "integer",
					"description": "top 时返回前 N 项，默认 5",
				},
			},
			"required": []string{"action"},
		},
		Fn: reportFn,
	}
}

func reportFn(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	records := loadFinance()

	switch action {
	case "monthly":
		month, _ := args["month"].(string)
		if month == "" {
			month = time.Now().Format("2006-01")
		}

		// 1. 汇总本月支出
		total := 0.0
		catMap := map[string]float64{}
		for _, r := range records {
			if len(r.Date) >= 7 && r.Date[:7] == month {
				total += r.Amount
				catMap[r.Category] += r.Amount
			}
		}

		// 2. 计算分类占比
		type CatItem struct {
			Name    string  `json:"name"`
			Amount  float64 `json:"amount"`
			Percent float64 `json:"percent"`
		}
		var cats []CatItem
		for name, amount := range catMap {
			pct := 0.0
			if total > 0 {
				pct = amount / total * 100

			}
			cats = append(cats, CatItem{name, amount, pct})
		}
		sort.Slice(cats, func(i, j int) bool {
			return cats[i].Amount > cats[j].Amount
		})

		// 3. 环比：读上个月
		prevMonth := getPrevMonth(month)
		prevTotal := 0.0
		for _, r := range records {
			if len(r.Date) >= 7 && r.Date[:7] == prevMonth {
				prevTotal += r.Amount
			}
		}
		momChange := 0.0
		if prevTotal > 0 {
			momChange = (total - prevTotal) / prevTotal * 100
		}

		result := map[string]any{
			"month":      month,
			"total":      total,
			"categories": cats,
			"prev_total": prevTotal,
			"mom_change": momChange,
		}
		return jsonMarshal(result), nil

	case "weekly":
		dateStr, _ := args["date"].(string)
		if dateStr == "" {
			dateStr = time.Now().Format("2006-01-02")
		}
		target, _ := time.Parse("2006-01-02", dateStr)

		// 本周一
		weekday := int(target.Weekday())
		if weekday == 0 {
			weekday = 7 // 周日算第 7 天
		}
		monday := target.AddDate(0, 0, -(weekday - 1))
		sunday := monday.AddDate(0, 0, 6)

		total := 0.0
		catMap := map[string]float64{}
		for _, r := range records {
			t, err := time.Parse("2006-01-02", r.Date)
			if err != nil {
				continue
			}
			if !t.Before(monday) && !t.After(sunday) {
				total += r.Amount
				catMap[r.Category] += r.Amount
			}
		}

		result := map[string]any{
			"start":      monday.Format("2006-01-02"),
			"end":        sunday.Format("2006-01-02"),
			"total":      total,
			"daily_avg":  total / 7,
			"categories": catMap,
		}
		return jsonMarshal(result), nil

	case "trend":
		months := 6
		if v, ok := args["months"].(float64); ok {
			months = int(v)
		}
		now := time.Now()
		type MonthTotal struct {
			Month string  `json:"month"`
			Total float64 `json:"total"`
		}

		var trend []MonthTotal
		for i := months - 1; i >= 0; i-- {
			m := now.AddDate(0, -i, 0).Format("2006-01")
			total := 0.0
			for _, r := range records {
				if len(r.Date) >= 7 && r.Date[:7] == m {
					total += r.Amount
				}
			}
			trend = append(trend, MonthTotal{m, total})
		}
		return jsonMarshal(trend), nil

	case "top":
		month, _ := args["month"].(string)
		if month == "" {
			month = time.Now().Format("2006-01")
		}
		n := 5
		if v, ok := args["n"].(float64); ok {
			n = int(v)
		}

		var monthRecords []FinanceRecord
		for _, r := range records {
			if len(r.Date) >= 7 && r.Date[:7] == month {
				monthRecords = append(monthRecords, r)
			}
		}

		sort.Slice(monthRecords, func(i, j int) bool {
			return monthRecords[i].Amount > monthRecords[j].Amount
		})

		if n > len(monthRecords) {
			n = len(monthRecords)
		}
		return jsonMarshal(monthRecords[:n]), nil
	}

	return "", fmt.Errorf("未知 action: %s", action)

}

// getPrevMonth 返回上一个月，如 "2026-10" → "2026-09"
func getPrevMonth(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}
