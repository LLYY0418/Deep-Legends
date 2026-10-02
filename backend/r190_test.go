package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestR190LCUAndRiotPerkStats(t *testing.T) {
	var game lcuGame
	err := json.Unmarshal([]byte(`{"gameId":190,"participants":[{"participantId":4,"stats":{"perk0":8005,"perk0Var1":0,"perk0Var2":0,"perk0Var3":0,"perk1":9111,"perk1Var1":804,"perk1Var2":300,"perk2":9103,"perk2Var1":0,"perk2Var2":0,"perk2Var3":0,"perk3":8017,"perk3Var1":0,"perk3Var2":0,"perk3Var3":0,"perk4":8321,"perk4Var1":0,"perk4Var2":0,"perk4Var3":0,"perk5":8347,"perk5Var1":0,"perk5Var2":0,"perk5Var3":0,"statPerk0":5005}}]}`), &game)
	if err != nil {
		t.Fatal(err)
	}
	lcu := normalizeGameplayMatch(game, gameplayReference{}, nil, nil)
	if len(lcu.Participants[0].PerkStats) != 6 || lcu.Participants[0].PerkStats[1] != (gameplayPerkStat{9111, [3]int64{804, 300, 0}}) || lcu.Participants[0].PerkStats[5].Vars != [3]int64{} {
		t.Fatalf("LCU stats=%#v", lcu.Participants[0].PerkStats)
	}
	var raw riotMatch
	err = json.Unmarshal([]byte(`{"metadata":{"matchId":"KR_190"},"info":{"gameId":190,"participants":[{"participantId":4,"perks":{"styles":[{"style":8000,"selections":[{"perk":8005,"var1":0,"var2":0,"var3":0},{"perk":9111,"var1":804,"var2":300,"var3":0},{"perk":9103,"var1":0,"var2":0,"var3":0},{"perk":8017,"var1":0,"var2":0,"var3":0}]},{"style":8300,"selections":[{"perk":8321,"var1":0,"var2":0,"var3":0},{"perk":8347,"var1":0,"var2":0,"var3":0}]}],"statPerks":{"offense":5005}}}]}}`), &raw)
	if err != nil {
		t.Fatal(err)
	}
	riot := riotConvertMatch(&raw, "", nil)
	if !reflect.DeepEqual(riot.Participants[0].PerkStats, lcu.Participants[0].PerkStats) {
		t.Fatalf("Riot stats=%#v", riot.Participants[0].PerkStats)
	}
	sgp := convertRiotMatchInfo(&raw.Info, "", nil, nil, "cn", "HN1")
	if !reflect.DeepEqual(sgp.Participants[0].PerkStats, lcu.Participants[0].PerkStats) {
		t.Fatal("SGP stats missing")
	}
}

func TestR190RiotV1ReadRefreshV2AndHTTP(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var calls atomic.Int32
	payload := `{"metadata":{"matchId":"KR_190"},"info":{"gameId":190,"participants":[{"participantId":4,"championId":22,"perks":{"styles":[{"style":8000,"selections":[{"perk":9111,"var1":804,"var2":300}]}]}}]}}`
	p := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return hexdataResponse(r, []byte(payload)), nil
	}))
	riot := newRiotProvider(p)
	var old riotMatch
	if err := json.Unmarshal([]byte(payload), &old); err != nil {
		t.Fatal(err)
	}
	riot.persistRiotMatch("riot-match-v1|KR_190", &old)
	raw, err := riot.matchByID(context.Background(), "KR_190")
	if err != nil {
		t.Fatal(err)
	}
	converted := riotConvertMatch(raw, "", nil)
	if !converted.PerkStatsStale || len(converted.Participants[0].PerkStats) != 0 || calls.Load() != 0 {
		t.Fatalf("old cache=%#v calls=%d", converted, calls.Load())
	}
	a := &app{riot: riot}
	recorder := httptest.NewRecorder()
	a.handleGameplayMatch(recorder, httptest.NewRequest("POST", "/api/gameplay/match", strings.NewReader(`{"gameId":190,"participantId":4,"region":"kr","refresh":"perk-stats"}`)))
	if recorder.Code != 200 {
		t.Fatalf("refresh code=%d body=%s", recorder.Code, recorder.Body)
	}
	var fresh gameplayMatch
	if err := json.Unmarshal(recorder.Body.Bytes(), &fresh); err != nil {
		t.Fatal(err)
	}
	if fresh.PerkStatsStale || fresh.SubjectParticipantID != 4 || fresh.Participants[0].PerkStats[0].Vars != [3]int64{804, 300, 0} || calls.Load() != 1 {
		t.Fatalf("fresh=%#v calls=%d", fresh, calls.Load())
	}
	restart := newRiotProvider(p)
	disk, err := restart.matchByID(context.Background(), "KR_190")
	if err != nil || disk.Info.PerkStatsStale || calls.Load() != 1 {
		t.Fatalf("v2 disk=%#v err=%v calls=%d", disk, err, calls.Load())
	}
	if _, err := restart.matchDisk.readDisk("riot-match-v2|KR_190"); err != nil {
		t.Fatal(err)
	}
}

