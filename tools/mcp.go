package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	mcpTransport "github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/openai/openai-go/v3"
)

// MCPConfig 对应 mcp_servers.json 的顶层结构
type MCPConfig struct {
	Servers []MCPServerConfig `json:"servers"`
}

// MCPServerConfig 单个 MCP 服务器的连接配置
type MCPServerConfig struct {
	Name      string            `json:"name"`      // 服务器名称，用于日志与工具名前缀
	Transport string            `json:"transport"` // stdio / sse / http，缺省为 stdio
	Command   string            `json:"command"`   // stdio：可执行命令，如 npx、uvx
	Args      []string          `json:"args"`      // stdio：命令参数
	Env       map[string]string `json:"env"`       // stdio：子进程环境变量
	URL       string            `json:"url"`       // sse / http：服务地址
	Include   []string          `json:"include"`   // 可选，只注册这些工具；为空则注册全部
	Enabled   *bool             `json:"enabled"`   // 缺省视为启用
}

// IsEnabled 判断该服务器是否启用（未显式配置 enabled 时默认启用）
func (c MCPServerConfig) IsEnabled() bool {
	return c.Enabled == nil || *c.Enabled
}

// LoadMCPConfig 读取并解析 JSON 配置文件
func LoadMCPConfig(path string) (*MCPConfig, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg MCPConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
	}
	return &cfg, nil
}

// LoadMCPTools 连接一个 MCP 服务器，把它暴露的工具转换成本地的 ToolDef。
// 返回的 closer 用于断开连接、释放子进程，调用方应在退出时执行。
func LoadMCPTools(ctx context.Context, srv MCPServerConfig) (map[string]*ToolDef, func() error, error) {
	var (
		c   *client.Client
		err error
	)
	transport := strings.ToLower(srv.Transport)
	if transport == "" {
		// 只写了地址就按远程 HTTP 服务器处理，否则按本地 stdio 进程处理
		if srv.URL != "" {
			transport = "http"
		} else {
			transport = "stdio"
		}
	}

	switch transport {
	case "stdio":
		if srv.Command == "" {
			return nil, nil, fmt.Errorf("stdio 服务器需要配置 command")
		}
		// 丢弃子进程 stderr，避免 Playwright 等服务器的调试日志刷屏终端
		c, err = client.NewStdioMCPClientWithOptions(
			srv.Command, envList(srv.Env), srv.Args,
			mcpTransport.WithCommandStderrWriter(io.Discard),
		)
	case "sse":
		if srv.URL == "" {
			return nil, nil, fmt.Errorf("sse 服务器需要配置 url")
		}
		c, err = client.NewSSEMCPClient(srv.URL)
	case "http", "streamable-http", "streamablehttp":
		if srv.URL == "" {
			return nil, nil, fmt.Errorf("http 服务器需要配置 url")
		}
		c, err = client.NewStreamableHttpClient(srv.URL)
	default:
		return nil, nil, fmt.Errorf("不支持的 transport: %s", srv.Transport)
	}
	if err != nil {
		return nil, nil, err
	}
	closeFn := func() error { return c.Close() }

	// Start 对 stdio 是幂等的，对 sse/http 是必须的建连步骤
	if err := c.Start(ctx); err != nil {
		_ = closeFn()
		return nil, nil, fmt.Errorf("连接失败: %w", err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "beijing-autumn", Version: "1.0.0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = closeFn()
		return nil, nil, fmt.Errorf("初始化失败: %w", err)
	}

	listRes, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		_ = closeFn()
		return nil, nil, fmt.Errorf("获取工具列表失败: %w", err)
	}

	out := make(map[string]*ToolDef, len(listRes.Tools))
	for _, t := range listRes.Tools {
		// 配了 include 白名单时只注册指定工具，减少发给模型的工具定义体积
		if len(srv.Include) > 0 && !slices.Contains(srv.Include, t.Name) {
			continue
		}
		tool := t
		name := SanitizeToolName(tool.Name)
		out[name] = &ToolDef{
			Name:        name,
			Description: tool.Description,
			Params:      mcpInputSchema(tool),
			Fn: func(ctx context.Context, args map[string]any) (string, error) {
				req := mcp.CallToolRequest{}
				req.Params.Name = tool.Name
				req.Params.Arguments = args
				res, err := c.CallTool(ctx, req)
				if err != nil {
					return "", err
				}
				text := mcpResultText(res)
				if res.IsError {
					return "", fmt.Errorf("%s", text)
				}
				return text, nil
			},
		}
	}
	return out, closeFn, nil
}

// mcpInputSchema 把 MCP 的 inputSchema 转成 OpenAI function parameters
func mcpInputSchema(t mcp.Tool) openai.FunctionParameters {
	if len(t.RawInputSchema) > 0 {
		var params openai.FunctionParameters
		if err := json.Unmarshal(t.RawInputSchema, &params); err == nil {
			return params
		}
	}
	params := openai.FunctionParameters{"type": "object"}
	if t.InputSchema.Type != "" {
		params["type"] = t.InputSchema.Type
	}
	if t.InputSchema.Properties != nil {
		params["properties"] = t.InputSchema.Properties
	}
	if len(t.InputSchema.Required) > 0 {
		params["required"] = t.InputSchema.Required
	}
	return params
}

// mcpResultText 提取 MCP 工具返回的文本内容
func mcpResultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, item := range res.Content {
		switch v := item.(type) {
		case mcp.TextContent:
			b.WriteString(v.Text)
		case *mcp.TextContent:
			b.WriteString(v.Text)
		default:
			if data, err := json.Marshal(item); err == nil {
				b.WriteString(string(data))
			}
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if data, err := json.Marshal(res.StructuredContent); err == nil {
			b.WriteString(string(data))
		}
	}
	return b.String()
}

// envList 把 map 形式的环境变量转成 KEY=VALUE 列表
func envList(env map[string]string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+env[k])
	}
	return out
}

// SanitizeToolName 清洗工具名，满足 OpenAI 对 tool name 的字符与长度限制
func SanitizeToolName(name string) string {
	var b strings.Builder
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	s := b.String()
	if s == "" {
		s = "mcp_tool"
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}
