package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	proAccountDormantAfter = 120 * 24 * time.Hour
	proPlayersPath         = "/zh-cn/lol/spectate/list/pro-gamer"
	proPlayersTTL          = 5 * time.Minute
	proPlayersRetry        = 30 * time.Second
	proPlayersMaxStale     = 24 * time.Hour
	proPlayersMaxBytes     = 8 << 20
)

// Directory contents and stable account IDs live only in bounded process memory.
// Do not use the generic OP.GG disk cache for this endpoint.
type proPlayersCache struct {
	mu          sync.Mutex
	teams       []opggProTeam
	fetchedAt   time.Time
	attemptedAt time.Time
	err         error
	flight      chan struct{}
	updating    bool
}

type opggProTeam struct {
	ID        int             `json:"id"`
	Name      string          `json:"name"`
	ShortName string          `json:"short_name"`
	Members   []opggProMember `json:"members"`
}
type opggProMember struct {
	TeamID     int              `json:"team_id"`
	Nickname   string           `json:"nickname"`
	RealName   string           `json:"real_name"`
	Position   string           `json:"position"`
	Authority  string           `json:"authority"`
	Summoners  []opggProAccount `json:"summoners"`
	Incomplete bool             `json:"-"`
	Supplement bool             `json:"-"`
	FetchedAt  time.Time        `json:"-"`
}
type opggProAccount struct {
	PUUID           string          `json:"puuid"`
	Source          string          `json:"-"`
	Inactive        bool            `json:"-"`
	Stale           bool            `json:"-"`
	GameName        string          `json:"game_name"`
	TagLine         string          `json:"tagline"`
	Region          string          `json:"region"`
	UpdatedAt       string          `json:"updated_at"`
	Rank            json.RawMessage `json:"solo_tier_info"`
	LadderRank      int             `json:"-"`
	LadderRankKnown bool            `json:"-"`
}

type proPlayersResponse struct {
	Updating          bool      `json:"updating"`
	Teams             []proTeam `json:"teams"`
	FetchedAt         time.Time `json:"fetchedAt"`
	RosterVerifiedAt  string    `json:"rosterVerifiedAt"`
	RosterStale       bool      `json:"rosterStale"`
	Stale             bool      `json:"stale"`
	Partial           bool      `json:"partial"`
	Unavailable       bool      `json:"unavailable"`
	Warnings          []string  `json:"warnings"`
	PlayerCount       int       `json:"playerCount"`
	DormantCount      int       `json:"dormantCount"`
	AccountCount      int       `json:"accountCount"`
	MissingCount      int       `json:"missingCount"`
	LadderRankPartial bool      `json:"ladderRankPartial"`
}
type proTeam struct {
	Code    string      `json:"code"`
	Name    string      `json:"name"`
	League  string      `json:"league"`
	Players []proPlayer `json:"players"`
}
type proPlayer struct {
	Key      string       `json:"key"`
	Name     string       `json:"name"`
	Position string       `json:"position"`
	Status   string       `json:"status"`
	Accounts []proAccount `json:"accounts"`
}
type proAccount struct {
	Primary         bool   `json:"primary"`
	Dormant         bool   `json:"dormant"`
	Confidence      string `json:"confidence,omitempty"`
	GameName        string `json:"gameName"`
	Source          string `json:"source"`
	Inactive        bool   `json:"inactive,omitempty"`
	Stale           bool   `json:"stale,omitempty"`
	LPKnown         bool   `json:"lpKnown"`
	TagLine         string `json:"tagLine"`
	RankStatus      string `json:"rankStatus"`
	Tier            string `json:"tier,omitempty"`
	Division        int    `json:"division,omitempty"`
	LP              int    `json:"lp"`
	UpdatedAt       string `json:"updatedAt,omitempty"`
	LadderRank      int    `json:"ladderRank"`
	LadderRankKnown bool   `json:"ladderRankKnown"`
}

