package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Deliberately independent from gameplayPlayer: no player name, ref or identity
// may enter this fast channel, including when LCU exposes obfuscated identities.
type champSelectLiteCell struct {
	ChampionName     string `json:"championName,omitempty"`
	CellID           int64  `json:"cellId"`
	Team             int64  `json:"team"`
	ChampionID       int64  `json:"championId"`
	PickIntent       int64  `json:"pickIntent"`
	Locked           bool   `json:"locked"`
	AssignedPosition string `json:"assignedPosition"`
	Spell1ID         int64  `json:"spell1Id"`
	Spell2ID         int64  `json:"spell2Id"`
}
type champSelectLite struct {
	EventAtMS         int64                 `json:"eventAtMs"`
	SentAtMS          int64                 `json:"sentAtMs"`
	Type              string                `json:"type"`
	GameID            int64                 `json:"gameId"`
	LocalPlayerCellID *int64                `json:"localPlayerCellId"`
	Version           uint64                `json:"version"`
	Cells             []champSelectLiteCell `json:"cells"`
}
type champSelectLiteState struct {
	latestAt time.Time
	mu       sync.Mutex
	client   *LCUClient
	latest   champSelectLite
	samples  map[string][]int64
	counts   map[string]int
	maxima   map[string]int64
}

func champSelectLiteFromSession(session lcuChampSelectSession) champSelectLite {
	value := champSelectLite{Type: "champselect-lite", GameID: session.GameID, LocalPlayerCellID: session.LocalPlayerCellID, Cells: []champSelectLiteCell{}}
	for team, players := range map[int64][]lcuChampSelectPlayer{100: session.MyTeam, 200: session.TheirTeam} {
		for _, p := range players {
			if p.CellID == nil {
				continue
			}
			championID := p.ChampionID
			if championID <= 0 {
				championID = positiveChampionPickIntent(p.ChampionPickIntent)
			}
			value.Cells = append(value.Cells, champSelectLiteCell{bundledChampionNames()[championID], *p.CellID, team, p.ChampionID, p.ChampionPickIntent, p.ChampionID > 0, p.AssignedPosition, p.Spell1ID, p.Spell2ID})
		}
	}
	sort.Slice(value.Cells, func(i, j int) bool { return value.Cells[i].CellID < value.Cells[j].CellID })
	return value
}
func (v champSelectLite) signature() string { raw, _ := json.Marshal(v.Cells); return string(raw) }
func champSelectResponseSignature(v gameplayLiveResponse) string {
	lite := champSelectLite{Cells: []champSelectLiteCell{}}
	for _, p := range v.Players {
		if p.CellID != nil {
			lite.Cells = append(lite.Cells, champSelectLiteCell{p.ChampionName, *p.CellID, p.TeamID, p.ChampionID, p.ChampionPickIntent, p.ChampionLocked, p.Position, p.Spell1ID, p.Spell2ID})
		}
	}
	sort.Slice(lite.Cells, func(i, j int) bool { return lite.Cells[i].CellID < lite.Cells[j].CellID })
	return lite.signature()
}
func mergeChampSelectLite(v gameplayLiveResponse, lite champSelectLite) gameplayLiveResponse {
	if v.Phase != "ChampSelect" || v.GameID != lite.GameID {
		return v
	}
	v.Players = append([]gameplayLivePlayer(nil), v.Players...)
	for i, p := range v.Players {
		if p.CellID == nil {
			continue
		}
		for _, cell := range lite.Cells {
			if *p.CellID != cell.CellID {
				continue
			}
			if p.ChampionID != cell.ChampionID || p.ChampionPickIntent != cell.PickIntent {
				p.ChampionName = cell.ChampionName
			}
			p.ChampionID, p.ChampionPickIntent, p.ChampionLocked = cell.ChampionID, positiveChampionPickIntent(cell.PickIntent), cell.Locked
			p.ChampionPickPending = cell.PickIntent < 0
			p.Position = normalizeGameflowPosition(cell.AssignedPosition, "")
			p.Spell1ID, p.Spell2ID = cell.Spell1ID, cell.Spell2ID
			if p.IsCurrent {
				old := v.ResolvedChampionID
				v.CurrentChampionID = cell.ChampionID
				v.ResolvedChampionID = cell.ChampionID
				if v.ResolvedChampionID <= 0 {
					v.ResolvedChampionID = positiveChampionPickIntent(cell.PickIntent)
				}
				if old != v.ResolvedChampionID {
					v.ClientRecommendation = nil
					v.ChampionAbilities = nil
				}
			}
			v.Players[i] = p
			break
		}
	}
	return v
}
func (a *app) mergeLatestChampSelect(client *LCUClient, v gameplayLiveResponse) gameplayLiveResponse {
	s := &a.champLite
	s.mu.Lock()
	lite, valid := s.latest, s.client == client && s.latestAt.After(v.champSelectObservedAt)
	s.mu.Unlock()
	if valid {
		return mergeChampSelectLite(v, lite)
	}
	return v
}

