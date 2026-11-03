package tools

// 管理房租、订阅等每月固定支出，支持"一键入账"。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/openai/openai-go/v3"
)

const recurringFile = "data/recurring.json"

// RecurringItem 表示一项固定支出
type RecurringItem struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`     // 名称，如房租
	Amount   float64 `json:"amount"`   // 金额
	Day      int     `json:"day"`      // 每月几号扣款
	Category string  `json:"category"` // 分类
}

func loadRecurring() []RecurringItem {
	b, err := os.ReadFile(recurringFile)
	if err != nil {
		return []RecurringItem{}
	}
	var items []RecurringItem
	json.Unmarshal(b, &items)
	return items
}

func saveRecurring(items []RecurringItem) {
	os.MkdirAll("data", 0755)
	b, _ := json.MarshalIndent(items, "", "  ")
	os.WriteFile(recurringFile, b, 0644)
}

func NewRecurring() *ToolDef {
	return &ToolDef{
		Name:        "recurring",
		Description: "管理每月固定支出，如房租、会员、订阅。支持 add、list、remove、apply（把固定支出计入当月账目）。",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{
					"type":        "string",
					"description": "操作：add、list、remove、apply",
				},
				"name": map[string]string{
					"type":        "string",
					"description": "名称，如房租、Netflix",
				},
				"amount": map[string]string{
					"type":        "number",
					"description": "金额",
				},
				"day": map[string]string{
					"type":        "integer",
					"description": "每月几号扣款，1-28",
				},
				"category": map[string]string{
					"type":        "string",
					"description": "分类",
				},
				"id": map[string]string{
					"type":        "string",
					"description": "remove 时的 ID",
				},
				"month": map[string]string{
					"type":        "string",
					"description": "apply 时的月份，如 2026-10，默认本月",
				},
			},
			"required": []string{"action"},
		},
		Fn: recurringFn,
	}
}

func recurringFn(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	items := loadRecurring()

	switch action {
	case "add":
		name, _ := args["name"].(string)
		amount, _ := args["amount"].(float64)
		day := 1
		if v, ok := args["day"].(float64); ok {
			day = int(v)
		}
		category, _ := args["category"].(string)

		item := RecurringItem{
			ID:       strconv.Itoa(len(items) + 1),
			Name:     name,
			Amount:   amount,
			Day:      day,
			Category: category,
		}
		items = append(items, item)

		saveRecurring(items)
		return jsonMarshal(item), nil

	case "list":
		return jsonMarshal(items), nil

	case "remove":
		id, _ := args["id"].(string)
		for i, item := range items {
			if item.ID == id {
				items = append(items[:i], items[i+1:]...)
				saveRecurring(items)
				return fmt.Sprintf(`{"removed":"%s"}`, id), nil
			}
		}
		return "", fmt.Errorf("未找到 id=%s", id)

	case "apply":
		//把固定支出写入 finance.json
		month, _ := args["month"].(string)
		if month == "" {
			month = time.Now().Format("2006-01")
		}
		records := loadFinance()
		applied := 0
		for _, item := range items {
			date := fmt.Sprintf("%s-%02d", month, item.Day)
			// 去重：如果本月已入账，跳过
			exists := false
			for _, r := range records {
				if r.Date == date && r.Note == item.Name {
					exists = true
					break
				}
			}
			if !exists {
				records = append(records, FinanceRecord{
					ID:       strconv.Itoa(len(records) + 1),
					Amount:   item.Amount,
					Category: item.Category,
					Note:     item.Name,
					Date:     date,
				})
				applied++
			}
		}
		saveFinance(records)
		return fmt.Sprintf(`{"applied":%d,"month":"%s"}`, applied, month), nil
	}

	return "", fmt.Errorf("未知 action: %s", action)
}