func TestR190PerkCatalogTemplates(t *testing.T) {
	var perks []gameplayPerk
	if err := json.Unmarshal([]byte(`[{"id":9111,"name":"凯旋","iconPath":"/original.png","endOfGameStatDescs":["回复生命值总和：@eogvar1@"]}]`), &perks); err != nil {
		t.Fatal(err)
	}
	if len(perks[0].EndOfGameStatDescs) != 1 {
		t.Fatal("LCU EOG template lost")
	}
	merged := mergePerkEffectTemplates([]gameplayPerk{{ID: 9111, Name: "客户端名称", IconPath: "/keep.png"}}, perks)
	if merged[0].Name != "客户端名称" || merged[0].IconPath != "/keep.png" || len(merged[0].EndOfGameStatDescs) != 1 {
		t.Fatalf("merge=%#v", merged)
	}
	styles, _, _, _ := normalizeGameplayPerkCatalog([]gameplayPerkStyle{{ID: 8000, Slots: []gameplayPerkSlot{{Perks: []gameplayPerk{{ID: 9111}}}}}}, merged)
	if len(styles[0].Slots[0].Perks[0].EndOfGameStatDescs) != 1 {
		t.Fatal("slot template not enriched")
	}
	data, _ := json.Marshal(merged)
	var again []gameplayPerk
	_ = json.Unmarshal(data, &again)
	if !reflect.DeepEqual(merged, again) {
		t.Fatalf("normalized eogDescs roundtrip lost: %s", data)
	}
	if !championCacheDiskAllowed("normalized-perks-v2|ddragon") || !championCacheDiskAllowed("riot-match-v2|KR_190") {
		t.Fatal("v2 cache denied")
	}
}

func r190DetailPage(description string) []byte {
	var body strings.Builder
	body.WriteString(`<meta name="description" content="WRONG SEO 胜率 52.7%"><table><tbody>`)
	for id := 1; id <= 12; id++ {
		fmt.Fprintf(&body, `<tr><td><a href="/hero/%d-hero%d">英雄%d</a></td><td>90</td><td>60%%</td><td>1000</td></tr>`, id, id, id)
	}
	body.WriteString(`</tbody></table><section data-seo-guide><p>双刀流更适合先在英雄这类高分高样本英雄上考虑。` + description + `如果你的英雄机制能触发。</p></section>`)
	return hexdataTestPage(body.String())
}

