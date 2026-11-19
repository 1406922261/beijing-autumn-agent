package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openai/openai-go/v3"
)

// Skill 一个本地技能：目录 skills/<name>/SKILL.md，头部用 --- 包裹的 frontmatter 描述名称与用途
type Skill struct {
	Name        string // 技能名，如 webpage
	Description string // 一句话用途，供模型判断何时使用
	Dir         string // 技能目录绝对路径
	Body        string // SKILL.md 正文（去掉 frontmatter）
}

// LoadSkills 扫描 skills 目录，加载全部技能；目录不存在时返回空列表
func LoadSkills(root string) ([]Skill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		f, err := os.Open(filepath.Join(dir, "SKILL.md"))
		if err != nil {
			continue // 没有 SKILL.md 的目录不算技能
		}
		raw, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
		f.Close()
		if err != nil {
			return nil, err
		}
		name, desc, body := parseSkillFile(string(raw))
		if name == "" {
			name = e.Name()
		}
		abs, _ := filepath.Abs(dir)
		skills = append(skills, Skill{Name: name, Description: desc, Dir: abs, Body: body})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

// parseSkillFile 解析 --- 包裹的 frontmatter，返回 (name, description, 正文)
func parseSkillFile(raw string) (string, string, string) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(strings.TrimSpace(raw), "---") {
		return "", "", strings.TrimSpace(raw)
	}
	lines := strings.Split(raw, "\n")
	// 跳过开头的空行，找到第一行 ---
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(lines[i]) != "---" {
		return "", "", strings.TrimSpace(raw)
	}
	var name, desc string
	j := i + 1
	for ; j < len(lines); j++ {
		line := strings.TrimSpace(lines[j])
		if line == "---" {
			break
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(strings.Trim(strings.TrimSpace(val), `"'`))
		switch strings.TrimSpace(key) {
		case "name":
			name = val
		case "description":
			desc = val
		}
	}
	body := ""
	if j+1 < len(lines) {
		body = strings.TrimSpace(strings.Join(lines[j+1:], "\n"))
	}
	return name, desc, body
}

// NewListSkills 列出可用技能
func NewListSkills(skills []Skill) *ToolDef {
	return &ToolDef{
		Name:        "list_skills",
		Description: "列出当前可用的本地技能（Skill）及其用途，用于判断该不该加载某个技能",
		Params: openai.FunctionParameters{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			if len(skills) == 0 {
				return "当前没有可用技能（skills 目录为空）。", nil
			}
			var b strings.Builder
			fmt.Fprintf(&b, "共 %d 个技能：\n", len(skills))
			for _, s := range skills {
				fmt.Fprintf(&b, "- %s：%s\n", s.Name, s.Description)
			}
			return b.String(), nil
		},
	}
}

// NewLoadSkill 读取指定技能的完整说明，模型拿到后按说明执行
func NewLoadSkill(skills []Skill) *ToolDef {
	names := make([]string, 0, len(skills))
	for _, s := range skills {
		names = append(names, s.Name)
	}
	return &ToolDef{
		Name:        "load_skill",
		Description: "加载指定技能的完整操作说明（SKILL.md 正文）。需要写网页、做海报等专项任务时先调用它拿到步骤，再按步骤执行。可用技能：" + strings.Join(names, "、"),
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]string{
					"type":        "string",
					"description": "技能名，如 webpage；可用值：" + strings.Join(names, "、"),
				},
			},
			"required": []string{"name"},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			name, _ := args["name"].(string)
			name = strings.TrimSpace(name)
			for _, s := range skills {
				if s.Name == name {
					var b strings.Builder
					fmt.Fprintf(&b, "技能：%s\n用途：%s\n目录：%s\n\n%s\n", s.Name, s.Description, s.Dir, s.Body)
					return b.String(), nil
				}
			}
			return "", fmt.Errorf("没有名为 %s 的技能，可用：%s", name, strings.Join(names, "、"))
		},
	}
}

// SkillsPromptBlock 生成注入系统提示词的技能目录
func SkillsPromptBlock(skills []Skill) string {
	if len(skills) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n【技能（Skill）】当前已安装以下本地技能：\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- %s：%s\n", s.Name, s.Description)
	}
	b.WriteString("使用规则：\n")
	b.WriteString("- 当用户的需求匹配某个技能（如「写个网页」「做张海报」）时，先调用 load_skill 读取该技能的完整步骤，再严格照做\n")
	b.WriteString("- 执行技能时用 write_file 落盘、用 exec_shell 跑脚本；不要在没加载技能的情况下凭感觉乱写\n")
	b.WriteString("- 技能里有明确的产物要求（路径、格式），必须实际生成文件，不能只回复文字\n")
	return b.String()
}
