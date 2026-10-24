// R116-D：推荐页克制/协同提示（P1-2/P1-3）+ 局内出装解析（P1-4 阶段一，
// 阶段二只测纯函数）+ 队伍画像缺口提示（P1-5）的验证。
//
// 数据一律用仓库里的真实上游夹具（backend/testdata/r116/），不手编期望值：
//
//	（全部 evidence="supported"、qValue=0）、terminalItemTrios 前 10 条；
//
// 三类断言：
//  1. 交集与阈值本身（纯函数，含工单要求的两处对抗变异）；
//     （Anti-scope 第 1 条，评审 4.2/4.4）；
//  3. 降级红线——取不到就不生成，绝不用 0 或猜测值顶替。
//
// P1-4 阶段二（下一步出装）已实现纯计算但**未接线**，本文件既测它的行为，
// 也钉住「它确实没有生产调用方」这条事实（TestR116DStageTwoStaysUnwired）。
package main

import (
	"encoding/json"

	"os"
	"path/filepath"
	"reflect"

	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// 夹具装载
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// P1-2 / P1-3：交集、排序、上限、字段透传
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// P1-2 / P1-3 / P1-5：请求预算（Anti-scope 第 1 条）
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// P1-4 阶段一：playerlist 的 items 解析
// ---------------------------------------------------------------------------

// r116dPlayerList 拼一份海斗形状的 playerlist：10 人、position 恒为 "OTHER"
// （斗魂实测基线是 position_values:{"OTHER":18}）、
// 无 subteam 字段，items 用斗魂实测的 itemID/slot/count/canUse/consumable 五键。
func r116dPlayerList(t *testing.T, entries ...map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func r116dPlayerEntry(name string, items any) map[string]any {
	entry := map[string]any{
		"championName": "Yasuo", "rawChampionName": "Yasuo", "skinName": "default", "rawSkinName": "",
		"riotId": name, "riotIdGameName": name, "riotIdTagLine": "CN1", "summonerName": name,
		"isBot": false, "isDead": false, "level": 11, "position": "OTHER", "respawnTimer": 0,
		"skinID": 0, "team": "ORDER", "runes": map[string]any{}, "scores": map[string]any{},
		"summonerSpells": map[string]any{},
	}
	if items != nil {
		entry["items"] = items
	}
	return entry
}

func r116dItemElement(itemID, slot, count int, canUse, consumable bool) map[string]any {
	return map[string]any{"itemID": itemID, "slot": slot, "count": count, "canUse": canUse, "consumable": consumable}
}

// 工单 P1-4 阶段一第 1 条：读 items、存进 ItemsByIdentity，身份匹配复用
// liveClientEntryIdentityKeys（与 PositionByIdentity 同一套）。
func TestParseLiveClientPlayerListReadsItems(t *testing.T) {
	raw := r116dPlayerList(t,
		r116dPlayerEntry("alpha", []any{
			r116dItemElement(3153, 0, 1, false, false),
			r116dItemElement(6333, 1, 1, true, false),
			r116dItemElement(2003, 6, 2, false, true),
		}),
		r116dPlayerEntry("beta", []any{}),
		r116dPlayerEntry("gamma", nil),
	)
	snapshot, shape, err := parseLiveClientPlayerList(raw, 3)
	if err != nil {
		t.Fatal(err)
	}
	if shape.PlayerCount != 3 {
		t.Fatalf("player_count = %d, want 3", shape.PlayerCount)
	}
	// 身份键是 normalizeLiveClientPlayerName 的小写形式，"alpha#cn1" 与 "alpha" 都在。
	items := snapshot.ItemsByIdentity["alpha"]
	if len(items) != 3 {
		t.Fatalf("alpha items = %#v, want 3", items)
	}
	if items[0].ItemID != 3153 || items[0].Slot != 0 || items[0].Count != 1 || items[0].CanUse || items[0].Consumable {
		t.Fatalf("first item parsed wrong: %#v", items[0])
	}
	if items[1].ItemID != 6333 || !items[1].CanUse {
		t.Fatalf("second item parsed wrong: %#v", items[1])
	}
	if items[2].ItemID != 2003 || !items[2].Consumable || items[2].Count != 2 {
		t.Fatalf("consumable item parsed wrong: %#v", items[2])
	}
	// 顺序必须原样保留：重排就是编造上游没给的顺序。
	if items[0].ItemID != 3153 || items[1].ItemID != 6333 || items[2].ItemID != 2003 {
		t.Fatalf("item order was not preserved: %#v", items)
	}
	// "items": [] 与「没有 items 键」必须能区分：前者写空切片，后者不写键。
	betaItems, betaPresent := snapshot.ItemsByIdentity["beta"]
	if !betaPresent || len(betaItems) != 0 {
		t.Fatalf("empty items array was not distinguished from a missing key: present=%v %#v", betaPresent, betaItems)
	}
	if _, gammaPresent := snapshot.ItemsByIdentity["gamma"]; gammaPresent {
		t.Fatal("a player without the items key got an entry")
	}
	// 元素键名并集是判据二的证据之一，必须原样落进快照。
	if got := strings.Join(snapshot.ItemElementKeys, ","); got != "canUse,consumable,count,itemID,slot" {
		t.Fatalf("item element keys = %q, want the five Arena-baseline keys", got)
	}
	if snapshot.ItemElementsSeen != 3 || snapshot.ItemElementsSkipped != 0 {
		t.Fatalf("item element counters = seen:%d skipped:%d, want 3/0", snapshot.ItemElementsSeen, snapshot.ItemElementsSkipped)
	}
}

// 工单 P1-4 阶段一：解析必须容错。items 缺失 / 不是数组 / 元素不是对象 /
// itemID 不是数字，全部安全跳过并计入诊断，绝不 panic。
// 海斗的真实形状未经实测，所以键名大小写要宽松匹配。
func TestParseLiveClientItemsToleratesMalformedShapes(t *testing.T) {
	tests := []struct {
		name        string
		entry       map[string]any
		wantPresent bool
		wantItems   int
		wantSeen    int
		wantSkipped int
	}{
		{name: "no items key", entry: map[string]any{"riotId": "a"}, wantPresent: false},
		{name: "items is null", entry: map[string]any{"riotId": "a", "items": nil}, wantPresent: true},
		{name: "items is an object", entry: map[string]any{"riotId": "a", "items": map[string]any{"0": 3153}}, wantPresent: true},
		{name: "items is a string", entry: map[string]any{"riotId": "a", "items": "3153"}, wantPresent: true},
		{
			name:        "element is not an object",
			entry:       map[string]any{"riotId": "a", "items": []any{3153, "6333", []any{1}}},
			wantPresent: true, wantSeen: 3, wantSkipped: 3,
		},
		{
			name:        "itemID is not numeric",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": "not-a-number", "slot": 0}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID missing",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"slot": 1, "count": 1}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID is zero",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": 0, "slot": 0}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			name:        "itemID is fractional",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": 3153.5}}},
			wantPresent: true, wantSeen: 1, wantSkipped: 1,
		},
		{
			// 数字字符串要认。
			name:        "itemID is a numeric string",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemID": "3153", "slot": "2", "count": "1", "canUse": "true", "consumable": "false"}}},
			wantPresent: true, wantItems: 1, wantSeen: 1,
		},
		{
			// 键名大小写宽松匹配：itemId / itemid 都要认。
			name:        "lowercase key spelling",
			entry:       map[string]any{"riotId": "a", "items": []any{map[string]any{"itemid": 6333, "SLOT": 1}}},
			wantPresent: true, wantItems: 1, wantSeen: 1,
		},
		{
			name:        "mixed valid and invalid elements",
			entry:       map[string]any{"riotId": "a", "items": []any{r116dItemElement(3153, 0, 1, false, false), "junk", map[string]any{"itemID": -1}}},
			wantPresent: true, wantItems: 1, wantSeen: 3, wantSkipped: 2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			items, _, seen, skipped, present := parseLiveClientItems(test.entry)
			if present != test.wantPresent {
				t.Fatalf("present = %v, want %v", present, test.wantPresent)
			}
			if len(items) != test.wantItems {
				t.Fatalf("items = %#v, want %d", items, test.wantItems)
			}
			if seen != test.wantSeen || skipped != test.wantSkipped {
				t.Fatalf("counters = seen:%d skipped:%d, want %d/%d", seen, skipped, test.wantSeen, test.wantSkipped)
			}
		})
	}
	// 整份 playerlist 都是垃圾也不能 panic，且既有的 position/分组行为不变。
	snapshot, shape, err := parseLiveClientPlayerList(r116dPlayerList(t,
		map[string]any{"riotId": "a", "items": "junk", "position": "OTHER"},
		map[string]any{"riotId": "b", "items": []any{nil, 1, "x"}, "position": "OTHER"},
	), 3)
	if err != nil {
		t.Fatalf("malformed playerlist returned an error: %v", err)
	}
	if shape.PlayerCount != 2 || snapshot.ItemElementsSkipped != 3 {
		t.Fatalf("shape/counters = %d/%d, want 2/3", shape.PlayerCount, snapshot.ItemElementsSkipped)
	}
}

