package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// R119 引入、R121 收紧的「补位」标签测试。
//
// R121 之后的三条前提，改这里之前先读 docs/r119-execution-ledger.md §7 与
// WORKLIST-R121：
//  1. match-v5 的 teamPosition / individualPosition 按 Riot 官方定义都是服务器
//     **推算**的「最可能打的位置」，不记录大厅选位，也不记录是否补位。所以这里
//     测的是「候选计算」，不是「补位事实」。
//  2. 纯计算函数是 riotAutofillCandidates（不受总开关影响，诊断计数走它）；
//     对外口径是 riotAutofillFlags（受 autofillLabelGate 门控，默认关闭）。
//     本文件的用例一律打在 candidates 上，因此开关开或关都必须通过。
//  3. 整场护栏是「有效 individualPosition 覆盖率 ≥ 90%」，10 人局里最多允许
//     1 人缺失。fixture 因此默认造成 10 人、全场证据齐全的形状。

// r119ClassicInfo 造一场 10 人的召唤师峡谷对局。默认每个位置的两个字段一致
// （正常选位、全场证据齐全、零候选），overrides 按下标覆盖 teamPosition 与
// individualPosition，用例只需要声明它关心的那几个异常位。
func r119ClassicInfo(queueID int64, overrides map[int][2]string) *riotMatchInfo {
	// 刻意不给 queueID 兜底：0 本身就是要被测的「未知队列」，兜成 420 会让
	// TestR119AutofillCandidatesStayInsideClassicRankedQueues 的第一条用例失效。
	lanes := [5]string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	participants := make([]riotParticipant, 10)
	for index := range participants {
		team, individual := lanes[index%5], lanes[index%5]
		if override, ok := overrides[index]; ok {
			team, individual = override[0], override[1]
		}
		participants[index] = riotParticipant{
			PUUID: "p" + string(rune('a'+index)), TeamID: 100, ChampionID: 1,
			TeamPosition: team, IndividualPosition: individual,
		}
	}
	return &riotMatchInfo{
		GameID: 1, QueueID: queueID, GameMode: "CLASSIC", GameType: "MATCHED_GAME",
		GameDuration: 1800, Participants: participants,
	}
}

func r119CountFlags(flags []bool) int {
	count := 0
	for _, flag := range flags {
		if flag {
			count++
		}
	}
	return count
}

func TestR119AutofillCandidatesOnlyMarkMissingIndividualPosition(t *testing.T) {
	for name, individual := range map[string]string{"空串": "", "Invalid": "Invalid", "invalid 小写": "invalid", "纯空白": "   "} {
		t.Run(name, func(t *testing.T) {
			// 10 人里只有下标 9 的个人位置拿不到：覆盖率恰好 9/10 达标，
			// 因此只有他是候选，其余 9 人不受影响。
			flags, evidence := riotAutofillCandidates(r119ClassicInfo(420, map[int][2]string{9: {"UTILITY", individual}}))
			if !evidence {
				t.Fatal("9/10 覆盖率达标，evidence 必须为 true")
			}
			if !flags[9] {
				t.Fatalf("下标 9 必须是候选：%#v", flags)
			}
			if got := r119CountFlags(flags); got != 1 {
				t.Fatalf("候选人数 = %d, want 1（%#v）", got, flags)
			}
		})
	}

	// 两个推算位置都有值但不一致：不是候选（成因未证实，只计入 position_mismatch）。
	flags, evidence := riotAutofillCandidates(r119ClassicInfo(420, map[int][2]string{9: {"UTILITY", "MIDDLE"}}))
	if !evidence || r119CountFlags(flags) != 0 {
		t.Fatalf("位置不一致不该被当成候选：flags = %#v, evidence = %v", flags, evidence)
	}
}

func TestR119AutofillCandidatesDegradeWhenWholeGameLacksEvidence(t *testing.T) {
	// 整场都没有个人位置：绝不能读成「十个人全都是候选」。
	allMissing := r119ClassicInfo(420, nil)
	for index := range allMissing.Participants {
		allMissing.Participants[index].IndividualPosition = ""
	}
	flags, evidence := riotAutofillCandidates(allMissing)
	if evidence {
		t.Fatal("整场没有 individualPosition 时 evidence 必须为 false")
	}
	if got := r119CountFlags(flags); got != 0 {
		t.Fatalf("整场降级时候选人数必须为 0，实际 %d", got)
	}

	// 整场都没有最终分路（海克斯大乱斗的形状，见 r116e ledger §5.4）同样降级。
	noTeam := r119ClassicInfo(420, nil)
	for index := range noTeam.Participants {
		noTeam.Participants[index].TeamPosition = ""
	}
	flags, evidence = riotAutofillCandidates(noTeam)
	if evidence || r119CountFlags(flags) != 0 {
		t.Fatalf("整场没有 teamPosition 时必须降级：flags = %#v, evidence = %v", flags, evidence)
	}

	if flags, evidence := riotAutofillCandidates(nil); flags != nil || evidence {
		t.Fatalf("nil 对局必须安全返回：%#v, %v", flags, evidence)
	}
	if flags, evidence := riotAutofillCandidates(&riotMatchInfo{QueueID: 420}); flags != nil || evidence {
		t.Fatalf("空参与者列表必须安全返回：%#v, %v", flags, evidence)
	}
}

