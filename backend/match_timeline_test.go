package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseTimelineFramesAcceptsKnownWrappers(t *testing.T) {
	for name, body := range map[string]string{
		"top-level": `{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055}]}]}`,
		"info":      `{"info":{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055}]}]}}`,
		"json":      `{"json":{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055}]}]}}`,
		"bare":      `[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			frames, err := parseTimelineFrames([]byte(body))
			if err != nil || len(frames) != 1 || len(frames[0].Events) != 1 || frames[0].Events[0].ItemID != 1055 {
				t.Fatalf("frames=%#v err=%v", frames, err)
			}
		})
	}
}

func TestTimelineWrapperSourceKeepsExplicitEmptyLayer(t *testing.T) {
	var timeline lcuGameTimeline
	if err := json.Unmarshal([]byte(`{"json":{"frames":[]}}`), &timeline); err != nil {
		t.Fatal(err)
	}
	frames, source := timeline.framesWithSource()
	if source != "JSON.Frames" || frames == nil || len(frames) != 0 {
		t.Fatalf("frames=%#v source=%q", frames, source)
	}
}

func TestTimelineEventDiagnosticsCaptureKeysWithoutValues(t *testing.T) {
	frames, err := parseTimelineFrames([]byte(`{"frames":[{"events":[{"eventType":"ITEM_PURCHASED","actorId":3,"itemId":1055},{"type":"SKILL_LEVEL_UP","subject":"hidden"}]}]}`))
	if err != nil || len(frames) != 1 {
		t.Fatalf("frames=%#v err=%v", frames, err)
	}
	if timelineHasParticipantEvents(frames) {
		t.Fatal("unknown participant aliases must not count as participantId")
	}
	samples := sampleTimelineEventKeys(frames, 3)
	if len(samples) != 2 || strings.Join(samples[0], ",") != "actorId,eventType,itemId" || strings.Join(samples[1], ",") != "subject,type" {
		t.Fatalf("event key samples = %#v", samples)
	}
	if strings.Contains(strings.Join(samples[1], ","), "hidden") {
		t.Fatalf("event values leaked into diagnostics: %#v", samples)
	}
}

func TestTimelineEventDiagnosticsScanAcrossFramesAndPreferUsefulEvents(t *testing.T) {
	frames, err := parseTimelineFrames([]byte(`{"frames":[{"events":[{"type":"PAUSE_END","realTimestamp":1,"timestamp":2}]},{"events":[{"type":"WARD_PLACED","actorId":3,"timestamp":4,"extra":"ignored"},{"eventType":"ITEM_PURCHASED","actorId":3,"itemId":1055,"timestamp":5},{"type":"SKILL_LEVEL_UP","subject":3,"skillSlot":1}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	samples := sampleTimelineEventKeys(frames, 2)
	if len(samples) != 2 || strings.Join(samples[0], ",") != "actorId,eventType,itemId,timestamp" || strings.Join(samples[1], ",") != "skillSlot,subject,type" {
		t.Fatalf("useful event samples = %#v", samples)
	}
}

func TestTimelineEventDiagnosticsFallbackUsesMostInformativeEvent(t *testing.T) {
	frames, err := parseTimelineFrames([]byte(`{"frames":[{"events":[{"type":"PAUSE_END","timestamp":1}]},{"events":[{"type":"WARD_PLACED","actorId":3,"timestamp":4,"extra":"kept"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	samples := sampleTimelineEventKeys(frames, 1)
	if len(samples) != 1 || strings.Join(samples[0], ",") != "actorId,extra,timestamp,type" {
		t.Fatalf("fallback event sample = %#v", samples)
	}
}

func TestTimelineParticipantEventValidity(t *testing.T) {
	if timelineHasParticipantEvents([]timelineFrame{{Events: []timelineEvent{{ParticipantID: 0}, {ParticipantID: -1}}}}) {
		t.Fatal("non-positive participant ids must not pass LCU validity check")
	}
	if !timelineHasParticipantEvents([]timelineFrame{{Events: []timelineEvent{{ParticipantID: 7}}}}) {
		t.Fatal("positive participant id should pass LCU validity check")
	}
}

func TestTimelineDecisionReturnsStructuredAttempts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"frames":[{"events":[{"type":"ITEM_PURCHASED","participantId":1,"itemId":1055}]}]}`))
	}))
	defer server.Close()
	client := &LCUClient{baseURL: server.URL, token: "test", http: server.Client(), region: "TENCENT", rsoPlatform: "HN1", platformProbe: true}
	a := &app{}
	frames, source, attempts, fallbackReason, err := a.loadMatchTimelineCNDecision(context.Background(), client, "HN1", 42)
	if err != nil || len(frames) != 1 || source != dataSourceLCU || fallbackReason != "" {
		t.Fatalf("timeline decision: source=%q fallback=%q frames=%#v err=%v", source, fallbackReason, frames, err)
	}
	if len(attempts) != 1 || attempts[0].Source != dataSourceLCU || attempts[0].Outcome != dataSourceSuccess {
		t.Fatalf("timeline attempts = %#v", attempts)
	}
}

func TestExtractParticipantTimelineGroupsPurchasesByMinute(t *testing.T) {
	frames := []timelineFrame{
		{Timestamp: 0, Events: []timelineEvent{
			{Type: "ITEM_PURCHASED", Timestamp: 10_000, ParticipantID: 3, ItemID: 1055},
			{Type: "ITEM_PURCHASED", Timestamp: 12_000, ParticipantID: 3, ItemID: 2003},
			{Type: "ITEM_PURCHASED", Timestamp: 15_000, ParticipantID: 4, ItemID: 1054},
			{Type: "SKILL_LEVEL_UP", Timestamp: 20_000, ParticipantID: 3, SkillSlot: 1, LevelUpType: "NORMAL"},
		}},
		{Timestamp: 60_000, Events: []timelineEvent{
			{Type: "SKILL_LEVEL_UP", Timestamp: 95_000, ParticipantID: 3, SkillSlot: 3, LevelUpType: "NORMAL"},
			{Type: "ITEM_PURCHASED", Timestamp: 200_000, ParticipantID: 3, ItemID: 3134},
			{Type: "ITEM_SOLD", Timestamp: 205_000, ParticipantID: 3, ItemID: 1055},
		}},
	}
	groups, skills := extractParticipantTimeline(frames, 3)
	if len(groups) != 2 {
		t.Fatalf("expected 2 minute groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Minute != 0 || len(groups[0].Events) != 2 {
		t.Fatalf("unexpected first group: %+v", groups[0])
	}
	if groups[1].Minute != 3 || len(groups[1].Events) != 2 {
		t.Fatalf("unexpected second group: %+v", groups[1])
	}
	if !groups[1].Events[1].Sold || groups[1].Events[1].ItemID != 1055 {
		t.Fatalf("expected sold 1055 in second group, got %+v", groups[1].Events)
	}
	if len(skills) != 2 || skills[0] != (timelineSkillUp{Level: 1, Slot: 1}) || skills[1] != (timelineSkillUp{Level: 2, Slot: 3}) {
		t.Fatalf("unexpected skill order: %+v", skills)
	}
	// 其他玩家的事件不得混入。
	for _, group := range groups {
		for _, event := range group.Events {
			if event.ItemID == 1054 {
				t.Fatal("participant 4 purchase leaked into participant 3 route")
			}
		}
	}
}

func TestExtractParticipantTimelineAcceptsEventTypeAlias(t *testing.T) {
	frames := []timelineFrame{{Events: []timelineEvent{{EventType: "item_purchased", ParticipantID: 1, ItemID: 1055}}}}
	groups, _ := extractParticipantTimeline(frames, 1)
	if len(groups) != 1 || len(groups[0].Events) != 1 || groups[0].Events[0].ItemID != 1055 {
		t.Fatalf("eventType alias was not parsed: %+v", groups)
	}
}

func TestExtractParticipantTimelineUndoCancelsPurchase(t *testing.T) {
	frames := []timelineFrame{
		{Events: []timelineEvent{
			{Type: "ITEM_PURCHASED", Timestamp: 30_000, ParticipantID: 1, ItemID: 1001},
			{Type: "ITEM_PURCHASED", Timestamp: 31_000, ParticipantID: 1, ItemID: 1055},
			{Type: "ITEM_UNDO", Timestamp: 32_000, ParticipantID: 1, BeforeID: 1055},
		}},
	}
	groups, _ := extractParticipantTimeline(frames, 1)
	if len(groups) != 1 || len(groups[0].Events) != 1 || groups[0].Events[0].ItemID != 1001 {
		t.Fatalf("undo should cancel the 1055 purchase, got %+v", groups)
	}
}

func TestExtractParticipantTimelineSkipsEvolveSkillUps(t *testing.T) {
	frames := []timelineFrame{
		{Events: []timelineEvent{
			{Type: "SKILL_LEVEL_UP", Timestamp: 10_000, ParticipantID: 2, SkillSlot: 2, LevelUpType: "NORMAL"},
			{Type: "SKILL_LEVEL_UP", Timestamp: 11_000, ParticipantID: 2, SkillSlot: 1, LevelUpType: "EVOLVE"},
			{Type: "SKILL_LEVEL_UP", Timestamp: 12_000, ParticipantID: 2, SkillSlot: 4, LevelUpType: "NORMAL"},
		}},
	}
	_, skills := extractParticipantTimeline(frames, 2)
	if len(skills) != 2 || skills[0].Slot != 2 || skills[1].Slot != 4 {
		t.Fatalf("evolve should not consume a skill point, got %+v", skills)
	}
}

func TestMatchTimelineCacheEvictsOldestEntries(t *testing.T) {
	cache := newMatchTimelineCache()
	for index := 0; index < matchTimelineCacheMax+5; index++ {
		cache.put(string(rune('a'+index%26))+string(rune('0'+index/26)), matchTimelineResponse{Available: true})
	}
	if len(cache.entries) > matchTimelineCacheMax {
		t.Fatalf("cache should stay bounded, got %d entries", len(cache.entries))
	}
	// nil 缓存不得崩溃（测试用的最小 app 可能未初始化该字段）。
	var missing *matchTimelineCache
	if _, ok := missing.get("x"); ok {
		t.Fatal("nil cache should miss")
	}
	missing.put("x", matchTimelineResponse{})
}
