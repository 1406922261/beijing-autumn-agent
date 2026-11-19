package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"tui/beijingAutumn/core"
	"tui/beijingAutumn/prompts"
	"tui/beijingAutumn/tools"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const MaxLoop = 16

// 商汤日日新（SenseNova）Token Plan 的 OpenAI 兼容接口
const (
	sensenovaBaseURL = "https://token.sensenova.cn/v1"
	sensenovaModel   = "sensenova-6.8-flash-lite"
)

// loadAPIKey 优先读取环境变量 SENSENOVA_API_KEY；为空时回退到同目录下的 sensenova_api_key.txt
func loadAPIKey() string {
	if k := os.Getenv("SENSENOVA_API_KEY"); k != "" {
		return k
	}
	b, err := os.ReadFile("sensenova_api_key.txt")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// getLlmTools 注册「北京赏秋」可用的工具
func getLlmTools(mem *core.Memory, skills []tools.Skill, root string) map[string]*tools.ToolDef {
	llmTools := make(map[string]*tools.ToolDef)

	autumnSpots := tools.NewGetAutumnSpots() // 赏秋地点库
	autumnIndex := tools.NewGetAutumnIndex() // 当前观赏指数
	autumnGuide := tools.NewGetAutumnGuide() // 菜单板块攻略库（路线/交通/美食/住宿/拍照/穿搭/避坑）
	weather := tools.NewGetRealWeather()     // 真实天气
	currentTime := tools.NewGetCurrentTime() // 当前时间
	shell := tools.NewExecShell()            // 兜底：执行脚本/网络请求
	searchWeb := tools.NewSearchWeb()        // 联网搜索（豆包搜索）

	// 长记忆：写入事实 / 检索历史
	memorySave := tools.NewMemorySave(mem)
	memoryRecall := tools.NewMemoryRecall(mem)

	// 技能：列出 / 加载
	listSkills := tools.NewListSkills(skills)
	loadSkill := tools.NewLoadSkill(skills)

	// 写文件：生成网页、报告等产物
	writeFile := tools.NewWriteFile(root)

	for _, t := range []*tools.ToolDef{
		autumnSpots, autumnIndex, autumnGuide, weather, currentTime,
		shell, searchWeb, memorySave, memoryRecall, listSkills, loadSkill, writeFile,
	} {
		llmTools[t.Name] = t
	}
	return llmTools
}

// buildChatTools 生成 chat completions 格式的工具
func buildChatTools(tools map[string]*tools.ToolDef) []openai.ChatCompletionToolUnionParam {
	chatTools := make([]openai.ChatCompletionToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		chatTools = append(chatTools, tool.ToParam())
	}
	return chatTools
}

// mcpConfigPath MCP 服务器列表配置文件
const mcpConfigPath = "mcp_servers.json"

// loadMCPTools 读取 mcp_servers.json，循环调用 LoadMCPTools 把各 MCP 服务器的工具并入本地工具表，
// 返回每个连接的关闭函数，供程序退出时统一释放
func loadMCPTools(ctx context.Context, llmTools map[string]*tools.ToolDef) []func() error {
	cfg, err := tools.LoadMCPConfig(mcpConfigPath)
	if err != nil {
		fmt.Printf("⚠️ 未加载 MCP 配置（%s）：%v\n", mcpConfigPath, err)
		return nil
	}

	var closers []func() error
	servers, added := 0, 0
	for i, srv := range cfg.Servers {
		if !srv.IsEnabled() {
			continue
		}
		if srv.Name == "" {
			srv.Name = fmt.Sprintf("server-%d", i+1)
		}

		// 单个服务器最多等 60 秒，避免某个 MCP 进程卡住整个启动
		serverCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		mcpTools, closeFn, err := tools.LoadMCPTools(serverCtx, srv)
		cancel()
		if err != nil {
			fmt.Printf("⚠️ MCP 服务器「%s」接入失败：%v\n", srv.Name, err)
			continue
		}
		closers = append(closers, closeFn)
		servers++

		for name, t := range mcpTools {
			// 与内置工具重名时加服务器前缀，避免互相覆盖
			if _, exists := llmTools[name]; exists {
				name = "mcp_" + tools.SanitizeToolName(srv.Name) + "_" + name
				t.Name = name
			}
			if t.Description == "" {
				t.Description = "MCP 工具"
			}
			t.Description = "[MCP:" + srv.Name + "] " + t.Description
			llmTools[name] = t
			added++
		}
	}
	if servers > 0 {
		fmt.Printf("🔌 MCP 已接入 %d 个服务器、%d 个工具\n", servers, added)
	}
	return closers
}

// summarize 把工具结果压成一行短摘要，避免大段检索原文刷屏
func summarize(s string) string {
	const maxRunes = 120
	// 去掉 Playwright 等工具输出里的 ANSI 颜色控制符，避免终端出现乱码
	s = ansiEscape.ReplaceAllString(s, "")
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + " …"
	}
	return string(r)
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// judgeAutumnSuitable 借助模型，根据联网检索结果判断某地点是否值得专程赏秋
func judgeAutumnSuitable(ctx context.Context, client openai.Client, spot, searchText string) (bool, string) {
	if r := []rune(searchText); len(r) > 1500 {
		searchText = string(r[:1500])
	}
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("你是北京赏秋向导。根据给定的联网检索结果，判断**该地点本身**是否有明确的秋色可赏（红叶/银杏/彩叶/秋景）。\n判断标准：\n- 只有检索结果**明确提到该地点自己**有红叶/银杏/彩叶/秋景或最佳观赏期时，才判「适合」\n- 若只是泛泛的北京秋天文章、或讲的是别的景点的秋色、或该地点以别的景观为主（商业街/胡同/美食/夜景/游乐等），一律判「不适合」\n只回一行：以「适合」或「不适合」开头，后面跟不超过 30 字的理由。"),
			openai.UserMessage(fmt.Sprintf("地点：%s\n\n联网检索结果：\n%s", spot, searchText)),
		},
		Model:     sensenovaModel,
		MaxTokens: openai.Int(1024),
	})
	if err != nil {
		fmt.Println("----->error: 赏秋判断失败:", err)
		return true, ""
	}
	if len(resp.Choices) == 0 {
		return true, ""
	}
	line := strings.TrimSpace(resp.Choices[0].Message.Content)
	if strings.HasPrefix(line, "不适合") {
		reason := strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(line, "不适合")), "，,。：: ")
		return false, reason
	}
	return true, strings.TrimLeft(strings.TrimSpace(strings.TrimPrefix(line, "适合")), "，,。：: ")
}

