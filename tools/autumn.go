package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

// AutumnSpot 北京赏秋地点
type AutumnSpot struct {
	Name      string   // 地点名称
	District  string   // 所在区
	Type      string   // 类型：银杏 / 红叶 / 彩叶 / 古树银杏
	BestStart string   // 最佳观赏期开始  MM-DD
	BestEnd   string   // 最佳观赏期结束  MM-DD
	Address   string   // 地址
	Ticket    string   // 门票
	Highlight string   // 一句话亮点（抖音风）
	Tags      []string // 标签
	Traffic   string   // 交通
	Tip       string   // 避坑提醒
}

// beijingAutumnSpots 精选北京赏秋地点库（本地向导私藏版）
var beijingAutumnSpots = []AutumnSpot{
	{"香山公园", "海淀区", "红叶", "10-20", "11-10", "北京市海淀区买卖街40号", "门票10元（旺季）",
		"漫山红遍时，老北京人一年最上头的那天就在这儿", []string{"黄栌", "登高", "经典"}, "西郊线 香山站", "周末人从众，工作日上午去，可坐索道上下山"},
	{"钓鱼台银杏大道", "海淀区", "银杏", "10-28", "11-15", "北京市海淀区三里河路钓鱼台国宾馆东墙外", "免费",
		"一整条街的金色隧道，随手一拍就是壁纸级大片", []string{"银杏", "免费", "情侣"}, "地铁1号线 木樨地站", "清晨人少光最好，别踩草坪别摇树"},
	{"地坛公园银杏大道", "东城区", "银杏", "11-01", "11-18", "北京市东城区安定门外大街", "门票2元",
		"北京最经典的银杏大道，一脚踏进金色的海", []string{"银杏", "市井", "文艺"}, "地铁2号线 雍和宫站", "银杏文化节期间最热闹，早上去光线最佳"},
	{"五塔寺（真觉寺）", "海淀区", "古树银杏", "10-25", "11-12", "北京市海淀区五塔寺村24号", "免费",
		"600岁金刚宝座塔配两株古银杏，安静得能听见风", []string{"古银杏", "石刻", "小众"}, "地铁4号线 国家图书馆站", "小众宝藏人少出片，隔壁就是北京动物园"},
	{"大觉寺", "海淀区", "古树银杏", "10-25", "11-12", "北京市海淀区苏家坨镇大觉寺路9号", "门票20元",
		"千年银杏王，一树金黄压满整座古寺的秋天", []string{"古银杏", "红墙", "禅意"}, "公交633路 大觉寺站", "山门内那棵千年银杏是主角，配着玉兰别有意境"},
	{"潭柘寺", "门头沟区", "古树银杏", "10-25", "11-15", "北京市门头沟区潭柘寺镇", "门票55元",
		"先有潭柘寺后有北京城，帝王树下满院鎏金", []string{"帝王树", "古寺", "千年"}, "地铁S1线 + 公交931路", "帝王树银杏是招牌，山路多，记得穿舒服的鞋"},
	{"奥林匹克森林公园", "朝阳区", "彩叶", "10-15", "11-05", "北京市朝阳区科荟路33号", "免费",
		"城区最大彩叶林，跑步党最爱的五彩秋天", []string{"彩叶", "免费", "亲子"}, "地铁8号线 森林公园南门站", "南园人多北园安静，适合骑行和野餐"},
	{"三里屯东五街银杏大道", "朝阳区", "银杏", "11-01", "11-18", "北京市朝阳区三里屯东五街", "免费",
		"使馆区限定的金色街区，CBD 里藏着的秋天彩蛋", []string{"银杏", "城市", "街拍"}, "地铁10号线 农业展览馆站", "只有秋天这条街才这么美，工作日更清静"},
	{"慕田峪长城", "怀柔区", "红叶", "10-15", "11-05", "北京市怀柔区渤海镇慕田峪村", "门票45元（不含缆车）",
		"长城蜿蜒着扎进红叶海，这才叫中国秋天的排面", []string{"长城", "红叶", "登高"}, "东直门乘916快 + 换乘", "比八达岭人少，坐缆车省力，尽量早点去避人流"},
	{"坡峰岭", "房山区", "红叶", "10-18", "11-08", "北京市房山区周口店镇黄山店村", "门票50元",
		"京郊红叶天花板，漫山红得像打翻了调色盘", []string{"黄栌", "红叶", "徒步"}, "地铁房山线 + 公交F31", "海拔不高但路长，穿运动鞋，山下的柿子别错过"},
	{"景山公园", "西城区", "彩叶", "10-20", "11-12", "北京市西城区景山西街44号", "门票2元",
		"站上中轴线，看故宫红墙配上整片金秋", []string{"中轴线", "故宫", "俯瞰"}, "地铁8号线 什刹海站", "万春亭拍故宫全景，日落前光线最绝"},
	{"北海公园", "西城区", "银杏", "10-25", "11-15", "北京市西城区文津街1号", "门票10元（旺季）",
		"白塔映金叶，划一叶小船赏尽古典浪漫", []string{"白塔", "银杏", "划船"}, "地铁6号线 北海北站", "北门进人少，太液池边的银杏最出片"},
	{"红螺寺", "怀柔区", "古树银杏", "10-25", "11-12", "北京市怀柔区怀柔镇卢庄村北", "门票54元",
		"千年雌雄银杏，一树金黄一树翠，锁住整个秋天", []string{"古银杏", "祈福", "御竹林"}, "东直门乘916快 + 换乘", "大雄宝殿前两株千年银杏必看，还能免费品茶"},
	{"玉渊潭公园", "海淀区", "彩叶", "10-20", "11-10", "北京市海淀区西三环中路10号", "门票2元",
		"靠樱花出名的园子，秋天一样美到犯规", []string{"彩叶", "湖景", "citywalk"}, "地铁16号线 玉渊潭东门站", "湖边水杉和秋叶倒影绝了，适合慢慢走"},
	{"上方山国家森林公园", "房山区", "红叶", "10-18", "11-08", "北京市房山区韩村河镇圣水峪村", "门票40元",
		"藏在房山的彩叶秘境，人少景美还超好出片", []string{"红叶", "秘境", "登山"}, "地铁房山线 + 公交F25", "小众版香山，云水洞和兜率寺可顺路一并逛"},
	{"圆明园遗址公园", "海淀区", "银杏", "10-20", "11-15", "北京市海淀区清华西路28号", "门票10元（西洋楼遗址区15元，联票25元）",
		"断壁残垣配一地金黄，一半沧桑一半灿烂", []string{"银杏", "遗址", "银杏大道"}, "地铁4号线 圆明园站", "三园交界和遗址东口的银杏大道最出片，园区落叶缓扫"},
	{"什刹海（后海）", "西城区", "银杏", "10-25", "11-15", "北京市西城区羊房胡同23号", "免费",
		"一汪什刹海半部京城史，银杏落进胡同里", []string{"银杏", "胡同", "划船"}, "地铁8号线 什刹海站", "银锭桥看落日最绝，往小胡同里钻更出片"},
	{"天坛公园", "东城区", "银杏", "10-28", "11-15", "北京市东城区天坛路甲1号", "门票15元（旺季）",
		"祈年殿的红墙下，百棵银杏披上黄金衣", []string{"银杏", "祈年殿", "红墙"}, "地铁5号线 天坛东门站", "北天门内那两排银杏配祈年殿，出片封神"},
	{"中山公园", "东城区", "银杏", "10-28", "11-15", "北京市东城区中华路4号", "门票3元",
		"故宫西侧的私藏银杏道，闹中取静的秋", []string{"银杏", "故宫", "小众"}, "地铁1号线 天安门西站", "社稷坛南侧的银杏大道最经典，人比地坛少"},
	{"颐和园", "海淀区", "彩叶", "10-20", "11-10", "北京市海淀区新建宫门路19号", "门票30元（旺季）",
		"西堤秋色倒映昆明湖，皇家园林的秋日画卷", []string{"彩叶", "皇家园林", "湖景"}, "地铁4号线 北宫门站", "西堤和后湖最安静，傍晚湖面倒影最美"},
	{"故宫博物院", "东城区", "银杏", "10-25", "11-12", "北京市东城区景山前街4号", "门票60元（旺季）",
		"红墙黄瓦配银杏，别处复制不来的北平秋", []string{"银杏", "红墙", "角楼"}, "地铁1号线 天安门东站", "武英殿、延禧宫的银杏最经典，角楼清晨8点前几乎无人"},
	{"八达岭国家森林公园（红叶岭）", "延庆区", "红叶", "10-10", "11-05", "北京市延庆区八达岭镇", "门票40元",
		"长城外的漫山红遍，比城里早红半个月", []string{"红叶", "长城", "登高"}, "市郊铁路S2线 八达岭站", "红叶岭人少景野，比八达岭主景区更好拍"},
	{"北宫国家森林公园", "丰台区", "彩叶", "10-15", "11-05", "北京市丰台区长辛店镇大灰厂东路55号", "门票20元",
		"京城西南的彩叶森林，人少到能包场", []string{"彩叶", "小众", "亲子"}, "地铁14号线 张郭庄站 + 公交", "山不高、路野趣十足，适合带娃慢慢逛"},
	{"百望山森林公园", "海淀区", "红叶", "10-15", "11-05", "北京市海淀区黑山扈北口19号", "门票6元",
		"离市区最近的红叶山，登顶就能望见全城", []string{"红叶", "登高", "近郊"}, "地铁16号线 马连洼站", "登顶可俯瞰京城，比香山清静太多"},
	{"妙峰山", "门头沟区", "红叶", "10-10", "11-05", "北京市门头沟区妙峰山镇涧沟村", "门票40元",
		"京西门头沟的红叶秘境，盘山路上全是秋", []string{"红叶", "山路", "秘境"}, "地铁1号线 苹果园站 + 公交929路", "盘山路自驾最美，山路弯多注意安全"},
	{"凤凰岭", "海淀区", "彩叶", "10-15", "11-05", "北京市海淀区苏家坨镇凤凰岭路19号", "门票25元",
		"三条登山步道，古寺藏在漫山彩叶里", []string{"彩叶", "登山", "古寺"}, "地铁16号线 北安河站 + 公交", "中线古刹遗址配古银杏，人少路野"},
	{"国家植物园", "海淀区", "彩叶", "10-20", "11-10", "北京市海淀区香山卧佛寺路", "门票10元（含北园）",
		"上万种植物染上秋色，科技感与浪漫齐飞", []string{"彩叶", "植物", "园林"}, "西郊线 植物园站", "北园彩叶区最集中，人少树多适合慢逛"},
	{"戒台寺", "门头沟区", "古树银杏", "10-25", "11-12", "北京市门头沟区永定镇戒台寺", "门票45元",
		"千年古银杏守着满山古松，佛门里的鎏金秋", []string{"古银杏", "古寺", "禅意"}, "地铁S1线 石门营站 + 公交", "古银杏配古松，香火清静人少出片"},
	{"喇叭沟门原始森林公园", "怀柔区", "彩叶", "09-25", "10-25", "北京市怀柔区喇叭沟门满族乡", "门票30元",
		"北京秋天最早到的地方，原始次生林的色彩爆炸", []string{"彩叶", "原始林", "最早"}, "东直门乘916快 + 换乘H线", "比市区早红近一个月，路程远建议自驾过夜"},
}

