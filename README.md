# 北平之秋 · 北京赏秋 Agent

一个跑在终端里的「北京赏秋搭子」：用数字菜单点单，用抖音口吻讲攻略，背后靠大模型 + 本地精选库 + 联网搜索 + MCP 工具拿事实。

## 功能

- **数字菜单交互**：输入 1~10 选择要查的板块，0 退出
- **29 个精选赏秋地点**：香山、钓鱼台、地坛、五塔寺、潭柘寺、慕田峪、坡峰岭、喇叭沟门等，含最佳观赏期、门票、交通、避坑提醒
- **选定景点后联动**：路线、住宿、花期、拍照等菜单项只围绕你选的那个景点展开
   - 菜单 2 出行路线：只给一条必经该景点的时间轴
   - 菜单 6 住宿：按该景点所在行政区给住宿片区和预算
   - 菜单 4 花期：直接算当前观赏指数和距最佳期的天数
- **库外景点先联网再判断**：不在精选库里的地点，先联网搜索，判断赏秋指数；不适合就退回菜单让你重选
- **抖音文案风格**：钩子、短句、emoji 点缀、话题标签，让人隔着屏幕想去北京
- **MCP 扩展**：支持通过 `mcp_servers.json` 挂载任意 MCP 服务器，工具在启动时从服务器动态拉取
   - 已接入 Playwright MCP，可用菜单 10 打开北京旅游网等真实网页查信息
   - 附带一个本地「赏秋 MCP 服务器」，把精选库通过 MCP 协议对外暴露

## 菜单

```
1  推荐目的地（景点）
2  出行路线规划
3  交通指南
4  观赏时间&花期预报
5  美食推荐
6  住宿推荐
7  拍照攻略
8  出行准备&穿搭装备
9  避坑&注意事项
10 官网实况（北京旅游网）
0  返回/退出
```

## 技术栈

- Go 1.26，OpenAI 兼容的 Chat Completions 接口
- 模型：商汤日日新 SenseNova Token Plan（`sensenova-6.8-flash-lite`）
- 联网搜索：豆包搜索 API
- MCP：`github.com/mark3labs/mcp-go`，支持 stdio / SSE / Streamable HTTP

## 目录结构

```
.
├── main.go                 # 主循环、数字菜单、Agent 工具调用循环
├── core/session.go         # 多轮会话消息历史
├── prompts/prompt.go       # 系统提示词（人设、菜单模式、输出规则）
├── tools/
│   ├── autumn.go           # 精选景点库 + 观赏指数
│   ├── guide.go            # 各菜单板块攻略库
│   ├── search_web.go       # 豆包联网搜索
│   ├── get_real_weather.go # 实时天气
│   ├── get_current_time.go # 当前时间
│   ├── mcp.go              # MCP 客户端：读配置、连服务器、拉工具
│   └── tool.go             # 工具定义与调用封装
├── cmd/autumn-mcp/main.go  # 本地「赏秋」MCP 服务器
├── mcp_servers.json        # MCP 服务器列表
├── sensenova_api_key.txt   # 模型 Key
└── search_api_key.txt      # 豆包搜索 Key
```

## 运行

```bash
go build ./...
go run .
```

也可以把 Key 放进环境变量，替代同目录的 txt 文件：

```bash
SENSENOVA_API_KEY=... DOUBAO_SEARCH_API_KEY=... go run .
```

## MCP 配置

编辑 `mcp_servers.json`，只写服务器地址（远程）或启动命令（本地）即可，工具会自动从服务器拉取：

```json
{
  "servers": [
    {
      "name": "playwright",
      "transport": "stdio",
      "command": "npx",
      "args": ["-y", "@playwright/mcp@latest", "--browser=msedge", "--headless"],
      "include": ["browser_navigate", "browser_snapshot", "browser_find", "browser_click", "browser_close"],
      "enabled": true
    },
    {
      "name": "deepwiki",
      "url": "https://mcp.deepwiki.com/mcp",
      "enabled": false
    }
  ]
}
```

- `include`：可选，只注册指定工具，减少传给模型的工具数量、加快响应
- `enabled`：缺省视为启用；连接失败只会跳过该服务器，不影响启动
- 本地赏秋 MCP 服务器：`go build -o bin/autumn-mcp.exe ./cmd/autumn-mcp`

## 开源与安全

- 所有密钥文件（`*_api_key.txt`、`.env`）已写入 `.gitignore`，不会进入仓库
- 仓库里**没有任何硬编码密钥**，Key 只从环境变量或本地 txt 读取
- 推荐用环境变量传 Key，避免把密钥落到文件里：

```bash
SENSENOVA_API_KEY=... DOUBAO_SEARCH_API_KEY=... go run .
```

- 推送到 GitHub 前建议先在本地确认一次：`git status` 里不应出现任何 key 文件
- 如果密钥曾经被提交过，请去平台后台**重新生成 Key**，旧 Key 即使删掉提交记录也可能已被爬虫抓走
- `.playwright-mcp/`、`bin/` 等运行时与构建产物同样已忽略

## 说明

- 精选库数据为本地整理的赏秋常识信息，门票、开放时间等以景区官方为准
- 联网搜索与网页内容均标注来源，属于「网络信息」，与精选库数据区分
- 菜单 10 使用浏览器时只读浏览，不会登录、下单或提交表单