// 易漏点（执行契约第 11 条）：新增 ItemsByIdentity 之后 cloneLiveClientSnapshot
// 必须深拷贝每个 slice，否则快照跨 goroutine 传递时会共享底层数组 —— 既丢数据
// 也触发 race。
func TestCloneLiveClientSnapshotDeepCopiesItems(t *testing.T) {
	original := liveClientSnapshot{
		OrderedIdentities:  [][]string{{"alpha"}},
		PositionByIdentity: map[string]string{"alpha": "MIDDLE"},
		ItemsByIdentity: map[string][]liveClientItem{
			"alpha": {{ItemID: 3153, Slot: 0, Count: 1}, {ItemID: 6333, Slot: 1, Count: 1}},
			"beta":  {{ItemID: 3031, Slot: 2, Count: 1}},
		},
		ItemElementKeys:     []string{"count", "itemID", "slot"},
		ItemElementsSeen:    3,
		ItemElementsSkipped: 1,
	}
	cloned := cloneLiveClientSnapshot(original)
	if !reflect.DeepEqual(cloned.ItemsByIdentity, original.ItemsByIdentity) {
		t.Fatalf("clone lost items: %#v", cloned.ItemsByIdentity)
	}
	if !reflect.DeepEqual(cloned.ItemElementKeys, original.ItemElementKeys) {
		t.Fatalf("clone lost element keys: %#v", cloned.ItemElementKeys)
	}
	if cloned.ItemElementsSeen != 3 || cloned.ItemElementsSkipped != 1 {
		t.Fatalf("clone lost counters: %d/%d", cloned.ItemElementsSeen, cloned.ItemElementsSkipped)
	}
	// 改克隆不能影响原件（浅拷贝会在这里露馅）。
	cloned.ItemsByIdentity["alpha"][0].ItemID = 9999
	cloned.ItemsByIdentity["beta"] = append(cloned.ItemsByIdentity["beta"], liveClientItem{ItemID: 123430})
	cloned.ItemElementKeys[0] = "mutated"
	if original.ItemsByIdentity["alpha"][0].ItemID != 3153 {
		t.Fatal("clone shares the item backing array with the original")
	}
	if len(original.ItemsByIdentity["beta"]) != 1 {
		t.Fatal("clone shares the beta slice with the original")
	}
	if original.ItemElementKeys[0] != "count" {
		t.Fatal("clone shares the element-key slice with the original")
	}

	// 并发读同一份快照的多个克隆（-race 下这条会真的跑并发）。
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			local := cloneLiveClientSnapshot(original)
			local.ItemsByIdentity["alpha"][0].Slot = index
			if len(local.ItemsByIdentity["alpha"]) != 2 {
				t.Errorf("concurrent clone lost items")
			}
		}(index)
	}
	wait.Wait()
	if original.ItemsByIdentity["alpha"][0].ItemID != 3153 {
		t.Fatal("concurrent cloning corrupted the shared snapshot")
	}
}