// AutumnSpotNames 返回精选库里所有赏秋地点的名称（供菜单先展示地点名）
func AutumnSpotNames() []string {
	names := make([]string, 0, len(beijingAutumnSpots))
	for _, s := range beijingAutumnSpots {
		names = append(names, s.Name)
	}
	return names
}

// NewGetAutumnSpots 查询北京赏秋地点
func NewGetAutumnSpots() *ToolDef {
	return &ToolDef{
		Name:        "get_autumn_spots",
		Description: "查询北京适合赏秋（银杏/红叶/彩叶）的地点，可按类型、行政区或关键词筛选，返回地点、最佳观赏期、门票、交通和避坑提醒",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"type": map[string]string{
					"type":        "string",
					"description": "可选，按景观类型筛选：银杏、红叶、彩叶、古树银杏",
				},
				"district": map[string]string{
					"type":        "string",
					"description": "可选，按所在区筛选，如 海淀区、怀柔区、房山区",
				},
				"keyword": map[string]string{
					"type":        "string",
					"description": "可选，关键词模糊匹配地点名称、标签或亮点，如 长城、免费、小众、中轴线",
				},
			},
			"required": []string{},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			typ, _ := args["type"].(string)
			district, _ := args["district"].(string)
			keyword, _ := args["keyword"].(string)

			var matched []AutumnSpot
			for _, s := range beijingAutumnSpots {
				if typ != "" && !strings.Contains(s.Type, typ) {
					continue
				}
				if district != "" && !strings.Contains(s.District, district) {
					continue
				}
				if keyword != "" && !spotMatchKeyword(s, keyword) {
					continue
				}
				matched = append(matched, s)
			}

			if len(matched) == 0 {
				return "本精选库里没有匹配结果。如果用户点名了某个具体地点（如什刹海），不要改用 district/type 宽筛去找同区/同类的替代景点，必须立刻联网搜索用户问的这个具体地点。", nil
			}

			// 命中结果里如果全是宽筛出来的近似地点，也要提示模型别拿它们当用户点名的那个地点
			var b strings.Builder
			fmt.Fprintf(&b, "共找到 %d 个赏秋地点：\n", len(matched))
			for i, s := range matched {
				fmt.Fprintf(&b, "\n%d. %s（%s · %s）\n", i+1, s.Name, s.District, s.Type)
				fmt.Fprintf(&b, "   最佳观赏期：%s ~ %s\n", s.BestStart, s.BestEnd)
				fmt.Fprintf(&b, "   亮点：%s\n", s.Highlight)
				fmt.Fprintf(&b, "   地址：%s｜门票：%s\n", s.Address, s.Ticket)
				fmt.Fprintf(&b, "   交通：%s\n", s.Traffic)
				fmt.Fprintf(&b, "   标签：%s\n", strings.Join(s.Tags, "、"))
				fmt.Fprintf(&b, "   提醒：%s\n", s.Tip)
			}
			return b.String(), nil
		},
	}
}

