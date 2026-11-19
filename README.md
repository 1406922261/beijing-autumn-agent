# 北平之秋 · 北京赏秋 Agent

一个跑在终端里的「北京赏秋搭子」：用数字菜单点单，用抖音口吻讲攻略，背后靠大模型 + 本地精选库 + 联网搜索 + MCP 工具拿事实。

## 功能

- **数字菜单交互**：输入 1~11 选择要查的板块，0 退出；`/` 开头是进阶命令
- **29 个精选赏秋地点**：香山、钓鱼台、地坛、五塔寺、潭柘寺、慕田峪、坡峰岭、喇叭沟门等，含最佳观赏期、门票、交通、避坑提醒
- **选定景点后联动**：路线、住宿、花期、拍照等菜单项只围绕你选的那个景点展开
   - 菜单 2 出行路线：只给一条必经该景点的时间轴
   - 菜单 6 住宿：按该景点所在行政区给住宿片区和预算
   - 菜单 4 花期：直接算当前观赏指数和距最佳期的天数
- **库外景点先联网再判断**：不在精选库里的地点，先联网搜索，判断赏秋指数；不适合就退回菜单让你重选
- **抖音文案风格**：钩子、短句、emoji 点缀、话题标签，让人隔着屏幕想去北京
- **长期记忆（持久化 + 摘要）**：每轮问答写入 `memory/records.jsonl`，每 3 轮让模型压缩成 `memory/summary.md` 摘要；下次启动自动读回摘要与用户事实并注入系统提示词，实现跨会话记忆
   - 模型可用 `memory_save` 主动记下用户偏好，用 `memory_recall` 检索历史
   - `/memory` 随时查看当前摘要与统计
- **Skill 能力**：扫描 `skills/*/SKILL.md` 加载本地技能，模型通过 `list_skills` / `load_skill` 按需加载完整步骤再执行
   - 内置 `webpage`（当场写单文件网页）、`autumn-poster`（赏秋分享海报页）
   - 技能产物用 `write_file` 真正落盘，不是只回代码
- **Shell 执行**：`exec_shell` 可跑脚本、做网络请求，作为工具兜底
- **MCP 扩展**：支持通过 `mcp_servers.json` 挂载任意 MCP 服务器，工具在启动时从服务器动态拉取
   - 已接入 Playwright MCP，可用菜单 10 打开北京旅游网等真实网页查信息
   - 附带一个本地「赏秋 MCP 服务器」，把精选库通过 MCP 协议对外暴露

## 菜单与命令

```
1  推荐目的地（景点）        7  拍照攻略
2  出行路线规划              8  出行准备&穿搭装备
3  交通指南                  9  避坑&注意事项
4  观赏时间&花期预报         10 官网实况（北京旅游网）
5  美食推荐                  11 技能演示（当场写一个网页）
6  住宿推荐                  0  返回/退出
```

进阶命令（`/` 开头）：

```
/skills          列出本地技能
/skill <name>    查看某个技能的完整说明
/memory          查看长期记忆摘要与统计
/ask <问题>      自由提问（走完整 agent 流程）
/help            显示帮助
```

## 技术栈

- Go 1.26，OpenAI 兼容的 Chat Completions 接口
- 模型：商汤日日新 SenseNova Token Plan（`sensenova-6.8-flash-lite`）
- 联网搜索：豆包搜索 API
- MCP：`github.com/mark3labs/mcp-go`，支持 stdio / SSE / Streamable HTTP

## 目录结构

```
.
├── main.go                 # 主循环、数字菜单、命令、Agent 工具调用循环、记忆摘要
├── core/
│   ├── session.go          # 多轮会话消息历史
│   └── memory.go           # 长期记忆：摘要 + 逐轮记录 + 用户事实（落盘）
├── prompts/prompt.go       # 系统提示词（人设、菜单模式、输出规则）
├── tools/
│   ├── autumn.go           # 精选景点库 + 观赏指数
│   ├── guide.go            # 各菜单板块攻略库
│   ├── search_web.go       # 豆包联网搜索
│   ├── get_real_weather.go # 实时天气
│   ├── get_current_time.go # 当前时间
│   ├── get_shell.go        # 执行 shell 命令
│   ├── memory.go           # memory_save / memory_recall 工具
│   ├── skills.go           # 技能加载器 + list_skills / load_skill
│   ├── write_file.go       # 写文件（限制在项目目录内）
│   ├── mcp.go              # MCP 客户端：读配置、连服务器、拉工具
│   └── tool.go             # 工具定义与调用封装
├── skills/                 # 本地技能（每个子目录一个 SKILL.md）
│   ├── webpage/
│   └── autumn-poster/
├── cmd/autumn-mcp/main.go  # 本地「赏秋」MCP 服务器
├── mcp_servers.json        # MCP 服务器列表
├── memory/                 # 运行时生成：长期记忆（已 gitignore）
├── output/                 # 运行时生成：网页等产物（已 gitignore）
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

## 技能（Skill）

技能 = 一份写给模型看的操作手册，放在 `skills/<name>/SKILL.md`，头部用 `---` 包裹 frontmatter：

```markdown
---
name: webpage
description: 当场写一个网页。用户说"写个网页/做个页面"时使用
---

# 技能：当场写一个网页
1. 先用 get_autumn_spots 取真实数据
2. 用 write_file 写入 output/xxx.html
...
```

- 启动时自动扫描 `skills/`，把技能名与说明注入系统提示词（只注入目录，正文按需加载）
- 模型判断命中时调用 `load_skill` 拿到完整步骤，再按步骤用 `write_file` / `exec_shell` 执行
- 新增技能只需新建一个目录 + `SKILL.md`，无需改代码

## 长期记忆

```
memory/
├── summary.md      # 摘要：每 3 轮由模型压缩一次，启动时注入系统提示词
├── records.jsonl   # 每轮问答的原始记录（问 / 用过的工具 / 答）
├── facts.jsonl     # 模型用 memory_save 记下的长期事实
└── meta.json       # 摘要进度
```

- 跨会话生效：重启程序后 `/memory` 仍能看到之前的摘要与偏好
- 模型回答时可以自然承接记忆（之前选过的景点、提过的偏好），但不会生硬复述
- `memory/` 已加入 `.gitignore`，本地对话记录不会进入开源仓库

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