// items 查询与 position 查询同一套身份优先级（riotId 先于 summonerName），
// 且必须能区分「键存在但为空」与「键不存在」。
func TestLiveClientItemsForIdentitiesDistinguishesEmptyFromMissing(t *testing.T) {
	snapshot := liveClientSnapshot{ItemsByIdentity: map[string][]liveClientItem{
		"alpha":     {{ItemID: 3153}},
		"beta":      {},
		"alpha#cn1": {{ItemID: 3153}},
	}}
	items, source := liveClientItemsForIdentities(snapshot, []string{"alpha"}, []string{"alpha#CN1"})
	if len(items) != 1 || items[0].ItemID != 3153 || source != "riotId" {
		t.Fatalf("riotId lookup = %#v/%q, want one item via riotId", items, source)
	}
	empty, emptySource := liveClientItemsForIdentities(snapshot, []string{"beta"}, nil)
	if len(empty) != 0 || emptySource != "summonerName" {
		t.Fatalf("empty items = %#v/%q, want matched-but-empty via summonerName", empty, emptySource)
	}
	if missing, missingSource := liveClientItemsForIdentities(snapshot, []string{"gamma"}, nil); missing != nil || missingSource != "" {
		t.Fatalf("unknown identity = %#v/%q, want no match", missing, missingSource)
	}
	// 快照里一个 items 都没有（海斗没给这个字段）→ 直接不匹配，不做无谓遍历。
	if items, source := liveClientItemsForIdentities(liveClientSnapshot{}, []string{"alpha"}, nil); items != nil || source != "" {
		t.Fatalf("empty snapshot matched: %#v/%q", items, source)
	}
}

