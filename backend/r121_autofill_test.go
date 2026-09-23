package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// R121：WORKLIST-R121-AUTOFILL-LABEL-GAPS 的验收测试。
//
//   - P1-1 补位标签总开关默认关闭；关闭时 JSON 里没有 autofill 键，但诊断计数
//     照常产出（否则永远拿不到真机数据）。
//   - P2-1 「这名玩家 teamPosition 为空就跳过」的护栏必须有测试（R119 的对抗
//     变异里它是唯一存活的一条，对应 Riot developer-relations issue #554）。
//   - P2-2 整场证据门槛从「任意 1 人有值」收紧到「覆盖率 ≥ 90%」，并加一道
//     候选占比护栏。
//
// fixture 复用 r119_autofill_test.go 里的 r119ClassicInfo（10 人、默认全场
// 证据齐全、零候选）。

// r121OpenGate 打开总开关并在用例结束时恢复，避免污染同包其它测试。
func r121OpenGate(t *testing.T) {
	t.Helper()
	previous := autofillLabelGate
	autofillLabelGate = true
	t.Cleanup(func() { autofillLabelGate = previous })
}

func TestR121AutofillLabelGateIsClosedByDefault(t *testing.T) {
	// P1-1 的核心判据：真机对照结果回填 docs/r119-execution-ledger.md §5 之前，
	// 这个开关必须是 false。把它改成 true 的改动必须让本测试 FAIL。
	if autofillLabelGate {
		t.Fatal("autofillLabelGate 默认必须为 false（R121 P1-1）：口径尚未经真机验证")
	}
}

func TestR121GateClosedSuppressesFlagsButKeepsCandidateMath(t *testing.T) {
	info := r119ClassicInfo(420, map[int][2]string{9: {"UTILITY", ""}})

	// 纯计算不受开关影响：诊断计数依赖它。
	candidates, candidateEvidence := riotAutofillCandidates(info)
	if !candidateEvidence || !candidates[9] || r119CountFlags(candidates) != 1 {
		t.Fatalf("候选计算不应受开关影响：flags = %#v, evidence = %v", candidates, candidateEvidence)
	}

	// 对外口径必须全 false、evidence=false。
	flags, evidence := riotAutofillFlags(info)
	if evidence {
		t.Fatal("开关关闭时 evidence 必须为 false")
	}
	if got := r119CountFlags(flags); got != 0 {
		t.Fatalf("开关关闭时候选人数必须为 0，实际 %d", got)
	}
	if len(flags) != len(info.Participants) {
		t.Fatalf("开关关闭时也必须返回等长切片，got %d, want %d", len(flags), len(info.Participants))
	}
	if flags, evidence := riotAutofillFlags(nil); flags != nil || evidence {
		t.Fatalf("nil 对局必须安全返回：%#v, %v", flags, evidence)
	}

	// 开关打开后，同一份输入必须真的产出标签（否则「打开」这条路径没人验证过）。
	r121OpenGate(t)
	flags, evidence = riotAutofillFlags(info)
	if !evidence || !flags[9] || r119CountFlags(flags) != 1 {
		t.Fatalf("开关打开时必须产出候选：flags = %#v, evidence = %v", flags, evidence)
	}
}

func TestR121ConvertRiotMatchInfoTagsCandidateOnlyWhenGateOpens(t *testing.T) {
	newInfo := func() *riotMatchInfo {
		return r119ClassicInfo(420, map[int][2]string{9: {"UTILITY", ""}})
	}

	// 默认（开关关闭）：一个标签都没有，JSON 里连 autofill 键都不出现。
	closed := convertRiotMatchInfo(newInfo(), "", nil, nil, "", "HN1")
	payload, err := json.Marshal(closed.Participants)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "autofill") {
		t.Fatalf("开关关闭时 JSON 里不得出现 autofill 键（既有响应逐字节不变）：%s", payload)
	}

	// 打开开关：只有下标 9 带标签，其余 9 人不受影响，且 omitempty 生效。
	r121OpenGate(t)
	opened := convertRiotMatchInfo(newInfo(), "", nil, nil, "", "HN1")
	if !opened.Participants[9].Autofill {
		t.Fatal("开关打开后，个人位置缺失的参与者必须被标为候选")
	}
	if opened.Participants[9].Position != "utility" {
		t.Fatalf("候选玩家仍要显示最终分路，got %q", opened.Participants[9].Position)
	}
	for index := 0; index < 9; index++ {
		if opened.Participants[index].Autofill {
			t.Fatalf("Participants[%d] 不该被标为候选", index)
		}
	}
	payload, err = json.Marshal(opened.Participants)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(payload), `"autofill":true`); got != 1 {
		t.Fatalf("JSON 里 autofill:true 出现 %d 次，want 1：%s", got, payload)
	}
	if strings.Contains(string(payload), `"autofill":false`) {
		t.Fatalf("omitempty 必须让非候选参与者不带该字段：%s", payload)
	}
}

