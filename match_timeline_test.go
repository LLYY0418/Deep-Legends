package main

import "testing"

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