// runAgent 处理一轮用户输入，内部完成「思考 → 调工具 → 生成文案」的循环。
// 返回最终文案与本轮调用过的工具名，供长期记忆记录
func runAgent(ctx context.Context, client openai.Client, session *core.Session, llmTools map[string]*tools.ToolDef, chatTools []openai.ChatCompletionToolUnionParam) (string, []string) {
	fmt.Println() // 换行，避免查证日志紧贴输入提示
	var used []string
	for loop := 0; loop < MaxLoop; loop++ {
		resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
			Messages:  session.GetMessages(),
			Tools:     chatTools,
			Model:     sensenovaModel,
			MaxTokens: openai.Int(8192), // 写网页时 write_file 的参数很长，留足输出空间
		})
		if err != nil {
			var apiErr *openai.Error
			if errors.As(err, &apiErr) {
				fmt.Println("😵 请求失败，HTTP 状态码:", apiErr.StatusCode, "错误类型:", apiErr.Type)
				fmt.Println("原始响应体:", apiErr.RawJSON())
			} else {
				fmt.Println("😵 出错了:", err)
			}
			return "", used
		}
		if len(resp.Choices) == 0 {
			fmt.Println("😵 模型这次没有返回内容，请再试一次～")
			return "", used
		}

		msg := resp.Choices[0].Message

		// 没有工具调用：这就是最终文案
		if len(msg.ToolCalls) == 0 {
			fmt.Println()
			fmt.Println(msg.Content)
			return msg.Content, used
		}

		// 先把 assistant 这一轮的 tool_calls 写回历史，再逐条塞入工具结果
		assistant := openai.ChatCompletionAssistantMessageParam{
			ToolCalls: make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(msg.ToolCalls)),
		}
		for _, call := range msg.ToolCalls {
			assistant.ToolCalls = append(assistant.ToolCalls, openai.ChatCompletionMessageToolCallUnionParam{
				OfFunction: &openai.ChatCompletionMessageFunctionToolCallParam{
					ID: call.ID,
					Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
						Name:      call.Function.Name,
						Arguments: call.Function.Arguments,
					},
				},
			})
		}
		session.AddMessage(openai.ChatCompletionMessageParamUnion{OfAssistant: &assistant})

		for _, call := range msg.ToolCalls {
			name := call.Function.Name
			used = append(used, name)
			fmt.Printf("🔧 正在查证：%s(%s)\n", name, call.Function.Arguments)

			toolRe := ""
			if v, ok := llmTools[name]; ok {
				out, callErr := v.Call(ctx, call.Function.Arguments)
				if callErr != nil {
					toolRe = fmt.Sprintf("%s 调用出错：%v", name, callErr)
					fmt.Println("----->error:", toolRe)
				} else {
					toolRe = out
					fmt.Println("📄 结果：", summarize(toolRe))
				}
			} else {
				toolRe = fmt.Sprintf("没有名为 %s 的工具", name)
				fmt.Println("----->error:", toolRe)
			}
			session.AddMessage(openai.ToolMessage(toolRe, call.ID))
		}
	}
	fmt.Println("⚠️ 工具调用轮次达到上限，先聊到这，你可以继续追问～")
	return "", used
}

