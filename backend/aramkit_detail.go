package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type aramkitItemRef struct {
	ID              int               `json:"id"`
	AugmentSpecific []json.RawMessage `json:"augmentSpecific,omitempty"`
}
type aramkitRoute struct {
	aramkitStat
	PurchaseOrder []aramkitItemRef `json:"purchaseOrder"`
	Starters      []struct {
		aramkitStat
		Items []aramkitItemRef `json:"items"`
	} `json:"starters"`
	Boots []struct {
		aramkitStat
		Item aramkitItemRef `json:"item"`
	} `json:"boots"`
	LaterItems []struct {
		aramkitStat
		Item aramkitItemRef `json:"item"`
	} `json:"laterItems"`
}
type aramkitProfile struct {
	aramkitStat
	ItemSet []aramkitItemRef `json:"itemSet"`
	Routes  []aramkitRoute   `json:"routes"`
}
type aramkitArchetype struct {
	aramkitStat
	Key      string           `json:"key"`
	Profiles []aramkitProfile `json:"profiles"`
}
type aramkitItemScope struct {
	All   []aramkitStat            `json:"all"`
	Slots map[string][]aramkitStat `json:"slots"`
}
type aramkitRawCombo struct {
	aramkitStat
	AugmentIDs []int `json:"augmentIds"`
}
type aramkitRawDetail struct {
	Champion struct {
		ID    int         `json:"id"`
		Stats aramkitStat `json:"stats"`
		Rank  int         `json:"rank"`
		Tier  string      `json:"tier"`
	} `json:"champion"`
	Augments            aramkitTable                `json:"augments"`
	AugmentCombinations []aramkitRawCombo           `json:"augmentCombinations"`
	Items               map[string]aramkitItemScope `json:"items"`
	Builds              map[string]struct {
		Archetypes []aramkitArchetype `json:"archetypes"`
	} `json:"builds"`
	Summoners []struct {
		aramkitStat
		Spells []aramkitItemRef `json:"spells"`
	} `json:"summoners"`
	Skills []struct {
		aramkitStat
		Order string `json:"order"`
	} `json:"skills"`
}
type aramkitBuildView struct {
	Key      string              `json:"key"`
	Name     string              `json:"name"`
	WinRate  float64             `json:"winRate"`
	PickRate float64             `json:"pickRate"`
	Games    int                 `json:"games"`
	Core     []championMetricRow `json:"core"`
	Routes   []championMetricRow `json:"routes"`
	Starters []championMetricRow `json:"starters"`
	Boots    []championMetricRow `json:"boots"`
}
type aramkitScopeView struct {
	All        []championMetricRow            `json:"all"`
	Slots      map[string][]championMetricRow `json:"slots"`
	Archetypes []aramkitBuildView             `json:"archetypes"`
}
type aramkitChampionView struct {
	Balance aramkitBalance              `json:"balance"`
	Scopes  map[string]aramkitScopeView `json:"scopes"`
}

