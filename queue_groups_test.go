package main

import (
	"strings"
	"testing"
)

func TestSupportedQueueDefinitionsMatchCanonicalGroups(t *testing.T) {
	tests := []queueDefinition{
		{400, "匹配模式（征召）", "match", "more:match", "ranked"},
		{420, "单排/双排", "solo", "solo", "ranked"},
		{430, "匹配模式（盲选）", "match", "more:match", "ranked"},
		{440, "灵活组排", "flex", "flex", "ranked"},
		{450, "极地大乱斗", "aram", "more:aram", "aram"},
		{480, "快速模式", "match", "more:match", "ranked"},
		{490, "匹配模式（快速）", "match", "more:match", "ranked"},
		{700, "召唤师峡谷冠军杯赛", "clash", "more:clash", "ranked"},
		{720, "极地大乱斗冠军杯赛", "clash", "more:clash", "aram"},
		{820, "人机对战（新手）", "bots", "more:bots", "bots"},
		{830, "人机对战（入门）", "bots", "more:bots", "bots"},
		{840, "人机对战（新手）", "bots", "more:bots", "bots"},
		{850, "人机对战（一般）", "bots", "more:bots", "bots"},
		{860, "人机对战", "bots", "more:bots", "bots"},
		{870, "人机对战", "bots", "more:bots", "bots"},
		{880, "人机对战", "bots", "more:bots", "bots"},
		{890, "人机对战", "bots", "more:bots", "bots"},
		{900, "无限火力", "urf", "more:urf", "urf"},
		{930, "极地大乱斗冠军杯赛", "aram", "more:aram", "aram"},
		{950, "末日人工智能（投票）", "doombots", "more:doombots", "bots"},
		{960, "末日人工智能", "doombots", "more:doombots", "bots"},
		{1300, "极限闪击", "nexus-blitz", "more:nexus-blitz", "nexus-blitz"},
		{1700, "斗魂竞技场", "arena", "arena", "arena"},
		{1710, "斗魂竞技场", "arena", "arena", "arena"},
		{1750, "斗魂竞技场", "arena", "arena", "arena"},
		{1900, "无限火力", "urf", "more:urf", "urf"},
		{2300, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram"},
		{2400, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram"},
		{3100, "召唤师峡谷自选自定义", "custom", "excluded", "ranked"},
		{3270, "海克斯大乱斗", "hextech-aram", "hextech-aram", "hextech-aram"},
		{4210, "RUBY", "other", "more:special", "unsupported"},
		{4220, "RUBY", "other", "more:special", "unsupported"},
		{4240, "RUBY 试炼 1", "other", "more:special", "unsupported"},
		{4250, "RUBY 试炼 2", "other", "more:special", "unsupported"},
		{4260, "RUBY 试炼 3", "other", "more:special", "unsupported"},
	}
	if len(supportedQueueDefinitions) != len(tests) {
		t.Fatalf("supported queue count = %d, want %d; update the canonical matrix test when adding queues", len(supportedQueueDefinitions), len(tests))
	}
	for _, want := range tests {
		got, ok := supportedQueueDefinition(want.ID)
		if !ok || got != want {
			t.Errorf("queue %d = %#v ok=%v, want %#v", want.ID, got, ok, want)
		}
	}
}

func TestQueueGroupsForClientExposeAugmentSources(t *testing.T) {
	groups := queueGroupsForClient()
	want := map[int64]string{1700: "arena", 1710: "arena", 1750: "arena", 2300: "hextech", 2400: "hextech", 3270: "hextech"}
	for id, source := range want {
		var found *queueGroupResponse
		for index := range groups {
			if groups[index].ID == id {
				found = &groups[index]
				break
			}
		}
		if found == nil || found.AugmentSource != source {
			t.Fatalf("queue %d group = %#v, want augmentSource=%q", id, found, source)
		}
	}
}

func TestMatchHistoryFilterSpecsUseDocumentedSGPTags(t *testing.T) {
	tests := []struct {
		filter string
		tags   string
	}{
		{"solo", "q_420"},
		{"flex", "q_440"},
		{"more:aram", "q_450,q_930"},
		{"more:match", "q_400,q_430,q_480,q_490"},
		{"more:bots", "q_820,q_830,q_840,q_850,q_860,q_870,q_880,q_890"},
		{"more:urf", "q_900,q_1900"},
		{"more:clash", "q_700,q_720"},
		{"more:nexus-blitz", "q_1300"},
		{"more:doombots", "q_950,q_960"},
		{"hextech-aram", "q_2300,q_2400,q_3270"},
		{"arena", "q_1700,q_1710,q_1750"},
		{"ranked", "ranked"},
	}
	for _, test := range tests {
		spec := matchHistoryFilterFor(test.filter)
		got := strings.Join(spec.Tags, ",")
		if got != test.tags {
			t.Errorf("filter %q tags = %q, want %q", test.filter, got, test.tags)
		}
	}
}
