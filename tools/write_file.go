package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/openai/openai-go/v3"
)

// NewWriteFile 写入文本文件（网页/报告/脚本），路径被限制在项目目录内
func NewWriteFile(root string) *ToolDef {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	return &ToolDef{
		Name:        "write_file",
		Description: "把文本内容写入文件（用于生成网页、报告、脚本）。path 为相对项目根目录的路径，如 output/autumn.html；会自动创建目录，写入范围限制在项目目录内",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]string{
					"type":        "string",
					"description": "文件路径，相对项目根目录，如 output/beijing-autumn.html",
				},
				"content": map[string]string{
					"type":        "string",
					"description": "完整的文件内容（HTML 要包含 <!DOCTYPE html> 与内联样式）",
				},
			},
			"required": []string{"path", "content"},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			path = strings.TrimSpace(path)
			if path == "" {
				return "", fmt.Errorf("参数 path 必填")
			}
			if content == "" {
				return "", fmt.Errorf("参数 content 不能为空")
			}

			// 只允许写到项目目录内，防止越权写系统文件
			target := path
			if !filepath.IsAbs(target) {
				target = filepath.Join(absRoot, path)
			}
			abs, err := filepath.Abs(target)
			if err != nil {
				return "", err
			}
			if !isWithin(absRoot, abs) {
				return "", fmt.Errorf("路径 %s 超出项目目录，已拒绝写入", path)
			}

			if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
				return "", err
			}
			if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
				return "", err
			}
			rel, _ := filepath.Rel(absRoot, abs)
			return fmt.Sprintf("已写入 %s（%d 字节）", filepath.ToSlash(rel), len(content)), nil
		},
	}
}

// isWithin 判断 path 是否位于 root 目录之内
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
