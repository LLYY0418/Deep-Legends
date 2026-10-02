package main

import (
	"sort"
	"strconv"
	"strings"
)

type livePremadeLabels struct {
	gameID int64
	labels map[string]string
	next   int
}

func (a *app) applyLivePremades(gameID int64, players []gameplayLivePlayer, inputs []livePremadeInput, phase string, arenaMode bool, lobby []lcuLobbyMember) {
	if arenaMode {
		return
	}
	if phase == "ChampSelect" {
		selfTeam := int64(0)
		selfRef := ""
		for _, p := range players {
			if p.IsCurrent {
				selfTeam = p.TeamID
				selfRef = p.reference.PlayerRef
				break
			}
		}
		lobbyIDs := map[string]bool{}
		for _, m := range lobby {
			if m.PUUID != "" {
				lobbyIDs[m.PUUID] = true
			}
		}
		// Lobby is authoritative only if it contains the current player.
		hasSelf := selfRef != "" && lobbyIDs[selfRef]
		subset := []livePremadeInput{}
		indices := []int{}
		for i, p := range players {
			if p.IsCurrent || p.IsAlly || (selfTeam > 0 && p.TeamID == selfTeam) {
				input := inputs[i]
				input.TeamParticipantID = 0
				if hasSelf && lobbyIDs[p.reference.PlayerRef] {
					input.TeamParticipantID = 1
				}
				subset = append(subset, input)
				indices = append(indices, i)
			}
		}
		for i, assignment := range livePremadeAssignments(subset, livePremadeMinSharedGames) {
			p := &players[indices[i]]
			p.PremadeGroup = assignment.Group
			p.PremadeSize = assignment.Size
			p.PremadeSource = assignment.Source
			p.PremadeSessionSignal = assignment.SessionSignal
			if assignment.SessionSignal {
				p.PremadeSource = "lobby"
			}
		}
	} else {
		applyLivePremadeAssignments(players, inputs, phase, false)
	}
	if phase != "ChampSelect" && phase != "InProgress" && phase != "Reconnect" {
		return
	}
	groups := map[string][]int{}
	for i, p := range players {
		if p.PremadeGroup != "" {
			groups[p.PremadeGroup] = append(groups[p.PremadeGroup], i)
		}
	}
	order := make([]string, 0, len(groups))
	for group := range groups {
		order = append(order, group)
	}
	sort.Slice(order, func(i, j int) bool { return groups[order[i]][0] < groups[order[j]][0] })
	a.livePremadeMu.Lock()
	defer a.livePremadeMu.Unlock()
	if a.livePremadeLabels.gameID != gameID || a.livePremadeLabels.labels == nil {
		a.livePremadeLabels = livePremadeLabels{gameID: gameID, labels: map[string]string{}}
	}
	for _, group := range order {
		refs := []string{}
		for _, i := range groups[group] {
			ref := players[i].reference.PlayerRef
			if ref == "" {
				ref = players[i].PlayerRef
			}
			refs = append(refs, ref)
		}
		sort.Strings(refs)
		key := strings.Join(refs, "\x00")
		label := a.livePremadeLabels.labels[key]
		if label == "" {
			a.livePremadeLabels.next++
			label = strconv.Itoa(a.livePremadeLabels.next)
			a.livePremadeLabels.labels[key] = label
		}
		for _, i := range groups[group] {
			players[i].PremadeGroup = label
		}
	}
}