// Leading delivery, a fixed (never reset) 150ms window, and one trailing
// delivery. One consumer owns reads, so slow LCU responses cannot reorder cells.
func (a *app) runChampSelectLitePump(ctx context.Context, client *LCUClient, events <-chan time.Time) {
	var tick <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	var pending time.Time
	send := func(at time.Time) {
		a.publishChampSelectLite(ctx, client, at)
		a.broadcastEvent("champselect:changed")
		if timer == nil {
			timer = time.NewTimer(champSelectEventDebounce)
		} else {
			timer.Reset(champSelectEventDebounce)
		}
		tick = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-events:
			if tick == nil {
				send(at)
			} else if pending.IsZero() {
				pending = at
			}
		case <-tick:
			tick = nil
			if !pending.IsZero() {
				at := pending
				pending = time.Time{}
				send(at)
			}
		}
	}
}
func (a *app) publishChampSelectLite(ctx context.Context, client *LCUClient, arrived time.Time) {
	readCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var session lcuChampSelectSession
	if client.RequestJSON(readCtx, http.MethodGet, champSelectAPI+"/session", nil, &session) != nil || ctx.Err() != nil {
		return
	}
	lite := champSelectLiteFromSession(session)
	s := &a.champLite
	s.mu.Lock()
	lite.Version = s.latest.Version + 1
	lite.EventAtMS = arrived.UnixMilli()
	lite.SentAtMS = time.Now().UnixMilli()
	s.client, s.latest, s.latestAt = client, lite, time.Now()
	s.mu.Unlock()
	raw, _ := json.Marshal(lite)
	a.broadcastEvent(string(raw))
	a.noteChampSelectLatency("event_to_sse", time.Since(arrived).Milliseconds())
}
func (a *app) noteChampSelectLatency(stage string, ms int64) {
	s := &a.champLite
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.samples == nil {
		s.samples = map[string][]int64{}
		s.counts = map[string]int{}
		s.maxima = map[string]int64{}
	}
	s.counts[stage]++
	s.maxima[stage] = max(s.maxima[stage], ms)
	if len(s.samples[stage]) < 2048 {
		s.samples[stage] = append(s.samples[stage], ms)
	}
}
func (a *app) flushChampSelectLatency() {
	s := &a.champLite
	s.mu.Lock()
	samples, counts, maxima := s.samples, s.counts, s.maxima
	s.samples, s.counts, s.maxima = nil, nil, nil
	s.mu.Unlock()
	if len(samples) == 0 {
		return
	}
	stages := map[string]any{}
	for stage, values := range samples {
		sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
		stages[stage] = map[string]any{"count": counts[stage], "sample_count": len(values), "p50_ms": values[len(values)/2], "max_ms": maxima[stage]}
	}
	a.recordDiagnostic(map[string]any{"event": "champselect_latency", "stages": stages})
}
func (a *app) handleChampSelectLatency(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Stage      string `json:"stage"`
		DurationMS int64  `json:"durationMs"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	defer r.Body.Close()
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&req) != nil || req.DurationMS < 0 || req.DurationMS > 120000 {
		http.Error(w, "invalid latency", 400)
		return
	}
	switch req.Stage {
	case "card_render", "recommendation_loading", "client_ready", "pros_ready", "specialists_ready", "build_ready":
	default:
		http.Error(w, "invalid stage", 400)
		return
	}
	a.noteChampSelectLatency(req.Stage, req.DurationMS)
	w.WriteHeader(http.StatusNoContent)
}
func (a *app) handleClientRecommendation(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("championId"), 10, 64)
	mapID, _ := strconv.ParseInt(r.URL.Query().Get("mapId"), 10, 64)
	a.mu.RLock()
	client := a.lcu
	connected := a.clientSessionConnectedLocked()
	a.mu.RUnlock()
	if !connected || client == nil {
		http.Error(w, "客户端尚未连接", 409)
		return
	}
	if id <= 0 || id > 10000 {
		http.Error(w, "invalid champion", 400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	value, err := loadClientRecommendationContext(ctx, client, id, r.URL.Query().Get("position"), mapID)
	if err != nil {
		http.Error(w, fmt.Sprint("客户端推荐暂不可用"), http.StatusBadGateway)
		return
	}
	respondJSON(w, map[string]any{"championId": id, "clientRecommendation": value})
}
