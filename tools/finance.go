package tools

// 管理账目，支持 add、list、delete、get。

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/openai/openai-go/v3"
)

const financeFile = "data/finance.json"

// FinanceRecord 表示一笔账目
type FinanceRecord struct {
	ID       string  `json:"id"`
	Amount   float64 `json:"amount"`
	Category string  `json:"category"`
	Note     string  `json:"note"`
	Date     string  `json:"date"`
}

// loadFinance 从文件读取所有记录
func loadFinance() []FinanceRecord {
	b, err := os.ReadFile(financeFile)
	if err != nil {
		return []FinanceRecord{}
	}
	var records []FinanceRecord
	json.Unmarshal(b, &records)
	return records
}

// saveFinance 把所有记录写回文件
func saveFinance(records []FinanceRecord) {
	os.MkdirAll("data", 0755)
	b, _ := json.MarshalIndent(records, "", "  ")
	os.WriteFile(financeFile, b, 0644)

}

// jsonMarshal 辅助函数：把任意值转成 JSON 字符串
func jsonMarshal(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// NewFinance 创建 finance 工具
func NewFinance() *ToolDef {
	return &ToolDef{
		Name:        "finance",
		Description: "管理个人记账。支持 add（添加账目）、list（列出记录）、delete（删除记录）、get（按 ID 查询单条）。",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]string{
					"type":        "string",
					"description": "操作类型：add、list、delete、get",
				},
				"amount": map[string]string{
					"type":        "number",
					"description": "金额，add 时必填，单位元",
				},
				"category": map[string]string{
					"type":        "string",
					"description": "分类，如餐饮、交通、购物、住房、娱乐",
				},
				"note": map[string]string{
					"type":        "string",
					"description": "备注，如午饭、打车",
				},
				"date": map[string]string{
					"type":        "string",
					"description": "日期 YYYY-MM-DD，不传默认今天",
				},
				"id": map[string]string{
					"type":        "string",
					"description": "记录 ID，delete 和 get 时必填",
				},
				"limit": map[string]string{
					"type":        "integer",
					"description": "list 返回条数，默认 10",
				},
				"month": map[string]string{
					"type":        "string",
					"description": "list 时可选，筛选月份，如 2026-10",
				},
			},
			"required": []string{"action"},
		},

		Fn: financeFn, // 具体逻辑单独写
	}
}

// financeFn 是 finance 工具的实际执行函数
func financeFn(ctx context.Context, args map[string]any) (string, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return "", fmt.Errorf("finance: 缺少 action 参数")
	}

	records := loadFinance()

	switch action {
	case "add":
		// 1. 解析参数
		amount, _ := args["amount"].(float64)
		category, _ := args["category"].(string)
		note, _ := args["note"].(string)
		date, _ := args["date"].(string)
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}

		// 2. 创建记录，ID 用当前记录数 +1（简单实现）
		rec := FinanceRecord{
			ID:       strconv.Itoa(len(records) + 1),
			Amount:   amount,
			Category: category,
			Note:     note,
			Date:     date,
		}

		// 3. 追加并保存
		records = append(records, rec)
		saveFinance(records)

		// 4. 返回 JSON 字符串给 LLM
		return jsonMarshal(rec), nil

	case "list":
		// 1. 解析 limit 和 month
		limit := 10
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		month, _ := args["month"].(string)

		// 2. 按月份筛选
		var filtered []FinanceRecord
		for _, r := range records {
			if month != "" && len(r.Date) >= 7 && r.Date[:7] != month {
				continue
			}
			filtered = append(filtered, r)
		}

		// 3. 限制条数
		if limit > len(filtered) {
			limit = len(filtered)
		}
		return jsonMarshal(filtered[:limit]), nil

	case "delete":
		// 1. 找到目标 ID
		id, _ := args["id"].(string)
		for i, r := range records {
			if r.ID == id {
				// 2. 用切片操作删除第 i 个元素
				records = append(records[:i], records[i+1:]...)
				saveFinance(records)
				return fmt.Sprintf(`{"deleted":"%s"}`, id), nil
			}
		}
		return "", fmt.Errorf("未找到 id=%s", id)

	case "get":
		id, _ := args["id"].(string)
		for _, r := range records {
			if r.ID == id {
				return jsonMarshal(r), nil
			}
		}

		return "", fmt.Errorf("未找到 id=%s", id)
	}
	return "", fmt.Errorf("未知操作: %s", action)
}