func (a *app) handleProPlayers(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) > 1 || (len(r.URL.Query()) == 1 && !r.URL.Query().Has("refresh")) ||
		(r.URL.Query().Has("refresh") && (len(r.URL.Query()["refresh"]) != 1 || r.URL.Query().Get("refresh") != "1")) {
		http.Error(w, "invalid pro players query", http.StatusBadRequest)
		return
	}
	teams, fetchedAt, sourceErr := a.loadProPlayers(r.Context(), r.URL.Query().Get("refresh") == "1")
	if r.Context().Err() != nil {
		return
	}
	a.proPlayers.mu.Lock()
	// Read the snapshot and update flag together: otherwise enrichment could
	// finish between load and serialization, leaving old rows with no poll flag.
	if !a.proPlayers.fetchedAt.IsZero() && time.Since(a.proPlayers.fetchedAt) <= proPlayersMaxStale {
		teams, fetchedAt, sourceErr = a.proPlayers.teams, a.proPlayers.fetchedAt, a.proPlayers.err
	}
	updating := a.proPlayers.updating
	a.proPlayers.mu.Unlock()
	result := a.buildProPlayers(teams, proRoster)
	result.Updating = updating
	result.FetchedAt = fetchedAt
	result.RosterVerifiedAt = proRosterVerifiedAt
	verified, _ := time.Parse("2006-01-02", proRosterVerifiedAt)
	result.RosterStale = time.Since(verified) > 30*24*time.Hour
	result.Unavailable = len(teams) == 0
	result.Stale = !result.Unavailable && (sourceErr != nil || time.Since(fetchedAt) >= proPlayersTTL)
	if sourceErr != nil {
		// No upstream bodies, identifiers or proxy details in renderer errors.
		result.Warnings = append(result.Warnings, "OP.GG 账号来源暂不可用，请检查设置中的英雄数据网络后重试。")
	}
	if result.Stale {
		result.Warnings = append(result.Warnings, "当前为上次成功读取的缓存，账号与段位可能已变化。")
	}
	if result.RosterStale {
		result.Warnings = append(result.Warnings, "一队名单超过 30 天未核对，请勿将其视作实时注册名单。")
	}
	if result.LadderRankPartial {
		result.Warnings = append(result.Warnings, "部分韩服天梯排名暂不可用，未返回名次的账号显示“—”。")
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, result)
}

func (a *app) loadProPlayers(ctx context.Context, force bool) ([]opggProTeam, time.Time, error) {
	provider := a.championDataProvider()
	c := &a.proPlayers
	for {
		c.mu.Lock()
		if c.flight != nil {
			done := c.flight
			c.mu.Unlock()
			select {
			case <-done:
				force = false
				continue
			case <-ctx.Done():
				return nil, time.Time{}, ctx.Err()
			}
		}
		now := time.Now()
		fresh := len(c.teams) > 0 && now.Sub(c.fetchedAt) < proPlayersTTL
		backoff := !c.attemptedAt.IsZero() && now.Sub(c.attemptedAt) < proPlayersRetry
		if c.updating || (!force && fresh && c.err == nil && !proSupplementsIncomplete(c.teams)) || backoff {
			teams, at, err := c.teams, c.fetchedAt, c.err
			if now.Sub(at) > proPlayersMaxStale {
				teams = nil
			}
			c.mu.Unlock()
			return teams, at, err
		}
		done := make(chan struct{})
		c.flight, c.attemptedAt = done, now
		previous := c.teams // immutable snapshots; the flight is the only writer
		c.mu.Unlock()
		// One caller leaving a page must not cancel another caller's shared load.
		go func() {
			started := time.Now()
			loadCtx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
			defer cancel()
			directoryCtx, directoryCancel := context.WithTimeout(loadCtx, 12*time.Second)
			body, err := fetchProDirectory(directoryCtx, provider)
			directoryCancel()
			var teams []opggProTeam
			if err == nil {
				teams, err = parseOPGGProPlayers(body)
			}
			base := teams
			if err == nil {
				// Publish verified directory rows immediately; supplemental sources
				// and per-account ladder requests must not hold the entire page.
				teams = append(cloneProTeams(base), retainProSupplements(pendingProSupplements(), previous, time.Now())...)
			}
			c.mu.Lock()
			if err == nil {
				c.teams, c.fetchedAt = teams, time.Now()
				c.updating = true
			}
			c.err = err
			c.flight = nil
			close(done)
			c.mu.Unlock()
			a.recordDiagnostic(map[string]any{"event": "pro_directory_cost", "stage": "directory", "duration_ms": time.Since(started).Milliseconds(), "success": err == nil})
			if err != nil {
				return
			}
			// Every published snapshot is immutable, including nested accounts.
			supplements := retainProSupplements(loadProSupplements(loadCtx, provider, func(partial []opggProTeam) {
				snapshot := append(cloneProTeams(base), retainProSupplements(partial, previous, time.Now())...)
				c.mu.Lock()
				c.teams = snapshot
				c.mu.Unlock()
			}), previous, time.Now())
			completed := append(cloneProTeams(base), supplements...)
			c.mu.Lock()
			c.teams = cloneProTeams(completed)
			c.mu.Unlock()
			a.recordDiagnostic(map[string]any{"event": "pro_directory_cost", "stage": "supplements", "duration_ms": time.Since(started).Milliseconds(), "partial": proSupplementsIncomplete(completed)})
			enrichProLadderRanks(loadCtx, provider, completed)
			c.mu.Lock()
			c.teams, c.updating = completed, false
			c.mu.Unlock()
			a.recordDiagnostic(map[string]any{"event": "pro_directory_cost", "stage": "ladder", "duration_ms": time.Since(started).Milliseconds()})
		}()
		select {
		case <-done:
			force = false
		case <-ctx.Done():
			return nil, time.Time{}, ctx.Err()
		}
	}
}