// P2-1：teamPosition 为空的玩家是数据缺失，不是补位证据。
// 对抗变异：删掉 riotAutofillCandidates 里的
// `if riotLaneKeyValue(raw.TeamPosition) == "" { continue }`，本测试必须 FAIL。
func TestR121ParticipantWithoutTeamPositionIsNeverTagged(t *testing.T) {
	for name, individual := range map[string]string{"Invalid": "Invalid", "空串": ""} {
		t.Run(name, func(t *testing.T) {
			r121OpenGate(t)
			// Riot issue #554 的形状：individualPosition=INVALID 且 teamPosition 为空。
			// 其余 9 人证据齐全，覆盖率恰好 9/10 达标，所以这一场不会被整场降级——
			// 唯一挡住误标的就是那条 continue。
			info := r119ClassicInfo(420, map[int][2]string{9: {"", individual}})
			flags, evidence := riotAutofillCandidates(info)
			if !evidence {
				t.Fatal("这一场证据齐全，不该整场降级（否则本用例测不到目标护栏）")
			}
			if got := r119CountFlags(flags); got != 0 {
				t.Fatalf("连最终分路都没有的玩家不能被标为候选，实际候选 %d 个：%#v", got, flags)
			}

			match := convertRiotMatchInfo(info, "", nil, nil, "", "HN1")
			for index, participant := range match.Participants {
				if participant.Autofill {
					t.Fatalf("Participants[%d].Autofill 必须为 false", index)
				}
			}
			// 其余 9 人的分路展示不受影响。
			for index := 0; index < 9; index++ {
				if match.Participants[index].Position == "" {
					t.Fatalf("Participants[%d].Position 不该为空", index)
				}
			}
		})
	}
}

// P2-2：整场证据门槛。对抗变异：把覆盖率判断改回「任意 1 人有值」
// （即 autofillCoverageInsufficient 恒返回 false），前两行用例必须 FAIL。
func TestR121WholeGameDegradesWhenCoverageIsLow(t *testing.T) {
	lanes := []string{"TOP", "JUNGLE", "MIDDLE", "BOTTOM", "UTILITY"}
	tests := []struct {
		name         string
		missing      int
		wantEvidence bool
		wantTagged   int
	}{
		{"只有 1 人有个人位置、其余 9 人缺失", 9, false, 0},
		{"5 人缺失、5 人正常", 5, false, 0},
		{"2 人缺失（覆盖率 80%）", 2, false, 0},
		{"1 人缺失（覆盖率 90%，恰好达标）", 1, true, 1},
		{"全员齐全", 0, true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r121OpenGate(t)
			overrides := map[int][2]string{}
			for index := 0; index < test.missing; index++ {
				overrides[index] = [2]string{lanes[index%5], ""}
			}
			flags, evidence := riotAutofillCandidates(r119ClassicInfo(420, overrides))
			if evidence != test.wantEvidence {
				t.Fatalf("evidence = %v, want %v", evidence, test.wantEvidence)
			}
			if got := r119CountFlags(flags); got != test.wantTagged {
				t.Fatalf("候选人数 = %d, want %d（%#v）", got, test.wantTagged, flags)
			}
		})
	}
}

