package tools

// 专门负责规划和预测，下月/下周规划

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openai/openai-go/v3"
)

// NewPlan 创建规划工具。
// 职责：根据本月/本周实际支出，生成下月/下周预算草案，并可写入 budget.json。
func NewPlan() *ToolDef {
	return &ToolDef{
		Name: "plan",
		Description: "根据本月/本周实际支出，生成下月/下周预算规划。支持 monthly（月规划）、weekly（周规划）、forecast（趋势预测）、apply（应用规划并写入预算）。" +
			"策略 strategy 可选：penalty（超支扣下月，节余不结转）、carryover（超支扣，节余结转）、average（按实际支出重新分配）、fixed（不变）。",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{
					"type":        "string",
					"description": "操作：monthly、weekly、forecast、apply",
				},
				"month": map[string]string{
					"type":        "string",
					"description": "基准月份，如 2026-10，默认本月。apply 时表示目标月份（要写入的月份）",
				},
				"strategy": map[string]string{
					"type":        "string",
					"description": "规划策略：penalty、carryover、average、fixed，默认 penalty",
				},
				"months": map[string]string{
					"type":        "integer",
					"description": "forecast 时统计近 N 月，默认 3",
				},
			},
			"required": []string{"action"},
		},
		Fn: planFn,
	}
}

func planFn(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	switch action {
	case "monthly":
		return planMonthly(args)
	case "weekly":
		return planWeekly(args)
	case "forecast":
		return planForecast(args)
	case "apply":
		return planApply(args)
	}
	return "", fmt.Errorf("未知 action: %s", action)
}

// ---------- monthly：生成下月预算草案 ----------

func planMonthly(args map[string]any) (string, error) {
	month, _ := args["month"].(string)
	if month == "" {
		month = time.Now().Format("2006-01")

	}
	strategy, _ := args["strategy"].(string)
	if strategy == "" {
		strategy = "penalty"
	}

	records := loadFinance()
	budgets := loadBudgets()

	// 1. 汇总本月实际支出（分类 + 总额）
	spent := map[string]float64{}
	totalSpent := 0.0
	for _, r := range records {
		if len(r.Date) >= 7 && r.Date[:7] == month {
			spent[r.Category] += r.Amount
			totalSpent += r.Amount
		}
	}

	// 2. 读本月预算
	current, ok := budgets[month]
	if !ok {
		current = Budget{Total: 0, Categories: map[string]float64{}}
	}

	// 3. 读下月预算作为基准。如果没有，就以本月预算为基准。
	nextMonth := nextMonthOf(month)
	next, ok := budgets[nextMonth]
	if !ok {
		next = Budget{
			Total:      current.Total,
			Categories: copyMap(current.Categories),
		}
	}

	// 4. 按策略生成草案
	type CatPlan struct {
		Name       string  `json:"name"`
		Current    float64 `json:"current_budget"`
		Spent      float64 `json:"spent"`
		Over       float64 `json:"over"`
		NextBudget float64 `json:"next_budget"`
	}

	var plans []CatPlan
	for name := range next.Categories {
		over := spent[name] - current.Categories[name]
		nextBudget := next.Categories[name]

		switch strategy {
		case "penalty":
			if over > 0 {
				nextBudget = next.Categories[name] - over
				if nextBudget < 0 {
					nextBudget = 0
				}
			}
		case "carryover":
			nextBudget = next.Categories[name] - over
			if nextBudget < 0 {
				nextBudget = 0
			}
		case "average":
			nextBudget = spent[name]
		case "fixed":
			// 不变

		}

		plans = append(plans, CatPlan{
			Name:       name,
			Current:    current.Categories[name],
			Spent:      spent[name],
			Over:       over,
			NextBudget: nextBudget,
		})
	}

	// 总预算按同样策略
	totalOver := totalSpent - current.Total
	totalNext := next.Total

	switch strategy {
	case "penalty", "carryover":
		if totalOver > 0 {
			totalNext = next.Total - totalOver
			if totalNext < 0 {
				totalNext = 0
			}
		}
	case "average":
		totalNext = totalSpent
	case "fixed":
		// 不变
	}

	result := map[string]any{
		"base_month":  month,
		"next_month":  nextMonth,
		"strategy":    strategy,
		"total_spent": totalSpent,
		"total_over":  totalOver,
		"next_total":  totalNext,
		"categories":  plans,
	}

	return jsonMarshal(result), nil

}