func spotMatchKeyword(s AutumnSpot, keyword string) bool {
	if strings.Contains(s.Name, keyword) || strings.Contains(s.Highlight, keyword) || strings.Contains(s.District, keyword) {
		return true
	}
	for _, t := range s.Tags {
		if strings.Contains(t, keyword) {
			return true
		}
	}
	return false
}

// AutumnSpotMatches 判断某个名称是否命中精选库；命中则返回规范化后的地点
func AutumnSpotMatches(name string) (AutumnSpot, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return AutumnSpot{}, false
	}
	for _, s := range beijingAutumnSpots {
		if s.Name == name {
			return s, true
		}
	}
	for _, s := range beijingAutumnSpots {
		if strings.Contains(s.Name, name) {
			return s, true
		}
	}
	return AutumnSpot{}, false
}

// NewGetAutumnIndex 查询北京赏秋地点当前的观赏指数（结合今天的日期判断是否正当季）
func NewGetAutumnIndex() *ToolDef {
	return &ToolDef{
		Name:        "get_autumn_index",
		Description: "查询北京赏秋地点当前的观赏指数（红叶/银杏是否正当季、还差几天、是否已过季），不传地点则返回全部地点的指数",
		Params: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"spot": map[string]string{
					"type":        "string",
					"description": "可选，地点名称关键词，如 香山、钓鱼台；不传则返回全部",
				},
			},
			"required": []string{},
		},
		Fn: func(ctx context.Context, args map[string]any) (string, error) {
			spot, _ := args["spot"].(string)
			now := time.Now()

			var targets []AutumnSpot
			for _, s := range beijingAutumnSpots {
				if spot == "" || spotMatchKeyword(s, spot) {
					targets = append(targets, s)
				}
			}
			if len(targets) == 0 {
				return fmt.Sprintf("没找到和「%s」匹配的赏秋地点，换个说法再试试。", spot), nil
			}

			var b strings.Builder
			fmt.Fprintf(&b, "今天日期：%s\n", now.Format("2006-01-02"))
			for _, s := range targets {
				status, stars, days := autumnStatus(s, now)
				fmt.Fprintf(&b, "\n%s（%s · %s）\n", s.Name, s.District, s.Type)
				fmt.Fprintf(&b, "   观赏指数：%s\n", stars)
				fmt.Fprintf(&b, "   状态：%s\n", status)
				if days != 0 {
					fmt.Fprintf(&b, "   距最佳观赏期：%d 天\n", days)
				}
			}
			return b.String(), nil
		},
	}
}

