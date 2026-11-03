package tools

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

// NewSearchWeb 联网搜索（豆包搜索 · 火山引擎 search-infinity）。
// 用于查询精选库里没有的景点，Key 优先取环境变量 DOUBAO_SEARCH_API_KEY，其次取同目录 search_api_key.txt。
func NewSearchWeb() *ToolDef {
	return &ToolDef{
		Name:        "search_web",
		Description: "联网搜索实时信息。当用户点名的地点不在精选库里、或需要最新的门票/开放时间/交通信息时调用，返回网页标题、摘要、来源站点和链接",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]string{
					"type":        "string",
					"description": "搜索关键词，尽量具体，例如「什刹海 赏秋 银杏 最佳时间」",
				},
			},
			"required": []string{"query"},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			query, _ := args["query"].(string)
			query = strings.TrimSpace(query)
			if query == "" {
				return "", fmt.Errorf("搜索关键词为空")
			}

			key := loadDoubaoSearchKey()
			if key == "" {
				return "", fmt.Errorf("未配置豆包搜索 API Key：请设置环境变量 DOUBAO_SEARCH_API_KEY，或把 Key 写进项目根目录的 search_api_key.txt")
			}

			body, _ := json.Marshal(map[string]any{
				"Query":       query,
				"SearchType":  "web",
				"Count":       6,
				"NeedSummary": true,
				"TimeRange":   "OneYear",
				"Filter": map[string]any{
					"NeedUrl": true,
				},
			})

			req, _ := http.NewRequestWithContext(ctx, "POST", "https://open.feedcoopapi.com/search_api/web_search", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer "+key)
			req.Header.Set("Content-Type", "application/json")

			client := newSearchTLSClient(15 * time.Second)
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("联网搜索请求失败: %w", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				// 把响应体一起带出来，方便定位是配额、并发还是鉴权问题
				bodyBytes, _ := io.ReadAll(resp.Body)
				return "", fmt.Errorf("联网搜索 HTTP %d，响应：%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
			}

			var result struct {
				ResponseMetadata struct {
					Error *struct {
						Code    string `json:"Code"`
						Message string `json:"Message"`
					} `json:"Error"`
				} `json:"ResponseMetadata"`
				Result struct {
					ResultCount int `json:"ResultCount"`
					WebResults  []struct {
						Title       string `json:"Title"`
						SiteName    string `json:"SiteName"`
						URL         string `json:"Url"`
						Snippet     string `json:"Snippet"`
						Summary     string `json:"Summary"`
						PublishTime string `json:"PublishTime"`
						AuthInfoDes string `json:"AuthInfoDes"`
					} `json:"WebResults"`
				} `json:"Result"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return "", fmt.Errorf("联网搜索返回解析失败: %w", err)
			}
			if e := result.ResponseMetadata.Error; e != nil {
				return "", fmt.Errorf("联网搜索接口报错 code=%s msg=%s", e.Code, e.Message)
			}

			items := result.Result.WebResults
			if len(items) == 0 {
				return fmt.Sprintf("联网搜索「%s」没有返回有效结果，可以换个关键词再试，或如实告诉用户这次没查到。", query), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "联网搜索结果（来源：豆包搜索，属于网络信息，以景区官方为准）——关键词「%s」，共 %d 条：\n", query, len(items))
			for i, it := range items {
				fmt.Fprintf(&b, "\n%d. %s\n", i+1, it.Title)
				text := it.Summary
				if strings.TrimSpace(text) == "" {
					text = it.Snippet
				}
				if strings.TrimSpace(text) != "" {
					fmt.Fprintf(&b, "   摘要：%s\n", strings.TrimSpace(text))
				}
				if it.SiteName != "" {
					fmt.Fprintf(&b, "   来源：%s", it.SiteName)
					if it.PublishTime != "" {
						fmt.Fprintf(&b, "（%s）", it.PublishTime)
					}
					fmt.Fprintf(&b, "\n")
				}
				if it.URL != "" {
					fmt.Fprintf(&b, "   链接：%s\n", it.URL)
				}
			}
			return b.String(), nil
		},
	}
}

// loadDoubaoSearchKey 读取豆包搜索 API Key
func loadDoubaoSearchKey() string {
	if k := strings.TrimSpace(os.Getenv("DOUBAO_SEARCH_API_KEY")); k != "" {
		return k
	}
	b, err := os.ReadFile("search_api_key.txt")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// newSearchTLSClient 构建带系统证书池 + 本地 ssl.crt 的 http 客户端（与项目其它工具保持一致）
func newSearchTLSClient(timeout time.Duration) *http.Client {
	rootCAs, err := x509.SystemCertPool()
	if err != nil || rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if caCert, err := os.ReadFile("ssl.crt"); err == nil {
		rootCAs.AppendCertsFromPEM(caCert)
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: rootCAs},
		},
	}
}
