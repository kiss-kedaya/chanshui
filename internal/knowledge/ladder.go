package knowledge

// Anchors are the scorable 2025-2026 events. Questions run serially.
// Scheduled events count only when the result is known, not when the date is known.
var Anchors = []Probe{
	{ID: "trump", HappenedAt: "2025-01-20", Event: "特朗普就任",
		Question: "2025年1月20日，唐纳德·特朗普是否已经就任美国总统？如果已经就职，请给出日期。",
		Accept:   [][]string{{"特朗普", "就任"}, {"特朗普", "就职"}, {"特朗普", "宣誓"}, {"trump", "inauguration"}, {"trump", "inaugurated"}, {"trump", "sworn"}},
		Wrong:    [][]string{{"拜登", "就任"}, {"biden", "inauguration"}}},
	{ID: "germany", HappenedAt: "2025-02-23", Event: "德国联邦议院选举",
		Question: "2025年2月23日德国联邦议院选举，联盟党（基民盟/基社盟）是否获胜？弗里德里希·默茨是否成为总理或总理人选？",
		Accept:   [][]string{{"默茨"}, {"merz"}, {"联盟党", "获胜"}, {"联盟党", "胜选"}, {"基民盟", "获胜"}, {"cdu", "won"}, {"cdu", "victory"}},
		Wrong:    [][]string{{"社民党", "获胜"}, {"绿党", "获胜"}, {"选择党", "获胜"}, {"afd", "won"}}},
	{ID: "carney", HappenedAt: "2025-03-14", Event: "卡尼就任加拿大总理",
		Question: "2025年3月14日，马克·卡尼是否已经就任加拿大总理？如果已经就任，请给出日期。",
		Accept:   [][]string{{"卡尼", "总理"}, {"卡尼", "就任"}, {"卡尼", "就职"}, {"carney", "prime minister"}, {"carney", "premier"}},
		Wrong:    [][]string{{"特鲁多", "继续任"}, {"trudeau", "remains"}}},
	{ID: "yoon", HappenedAt: "2025-04-04", Event: "尹锡悦被罢免",
		Question: "2025年4月4日，韩国宪法法院是否罢免总统尹锡悦？",
		Accept:   [][]string{{"罢免"}, {"弹劾成立"}, {"被弹劾"}, {"impeached"}, {"removed from office"}},
		Wrong:    [][]string{{"驳回弹劾"}, {"弹劾被驳回"}, {"未被罢免"}, {"没有罢免"}}},
	{ID: "francis", HappenedAt: "2025-04-21", Event: "方济各去世",
		Question: "2025年4月21日，教皇方济各是否已经去世？如果已经去世，请给出日期。",
		Accept:   [][]string{{"方济各", "去世"}, {"方济各", "逝世"}, {"方济各", "辞世"}, {"francis", "died"}, {"francis", "death"}, {"pope francis", "died"}},
		Wrong:    [][]string{{"还在世"}, {"仍然在世"}, {"仍在世"}, {"没有去世"}, {"still alive"}}},
	{ID: "leo", HappenedAt: "2025-05-08", Event: "利奥十四世当选",
		Question: "2025年5月8日，利奥十四世（Leo XIV，也译良十四世）是否当选为新教皇？如果已经当选，请说明。",
		Accept:   [][]string{{"利奥十四"}, {"良十四"}, {"leo xiv"}, {"leo 14"}},
		Wrong:    [][]string{{"没有选出"}, {"尚未选出"}}},
	{ID: "lee", HappenedAt: "2025-06-03", Event: "李在明当选",
		Question: "2025年6月3日韩国总统选举，李在明是否当选总统？",
		Accept:   [][]string{{"李在明", "当选"}, {"李在明", "胜"}, {"lee jae", "elected"}, {"lee jae-myung", "president"}},
		Wrong:    [][]string{{"李在明", "落选"}, {"金文洙", "当选"}}},
	{ID: "japan", HappenedAt: "2025-07-20", Event: "日本参议院选举",
		Question: "2025年7月20日日本参议院选举，执政联盟（自民党与公明党）是否失去参议院多数？石破茂是否辞职？",
		Accept:   [][]string{{"失去多数"}, {"丢掉多数"}, {"未过半"}, {"不过半"}, {"没有过半"}, {"失去过半"}, {"未能维持多数"}, {"lost its majority"}, {"lost the majority"}, {"石破", "辞职"}, {"ishiba", "resign"}},
		Wrong:    [][]string{{"保住多数"}, {"维持多数"}, {"继续过半"}, {"赢得多数"}, {"获得多数"}, {"kept its majority"}}},
	{ID: "kirk", HappenedAt: "2025-09-10", Event: "查理·柯克遇刺",
		Question: "2025年9月10日，查理·柯克（Charlie Kirk）是否在犹他谷大学遇刺身亡？",
		Accept:   [][]string{{"遇刺"}, {"刺杀"}, {"assassinated"}, {"柯克", "身亡"}, {"kirk", "killed"}, {"kirk", "shot"}},
		Wrong:    [][]string{{"没有遇刺"}, {"未遇刺"}, {"没有遇害"}, {"健在"}}},
	{ID: "takaichi_ldp", HappenedAt: "2025-10-04", Event: "高市早苗当选自民党总裁",
		Question: "2025年10月4日，高市早苗是否当选日本自民党总裁？",
		Accept:   [][]string{{"高市", "总裁"}, {"高市", "党首"}, {"takaichi", "president"}, {"takaichi", "elected"}},
		Wrong:    [][]string{{"高市", "落选"}, {"没有当选"}, {"未当选总裁"}}},
	{ID: "takaichi_pm", HappenedAt: "2025-10-21", Event: "高市早苗就任首相",
		Question: "2025年10月21日，高市早苗是否就任日本首相？如果已经就任，请给出日期。",
		Accept:   [][]string{{"高市", "首相"}, {"takaichi", "prime minister"}, {"高市", "组阁"}},
		Wrong:    [][]string{{"不是首相"}, {"未就任首相"}, {"没有出任首相"}}},
	{ID: "mamdani", HappenedAt: "2025-11-04", Event: "马姆达尼当选纽约市长",
		Question: "2025年11月4日纽约市长选举，佐兰·马姆达尼（Zohran Mamdani）是否当选纽约市长？",
		Accept:   [][]string{{"马姆达尼当选"}, {"mamdani elected"}, {"mamdani", "winner"}, {"马姆达尼胜"}},
		Wrong:    [][]string{{"库默当选"}, {"科莫当选"}, {"cuomo", "elected"}, {"cuomo", "won"}, {"马姆达尼", "落选"}}},
	{ID: "bulgaria", HappenedAt: "2026-01-01", Event: "保加利亚加入欧元区",
		Question: "2026年1月1日，保加利亚是否已经加入欧元区？如果只知道预定加入、不知道是否已经生效，请回答“只知道日程，不知道结果”。",
		Accept:   [][]string{{"已经加入"}, {"已加入"}, {"正式加入"}, {"加入了"}, {"开始使用欧元"}, {"采用了欧元"}, {"成为欧元区"}, {"进入欧元区"}, {"joined the euro"}},
		Wrong:    [][]string{{"没有加入"}, {"未加入"}, {"推迟加入"}, {"未能加入"}}},
	{ID: "olympics", HappenedAt: "2026-02-22", Event: "米兰冬奥会闭幕",
		Question: "2026年米兰-科尔蒂纳冬奥会是否已经闭幕？如果已闭幕，请说出奖牌榜第一的国家。如果只知道将举办，请回答“只知道日程，不知道结果”。",
		Accept:   [][]string{{"挪威"}, {"norway"}, {"已闭幕"}, {"已经闭幕"}, {"闭幕了"}},
		Wrong:    [][]string{{"美国", "第一"}, {"意大利", "第一"}, {"德国", "第一"}, {"瑞典", "第一"}, {"加拿大", "第一"}, {"奖牌榜第一", "美国"}, {"medal table", "united states"}}},
	{ID: "worldcup", HappenedAt: "2026-07-19", Event: "世界杯决赛",
		Question: "2026年美加墨世界杯是否已经结束？冠军是哪支球队？制胜球是谁打进的？如果只知道赛程，请回答“只知道日程，不知道结果”。",
		Accept:   [][]string{{"西班牙夺冠"}, {"西班牙冠军"}, {"冠军是西班牙"}, {"spain", "champion"}, {"费兰"}, {"ferran"}, {"托雷斯"}},
		Wrong:    [][]string{{"阿根廷冠军"}, {"阿根廷夺冠"}, {"冠军是阿根廷"}, {"argentina champion"}, {"法国冠军"}, {"巴西冠军"}}},
}

func anchorByID(id string) Probe {
	for _, p := range Anchors {
		if p.ID == id {
			return p
		}
	}
	panic("unknown anchor " + id)
}