func cloneProTeams(teams []opggProTeam) []opggProTeam {
	out := append([]opggProTeam(nil), teams...)
	for i := range out {
		out[i].Members = append([]opggProMember(nil), teams[i].Members...)
		for j := range out[i].Members {
			out[i].Members[j].Summoners = append([]opggProAccount(nil), teams[i].Members[j].Summoners...)
		}
	}
	return out
}

// Only the fixed KR directory is allowed. Use the configured transport/proxy,
// but neither its cookie jar nor generic redirects (including alternate ports).
func fetchProDirectory(ctx context.Context, provider *championProvider) ([]byte, error) {
	if gate := featureGateForChampionHost(opggPageHost); gate != "" && !provider.featureGates.enabled(gate) {
		return nil, errors.New("pro directory source disabled")
	}
	client := *provider.httpClient()
	client.Jar = nil
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || r.URL.Scheme != "https" || r.URL.Host != opggPageHost || r.URL.User != nil || r.URL.Path != proPlayersPath || r.URL.RawQuery != "region=kr" {
			return errors.New("pro directory redirect rejected")
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+opggPageHost+proPlayersPath+"?region=kr", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.6")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pro directory HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, proPlayersMaxBytes)
}

// Decode complete Flight records rather than scraping the visible first rows.
// The wrapper's region is mandatory: null account.region is common and only
// meaningful when the containing directory has explicit KR provenance.
func parseOPGGProPlayers(body []byte) ([]opggProTeam, error) {
	if len(body) == 0 || len(body) > proPlayersMaxBytes {
		return nil, errors.New("invalid pro directory size")
	}
	flight := decodeNextFlight(body)
	if flight == "" {
		return nil, errors.New("pro directory Flight data missing")
	}
	teams := make(map[int]opggProTeam)
	memberCount := 0
	var walk func(any, int) error
	walk = func(value any, depth int) error {
		if depth > 80 {
			return errors.New("pro directory nesting limit")
		}
		switch node := value.(type) {
		case map[string]any:
			if rawTeam, ok := node["team"].(map[string]any); ok {
				if node["region"] != "kr" {
					return nil
				}
				if _, ok := rawTeam["members"].([]any); !ok {
					return errors.New("pro directory members missing")
				}
				data, err := json.Marshal(rawTeam)
				if err != nil {
					return err
				}
				var team opggProTeam
				if err := json.Unmarshal(data, &team); err != nil {
					return errors.New("pro directory schema changed")
				}
				if team.ID <= 0 || team.Name == "" || len(team.Members) > 1000 {
					return errors.New("invalid pro directory team")
				}
				memberCount += len(team.Members)
				if memberCount > 10000 || len(teams[team.ID].Members)+len(team.Members) > 1000 {
					return errors.New("pro directory member limit")
				}
				// Repeated SSR components are reconciled by account identity later.
				if previous, ok := teams[team.ID]; ok {
					if previous.Name != team.Name {
						return errors.New("conflicting pro directory team")
					}
					previous.Members = append(previous.Members, team.Members...)
					teams[team.ID] = previous
				} else {
					teams[team.ID] = team
				}
				if len(teams) > 1000 {
					return errors.New("pro directory team limit")
				}
				return nil
			}
			for _, child := range node {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range node {
				if err := walk(child, depth+1); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, line := range strings.Split(flight, "\n") {
		_, data, ok := strings.Cut(line, ":")
		if !ok || len(data) == 0 || (data[0] != '[' && data[0] != '{') {
			continue
		}
		var record any
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			if strings.Contains(data, `"members"`) {
				return nil, errors.New("incomplete pro directory record")
			}
			continue
		}
		if err := walk(record, 0); err != nil {
			return nil, err
		}
	}
	// Losing a whole requested team is a source failure, not an empty roster.
	for _, team := range proRoster {
		if _, ok := teams[team.OPGGID]; !ok {
			return nil, fmt.Errorf("pro directory requested team missing: %s", team.Code)
		}
	}
	result := make([]opggProTeam, 0, len(teams))
	for _, team := range teams {
		result = append(result, team)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

func proIdentityToken(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func proIdentityMatches(member opggProMember, player proRosterPlayer) bool {
	if !strings.EqualFold(strings.TrimSpace(member.Nickname), player.Name) {
		return false
	}
	parts := strings.FieldsFunc(member.RealName, func(r rune) bool { return r == '(' || r == ')' || r == '（' || r == '）' })
	for _, part := range parts {
		for _, name := range player.Names {
			if proIdentityToken(part) == proIdentityToken(name) && proIdentityToken(name) != "" {
				return true
			}
		}
	}
	return false
}
func proSecondaryTeam(team opggProTeam) bool {
	switch team.ID {
	case 101, 90, 167, 91, 104, 89, 98, 110, 105:
		return true
	}
	name := strings.ToLower(team.Name + " " + team.ShortName)
	for _, marker := range []string{"challenger", "academy", "junior", "youth", ".young", "二队", "青训"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return strings.HasSuffix(strings.TrimSpace(name), " cl")
}

var proTierOrder = map[string]int{"IRON": 1, "BRONZE": 2, "SILVER": 3, "GOLD": 4, "PLATINUM": 5, "EMERALD": 6, "DIAMOND": 7, "MASTER": 8, "GRANDMASTER": 9, "CHALLENGER": 10}

func normalizeProAccount(raw opggProAccount) (proAccount, bool) {
	account := proAccount{GameName: strings.TrimSpace(raw.GameName), TagLine: strings.TrimSpace(raw.TagLine), RankStatus: "unavailable", Source: "OP.GG", Inactive: raw.Inactive, Dormant: raw.Inactive, Stale: raw.Stale, LadderRank: raw.LadderRank, LadderRankKnown: raw.LadderRankKnown}
	if raw.PUUID == "" {
		account.Confidence = "low"
	}
	if raw.Source == "TrackingThePros" {
		account.Source = raw.Source
	}
	if raw.Region != "" && !strings.EqualFold(raw.Region, "kr") {
		return account, false
	}
	if account.GameName == "" || account.TagLine == "" || len(account.GameName) > 128 || len(account.TagLine) > 32 || strings.ContainsAny(account.GameName+account.TagLine, "#") || strings.ContainsFunc(account.GameName+account.TagLine, unicode.IsControl) {
		return account, false
	}
	if at, err := time.Parse(time.RFC3339, raw.UpdatedAt); err == nil {
		account.UpdatedAt = at.UTC().Format(time.RFC3339)
		account.Dormant = raw.Inactive || time.Since(at) > proAccountDormantAfter
	}
	// Missing source timestamps are unknown, never proof of inactivity.
	// UpdatedAt is a source record timestamp, not the last game played.
	// A null rank is not proof of being unranked; upstream may lack data.
	var rank struct {
		Tier     string `json:"tier"`
		Division *int   `json:"division"`
		LP       *int   `json:"lp"`
	}
	if json.Unmarshal(raw.Rank, &rank) != nil {
		return account, true
	}
	rank.Tier = strings.ToUpper(rank.Tier)
	if rank.Tier == "UNRANKED" {
		account.RankStatus = "unranked"
		return account, true
	}
	if proTierOrder[rank.Tier] == 0 || (rank.LP != nil && *rank.LP < 0) {
		return account, true
	}
	division := 1
	if proTierOrder[rank.Tier] < proTierOrder["MASTER"] {
		if rank.Division == nil || *rank.Division < 1 || *rank.Division > 4 {
			return account, true
		}
		division = *rank.Division
	}
	account.Tier, account.Division, account.RankStatus = rank.Tier, division, "ranked"
	if rank.LP != nil {
		account.LP, account.LPKnown = *rank.LP, true
	}
	return account, true
}
func proAccountLess(a, b proAccount) bool {
	if a.Dormant != b.Dormant {
		return !a.Dormant
	}
	if proTierOrder[a.Tier] != proTierOrder[b.Tier] {
		return proTierOrder[a.Tier] > proTierOrder[b.Tier]
	}
	if a.Division != b.Division {
		return a.Division < b.Division
	}
	if a.LPKnown != b.LPKnown {
		return a.LPKnown
	}
	if a.LP != b.LP {
		return a.LP > b.LP
	}
	if a.RankStatus != b.RankStatus {
		return a.RankStatus == "unranked"
	}
	return strings.ToLower(a.GameName+"#"+a.TagLine) < strings.ToLower(b.GameName+"#"+b.TagLine)
}

func (a *app) buildProPlayers(source []opggProTeam, roster []proRosterTeam) proPlayersResponse {
	// Main-directory freshness and partial supplemental freshness are distinct.
	result := proPlayersResponse{Teams: []proTeam{}, Warnings: []string{}, Partial: proSupplementsIncomplete(source)}
	type candidate struct {
		raw     opggProAccount
		account proAccount
		owner   string
	}
	candidates := []candidate{}
	issues := map[string]bool{}
	supplementIncomplete := false
	keys := proAccountKeys
	for _, team := range roster {
		out := proTeam{Code: team.Code, Name: team.Name, League: team.League, Players: []proPlayer{}}
		for _, player := range team.Players {
			key := team.Code + "/" + strings.ToLower(player.Name)
			out.Players = append(out.Players, proPlayer{Key: key, Name: player.Name, Position: player.Position, Accounts: []proAccount{}})
			for _, sourceTeam := range source {
				if proSecondaryTeam(sourceTeam) {
					continue
				}
				for _, member := range sourceTeam.Members {
					if (member.TeamID != 0 && member.TeamID != sourceTeam.ID) || strings.Contains(strings.ToUpper(member.Authority), "COACH") || strings.Contains(strings.ToUpper(member.Position), "COACH") || !strings.EqualFold(member.Authority, "PROGAMER") {
						continue
					}
					if !strings.EqualFold(strings.TrimSpace(member.Nickname), player.Name) {
						continue
					}
					if !proIdentityMatches(member, player) {
						issues[key] = true
						continue
					}
					allowed := sourceTeam.ID == team.OPGGID
					for _, id := range player.AllowTeams {
						allowed = allowed || sourceTeam.ID == id
					}
					if !allowed {
						issues[key] = true
						continue
					}
					if member.Incomplete {
						issues[key] = true
						supplementIncomplete = true
					}
					for _, raw := range member.Summoners {
						account, valid := normalizeProAccount(raw)
						if !valid {
							issues[key] = true
							continue
						}
						candidates = append(candidates, candidate{raw, account, key})
					}
				}
			}
		}
		result.Teams = append(result.Teams, out)
	}
	// Union the complete identity graph before selecting a row. A rename chain
	// may connect records by PUUID in one step and by Riot ID in the next;
	// pairwise "seen" checks can otherwise duplicate or misattribute accounts.
	parent := map[string]string{}
	var root func(string) string
	root = func(k string) string {
		p, ok := parent[k]
		if !ok {
			parent[k] = k
			return k
		}
		if p != k {
			parent[k] = root(p)
		}
		return parent[k]
	}
	for _, item := range candidates {
		ks := keys(item.raw)
		for _, k := range ks[1:] {
			parent[root(k)] = root(ks[0])
		}
	}
	owners := map[string]map[string]bool{}
	for _, item := range candidates {
		group := root(keys(item.raw)[0])
		if owners[group] == nil {
			owners[group] = map[string]bool{}
		}
		owners[group][item.owner] = true
	}
	// The latest valid source timestamp wins; rank sorts only after dedup.
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].account.UpdatedAt > candidates[j].account.UpdatedAt })
	seen := map[string]bool{}
	accounts := map[string][]proAccount{}
	for _, item := range candidates {
		group := root(keys(item.raw)[0])
		if len(owners[group]) > 1 {
			issues[item.owner] = true
			continue
		}
		if seen[group] {
			continue
		}
		seen[group] = true
		// Resolve the current Riot ID through the existing overview pipeline on
		// click. Upstream PUUIDs stay private and cannot leave cached stale
		// session references after client disconnect/account switching.
		accounts[item.owner] = append(accounts[item.owner], item.account)
	}
	for ti := range result.Teams {
		for pi := range result.Teams[ti].Players {
			player := &result.Teams[ti].Players[pi]
			if rows := accounts[player.Key]; len(rows) > 0 {
				player.Accounts = rows
			}
			sort.SliceStable(player.Accounts, func(i, j int) bool { return proAccountLess(player.Accounts[i], player.Accounts[j]) })
			primarySet := false
			for i := range player.Accounts {
				account := &player.Accounts[i]
				if account.Dormant {
					result.DormantCount++
				}
				if !primarySet && !account.Dormant && account.RankStatus == "ranked" {
					account.Primary = true
					primarySet = true
				}
			}
			player.Status = "available"
			if len(player.Accounts) == 0 {
				player.Status = "missing"
				result.MissingCount++
			}
			if issues[player.Key] {
				player.Status = "partial"
			}
			if len(source) == 0 {
				player.Status = "unavailable"
			}
			result.PlayerCount++
			result.AccountCount += len(player.Accounts)
			for _, account := range player.Accounts {
				if account.RankStatus == "ranked" && !account.LadderRankKnown {
					result.LadderRankPartial = true
				}
			}
		}
	}
	if supplementIncomplete {
		result.Warnings = append(result.Warnings, "部分补充身份页暂不可用或信息不完整；可用旧记录标记为缓存，相关选手已标记待核验。")
	}
	if len(issues) > 0 {
		result.Warnings = append(result.Warnings, "部分记录的来源或身份信息待核验；未通过归属核验的账号不会展示。")
	}
	return result
}

// Missing ranks are unknown, not proof of a mismatch. Only ranked solo counts.
func proOverviewMismatch(expected string, ranks []gameplayRank) bool {
	wanted := proTierOrder[strings.ToUpper(expected)]
	if wanted == 0 {
		return false
	}
	for _, rank := range ranks {
		if rank.QueueType != "RANKED_SOLO_5x5" {
			continue
		}
		actual := proTierOrder[strings.ToUpper(rank.Tier)]
		if actual == 0 {
			return false
		}
		delta := wanted - actual
		return delta >= 3 || delta <= -3
	}
	return false
}

func proAccountKeys(raw opggProAccount) []string {
	k := []string{"name:" + strings.ToLower(strings.TrimSpace(raw.GameName)+"#"+strings.TrimSpace(raw.TagLine))}
	if raw.PUUID != "" {
		k = append([]string{"puuid:" + raw.PUUID}, k...)
	}
	return k
}