// menuText 终端数字菜单
const menuText = `=====北京赏秋攻略菜单=====
请输入数字选择你想查看的内容：
1 推荐目的地（景点）
2 出行路线规划
3 交通指南
4 观赏时间&花期预报
5 美食推荐
6 住宿推荐
7 拍照攻略
8 出行准备&穿搭装备
9 避坑&注意事项
10 官网实况（北京旅游网）
11 技能演示（当场写一个网页）
0 返回/退出
=========================`

// menuPrompts 数字菜单对应的提问（带编号前缀，便于提示词按菜单项取数与定风格）
var menuPrompts = map[string]string{
	"1": "[菜单 1｜推荐目的地（景点）] 推荐几个北京赏秋的好去处",
	"2": "[菜单 2｜出行路线规划] 帮我规划一条北京赏秋的出行路线",
	"3": "[菜单 3｜交通指南] 在北京赏秋怎么出行最方便？地铁、公交、自驾怎么选",
	"4": "[菜单 4｜观赏时间&花期预报] 北京各地红叶、银杏的最佳观赏时间预报",
	"5": "[菜单 5｜美食推荐] 北京秋天有什么本地美食值得吃",
	"6": "[菜单 6｜住宿推荐] 北京赏秋期间住宿怎么选？住在哪个区域最方便、性价比高",
	"7": "[菜单 7｜拍照攻略] 北京赏秋的拍照攻略：机位、光线、最佳时段",
	"8": "[菜单 8｜出行准备&穿搭装备] 秋天来北京赏秋要准备什么、怎么穿搭",
	"9": "[菜单 9｜避坑&注意事项] 北京赏秋有哪些坑和注意事项",
	"10": "[菜单 10｜官网实况（北京旅游网）] 打开北京旅游网 https://s.visitbeijing.com.cn/ ，看看官网上有哪些赏秋专题、活动或相关景点页面，把看到的关键内容告诉我",
	"11": "[菜单 11｜技能演示（当场写一个网页）] 用本地技能当场给我写一个北京赏秋的网页，保存到 output/ 目录，写完用一句话告诉我文件路径",
}

