package tools

import (
	"context"
	"fmt"
	"strings"

	"tui/beijingAutumn/core"

	"github.com/openai/openai-go/v3"
)

// NewMemorySave 把值得长期记住的信息写入长期记忆（memory/facts.jsonl）
func NewMemorySave(mem *core.Memory) *ToolDef {
	return &ToolDef{
		Name:        "memory_save",
		Description: "把值得跨会话长期记住的信息写入长期记忆文件。适合记录用户偏好（喜欢人少/免费/带娃）、常去的景点、忌讳等；一句话一条，不要写寒暄",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"text": map[string]string{
					"type":        "string",
					"description": "要记住的一句话事实，例如「用户偏好免费且人少的赏秋地点」",
				},
			},
			"required": []string{"text"},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			text, _ := args["text"].(string)
			if strings.TrimSpace(text) == "" {
				return "", fmt.Errorf("参数 text 必填")
			}
			if err := mem.AddFact(text); err != nil {
				return "", err
			}
			return "已写入长期记忆：" + text, nil
		},
	}
}

// NewMemoryRecall 在长期记忆里检索历史记录与事实
func NewMemoryRecall(mem *core.Memory) *ToolDef {
	return &ToolDef{
		Name:        "memory_recall",
		Description: "检索长期记忆：按关键词查历史对话记录与已保存的用户事实；query 留空则返回最近若干条",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{
					"type":        "string",
					"description": "可选，关键词，如 香山、住宿、带娃；留空返回最近的记忆",
				},
				"limit": map[string]string{
					"type":        "integer",
					"description": "可选，返回条数，默认 5",
				},
			},
			"required": []string{},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			query, _ := args["query"].(string)
			limit := 5
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
			}
			lines := mem.Recall(query, limit)
			if len(lines) == 0 {
				return fmt.Sprintf("长期记忆里没有和「%s」相关的记录。", query), nil
			}
			return strings.Join(lines, "\n"), nil
		},
	}
}