func TestR119AutofillCandidatesStayInsideClassicRankedQueues(t *testing.T) {
	for _, queueID := range []int64{0, 400, 430, 450, 700, 850, 1700, 2400, 3270} {
		flags, evidence := riotAutofillCandidates(r119ClassicInfo(queueID, map[int][2]string{9: {"UTILITY", ""}}))
		if evidence {
			t.Fatalf("队列 %d 不在单双排/灵活组排范围内，evidence 必须为 false", queueID)
		}
		if got := r119CountFlags(flags); got != 0 {
			t.Fatalf("队列 %d 的候选人数必须为 0，实际 %d", queueID, got)
		}
	}

	// 灵活组排与单双排同口径。
	flags, evidence := riotAutofillCandidates(r119ClassicInfo(440, map[int][2]string{9: {"UTILITY", ""}}))
	if !evidence || !flags[9] || r119CountFlags(flags) != 1 {
		t.Fatalf("灵活组排判定错误：flags = %#v, evidence = %v", flags, evidence)
	}
}

func TestR119ConvertRiotMatchInfoAutofillKeyFollowsGate(t *testing.T) {
	// 端到端不变式：JSON 里的 autofill 键必须与总开关一致。
	//   开关关闭（默认）→ 一个键都不出现，既有响应逐字节不变；
	//   开关打开      → 恰好一个 "autofill":true，且不出现 "autofill":false。
	// 写成不变式而不是「断言开关必须关闭」，是为了让本用例在开关开/关两种
	// 配置下都成立（工单要求「开关打开时 R119 原有测试仍然全部通过」）。
	// 变异「去掉 riotAutofillFlags 里的开关判断」会让默认状态多出 autofill 键 → FAIL。
	match := convertRiotMatchInfo(r119ClassicInfo(420, map[int][2]string{9: {"UTILITY", ""}}), "", nil, nil, "", "HN1")
	if len(match.Participants) != 10 {
		t.Fatalf("参与者数量 = %d, want 10", len(match.Participants))
	}
	tagged := 0
	for _, participant := range match.Participants {
		if participant.Autofill {
			tagged++
		}
	}
	payload, err := json.Marshal(match.Participants)
	if err != nil {
		t.Fatal(err)
	}
	if autofillLabelGate {
		if tagged != 1 || strings.Count(string(payload), `"autofill":true`) != 1 {
			t.Fatalf("开关打开时应恰好 1 个候选：tagged = %d, payload = %s", tagged, payload)
		}
	} else if tagged != 0 || strings.Contains(string(payload), "autofill") {
		t.Fatalf("开关关闭时 JSON 里不得出现 autofill 键：tagged = %d, payload = %s", tagged, payload)
	}
	if strings.Contains(string(payload), `"autofill":false`) {
		t.Fatalf("omitempty 必须让非候选参与者不带该字段：%s", payload)
	}
	// 分路展示不受影响：候选与否都不改 Position 的取值口径。
	if match.Participants[9].Position != "utility" {
		t.Fatalf("下标 9 的分路应为 utility，got %q", match.Participants[9].Position)
	}
}

func TestR119SummarizeRiotParticipantsCountsAutofillEvidence(t *testing.T) {
	// 计数走 candidates，因此与总开关无关；这里在开关关闭的默认状态下断言。
	infos := []*riotMatchInfo{
		// 第 1 场：1 个候选 + 1 个位置不一致 + 8 个正常。
		r119ClassicInfo(420, map[int][2]string{8: {"MIDDLE", "BOTTOM"}, 9: {"UTILITY", ""}}),
		// 第 2 场：全场证据齐全、零候选 → 不计降级。
		r119ClassicInfo(440, nil),
		// 第 3 场：非单双排/灵活组排，完全不进计数。
		r119ClassicInfo(450, map[int][2]string{9: {"UTILITY", ""}}),
	}
	degraded := r119ClassicInfo(420, nil)
	for index := range degraded.Participants {
		degraded.Participants[index].IndividualPosition = "Invalid"
	}
	infos = append(infos, degraded)

	summary := summarizeRiotParticipants(infos)
	if summary.AutofillCandidates != 1 {
		t.Fatalf("AutofillCandidates = %d, want 1", summary.AutofillCandidates)
	}
	if summary.PositionMismatch != 1 {
		t.Fatalf("PositionMismatch = %d, want 1", summary.PositionMismatch)
	}
	if summary.AutofillNoEvidence != 1 {
		t.Fatalf("AutofillNoEvidence = %d, want 1", summary.AutofillNoEvidence)
	}
}

func TestR119RiotPositionKeyBehaviourIsUnchangedByLaneHelper(t *testing.T) {
	// riotPositionKey 拆出 riotLaneKeyValue 之后，取值优先级必须与改动前**逐字
	// 等价**：只有 teamPosition 的原始值为空（含纯空白）才回退 individualPosition；
	// teamPosition 非空但无效（"Invalid"）时**不回退**，直接返回空串。这条不回退
	// 的行为是历史实现就有的，重构不能顺手改掉。
	tests := []struct {
		team, individual, want string
	}{
		{"TOP", "JUNGLE", "top"},
		{"", "MIDDLE", "middle"},
		{"   ", "MIDDLE", "middle"},
		{"Invalid", "BOTTOM", ""},
		{"UTILITY", "", "utility"},
		{"SUPPORT", "", "utility"},
		{"MID", "", "middle"},
		{" middle ", "", "middle"},
		{"", "", ""},
		{"Invalid", "Invalid", ""},
		{"", "Invalid", ""},
	}
	for _, test := range tests {
		got := riotPositionKey(riotParticipant{TeamPosition: test.team, IndividualPosition: test.individual})
		if got != test.want {
			t.Fatalf("riotPositionKey(%q, %q) = %q, want %q", test.team, test.individual, got, test.want)
		}
	}
}