func printWelcome() {
	fmt.Println()
	fmt.Println("北平的秋，是红墙配金叶、长城漫山红，是别处复制不来的浪漫。🍂")
	fmt.Println(menuText)
}

const (
	memoryDir = "memory" // 长期记忆目录（摘要 + 记录 + 事实）
	skillsDir = "skills" // 本地技能目录

	// summarizeEvery 每积累这么多轮对话，重写一次长期记忆摘要
	summarizeEvery = 3
)

// buildSystemPrompt 拼出完整系统提示词：人设 + 长期记忆 + 技能目录
func buildSystemPrompt(mem *core.Memory, skills []tools.Skill) string {
	return prompts.SystemPrompt + mem.PromptBlock() + tools.SkillsPromptBlock(skills)
}

// summarizeMemory 把「旧摘要 + 新增记录」交给模型压缩成新的长期记忆摘要，并落盘
func summarizeMemory(ctx context.Context, client openai.Client, mem *core.Memory) {
	pending := mem.PendingRecords()
	if len(pending) == 0 {
		return
	}
	old := mem.Summary()
	if old == "" {
		old = "（无）"
	}
	var b strings.Builder
	b.WriteString("已有摘要：\n" + old + "\n\n新增对话记录：\n")
	for _, r := range pending {
		fmt.Fprintf(&b, "- 问：%s\n  用过的工具：%s\n  答：%s\n", truncate(r.Input, 80), strings.Join(r.Tools, "、"), truncate(r.Answer, 200))
	}

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("你是对话记忆整理助手，负责把历史对话压缩成长期记忆摘要。"),
			openai.UserMessage(b.String() + "\n请输出一份不超过 300 字的中文摘要，要求：\n" +
				"1）保留用户的偏好与约束（如只要免费/人少、不喜欢某地、带娃等）\n" +
				"2）保留选过/问过的景点与关键结论（花期、交通、住宿、路线要点）\n" +
				"3）按主题分条，用短句；不要寒暄、不要复述完整文案、不要编造新信息\n只输出摘要正文。"),
		},
		Model:     sensenovaModel,
		MaxTokens: openai.Int(600),
	})
	if err != nil {
		fmt.Println("⚠️ 记忆摘要更新失败：", err)
		return
	}
	if len(resp.Choices) == 0 {
		return
	}
	text := strings.TrimSpace(resp.Choices[0].Message.Content)
	if text == "" {
		return
	}
	if err := mem.SaveSummary(text); err != nil {
		fmt.Println("⚠️ 记忆摘要写入失败：", err)
		return
	}
	fmt.Printf("🧠 长期记忆已更新（摘要 %d 字，累计 %d 轮）\n", len([]rune(text)), mem.Count())
}

// recordTurn 把一轮问答写入长期记忆文件
func recordTurn(mem *core.Memory, menu, spot, input, answer string, used []string) {
	if strings.TrimSpace(answer) == "" {
		return
	}
	err := mem.Append(core.Record{Menu: menu, Spot: spot, Input: input, Tools: used, Answer: answer})
	if err != nil {
		fmt.Println("⚠️ 记忆写入失败：", err)
	}
}

