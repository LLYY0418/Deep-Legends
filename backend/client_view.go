package main

import (
	"encoding/json"
	"strings"
)

// clientView is the only connection presentation model. It is built from cached
// session fields; discovery/probe results and credentials are diagnostic only.
type clientView struct {
	Type          string         `json:"type"`
	State         string         `json:"state"`
	Summoner      publicSummoner `json:"summoner"`
	Region        string         `json:"region"`
	ServerID      string         `json:"serverId"`
	SGPReady      bool           `json:"sgpReady"`
	Generation    uint64         `json:"generation"`
	ClientVersion string         `json:"clientVersion"`
}

// Serialize capture, generation and publication so a late publisher cannot put
// an older ready frame after an exiting frame. Status and SSE use this producer.
func (a *app) currentClientView() clientView {
	a.clientViewMu.Lock()
	defer a.clientViewMu.Unlock()
	view := clientView{Type: "client-view", State: "no-client"}
	a.mu.RLock()
	client, identity := a.lcu, a.summoner
	connected := a.clientSessionConnectedLocked()
	ready := a.identityReady || connected && identity.SummonerID != 0
	exiting := client != nil && a.shutdownClient == client
	if client != nil && (connected || exiting) {
		client.mu.RLock()
		region, platform := client.region, client.rsoPlatform
		view.ClientVersion = client.gameVersion
		client.mu.RUnlock()
		if strings.EqualFold(region, "TENCENT") {
			view.Region = "TENCENT"
			view.ServerID, _ = normalizeTencentServerID(platform)
		} else {
			view.Region = riotPlatformFromClientRegion(platform)
			if view.Region == "" {
				view.Region = riotPlatformFromClientRegion(region)
			}
		}
		view.SGPReady = a.selfReadinessClient == client && a.selfReadinessAccount == identity.PUUID && a.selfSGPReady
		bgID, bgName, bgSource, bgPath := profileBackground(a.account.Profile)
		view.Summoner = publicSummoner{DisplayName: identity.DisplayName, GameName: identity.GameName, TagLine: identity.TagLine, ProfileIconID: identity.ProfileIconID, SummonerLevel: identity.SummonerLevel, BackgroundSkinID: bgID, BackgroundSkinName: bgName, BackgroundSource: bgSource, BackgroundPath: bgPath}
		view.Summoner.BackgroundPosterPath, view.Summoner.BackgroundVideoPath = overviewSkinMedia(a.allSkins, bgID)
		if exiting {
			view.State = "exiting"
		} else if ready && (view.Region != "" || view.ServerID != "") && (view.Region != "TENCENT" || view.ServerID != "" && view.SGPReady) {
			view.State = "ready"
		}
	}
	a.mu.RUnlock()
	if (connected || exiting) && client != nil {
		refRegion := view.Region
		if refRegion == "TENCENT" {
			refRegion = ""
		}
		view.Summoner.PlayerRef = a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: identity.PUUID, Region: refRegion, ServerID: view.ServerID})
	}
	signature, _ := json.Marshal(view)
	if string(signature) != a.clientViewSignature {
		a.clientViewGeneration++
		a.clientViewSignature = string(signature)
		view.Generation = a.clientViewGeneration
		data, _ := json.Marshal(view)
		a.broadcastEvent(string(data))
	} else {
		view.Generation = a.clientViewGeneration
	}
	return view
}

func (a *app) publishClientView() { a.currentClientView() }