func TestR121CoverageAndRatioThresholds(t *testing.T) {
	// 覆盖率门槛 90%：10 人局最多允许 1 人缺失。
	if !autofillCoverageInsufficient(8, 10) {
		t.Fatal("8/10（80%）必须判为覆盖不足")
	}
	if autofillCoverageInsufficient(9, 10) {
		t.Fatal("9/10（90%）恰好达标，不该判为不足")
	}
	if !autofillCoverageInsufficient(1, 10) {
		t.Fatal("1/10 必须判为覆盖不足（R121 P2-2 的实测误标形状）")
	}
	if !autofillCoverageInsufficient(0, 0) {
		t.Fatal("空对局必须判为覆盖不足")
	}

	// 候选占比门槛 30%。注意：当前覆盖率护栏更严（≥90% 覆盖意味着最多 10%
	// 候选），所以这条护栏在 riotAutofillCandidates 的正常路径上轮不到触发；
	// 保留它是为了防止将来放宽覆盖率时出现大面积误标，因此这里直接单测函数。
	if autofillFlagRatioExcessive(3, 10) {
		t.Fatal("3/10（30%）仍在允许范围内")
	}
	if !autofillFlagRatioExcessive(4, 10) {
		t.Fatal("4/10（40%）必须判为占比过高")
	}
	if !autofillFlagRatioExcessive(9, 10) {
		t.Fatal("9/10 必须判为占比过高")
	}
	if autofillFlagRatioExcessive(0, 0) {
		t.Fatal("空对局不该判为占比过高")
	}
}

// P1-1 要求：开关关闭期间诊断计数照常统计，否则真机数据永远收不上来。
func TestR121DiagnosticsKeepCountingWhileGateClosed(t *testing.T) {
	if autofillLabelGate {
		t.Fatal("本用例必须在总开关关闭的前提下运行")
	}
	degraded := r119ClassicInfo(420, nil)
	for index := range degraded.Participants {
		degraded.Participants[index].IndividualPosition = ""
	}
	summary := summarizeRiotParticipants([]*riotMatchInfo{
		// 1 个候选 + 1 个位置不一致 + 8 个正常。
		r119ClassicInfo(420, map[int][2]string{8: {"MIDDLE", "BOTTOM"}, 9: {"UTILITY", ""}}),
		// 整场缺证据 → 计一次降级。
		degraded,
		// 非单双排/灵活组排 → 完全不进计数。
		r119ClassicInfo(450, map[int][2]string{9: {"UTILITY", ""}}),
	})
	if summary.AutofillCandidates != 1 {
		t.Fatalf("AutofillCandidates = %d, want 1（开关关闭时也必须统计）", summary.AutofillCandidates)
	}
	if summary.PositionMismatch != 1 {
		t.Fatalf("PositionMismatch = %d, want 1", summary.PositionMismatch)
	}
	if summary.AutofillNoEvidence != 1 {
		t.Fatalf("AutofillNoEvidence = %d, want 1", summary.AutofillNoEvidence)
	}
}

// P1-2：口径改名后，业务代码里不得再出现把推断当事实的措辞。
func TestR121NoUnprovenWordingInBusinessCode(t *testing.T) {
	for _, file := range []string{"riot_api.go", "gameplay.go", filepath.Join("web", "gameplay.js")} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		body := string(raw)
		for _, banned := range []string{"由系统补位", "他自己选到的"} {
			if strings.Contains(body, banned) {
				t.Fatalf("%s 里仍出现 %q：R121 P1-2 要求提示与注释只陈述数据事实", file, banned)
			}
		}
		// 旧的、把推断当事实的诊断键名：R121 P1-2 把 autofill_swapped 改成
		// position_mismatch；本轮（R122 P3-1）把 autofill_tagged 改成
		// autofill_candidates，因为开关关闭时一个标签都没打。
		for _, banned := range []string{"autofill_swapped", "autofill_tagged"} {
			if strings.Contains(body, banned) {
				t.Fatalf("%s 里仍出现旧诊断键名 %q：必须用中性名（position_mismatch / autofill_candidates）", file, banned)
			}
		}
	}
}
