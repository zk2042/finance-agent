package tools

// 设置预算、检查超支、调整预算。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/openai/openai-go/v3"
)

const budgetFile = "data/budget.json"

// Budget 表示某个月的预算
type Budget struct {
	Total      float64            `json:"total"`
	Categories map[string]float64 `json:"categories"`
}

// budgets 的结构：月份 → Budget
// 比如 {"2026-10": {"total": 5000, "categories": {"餐饮": 1500}}}
type BudgetMap map[string]Budget

// loadBudgets 从文件中加载预算数据
func loadBudgets() BudgetMap {
	b, err := os.ReadFile(budgetFile)
	if err != nil {
		return BudgetMap{}
	}

	var m BudgetMap
	json.Unmarshal(b, &m)
	return m
}

func saveBudgets(m BudgetMap) {
	os.MkdirAll("data", 0755)
	b, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(budgetFile, b, 0644)
}

func NewBudget() *ToolDef {
	return &ToolDef{
		Name:        "budget",
		Description: "管理月度预算。支持 set（设置预算）、get（查询预算）、check（检查是否超支）、adjust（调整预算）。",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{
					"type":        "string",
					"description": "操作：set、get、check、adjust",
				},
				"month": map[string]string{
					"type":        "string",
					"description": "月份，如 2026-10，默认本月",
				},
				"category": map[string]string{
					"type":        "string",
					"description": "分类，不传表示总预算",
				},
				"amount": map[string]string{
					"type":        "number",
					"description": "金额，set 时必填",
				},
				"delta": map[string]string{
					"type":        "number",
					"description": "增量，adjust 时必填，可为负",
				},
			},
			"required": []string{"action"},
		},
		Fn: budgetFn,
	}
}

func budgetFn(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	month, _ := args["month"].(string)
	if month == "" {
		month = time.Now().Format("2006-01") // 默认本月
	}
	category, _ := args["category"].(string)

	budgets := loadBudgets()
	b, ok := budgets[month]
	if !ok {
		b = Budget{Total: 0, Categories: map[string]float64{}}
	}

	switch action {
	case "set":
		amount, _ := args["amount"].(float64)
		if category == "" {
			b.Total = amount
		} else {
			b.Categories[category] = amount
		}

		budgets[month] = b
		saveBudgets(budgets)
		return fmt.Sprintf(`{"month":"%s","category":"%s","amount":%.2f}`, month, category, amount), nil

	case "get":
		return jsonMarshal(b), nil

	case "check":
		// 跨文件读取 finance.json，计算实际支出
		records := loadFinance()
		spent := map[string]float64{}
		totalSpent := 0.0

		for _, r := range records {
			if len(r.Date) >= 7 && r.Date[:7] == month {
				spent[r.Category] += r.Amount
				totalSpent += r.Amount
			}
		}

		// 对比预算与实际
		type CatResult struct {
			Name   string  `json:"name"`
			Budget float64 `json:"budget"`
			Spent  float64 `json:"spent"`
			Over   bool    `json:"over"`
		}

		var cats []CatResult
		for name, budget := range b.Categories {
			cats = append(cats, CatResult{
				Name:   name,
				Budget: budget,
				Spent:  spent[name],
				Over:   spent[name] > budget,
			})
		}

		result := map[string]any{
			"month":        month,
			"total_budget": b.Total,
			"total_spent":  totalSpent,
			"over_budget":  totalSpent > b.Total,
			"categories":   cats,
		}

		return jsonMarshal(result), nil

	case "adjust":
		delta, _ := args["delta"].(float64)
		if category == "" {
			b.Total += delta
		} else {
			b.Categories[category] += delta
		}

		budgets[month] = b
		saveBudgets(budgets)
		return fmt.Sprintf(`{"month":"%s","category":"%s","new_total":%.2f}`, month, category, b.Total), nil
	}

	return "", fmt.Errorf("未知 action: %s", action)
}
