package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
)

// The LCU does not expose split history in its documented ranked endpoint.
// Probe the small set of plausible routes once per process and record only
// response shape metadata so future implementations can be evidence-driven.
type rankedSplitProbeResult struct {
	Path         string   `json:"path"`
	Status       int      `json:"status,omitempty"`
	Bytes        int      `json:"bytes,omitempty"`
	TopLevelKeys []string `json:"topLevelKeys,omitempty"`
	ErrorKind    string   `json:"errorKind,omitempty"`
}

func (a *app) startRankedSplitProbe(client *LCUClient, puuid string, summonerID int64) {
	if a == nil || !a.rankedSplitProbeEnabled || client == nil {
		return
	}
	a.rankedSplitProbeOnce.Do(func() {
		go a.probeRankedSplitEndpoints(context.Background(), client, puuid, summonerID)
	})
}

func (a *app) probeRankedSplitEndpoints(ctx context.Context, client *LCUClient, puuid string, summonerID int64) {
	paths := []struct{ template, path string }{
		{"/lol-ranked/v1/splits-progress", "/lol-ranked/v1/splits-progress"},
		{"/lol-ranked/v1/current-ranked-stats", "/lol-ranked/v1/current-ranked-stats"},
		{"/lol-career-stats/v1/summoner-games/{puuid}", "/lol-career-stats/v1/summoner-games/" + url.PathEscape(puuid)},
		{"/lol-regalia/v2/summoners/{id}/regalia", "/lol-regalia/v2/summoners/" + strconv.FormatInt(summonerID, 10) + "/regalia"},
		{"/lol-seasons/v1/seasons", "/lol-seasons/v1/seasons"},
	}
	results := make([]rankedSplitProbeResult, 0, len(paths))
	for _, candidate := range paths {
		result := rankedSplitProbeResult{Path: candidate.template}
		data, err := client.GetBytesContext(ctx, candidate.path)
		if err != nil {
			var httpErr *LCUHTTPError
			if errors.As(err, &httpErr) {
				result.Status = httpErr.StatusCode
				result.ErrorKind = "http-" + strconv.Itoa(httpErr.StatusCode)
			} else {
				result.ErrorKind = "other"
			}
			results = append(results, result)
			continue
		}
		result.Status = 200
		result.Bytes = len(data)
		var object map[string]json.RawMessage
		if json.Unmarshal(data, &object) == nil {
			result.TopLevelKeys = diagnosticKeySet(data)
		}
		results = append(results, result)
	}
	a.recordDiagnostic(map[string]any{"event": "lcu_ranked_split_probe", "results": results})
}
