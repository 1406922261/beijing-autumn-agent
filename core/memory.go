package core

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Record 一轮对话的持久化记录
type Record struct {
	Time   string   `json:"time"`
	Menu   string   `json:"menu,omitempty"`
	Spot   string   `json:"spot,omitempty"`
	Input  string   `json:"input"`
	Tools  []string `json:"tools,omitempty"`
	Answer string   `json:"answer"`
}

// Fact 模型主动写下的长期事实（用户偏好、常去景点等）
type Fact struct {
	Time string `json:"time"`
	Text string `json:"text"`
}

// memoryMeta 记录「已经摘要到第几条」，用于算增量
type memoryMeta struct {
	SummarizedRecords int `json:"summarized_records"`
}

// Memory 长期记忆：摘要 + 逐轮记录 + 用户事实，全部持久化到磁盘
type Memory struct {
	dir     string
	summary string
	records []Record
	facts   []Fact
	meta    memoryMeta
}

// NewMemory 创建记忆对象，dir 为记忆目录（如 memory）
func NewMemory(dir string) *Memory {
	return &Memory{dir: dir}
}

// Dir 返回记忆目录
func (m *Memory) Dir() string { return m.dir }

// Load 从磁盘读取摘要、记录与事实；文件不存在时视为空记忆
func (m *Memory) Load() error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	if b, err := os.ReadFile(m.summaryPath()); err == nil {
		m.summary = strings.TrimSpace(string(b))
	}
	if err := m.loadJSONL(m.recordsPath(), func(line []byte) error {
		var r Record
		if err := json.Unmarshal(line, &r); err != nil {
			return nil // 跳过坏行，不让一条脏数据毁掉整份记忆
		}
		m.records = append(m.records, r)
		return nil
	}); err != nil {
		return err
	}
	if err := m.loadJSONL(m.factsPath(), func(line []byte) error {
		var f Fact
		if err := json.Unmarshal(line, &f); err != nil {
			return nil
		}
		m.facts = append(m.facts, f)
		return nil
	}); err != nil {
		return err
	}
	if b, err := os.ReadFile(m.metaPath()); err == nil {
		_ = json.Unmarshal(b, &m.meta)
	}
	return nil
}

// Summary 返回当前长期记忆摘要
func (m *Memory) Summary() string { return m.summary }

// Count 返回历史记录总条数
func (m *Memory) Count() int { return len(m.records) }

// Facts 返回已保存的事实
func (m *Memory) Facts() []Fact { return m.facts }

// Pending 返回自上次生成摘要以来新增的记录数
func (m *Memory) Pending() int {
	if n := len(m.records) - m.meta.SummarizedRecords; n > 0 {
		return n
	}
	return 0
}

// Append 追加一条对话记录并立即落盘
func (m *Memory) Append(r Record) error {
	if r.Time == "" {
		r.Time = time.Now().Format("2006-01-02 15:04:05")
	}
	m.records = append(m.records, r)
	return m.appendJSONL(m.recordsPath(), r)
}

// AddFact 追加一条长期事实并立即落盘
func (m *Memory) AddFact(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("事实内容不能为空")
	}
	f := Fact{Time: time.Now().Format("2006-01-02 15:04:05"), Text: text}
	m.facts = append(m.facts, f)
	return m.appendJSONL(m.factsPath(), f)
}

// SaveSummary 覆盖写入摘要，并把已摘要进度推进到当前记录数
func (m *Memory) SaveSummary(summary string) error {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil
	}
	m.summary = summary
	if err := os.WriteFile(m.summaryPath(), []byte(summary+"\n"), 0o644); err != nil {
		return err
	}
	m.meta.SummarizedRecords = len(m.records)
	b, err := json.MarshalIndent(m.meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.metaPath(), b, 0o644)
}

// PendingRecords 返回尚未并入摘要的记录
func (m *Memory) PendingRecords() []Record {
	from := m.meta.SummarizedRecords
	if from >= len(m.records) {
		return nil
	}
	return m.records[from:]
}

// RecentRecords 返回最近 n 条记录
func (m *Memory) RecentRecords(n int) []Record {
	if n <= 0 || n > len(m.records) {
		n = len(m.records)
	}
	return m.records[len(m.records)-n:]
}

// Recall 在事实与历史记录里做关键词检索，返回最相关的若干条文本
func (m *Memory) Recall(query string, limit int) []string {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 5
	}
	var out []string
	for i := len(m.facts) - 1; i >= 0; i-- {
		f := m.facts[i]
		if query == "" || strings.Contains(f.Text, query) {
			out = append(out, fmt.Sprintf("[%s] 事实：%s", f.Time, f.Text))
		}
		if len(out) >= limit {
			return out
		}
	}
	for i := len(m.records) - 1; i >= 0; i-- {
		r := m.records[i]
		hit := query == "" || strings.Contains(r.Input, query) || strings.Contains(r.Answer, query) || strings.Contains(r.Spot, query)
		if !hit {
			continue
		}
		out = append(out, fmt.Sprintf("[%s] 问：%s｜答：%s", r.Time, clip(r.Input, 40), clip(r.Answer, 80)))
		if len(out) >= limit {
			break
		}
	}
	return out
}

// PromptBlock 生成注入系统提示词的长期记忆文本
func (m *Memory) PromptBlock() string {
	var b strings.Builder
	b.WriteString("\n\n【长期记忆（来自 memory/ 目录，跨会话有效）】\n")
	if m.summary != "" {
		b.WriteString("摘要：\n" + m.summary + "\n")
	} else {
		b.WriteString("摘要：暂无（本次是第一次对话）\n")
	}
	if len(m.facts) > 0 {
		b.WriteString("已记录的用户事实：\n")
		start := 0
		if len(m.facts) > 10 {
			start = len(m.facts) - 10
		}
		for _, f := range m.facts[start:] {
			b.WriteString("- " + f.Text + "\n")
		}
	}
	b.WriteString(fmt.Sprintf("历史对话轮次：%d 轮\n", len(m.records)))
	b.WriteString("使用方式：回答时可以自然地承接这些记忆（例如用户之前选过的景点、提过的偏好），但不要生硬复述；\n")
	b.WriteString("发现值得长期记住的信息（用户偏好、常去地点、忌讳等）时，调用 memory_save 记下来；需要回忆细节时调用 memory_recall。\n")
	return b.String()
}

func (m *Memory) summaryPath() string { return filepath.Join(m.dir, "summary.md") }
func (m *Memory) recordsPath() string { return filepath.Join(m.dir, "records.jsonl") }
func (m *Memory) factsPath() string   { return filepath.Join(m.dir, "facts.jsonl") }
func (m *Memory) metaPath() string    { return filepath.Join(m.dir, "meta.json") }

func (m *Memory) loadJSONL(path string, fn func(line []byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if err := fn([]byte(line)); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (m *Memory) appendJSONL(path string, v any) error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}