func (r aramkitResources) referenceRow(refs []aramkitItemRef, s aramkitStat, kind string) (championMetricRow, bool) {
	row := championMetricRow{WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount}
	for _, ref := range refs {
		a, ok := r.asset(kind, ref.ID)
		if !ok {
			return row, false
		}
		row.Assets = append(row.Assets, a)
	}
	return row, len(row.Assets) > 0 && validARAMKitRates(s)
}
func (r aramkitResources) combos(raw []aramkitRawCombo) ([]championMetricRow, int) {
	var out []championMetricRow
	dropped := 0
	for _, s := range raw {
		if len(s.AugmentIDs) != 2 {
			dropped++
			continue
		}
		if s.SampleCount < 200 {
			continue
		}
		refs := []aramkitItemRef{{ID: s.AugmentIDs[0]}, {ID: s.AugmentIDs[1]}}
		row, ok := r.referenceRow(refs, s.aramkitStat, "augment")
		if !ok {
			dropped++
			continue
		}
		out = append(out, row)
	}
	return out, dropped
}
func filteredARAMKitBalance(b aramkitBalance) (aramkitBalance, int) {
	out := aramkitBalance{AbilityChanges: b.AbilityChanges}
	dropped := 0
	for _, m := range b.Modifiers {
		switch m.Key {
		case "damageDealt", "damageTaken", "abilityHaste", "tenacity", "healing", "attackSpeed", "shielding", "energyRegen":
			if m.Unit == "percent" || m.Unit == "number" {
				out.Modifiers = append(out.Modifiers, m)
			} else {
				dropped++
			}
		default:
			dropped++
		}
	}
	return out, dropped
}
func parseARAMKitDetail(b []byte, id int, r aramkitResources) (championDetailResponse, map[string]int, error) {
	var raw aramkitRawDetail
	if e := json.Unmarshal(b, &raw); e != nil {
		return championDetailResponse{}, nil, e
	}
	if raw.Champion.ID != id || !validARAMKitRates(raw.Champion.Stats) {
		return championDetailResponse{}, nil, errors.New("ARAMKit champion detail changed")
	}
	resource, ok := r.Champions[strconv.Itoa(id)]
	if !ok {
		return championDetailResponse{}, nil, errors.New("ARAMKit champion identity rejected")
	}
	out := championDetailResponse{Mode: "hextech-aram", Region: "global", Source: "ARAMKit", Patch: aramkitPatch(r.Version.Version), FetchedAt: time.Now(), Stats: championDetailStats{Grade: raw.Champion.Tier, WinRate: raw.Champion.Stats.WinRate * 100, PickRate: raw.Champion.Stats.PickRate * 100, Games: raw.Champion.Stats.SampleCount}}
	out.ARAMKit = &aramkitChampionView{Scopes: map[string]aramkitScopeView{}}
	counts := map[string]int{}
	out.ARAMKit.Balance, counts["dropped_balance"] = filteredARAMKitBalance(resource.Balance)
	out.RecommendedAugments = r.metrics(raw.Augments.All, "augment")
	for i := range out.RecommendedAugments {
		for stage, rows := range raw.Augments.Stages {
			n, e := strconv.Atoi(stage)
			if e != nil || n < 1 || n > 4 {
				continue
			}
			for _, s := range rows {
				if s.ID == out.RecommendedAugments[i].Assets[0].ID && s.SampleCount > 0 {
					out.RecommendedAugments[i].Stages = append(out.RecommendedAugments[i].Stages, championMetricStageRow{Stage: n, Grade: s.Tier, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount})
				}
			}
		}
		sort.Slice(out.RecommendedAugments[i].Stages, func(a, b int) bool {
			return out.RecommendedAugments[i].Stages[a].Stage < out.RecommendedAugments[i].Stages[b].Stage
		})
	}
	counts["dropped_augments"] = len(raw.Augments.All) - len(out.RecommendedAugments)
	out.AugmentCombinations, counts["dropped_combos"] = r.combos(raw.AugmentCombinations)
	for _, scope := range []string{"unfiltered", "filtered"} {
		items := raw.Items[scope]
		v := aramkitScopeView{All: r.metrics(items.All, "item"), Slots: map[string][]championMetricRow{}}
		for slot, rows := range items.Slots {
			v.Slots[slot] = r.metrics(rows, "item")
		}
		for _, a := range raw.Builds[scope].Archetypes {
			resource, ok := r.Archetypes[a.Key]
			name := resource.Name
			if !ok {
				counts["dropped_archetypes"]++
				continue
			}
			view := aramkitBuildView{Key: a.Key, Name: name, WinRate: a.WinRate * 100, PickRate: a.PickRate * 100, Games: a.SampleCount}
			for _, profile := range a.Profiles {
				if row, ok := r.referenceRow(profile.ItemSet, profile.aramkitStat, "item"); ok && len(row.Assets) == 3 {
					view.Core = append(view.Core, row)
				}
				for _, route := range profile.Routes {
					if row, ok := r.referenceRow(route.PurchaseOrder, route.aramkitStat, "item"); ok {
						view.Routes = append(view.Routes, row)
					}
					for _, s := range route.Starters {
						if row, ok := r.referenceRow(s.Items, s.aramkitStat, "item"); ok {
							view.Starters = append(view.Starters, row)
						}
					}
					for _, s := range route.Boots {
						if row, ok := r.referenceRow([]aramkitItemRef{s.Item}, s.aramkitStat, "item"); ok {
							view.Boots = append(view.Boots, row)
						}
					}
				}
			}
			v.Archetypes = append(v.Archetypes, view)
		}
		out.ARAMKit.Scopes[scope] = v
	}
	normal := out.ARAMKit.Scopes["unfiltered"]
	out.ItemRanking = normal.All
	out.Build.ItemSource = "aramkit"
	for _, a := range normal.Archetypes {
		out.Build.CoreItems = append(out.Build.CoreItems, a.Core...)
		out.Build.StarterItems = append(out.Build.StarterItems, a.Starters...)
		out.Build.Boots = append(out.Build.Boots, a.Boots...)
	}
	for _, s := range raw.Summoners {
		if row, ok := r.referenceRow(s.Spells, s.aramkitStat, "spell"); ok {
			out.Build.SummonerSpells = append(out.Build.SummonerSpells, row)
		}
	}
	for _, s := range raw.Skills {
		keys := strings.Split(s.Order, ">")
		if len(keys) != 3 || s.SampleCount <= 0 {
			continue
		}
		valid := true
		for _, k := range keys {
			if k != "Q" && k != "W" && k != "E" {
				valid = false
			}
		}
		if valid {
			out.Build.Skills = append(out.Build.Skills, championMetricRow{SkillPriority: keys, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount})
		}
	}
	return out, counts, nil
}
func (p *championProvider) loadARAMKitDetail(ctx context.Context, champion string) (championDetailResponse, error) {
	id, _, e := p.resolveChampionID(ctx, champion)
	if e != nil {
		return championDetailResponse{}, e
	}
	c := p.aramkitClient()
	v, e := c.current(ctx)
	if e != nil {
		return championDetailResponse{}, e
	}
	r, e := p.loadARAMKitResources(ctx, v)
	if e != nil {
		return championDetailResponse{}, e
	}
	b, e := c.immutable(ctx, fmt.Sprintf("%s/stats/all/champion-details/%d.json", v.DataPath, id))
	if e != nil {
		return championDetailResponse{}, e
	}
	detail, counts, e := parseARAMKitDetail(b, id, r)
	fields := map[string]any{"champion_id": id, "stats_source": "aramkit", "items_source": "aramkit"}
	for k, n := range counts {
		fields[k] = n
	}
	if e != nil {
		fields["parse_failed"] = 1
	}
	c.diagnostic("aramkit_detail_parse", fields)
	return detail, e
}
func (p *championProvider) loadARAMKitRankings(ctx context.Context) (championRankingResponse, error) {
	c := p.aramkitClient()
	v, e := c.current(ctx)
	if e != nil {
		return championRankingResponse{}, e
	}
	r, e := p.loadARAMKitChampionResources(ctx, v)
	if e != nil {
		return championRankingResponse{}, e
	}
	b, e := c.immutable(ctx, v.DataPath+"/stats/all/champion-rankings.json")
	if e != nil {
		return championRankingResponse{}, e
	}
	var raw struct {
		Rows []aramkitStat `json:"rows"`
	}
	if e = json.Unmarshal(b, &raw); e != nil {
		return championRankingResponse{}, e
	}
	out := championRankingResponse{Mode: "hextech-aram", Region: "global", Source: "ARAMKit", Patch: aramkitPatch(v.Version), FetchedAt: time.Now()}
	for _, s := range raw.Rows {
		if _, ok := r.Champions[strconv.Itoa(s.ID)]; !ok || !validARAMKitStat(s) {
			continue
		}
		m := r.Metadata[s.ID]
		out.Rows = append(out.Rows, championRankingRow{ChampionID: s.ID, Key: m.Key, Name: m.NameZH, ImageSource: m.ImageSource, ImagePath: m.ImagePath, Rank: s.Rank, Grade: s.Tier, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Play: s.SampleCount})
	}
	if len(out.Rows) == 0 {
		return out, errors.New("ARAMKit rankings empty")
	}
	sort.SliceStable(out.Rows, func(i, j int) bool { return out.Rows[i].Rank < out.Rows[j].Rank })
	return out, nil
}
func (p *championProvider) loadARAMKitAugments(ctx context.Context) (championAugmentResponse, error) {
	c := p.aramkitClient()
	v, e := c.current(ctx)
	if e != nil {
		return championAugmentResponse{}, e
	}
	r, e := p.loadARAMKitResources(ctx, v)
	if e != nil {
		return championAugmentResponse{}, e
	}
	b, e := c.immutable(ctx, v.DataPath+"/stats/all/augment-rankings.json")
	if e != nil {
		return championAugmentResponse{}, e
	}
	var raw aramkitTable
	if e = json.Unmarshal(b, &raw); e != nil {
		return championAugmentResponse{}, e
	}
	out := championAugmentResponse{Source: "ARAMKit", Mode: "hextech-aram", FetchedAt: time.Now()}
	for _, s := range raw.All {
		if s.SampleCount < 200 || !validARAMKitStat(s) {
			continue
		}
		a, ok := r.asset("augment", s.ID)
		if !ok {
			continue
		}
		resource := r.Augments[strconv.Itoa(s.ID)]
		row := championAugment{ID: s.ID, Key: resource.Slug, Name: a.Name, Grade: s.Tier, Rarity: resource.Rarity, Description: a.Description, Tooltip: a.Description, ImageSource: a.Source, ImagePath: a.Path, WinRate: s.WinRate * 100, Popularity: s.PickRate * 100, Rank: s.Rank, Games: s.SampleCount, AvailableStages: s.AvailableStages}
		for stage, rows := range raw.Stages {
			n, _ := strconv.Atoi(stage)
			for _, s := range rows {
				if s.ID == row.ID {
					row.Stages = append(row.Stages, aramkitAtlasStage{Stage: n, Rank: s.Rank, Grade: s.Tier, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount})
				}
			}
		}
		out.Rows = append(out.Rows, row)
	}
	if len(out.Rows) == 0 {
		return out, errors.New("ARAMKit augment rankings empty")
	}
	return out, nil
}

