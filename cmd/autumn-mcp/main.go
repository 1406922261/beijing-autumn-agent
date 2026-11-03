package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"tui/beijingAutumn/tools"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// 本地「北京赏秋」MCP 服务器：把精选库里的赏秋数据通过 MCP 协议暴露出去
func main() {
	s := server.NewMCPServer(
		"beijing-autumn",
		"1.0.0",
		server.WithToolCapabilities(false),
	)

	// 工具一：列出精选库地点，可按类型 / 行政区筛选
	listTool := mcp.NewTool("autumn_list_spots",
		mcp.WithDescription("列出北京精选赏秋地点，可按景观类型或行政区筛选"),
		mcp.WithString("type", mcp.Description("可选，景观类型：银杏、红叶、彩叶、古树银杏")),
		mcp.WithString("district", mcp.Description("可选，所在区，如 海淀区、怀柔区")),
	)
	s.AddTool(listTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		typ := req.GetString("type", "")
		district := req.GetString("district", "")

		var lines []string
		for _, spot := range tools.AllAutumnSpots() {
			if typ != "" && !strings.Contains(spot.Type, typ) {
				continue
			}
			if district != "" && !strings.Contains(spot.District, district) {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s（%s · %s）最佳期 %s~%s",
				spot.Name, spot.District, spot.Type, spot.BestStart, spot.BestEnd))
		}
		if len(lines) == 0 {
			return mcp.NewToolResultText("精选库里没有匹配的地点"), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("共 %d 个：\n%s", len(lines), strings.Join(lines, "\n"))), nil
	})

	// 工具二：单个地点详情 + 当前观赏指数
	detailTool := mcp.NewTool("autumn_spot_detail",
		mcp.WithDescription("查询一个赏秋地点的详细信息与当前观赏指数"),
		mcp.WithString("name", mcp.Required(), mcp.Description("地点名称，如 香山公园")),
	)
	s.AddTool(detailTool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError("缺少参数 name"), nil
		}
		spot, index, ok := tools.SpotIndex(name, time.Now())
		if !ok {
			return mcp.NewToolResultText(fmt.Sprintf("精选库里没有「%s」", name)), nil
		}
		text := fmt.Sprintf("%s（%s · %s）\n最佳观赏期：%s ~ %s\n观赏指数：%s\n亮点：%s\n地址：%s｜门票：%s\n交通：%s\n提醒：%s",
			spot.Name, spot.District, spot.Type, spot.BestStart, spot.BestEnd,
			index, spot.Highlight, spot.Address, spot.Ticket, spot.Traffic, spot.Tip)
		return mcp.NewToolResultText(text), nil
	})

	if err := server.ServeStdio(s); err != nil {
		fmt.Println("MCP 服务器退出:", err)
	}
}