// SpotIndex 返回某地点当前的观赏指数与状态描述，供 MCP 服务器等外部调用
func SpotIndex(name string, now time.Time) (AutumnSpot, string, bool) {
	s, ok := AutumnSpotMatches(name)
	if !ok {
		return AutumnSpot{}, "", false
	}
	status, stars, days := autumnStatus(s, now)
	text := stars + " " + status
	if days != 0 {
		text += fmt.Sprintf("（距最佳观赏期 %d 天）", days)
	}
	return s, text, true
}

// AllAutumnSpots 返回精选库全部地点，供外部只读使用
func AllAutumnSpots() []AutumnSpot {
	out := make([]AutumnSpot, len(beijingAutumnSpots))
	copy(out, beijingAutumnSpots)
	return out
}

// autumnStatus 根据当前日期计算某地点的观赏状态、指数星级、距最佳期天数
func autumnStatus(s AutumnSpot, now time.Time) (status, stars string, days int) {
	year := now.Year()
	start, err1 := time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", year, s.BestStart), now.Location())
	end, err2 := time.ParseInLocation("2006-01-02", fmt.Sprintf("%d-%s", year, s.BestEnd), now.Location())
	if err1 != nil || err2 != nil {
		return "最佳观赏期数据异常", "☆☆☆☆☆", 0
	}

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	switch {
	case today.Before(start):
		days = int(start.Sub(today).Hours() / 24)
		return fmt.Sprintf("🍃 还没到最佳观赏期，再等 %d 天，先收藏！", days), "★★☆☆☆", days
	case today.After(end):
		return "🍂 最佳观赏期已过，只能等明年啦，先码住这条攻略", "★☆☆☆☆", 0
	}
	total := end.Sub(start).Hours()
	passed := today.Sub(start).Hours()
	switch ratio := passed / total; {
	case ratio < 0.2:
		return "✨ 刚开季！人少景美，冲第一波最划算", "★★★★☆", 0
	case ratio > 0.8:
		return "⏰ 已到尾声，再不去就要等一年了，抓紧冲！", "★★★★☆", 0
	default:
		return "🔥 正当季！这就是一年里最好看的那些天，别犹豫", "★★★★★", 0
	}
}
