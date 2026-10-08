package main

import (
	"crypto/sha256"
	"strconv"
	"strings"
	"time"
)

type proIdentityBadge struct {
	PlayerName string `json:"playerName"`
	TeamCode   string `json:"teamCode"`
	TeamName   string `json:"teamName"`
	Secondary  bool   `json:"secondary"`
}

func proDirectoryMemberValid(team opggProTeam, m opggProMember) bool {
	for _, t := range proRoster {
		for _, p := range t.Players {
			allowed := team.ID == t.OPGGID
			for _, id := range p.AllowTeams {
				allowed = allowed || team.ID == id
			}
			if allowed && !proSecondaryTeam(team) && strings.EqualFold(strings.TrimSpace(m.Nickname), p.Name) && !proIdentityMatches(m, p) {
				return false
			}
		}
	}
	return (m.TeamID == 0 || m.TeamID == team.ID) && strings.EqualFold(m.Authority, "PROGAMER") && !strings.Contains(strings.ToUpper(m.Position), "COACH") && strings.TrimSpace(m.Nickname) != ""
}

type proIdentityIndex struct {
	byKey      map[string]*proIdentityBadge
	candidates int
}

// Production badges use the same reviewed first-team roster as the directory
// page, including only its explicit historical-team exceptions.
func proReviewedMemberBadge(team opggProTeam, member opggProMember) (proIdentityBadge, bool) {
	if proSecondaryTeam(team) || !proDirectoryMemberValid(team, member) {
		return proIdentityBadge{}, false
	}
	for _, reviewed := range proRoster {
		for _, player := range reviewed.Players {
			allowed := team.ID == reviewed.OPGGID
			for _, id := range player.AllowTeams {
				allowed = allowed || team.ID == id
			}
			if allowed && proIdentityMatches(member, player) {
				return proIdentityBadge{player.Name, reviewed.Code, reviewed.Name, false}, true
			}
		}
	}
	return proIdentityBadge{}, false
}

func (a *app) proIdentitySnapshot() proIdentityIndex {
	a.proPlayers.mu.Lock()
	var teams []opggProTeam
	if a.proPlayers.fetchedAt.IsZero() || time.Since(a.proPlayers.fetchedAt) <= proPlayersMaxStale {
		teams = cloneProTeams(a.proPlayers.teams)
	}
	a.proPlayers.mu.Unlock()
	index := proIdentityIndex{byKey: map[string]*proIdentityBadge{}}
	type candidate struct {
		keys  []string
		badge proIdentityBadge
	}
	rows := []candidate{}
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
	for _, team := range teams {
		for _, m := range team.Members {
			badge, reviewed := proReviewedMemberBadge(team, m)
			if !reviewed {
				continue
			}
			for _, account := range m.Summoners {
				if _, valid := normalizeProAccount(account); !valid {
					continue
				}
				keys := proAccountKeys(account)
				rows = append(rows, candidate{keys, badge})
				for _, k := range keys[1:] {
					parent[root(k)] = root(keys[0])
				}
			}
		}
	}
	owners := map[string]proIdentityBadge{}
	ambiguous := map[string]bool{}
	for _, row := range rows {
		key := root(row.keys[0])
		if old, ok := owners[key]; ok && old != row.badge {
			ambiguous[key] = true
		}
		owners[key] = row.badge
	}
	for _, row := range rows {
		key := root(row.keys[0])
		for _, k := range row.keys {
			index.byKey[k] = nil
			if !ambiguous[key] {
				badge := owners[key]
				index.byKey[k] = &badge
			}
		}
	}
	index.candidates = len(rows)
	return index
}
func (a *app) matchProIdentity(index proIdentityIndex, surface string, reference gameplayReference, gameIDs ...int64) *proIdentityBadge {
	var badge *proIdentityBadge
	matchedBy := "none"
	if strings.EqualFold(strings.TrimSpace(reference.Region), "kr") {
		if reference.PlayerRef != "" {
			if value, exists := index.byKey["puuid:"+strings.TrimSpace(reference.PlayerRef)]; exists {
				badge = value
				if badge != nil {
					matchedBy = "puuid"
				}
				goto emit
			}
		}
		if strings.TrimSpace(reference.GameName) != "" && strings.TrimSpace(reference.TagLine) != "" {
			badge = index.byKey["name:"+strings.ToLower(strings.TrimSpace(reference.GameName)+"#"+strings.TrimSpace(reference.TagLine))]
			if badge != nil {
				matchedBy = "riotid"
			}
		}
	}
emit:
	gameID := int64(0)
	if len(gameIDs) > 0 {
		gameID = gameIDs[0]
	}
	if a.claimProIdentityDiagnostic(reference, gameID) {
		a.recordDiagnostic(map[string]any{"event": "pro_identity_match", "surface": surface, "region": reference.Region, "candidates": index.candidates, "matched_by": matchedBy, "secondary": badge != nil && badge.Secondary})
	}
	if badge == nil {
		return nil
	}
	copy := *badge
	return &copy
}

// R238: repeated overview/history renders do not feed the noisy-event counter.
// Identity exists only as a bounded in-memory digest and is never logged. Keep
// this guard across diagnostic rotations, which previously restarted flooding.
func (a *app) claimProIdentityDiagnostic(reference gameplayReference, gameID int64) bool {
	identity := "puuid:" + strings.TrimSpace(reference.PlayerRef)
	if reference.PlayerRef == "" {
		identity = "name:" + strings.ToLower(strings.TrimSpace(reference.GameName)+"#"+strings.TrimSpace(reference.TagLine))
	}
	key := sha256.Sum256([]byte(strings.ToLower(reference.Region) + "\x00" + strconv.FormatInt(gameID, 10) + "\x00" + identity))
	a.proIdentityDiagnosticMu.Lock()
	defer a.proIdentityDiagnosticMu.Unlock()
	if a.proIdentityDiagnosticKeys == nil {
		a.proIdentityDiagnosticKeys = make(map[[32]byte]struct{})
	}
	if _, exists := a.proIdentityDiagnosticKeys[key]; exists {
		return false
	}
	const limit = 8192
	if len(a.proIdentityDiagnosticOrder) >= limit {
		delete(a.proIdentityDiagnosticKeys, a.proIdentityDiagnosticOrder[0])
		a.proIdentityDiagnosticOrder = a.proIdentityDiagnosticOrder[1:]
	}
	a.proIdentityDiagnosticKeys[key] = struct{}{}
	a.proIdentityDiagnosticOrder = append(a.proIdentityDiagnosticOrder, key)
	return true
}