// 工单 P1-4 阶段一第 2 条：诊断事件 live_client_items_parsed 记录本人识别到的
// items 长度与 itemID 集合，且明确标注 ui_visible=false（生产 UI 不展示任何内容）。
func TestRecordLiveClientItemsParsedEmitsBoundedDiagnostic(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "logs"), 0o700); err != nil {
		t.Fatal(err)
	}
	storage := trackTestStore(t, &localStore{root: root})
	a := &app{storage: storage}
	snapshot := liveClientSnapshot{
		OrderedIdentities:   [][]string{{"alpha"}, {"beta"}},
		ItemElementKeys:     []string{"canUse", "consumable", "count", "itemID", "slot"},
		ItemElementsSeen:    5,
		ItemElementsSkipped: 1,
		ItemsByIdentity:     map[string][]liveClientItem{"alpha": {{ItemID: 6333}, {ItemID: 3153}}},
	}
	items := []liveClientItem{{ItemID: 6333, Slot: 1, Count: 1}, {ItemID: 3153, Slot: 0, Count: 1}}
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", items)
	// 同一套装备再记一次：去重键相同，不应产生第二条。
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", items)
	// 出装变了：必须产生新的一条（这正是真机要看的「解析是否随出装推进」）。
	a.recordLiveClientItemsParsed(9001, "InProgress", snapshot, true, "riotId", append(append([]liveClientItem(nil), items...), liveClientItem{ItemID: 123430, Slot: 2, Count: 1}))

	events := r116dDiagnosticEvents(t, storage, "live_client_items_parsed")
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (deduplicated per item set)", len(events))
	}
	first := events[0]
	if first["game_id"] != float64(9001) || first["phase"] != "InProgress" {
		t.Fatalf("event identity = %#v", first)
	}
	if first["ui_visible"] != false {
		t.Fatalf("ui_visible = %#v, want false (stage one is diagnostics only)", first["ui_visible"])
	}
	if first["self_matched"] != true || first["identity_source"] != "riotId" {
		t.Fatalf("self match = %#v/%#v", first["self_matched"], first["identity_source"])
	}
	if first["self_item_count"] != float64(2) {
		t.Fatalf("self_item_count = %#v, want 2", first["self_item_count"])
	}
	ids, ok := first["self_item_ids"].([]any)
	if !ok || len(ids) != 2 || ids[0] != float64(3153) || ids[1] != float64(6333) {
		t.Fatalf("self_item_ids = %#v, want sorted [3153 6333]", first["self_item_ids"])
	}
	keys, ok := first["item_element_keys"].([]any)
	if !ok || len(keys) != 5 || keys[3] != "itemID" {
		t.Fatalf("item_element_keys = %#v, want the five observed keys", first["item_element_keys"])
	}
	if first["item_elements_seen"] != float64(5) || first["item_elements_skipped"] != float64(1) {
		t.Fatalf("element counters = %#v/%#v", first["item_elements_seen"], first["item_elements_skipped"])
	}
	if first["identities_with_items"] != float64(1) || first["player_count"] != float64(2) {
		t.Fatalf("coverage counters = %#v/%#v", first["identities_with_items"], first["player_count"])
	}
	second := events[1]
	if second["self_item_count"] != float64(3) {
		t.Fatalf("second event item count = %#v, want 3", second["self_item_count"])
	}

	// 本人没匹配上也要记：这正是「items 键在、身份匹配不上」与「items 键根本没有」
	// 的分界，少了它真机日志无法判读。
	a.recordLiveClientItemsParsed(9002, "InProgress", snapshot, false, "", nil)
	unmatched := r116dDiagnosticEvents(t, storage, "live_client_items_parsed")
	last := unmatched[len(unmatched)-1]
	if last["self_matched"] != false || last["self_item_count"] != float64(0) {
		t.Fatalf("unmatched event = %#v", last)
	}
}