type aramkitAtlasStage struct {
	Stage    int     `json:"stage"`
	Rank     int     `json:"rank"`
	Grade    string  `json:"grade"`
	WinRate  float64 `json:"winRate"`
	PickRate float64 `json:"pickRate"`
	Games    int     `json:"games"`
}

func (p *championProvider) loadARAMKitAugmentDetail(ctx context.Context, id int) (championAugmentDetailResponse, error) {
	c := p.aramkitClient()
	v, e := c.current(ctx)
	if e != nil {
		return championAugmentDetailResponse{}, e
	}
	r, e := p.loadARAMKitResources(ctx, v)
	if e != nil {
		return championAugmentDetailResponse{}, e
	}
	b, e := c.immutable(ctx, fmt.Sprintf("%s/stats/all/augment-details/%d.json", v.DataPath, id))
	if e != nil {
		return championAugmentDetailResponse{}, e
	}
	var raw struct {
		Augment      aramkitStat              `json:"augment"`
		Champions    aramkitTable             `json:"champions"`
		Stages       map[string][]aramkitStat `json:"stages"`
		Combinations []aramkitRawCombo        `json:"combinations"`
		Items        aramkitItemScope         `json:"items"`
	}
	if e = json.Unmarshal(b, &raw); e != nil {
		return championAugmentDetailResponse{}, e
	}
	if raw.Augment.ID != id {
		return championAugmentDetailResponse{}, errors.New("ARAMKit augment identity changed")
	}
	a, ok := r.asset("augment", id)
	if !ok {
		return championAugmentDetailResponse{}, errors.New("ARAMKit augment missing")
	}
	out := championAugmentDetailResponse{ID: id, Slug: r.Augments[strconv.Itoa(id)].Slug, Source: "ARAMKit", Description: a.Description, effectDescription: a.Description, ChampionStages: map[string][]championAugmentChampion{}}
	convert := func(rows []aramkitStat) []championAugmentChampion {
		var result []championAugmentChampion
		for _, s := range rows {
			if s.SampleCount < 200 || !validARAMKitRates(s) {
				continue
			}
			if _, ok := r.Champions[strconv.Itoa(s.ChampionID)]; !ok {
				continue
			}
			m := r.Metadata[s.ChampionID]
			result = append(result, championAugmentChampion{ID: s.ChampionID, Name: aramkitChampionDisplayName(m), Key: m.Key, ImageSource: m.ImageSource, ImagePath: m.ImagePath, WinRate: s.WinRate * 100, Popularity: s.PickRate * 100, Games: s.SampleCount, Rank: s.Rank, Grade: s.Tier, ChampionWinRate: s.ChampionWinRate * 100, DeltaWinRate: (s.WinRate - s.ChampionWinRate) * 100})
		}
		return result
	}
	out.Champions = convert(raw.Champions.All)
	for stage, rows := range raw.Champions.Stages {
		out.ChampionStages[stage] = convert(rows)
	}
	for stage, rows := range raw.Stages {
		n, _ := strconv.Atoi(stage)
		for _, s := range rows {
			out.Stages = append(out.Stages, aramkitAtlasStage{Stage: n, Rank: s.Rank, Grade: s.Tier, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount})
		}
	}
	out.Combinations, _ = r.combos(raw.Combinations)
	out.Items = r.metrics(raw.Items.All, "item")
	return out, nil
}
func (p *championProvider) loadARAMKitRarity(ctx context.Context) (championAugmentRarityResponse, error) {
	c := p.aramkitClient()
	v, e := c.current(ctx)
	if e != nil {
		return championAugmentRarityResponse{}, e
	}
	b, e := c.immutable(ctx, v.DataPath+"/stats/all/augment-rarity.json")
	if e != nil {
		return championAugmentRarityResponse{}, e
	}
	var raw struct {
		Stages map[string]struct {
			SampleCount int `json:"sampleCount"`
			Rows        []struct {
				Rarity      string  `json:"rarity"`
				Probability float64 `json:"probability"`
			} `json:"rows"`
		} `json:"stages"`
	}
	if e = json.Unmarshal(b, &raw); e != nil {
		return championAugmentRarityResponse{}, e
	}
	out := championAugmentRarityResponse{Source: "ARAMKit"}
	for stage, s := range raw.Stages {
		n, err := strconv.Atoi(stage)
		if err != nil || n < 1 || n > 4 || s.SampleCount <= 0 {
			continue
		}
		seen := map[string]bool{}
		total := 0.0
		valid := true
		for _, v := range s.Rows {
			if seen[v.Rarity] || v.Probability < 0 || v.Probability > 1 {
				valid = false
			}
			switch v.Rarity {
			case "silver", "gold", "prismatic":
			default:
				valid = false
			}
			seen[v.Rarity] = true
			total += v.Probability
		}
		if !valid || len(seen) != 3 || total < 0.99 || total > 1.01 {
			continue
		}
		row := championRarityStage{Stage: n, Games: s.SampleCount}
		for _, v := range s.Rows {
			switch v.Rarity {
			case "silver":
				row.Silver = v.Probability * 100
			case "gold":
				row.Gold = v.Probability * 100
			case "prismatic":
				row.Prismatic = v.Probability * 100
			}
		}
		out.Stages = append(out.Stages, row)
	}
	sort.Slice(out.Stages, func(i, j int) bool { return out.Stages[i].Stage < out.Stages[j].Stage })
	if len(out.Stages) != 4 {
		return out, errors.New("ARAMKit rarity stages incomplete")
	}
	return out, nil
}