func TestR190AugmentDescriptionSources(t *testing.T) {
	for _, description := range []string{"攻击时施加攻击特效。", "", augmentOfflineDescription} {
		t.Run(description, func(t *testing.T) {
			var arenaCalls, detailCalls atomic.Int32
			catalog := strings.Replace(string(hexdataAugmentsTestPage()), "/augment/1000-augment1000", "/augment/1225-dual-wield", 1)
			provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				switch r.URL.Path {
				case "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
					return hexdataResponse(r, []byte(`[{"id":1225,"name":"元数据","description":"禁止补入的目录效果。"}]`)), nil
				case "/latest/cdragon/arena/zh_cn.json":
					arenaCalls.Add(1)
					return hexdataResponse(r, []byte(`{"augments":[{"id":120,"name":"竞技场","desc":"获得护盾。","rarity":0}]}`)), nil
				case "/augments":
					return hexdataResponse(r, []byte(catalog)), nil
				case "/augment/1225-dual-wield":
					detailCalls.Add(1)
					return hexdataResponse(r, r190DetailPage(description)), nil
				default:
					return hexdataResponse(r, hexdataTestPage("<p>fixture</p>")), nil
				}
			}))
			provider.hexdata.adoptCitation(championSourceCitation{Source: "Hexdata", Patch: "16.16", ReportDate: "2026-08-19", BuildID: hexdataTestBuild, CanonicalURL: "https://hexdata.com.cn/augments"})
			items := loadGameplayAugmentDescriptions(context.Background(), []int64{1225, 120}, provider.gameplayAugmentDescription)
			if items[1].Status != "ok" || items[1].Description != "获得护盾。" || arenaCalls.Load() == 0 || detailCalls.Load() != 1 {
				t.Fatalf("source items=%#v arena=%d detail=%d", items, arenaCalls.Load(), detailCalls.Load())
			}
			if description != "" && description != augmentOfflineDescription {
				if items[0].Status != "ok" || items[0].Description != description {
					t.Fatalf("guide=%#v", items)
				}
			} else if items[0].Status != "unavailable" || items[0].Description != "" {
				t.Fatalf("empty/placeholder=%#v", items)
			}
		})
	}
}

func TestR190AugmentDescriptionValidationAndConcurrency(t *testing.T) {
	for _, ids := range []string{"1,2,3,4,5,6,7", "", "0", "-1", "abc", "10001", "1,,2"} {
		r := httptest.NewRecorder()
		(&app{}).handleGameplayAugmentDescriptions(r, httptest.NewRequest("GET", "/api/gameplay/augment-descriptions?ids="+ids, nil))
		if r.Code != 400 {
			t.Fatalf("ids=%q code=%d", ids, r.Code)
		}
	}
	var active, peak atomic.Int32
	items := loadGameplayAugmentDescriptions(context.Background(), []int64{1, 2, 3, 4, 5, 6}, func(ctx context.Context, id int64) (string, error) {
		n := active.Add(1)
		for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
		}
		defer active.Add(-1)
		if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 8*time.Second {
			t.Error("missing item deadline")
		}
		time.Sleep(10 * time.Millisecond)
		return "效果", nil
	})
	if peak.Load() != 2 || len(items) != 6 {
		t.Fatalf("peak=%d items=%d", peak.Load(), len(items))
	}
	for i, item := range items {
		if item.ID != int64(i+1) || item.Status != "ok" {
			t.Fatalf("ordered=%#v", items)
		}
	}
}

func TestR190CommunityDragonTemplatesMergeAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			var calls atomic.Int32
			provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				if r.URL.Path != "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/perks.json" {
					t.Errorf("unexpected path=%s", r.URL.Path)
				}
				if fail {
					return &http.Response{StatusCode: 503, Header: make(http.Header), Body: http.NoBody, Request: r}, nil
				}
				return hexdataResponse(r, []byte(`[{"id":9111,"name":"不覆盖","iconPath":"/do-not-use.png","endOfGameStatDescs":["回复生命值总和：@eogvar1@"]}]`)), nil
			}))
			a := &app{champions: provider}
			perks := a.enrichPerkEffectTemplates(context.Background(), []gameplayPerk{{ID: 9111, Name: "凯旋", IconPath: "/keep.png"}})
			if calls.Load() != 1 || perks[0].Name != "凯旋" || perks[0].IconPath != "/keep.png" {
				t.Fatalf("supplement=%#v calls=%d", perks, calls.Load())
			}
			if fail && len(perks[0].EndOfGameStatDescs) != 0 || !fail && len(perks[0].EndOfGameStatDescs) != 1 {
				t.Fatalf("templates=%#v fail=%v", perks, fail)
			}
		})
	}
}