// ---------------------------------------------------------------------------
// P1-4 阶段二：纯函数（已实现、未接线）
// ---------------------------------------------------------------------------

// 阶段二尚未具备真机字段证据，删除未接线计算后仍要有测试钉住：一旦有人把它接到生产路径上
// 而没有先确认真实海斗 items 的字段形状，这条就会红。
func TestR116DStageTwoStaysUnwired(t *testing.T) {
	source, err := os.ReadFile("gameplay.go")
	if err != nil {
		t.Fatal(err)
	}
	// 注释里刻意点了这两个名字（说明接线条件与接线点），所以先把 // 注释行剔掉，
	// 只数真实代码里的出现次数：定义和调用都应为 0 次。
	code := r116dStripLineComments(string(source))
	for _, name := range []string{"gameplayNextItemSuggestionFromTrios", "gameplayOwnedTerminalItemIDs"} {
		if got := strings.Count(code, name); got != 0 {
			t.Fatalf("%s appears %d times in gameplay.go code, want exactly 0 (no definition or caller). "+
				"Wiring stage two requires observed live-client items and element keys from a real mayhem game.", name, got)
		}
	}
	// bundle 上不许出现「下一件」字段：那是接线时才加的东西。
	bundleType := reflect.TypeOf(gameplayRecommendationBundle{})
	for index := 0; index < bundleType.NumField(); index++ {
		field := bundleType.Field(index)
		if strings.Contains(strings.ToLower(field.Name), "nextitem") {
			t.Fatalf("gameplayRecommendationBundle gained %s before the probe verdict was recorded", field.Name)
		}
	}
	webSource, err := os.ReadFile(filepath.Join("web", "gameplay.js"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(webSource), "nextItem") {
		t.Fatal("gameplay.js renders a next-item row before the probe verdict was recorded")
	}
}

// ---------------------------------------------------------------------------
// P1-5：队伍画像缺口标签
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}

func countStrings(values []string, want string) int {
	total := 0
	for _, value := range values {
		if value == want {
			total++
		}
	}
	return total
}

// r116dStripLineComments 去掉整行的 // 注释与行尾 // 注释，只留下真实代码。
// 用于「阶段二未接线」这类按出现次数判定的断言——文档注释里刻意点了函数名，
// 不剔掉就会把注释当成调用。
func r116dStripLineComments(source string) string {
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		if position := strings.Index(line, "//"); position >= 0 {
			lines[index] = line[:position]
		}
	}
	return strings.Join(lines, "\n")
}

func r116dDiagnosticText(t *testing.T, storage *localStore) string {
	t.Helper()
	data, err := storage.readDiagnosticLog()
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// r116dDiagnosticEvents 按出现顺序取出指定事件名的全部诊断记录。
func r116dDiagnosticEvents(t *testing.T, storage *localStore, event string) []map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(r116dDiagnosticText(t, storage)), "\n")
	events := make([]map[string]any, 0, 4)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, `"event":"`+event+`"`) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("diagnostic line is not JSON: %v (%s)", err, line)
		}
		events = append(events, payload)
	}
	if len(events) == 0 {
		t.Fatalf("no %q event in the diagnostic log:\n%s", event, strings.Join(lines, "\n"))
	}
	return events
}

func r116dLastDiagnosticEvent(t *testing.T, text, event string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := strings.TrimSpace(lines[index])
		if line == "" || !strings.Contains(line, `"event":"`+event+`"`) {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("diagnostic line is not JSON: %v (%s)", err, line)
		}
		return payload
	}
	t.Fatalf("no %q event in the diagnostic log:\n%s", event, text)
	return nil
}