func (p *championProvider) loadMayhemDetail(ctx context.Context, champion string) (championDetailResponse, error) {
	started := time.Now()
	result, e := p.loadARAMKitDetail(ctx, champion)
	if e == nil {
		if p.diag != nil {
			p.diag(map[string]any{"event": "mayhem_detail_phases_ms", "stats_source": "aramkit", "items_source": "aramkit", "decorate": int64(0), "total": time.Since(started).Milliseconds()})
		}
		return result, nil
	}
	_, m, e := p.resolveChampionID(ctx, champion)
	if e != nil {
		return result, e
	}
	phases := mayhemRSCPhases{}
	rsc, e := p.loadMayhemRSCWithPhases(ctx, m.Slug, &phases)
	if e != nil {
		if p.diag != nil {
			p.diag(map[string]any{"event": "mayhem_detail_phases_ms", "stats_source": "none", "items_source": "none", "total": time.Since(started).Milliseconds()})
		}
		return championDetailResponse{Mode: "hextech-aram", Source: "none", FetchedAt: time.Now()}, nil
	}
	result = championDetailResponse{Mode: "hextech-aram", Region: "CN", Source: "OP.GG RSC", Patch: rsc.Citation.Patch, FetchedAt: rsc.FetchedAt, Citation: &rsc.Citation, BuildCitation: &rsc.Citation, Build: rsc.Build, RecommendedAugments: rsc.Augments}
	p.enrichCurrentMayhemDetail(ctx, &result)
	p.decorateDetailAssets(ctx, m.Slug, &result)
	if p.diag != nil {
		p.diag(map[string]any{"event": "mayhem_detail_phases_ms", "stats_source": "none", "items_source": "opgg", "total": time.Since(started).Milliseconds()})
	}
	return result, nil
}

func aramkitChampionDisplayName(m championMetadata) string {
	if m.TitleZH != "" && m.TitleZH != m.NameZH {
		if m.NameZH == "" {
			return m.TitleZH
		}
		return m.NameZH + " · " + m.TitleZH
	}
	return m.NameZH
}