// ---------- weekly：生成下周预算草案 ----------
func planWeekly(args map[string]any) (string, error) {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := now.AddDate(0, 0, -(weekday - 1))
	sunday := monday.AddDate(0, 0, 6)

	records := loadFinance()
	spent := map[string]float64{}
	totalSpent := 0.0

	for _, r := range records {
		t, err := time.Parse("2006-01-02", r.Date)
		if err != nil {
			continue
		}
		if !t.Before(monday) && !t.After(sunday) {
			spent[r.Category] += r.Amount
			totalSpent += r.Amount
		}
	}

	// 简单周规划：按天平均，下周总额 = 日均 * 7
	dailyAvg := totalSpent / 7
	nextWeekTotal := dailyAvg * 7

	result := map[string]any{
		"week_start":      monday.Format("2006-01-02"),
		"week_end":        sunday.Format("2006-01-02"),
		"total_spent":     totalSpent,
		"daily_avg":       dailyAvg,
		"next_week_total": nextWeekTotal,
		"categories":      spent,
	}
	return jsonMarshal(result), nil
}

// ---------- forecast：趋势预测 ----------
func planForecast(args map[string]any) (string, error) {
	months := 3
	if v, ok := args["months"].(float64); ok {
		months = int(v)
	}
	records := loadFinance()
	now := time.Now()

	type MonthTotal struct {
		Month string  `json:"month"`
		Total float64 `json:"total"`
	}

	var history []MonthTotal
	sum := 0.0
	for i := months - 1; i >= 0; i-- {
		m := now.AddDate(0, -i, 0).Format("2006-01")
		total := 0.0
		for _, r := range records {
			if len(r.Date) >= 7 && r.Date[:7] == m {
				total += r.Amount
			}
		}
		history = append(history, MonthTotal{
			Month: m,
			Total: total,
		})
		sum += total
	}

	avg := sum / float64(months)
	result := map[string]any{
		"history":       history,
		"average":       avg,
		"forecast_next": avg,
	}
	return jsonMarshal(result), nil
}

// ---------- apply：把规划草案真正写入 budget.json ----------
func planApply(args map[string]any) (string, error) {
	// 1. 确定目标月份（要写入的月份），默认下月
	month, _ := args["month"].(string)
	if month == "" {
		month = nextMonthOf(time.Now().Format("2006-01"))
	}
	strategy, _ := args["strategy"].(string)
	if strategy == "" {
		strategy = "penalty"
	}

	// 2. 以目标月份的上一个月为基准，跑一次 monthly 规划
	baseMonth := prevMonthOf(month)
	draftStr, err := planMonthly(map[string]any{
		"month":    baseMonth,
		"strategy": strategy,
	})
	if err != nil {
		return "", err
	}

	// 3. 解析草案
	var draft struct {
		NextMonth  string  `json:"next_month"`
		NextTotal  float64 `json:"next_total"`
		TotalSpent float64 `json:"total_spent"`
		TotalOver  float64 `json:"total_over"`
		Categories []struct {
			Name       string  `json:"name"`
			NextBudget float64 `json:"next_budget"`
		} `json:"categories"`
	}

	if err := json.Unmarshal([]byte(draftStr), &draft); err != nil {
		return "", err
	}

	// 4. 写入 budget.json
	budgets := loadBudgets()
	newBudget := Budget{
		Total:      draft.NextTotal,
		Categories: map[string]float64{},
	}
	for _, c := range draft.Categories {
		newBudget.Categories[c.Name] = c.NextBudget
	}

	budgets[month] = newBudget
	saveBudgets(budgets)

	// 5. 返回应用结果
	result := map[string]any{
		"applied":    true,
		"month":      month,
		"strategy":   strategy,
		"base_month": baseMonth,
		"total":      draft.NextTotal,
		"categories": newBudget.Categories,
		"total_over": draft.TotalOver,
	}
	return jsonMarshal(result), nil
}

// ---------- 辅助函数 ----------
func nextMonthOf(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 1, 0).Format("2006-01")
}

func prevMonthOf(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return ""
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}

func copyMap(m map[string]float64) map[string]float64 {
	out := map[string]float64{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