// truncate 按字符数截断文本（用于摘要输入，不影响终端展示）
func truncate(s string, n int) string {
	r := []rune(strings.Join(strings.Fields(s), " "))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		root = "."
	}

	// 长期记忆：启动时从 memory/ 读回摘要、历史记录与用户事实
	mem := core.NewMemory(memoryDir)
	if err := mem.Load(); err != nil {
		fmt.Println("⚠️ 长期记忆读取失败（按空记忆启动）：", err)
	}

	// 技能：扫描 skills/*/SKILL.md
	skills, err := tools.LoadSkills(skillsDir)
	if err != nil {
		fmt.Println("⚠️ 技能加载失败：", err)
	}

	session := core.NewSession()
	session.AddMessage(openai.SystemMessage(buildSystemPrompt(mem, skills)))

	ctx := context.Background()

	llmTools := getLlmTools(mem, skills, root)

	// 读取 mcp_servers.json，循环调用 LoadMCPTools 接入 MCP 扩展工具
	closers := loadMCPTools(ctx, llmTools)
	defer func() {
		for _, closeFn := range closers {
			_ = closeFn()
		}
	}()

	chatTools := buildChatTools(llmTools)

	client := openai.NewClient(
		option.WithAPIKey(loadAPIKey()),
		option.WithBaseURL(sensenovaBaseURL),
		option.WithRequestTimeout(60*time.Second),
	)

	// runAndRemember 跑一轮 agent，把问答写入长期记忆，并在积累够轮次时刷新摘要
	runAndRemember := func(question, menu, spot string) {
		answer, used := runAgent(ctx, client, session, llmTools, chatTools)
		recordTurn(mem, menu, spot, question, answer, used)
		if mem.Pending() >= summarizeEvery {
			summarizeMemory(ctx, client, mem)
		}
	}

	printWelcome()

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	// currentSpot 记录用户当前选定的景点；选定后各菜单项都围绕它回答
	currentSpot := ""

	for {
		// 一行式「继续选择」提示；已选定景点时带上景点名
		if currentSpot != "" {
			fmt.Printf("\n🍁 已选「%s」｜请继续选择菜单项（1-11），0 退出：", currentSpot)
		} else {
			fmt.Print("\n🍁 请选择数字（0 退出，/help 看命令）：")
		}
		if !scanner.Scan() {
			// Scan 返回 false 时要先看 Err：区分正常结束(EOF)与真实读取错误
			if err := scanner.Err(); err != nil {
				fmt.Println("\n😵 读取输入出错:", err)
				return
			}
			// 正常结束（Ctrl+Z / 管道 EOF）：补一句告别，避免留下悬空的提示符
			fmt.Println("\n🍂 秋天很短，北京很美，记得早点来。下次见！")
			return
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if input == "0" || input == "exit" || input == "quit" || input == "退出" {
			fmt.Println("\n🍂 秋天很短，北京很美，记得早点来。下次见！")
			return
		}

		// ===== 进阶命令：/ 开头，用于查看记忆与技能、自由提问 =====
		if strings.HasPrefix(input, "/") {
			fields := strings.Fields(input)
			cmd := strings.ToLower(fields[0])
			switch cmd {
			case "/help":
				fmt.Print(`
  命令：
    /skills          列出本地技能
    /skill <name>    查看某个技能的完整说明
    /memory          查看长期记忆摘要与统计
    /ask <问题>      自由提问（走完整 agent 流程）
    /help            显示本帮助
`)
			case "/skills":
				if len(skills) == 0 {
					fmt.Println("当前没有可用技能。")
				} else {
					fmt.Printf("\n🧩 已安装 %d 个技能：\n", len(skills))
					for _, s := range skills {
						fmt.Printf("  • %-14s %s\n", s.Name, s.Description)
					}
				}
			case "/skill":
				if len(fields) < 2 {
					fmt.Println("用法：/skill <技能名>，例如 /skill webpage")
					continue
				}
				found := false
				for _, s := range skills {
					if s.Name == fields[1] {
						found = true
						fmt.Printf("\n🧩 %s ｜ %s\n目录：%s\n\n%s\n", s.Name, s.Description, s.Dir, s.Body)
					}
				}
				if !found {
					fmt.Printf("没有名为 %s 的技能，输入 /skills 查看可用技能。\n", fields[1])
				}
			case "/memory":
				fmt.Printf("\n🧠 长期记忆目录：%s\n", mem.Dir())
				fmt.Printf("历史轮次：%d 轮｜用户事实：%d 条｜待摘要：%d 轮\n", mem.Count(), len(mem.Facts()), mem.Pending())
				if s := mem.Summary(); s != "" {
					fmt.Println("\n【摘要】\n" + s)
				} else {
					fmt.Println("\n【摘要】暂无（聊几轮后自动生成）")
				}
				if facts := mem.Facts(); len(facts) > 0 {
					fmt.Println("\n【用户事实】")
					for _, f := range facts {
						fmt.Printf("- %s（%s）\n", f.Text, f.Time)
					}
				}
			case "/ask":
				text := strings.TrimSpace(strings.TrimSpace(input)[len("/ask"):])
				if text == "" {
					fmt.Println("用法：/ask 你的问题，例如 /ask 帮我写个赏秋网页")
					continue
				}
				session.AddMessage(openai.UserMessage(text))
				runAndRemember(text, "ask", currentSpot)
				fmt.Println()
			default:
				fmt.Printf("未知命令 %s，输入 /help 查看可用命令。\n", fields[0])
			}
			continue
		}

		// 菜单 1：只列出精选库里的地点名称，让用户选定一个景点；选定后不立即出内容，改由菜单查询
		if input == "1" {
			names := tools.AutumnSpotNames()
			fmt.Printf("\n🍁 精选库里有这些赏秋地点（共 %d 个）：\n", len(names))
			fmt.Println("  " + strings.Join(names, " · "))
			fmt.Print("\n👉 你想去哪个地方？输入地点名称：")

			if !scanner.Scan() {
				if err := scanner.Err(); err != nil {
					fmt.Println("\n😵 读取输入出错:", err)
				} else {
					fmt.Println("\n🍂 秋天很短，北京很美，记得早点来。下次见！")
				}
				return
			}
			dest := strings.TrimSpace(scanner.Text())
			if dest == "" {
				continue
			}
			if dest == "0" || dest == "exit" || dest == "quit" || dest == "退出" {
				fmt.Println("\n🍂 秋天很短，北京很美，记得早点来。下次见！")
				return
			}

			// 库内命中：直接选定，不生成内容，随后由用户通过菜单编号查询
			if s, ok := tools.AutumnSpotMatches(dest); ok {
				currentSpot = s.Name
				fmt.Printf("\n🍁 已选定「%s」。请从菜单里选一项，查看它的相关内容（再输入 1 可重新选景点）：\n", currentSpot)
				fmt.Println(menuText)
				continue
			}

			// 库外地点：先联网搜索，判断适不适合赏秋
			fmt.Printf("\n🔧 正在联网查证：「%s」是否适合赏秋…\n", dest)
			query := dest + " 赏秋 红叶 银杏 彩叶 最佳观赏时间"
			searchOut, serr := llmTools["search_web"].Call(ctx, fmt.Sprintf(`{"query":%q}`, query))
			if serr != nil {
				fmt.Printf("----->error: 联网搜索失败：%v\n", serr)
				fmt.Println("🍂 本次联网没查到，先退回菜单，请重新选择景点。")
				fmt.Println(menuText)
				currentSpot = ""
				continue
			}
			fmt.Println("📄 结果：", summarize(searchOut))

			if suitable, reason := judgeAutumnSuitable(ctx, client, dest, searchOut); !suitable {
				fmt.Printf("\n🍂 「%s」赏秋指数不高，不建议专程去赏秋。%s\n", dest, reason)
				fmt.Println("👉 已退回菜单，请重新选择其他景点（输入 1 重新选景点）。")
				fmt.Println(menuText)
				currentSpot = ""
				continue
			} else {
				fmt.Printf("\n🍁 已选定「%s」（精选库外，内容来自联网，请以官方为准）。请从菜单里选一项：\n", dest)
				if reason != "" {
					fmt.Println("（联网判断：" + reason + "）")
				}
				fmt.Println(menuText)
				currentSpot = dest
				continue
			}
		}

		question, ok := menuPrompts[input]
		if !ok {
			fmt.Println("👉 请输入菜单里的数字（1-11），或 0 退出，/help 查看命令。")
			fmt.Println(menuText)
			continue
		}

		// 已选定景点时，把景点作为上下文带上，让各菜单项围绕它回答
		if currentSpot != "" {
			question = "[当前景点：" + currentSpot + "] " + question
		}

		session.AddMessage(openai.UserMessage(question))
		runAndRemember(question, input, currentSpot)
		fmt.Println()
	}
}
