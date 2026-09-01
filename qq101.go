package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	qq101VersionPath    = "/go/database/versionlist"
	qq101RiftPath       = "/go/battle_info/odp_proxy/lol_101strategy"
	qq101ResponseMax    = 2 << 20
	qq101RequestTimeout = 8 * time.Second
	qq101DetailWait     = 1500 * time.Millisecond
	qq101RetryDelay     = 250 * time.Millisecond
	qq101ProbeCooldown  = 30 * time.Minute
)

type qq101Envelope struct {
	Code   int             `json:"code"`
	Data   json.RawMessage `json:"data"`
	Result json.RawMessage `json:"result"`
}

type qq101ProbePart struct {
	Name      string `json:"name"`
	Outcome   string `json:"outcome"`
	Count     int    `json:"count"`
	Bytes     int    `json:"bytes"`
	ErrorKind string `json:"errorKind,omitempty"`
}

func (p *championProvider) fetchQQ101(ctx context.Context, requestPath string, query url.Values) ([]byte, error) {
	data, _, err := p.fetchWithMetadataCacheKeyLoader(ctx, qq101Host, requestPath, query, qq101ResponseMax, "application/json", "", func(loadCtx context.Context) ([]byte, error) {
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			attemptCtx, cancel := context.WithTimeout(loadCtx, qq101RequestTimeout)
			data, err := p.fetchDirect(attemptCtx, qq101Host, requestPath, query, qq101ResponseMax, "application/json")
			cancel()
			if err == nil {
				return data, nil
			}
			lastErr = err
			if loadCtx.Err() != nil || !qq101Retryable(err) || attempt == 1 {
				break
			}
			timer := time.NewTimer(qq101RetryDelay << attempt)
			select {
			case <-loadCtx.Done():
				timer.Stop()
				return nil, loadCtx.Err()
			case <-timer.C:
			}
		}
		return nil, lastErr
	})
	return data, err
}

func qq101Retryable(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || strings.Contains(strings.ToLower(err.Error()), "disabled by feature gate") {
		return false
	}
	status := championUpstreamHTTPStatus(err)
	return status == 0 || status == httpStatusTooManyRequests || status >= 500
}

const httpStatusTooManyRequests = 429

func parseQQ101LatestPatch(data []byte) (string, error) {
	var envelope qq101Envelope
	if json.Unmarshal(data, &envelope) != nil || envelope.Code != 0 {
		return "", errors.New("QQ101 patch response changed")
	}
	var patches []struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(envelope.Data, &patches) != nil || len(patches) == 0 {
		return "", errors.New("QQ101 patch list is empty")
	}
	patch := strings.TrimSpace(patches[0].Name)
	if !validQQ101PatchName(patch) {
		return "", errors.New("QQ101 patch name is invalid")
	}
	return patch, nil
}

func validQQ101PatchName(value string) bool {
	major, minor, ok := parsePatchVersion(value)
	return ok && major > 0 && minor >= 0 && strings.Contains(value, ".")
}

func qq101InnerPayload(data []byte, fieldID string) ([]byte, error) {
	var envelope qq101Envelope
	if json.Unmarshal(data, &envelope) != nil || envelope.Code != 0 {
		return nil, errors.New("QQ101 upstream returned an error")
	}
	var payload json.RawMessage
	if len(envelope.Data) > 0 && string(envelope.Data) != "null" {
		var body struct {
			Result      json.RawMessage            `json:"result"`
			FieldValues map[string]json.RawMessage `json:"_fieldValues"`
		}
		if json.Unmarshal(envelope.Data, &body) == nil {
			payload = body.FieldValues["R"+fieldID]
			if len(payload) == 0 {
				payload = body.Result
			}
		} else {
			payload = envelope.Data
		}
	}
	if len(payload) == 0 {
		payload = envelope.Result
	}
	return normalizeQQ101Payload(payload)
}

func normalizeQQ101Payload(payload json.RawMessage) ([]byte, error) {
	payload = json.RawMessage(strings.TrimSpace(string(payload)))
	if len(payload) == 0 || string(payload) == "null" {
		return nil, errors.New("QQ101 returned an empty response")
	}
	if payload[0] == '"' {
		var value string
		if json.Unmarshal(payload, &value) != nil {
			return nil, errors.New("QQ101 returned an invalid embedded payload")
		}
		value = strings.TrimSpace(value)
		if value == "" || value == "-1" {
			return nil, errors.New("QQ101 returned an empty response")
		}
		return []byte(value), nil
	}
	return payload, nil
}

func (p *championProvider) loadQQ101LatestPatch(ctx context.Context) (string, error) {
	data, err := p.fetchQQ101(ctx, qq101VersionPath, url.Values{"zone": {"lol"}, "from": {"h5"}})
	if err != nil {
		return "", err
	}
	return parseQQ101LatestPatch(data)
}

func qq101TierID(tier string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "all":
		return 255, true
	case "gold_plus":
		return 24, true
	case "platinum_plus":
		return 25, true
	case "emerald_plus":
		return 26, true
	case "diamond_plus":
		return 27, true
	case "master":
		return 8, true
	case "master_plus":
		return 28, true
	case "grandmaster":
		return 9, true
	case "challenger":
		return 10, true
	default:
		return 0, false
	}
}

func qq101PositionName(position string) string {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "top":
		return "TOP"
	case "jungle":
		return "JUNGLE"
	case "mid", "middle":
		return "MIDDLE"
	case "adc", "bottom":
		return "BOTTOM"
	case "support", "utility":
		return "SUPPORT"
	default:
		return "ALL"
	}
}

func qq101RiftQuery(patch, tier, position string, championID int) (url.Values, error) {
	tierID, ok := qq101TierID(tier)
	if !ok {
		return nil, fmt.Errorf("QQ101 tier %q is unsupported", tier)
	}
	return url.Values{
		"itier":      {strconv.Itoa(tierID)},
		"version_id": {patch},
		"lane":       {qq101PositionName(position)},
		"championid": {strconv.Itoa(championID)},
	}, nil
}

func (p *championProvider) loadQQ101Positions(ctx context.Context, championID int, tier string) ([]championPositionOption, string, error) {
	patch, err := p.loadQQ101LatestPatch(ctx)
	if err != nil {
		return nil, "", err
	}
	query, err := qq101RiftQuery(patch, tier, "", championID)
	if err != nil {
		return nil, patch, err
	}
	query.Del("lane")
	data, err := p.fetchQQ101(ctx, qq101RiftPath+"_newlane", query)
	if err != nil {
		return nil, patch, err
	}
	positions, err := parseQQ101Positions(data)
	return positions, patch, err
}

func parseQQ101Positions(data []byte) ([]championPositionOption, error) {
	payload, err := qq101InnerPayload(data, "18122")
	if err != nil {
		return nil, err
	}
	var body struct {
		LaneDetails string `json:"lane_details"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return nil, errors.New("QQ101 positions payload changed")
	}
	result := make([]championPositionOption, 0, 5)
	for _, record := range splitQQ101Records(body.LaneDetails) {
		positionName, values, ok := strings.Cut(record, "：")
		if !ok {
			positionName, values, ok = strings.Cut(record, ":")
		}
		if !ok {
			continue
		}
		position := normalizeQQ101Position(positionName)
		fields := strings.Split(strings.TrimSpace(values), "_")
		if position == "" || len(fields) < 6 {
			continue
		}
		pickRate, pickOK := parseQQ101Percent(fields[0])
		winRate, winOK := parseQQ101Percent(fields[1])
		banRate, banOK := parseQQ101Percent(fields[2])
		roleRate, roleOK := parseQQ101Percent(fields[5])
		if !pickOK || !winOK || !banOK || !roleOK || roleRate <= 0 {
			continue
		}
		tierValue, _ := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(strings.TrimSpace(fields[3])), "T"))
		rankValue, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
		result = append(result, championPositionOption{Position: position, PickRate: pickRate, WinRate: winRate, BanRate: banRate, RoleRate: roleRate, Tier: tierValue, Rank: rankValue})
	}
	if len(result) == 0 {
		return nil, errors.New("QQ101 positions response is empty")
	}
	return result, nil
}

func normalizeQQ101Position(value string) string {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "TOP":
		return "top"
	case "JUNGLE":
		return "jungle"
	case "MIDDLE", "MID":
		return "mid"
	case "BOTTOM", "ADC":
		return "adc"
	case "SUPPORT", "UTILITY":
		return "support"
	default:
		return ""
	}
}

func parseQQ101Percent(value string) (float64, bool) {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed, err == nil && parsed >= 0 && parsed <= 100
}

func mergeQQ101PositionShares(existing, official []championPositionOption) []championPositionOption {
	if len(official) == 0 {
		return existing
	}
	result := make([]championPositionOption, 0, len(existing)+len(official))
	merged := make(map[string]bool, len(official))
	for _, current := range existing {
		if item, ok := findQQ101Position(official, current.Position); ok {
			current.RoleRate = item.RoleRate
			if current.WinRate == 0 {
				current.WinRate = item.WinRate
			}
			if current.PickRate == 0 {
				current.PickRate = item.PickRate
			}
			if current.BanRate == 0 {
				current.BanRate = item.BanRate
			}
			if current.Tier == 0 {
				current.Tier = item.Tier
			}
			if current.Rank == 0 {
				current.Rank = item.Rank
			}
			merged[item.Position] = true
		}
		result = append(result, current)
	}
	for _, item := range official {
		if !merged[item.Position] {
			result = append(result, item)
		}
	}
	return result
}

func findQQ101Position(values []championPositionOption, position string) (championPositionOption, bool) {
	for _, item := range values {
		if item.Position == position {
			return item, true
		}
	}
	return championPositionOption{}, false
}

func splitQQ101Records(value string) []string {
	parts := strings.Split(strings.TrimSpace(value), "#")
	result := parts[:0]
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" && part != "-1" {
			result = append(result, part)
		}
	}
	return result
}

func parseQQ101BuildRows(data []byte) (map[int][]championMetricRow, error) {
	payload, err := qq101InnerPayload(data, "18087")
	if err != nil {
		return nil, err
	}
	var body struct {
		Fourth string `json:"forth_details"`
		Fifth  string `json:"fifth_details"`
		Sixth  string `json:"sixth_details"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return nil, errors.New("QQ101 build payload changed")
	}
	result := make(map[int][]championMetricRow, 3)
	for depth, raw := range map[int]string{4: body.Fourth, 5: body.Fifth, 6: body.Sixth} {
		for _, record := range splitQQ101Records(raw) {
			fields := strings.Split(record, "_")
			if len(fields) != 4 {
				continue
			}
			rank, rankErr := strconv.Atoi(strings.TrimSpace(fields[1]))
			pickRate, pickOK := parseQQ101Percent(fields[2])
			winRate, winOK := parseQQ101Percent(fields[3])
			assets := make([]championAsset, 0, 3)
			for _, rawID := range strings.Split(fields[0], ",") {
				itemID, itemErr := strconv.Atoi(strings.TrimSpace(rawID))
				if itemErr != nil || itemID <= 0 {
					assets = nil
					break
				}
				assets = append(assets, championAsset{ID: itemID, Kind: "item", Name: strconv.Itoa(itemID), Source: "ddragon"})
			}
			if rankErr != nil || rank <= 0 || !pickOK || !winOK || len(assets) == 0 {
				continue
			}
			result[depth] = append(result[depth], championMetricRow{
				Assets: assets, PickRate: pickRate, WinRate: winRate, GamesUnavailable: true,
			})
		}
	}
	if len(result[4])+len(result[5])+len(result[6]) == 0 {
		return nil, errors.New("QQ101 build response is empty")
	}
	return result, nil
}

func parseQQ101BuildCounts(data []byte) (int, int, int, error) {
	rows, err := parseQQ101BuildRows(data)
	if err != nil {
		return 0, 0, 0, err
	}
	return len(rows[4]), len(rows[5]), len(rows[6]), nil
}

func (p *championProvider) loadQQ101BuildRows(ctx context.Context, championID int, tier, position string) (map[int][]championMetricRow, string, error) {
	patch, err := p.loadQQ101LatestPatch(ctx)
	if err != nil {
		return nil, "", err
	}
	query, err := qq101RiftQuery(patch, tier, position, championID)
	if err != nil {
		return nil, patch, err
	}
	data, err := p.fetchQQ101(ctx, qq101RiftPath+"_build", query)
	if err != nil {
		return nil, patch, err
	}
	rows, err := parseQQ101BuildRows(data)
	if err == nil {
		for _, depth := range []int{4, 5, 6} {
			for rowIndex := range rows[depth] {
				for assetIndex := range rows[depth][rowIndex].Assets {
					asset := &rows[depth][rowIndex].Assets[assetIndex]
					asset.Path = p.ddragonAssetPath(asset.Kind, asset.ID)
				}
			}
		}
	}
	return rows, patch, err
}

func parseQQ101RuneCount(data []byte) (int, error) {
	payload, err := qq101InnerPayload(data, "18119")
	if err != nil {
		return 0, err
	}
	var body struct {
		RuneTop string `json:"rune_top_details"`
		Top     string `json:"top_details"`
		Runes   string `json:"rune_details"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return 0, errors.New("QQ101 rune payload changed")
	}
	count := 0
	for _, value := range []string{body.RuneTop, body.Top, body.Runes} {
		if count = len(splitQQ101Records(value)); count > 0 {
			break
		}
	}
	if count == 0 {
		return 0, errors.New("QQ101 rune response is empty")
	}
	return count, nil
}

func parseQQ101SpellCount(data []byte) (int, error) {
	payload, err := qq101InnerPayload(data, "18029")
	if err != nil {
		return 0, err
	}
	var body struct {
		Details string `json:"data_details"`
	}
	if json.Unmarshal(payload, &body) != nil {
		return 0, errors.New("QQ101 spell payload changed")
	}
	count := len(splitQQ101Records(body.Details))
	if count == 0 {
		return 0, errors.New("QQ101 spell response is empty")
	}
	return count, nil
}

func (p *championProvider) startQQ101Probe(ctx context.Context, champion string, championID int, tier, position string) {
	if p.diag == nil || !p.featureGates.enabled(featureGateQQ101) || championID <= 0 {
		return
	}
	baseKey := fmt.Sprintf("%d|%s|%s", championID, strings.ToLower(strings.TrimSpace(tier)), qq101PositionName(position))
	p.qq101ProbeMu.Lock()
	if p.qq101ProbeRunning == nil {
		p.qq101ProbeRunning = make(map[string]bool)
		p.qq101ProbeAt = make(map[string]time.Time)
	}
	if p.qq101ProbeRunning[baseKey] {
		p.qq101ProbeMu.Unlock()
		return
	}
	p.qq101ProbeRunning[baseKey] = true
	p.qq101ProbeMu.Unlock()
	go func() {
		started := time.Now()
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 36*time.Second)
		defer cancel()
		patch, patchErr := p.loadQQ101LatestPatch(probeCtx)
		key := baseKey + "|" + patch
		p.qq101ProbeMu.Lock()
		if patchErr == nil && time.Since(p.qq101ProbeAt[key]) < qq101ProbeCooldown {
			delete(p.qq101ProbeRunning, baseKey)
			p.qq101ProbeMu.Unlock()
			return
		}
		p.qq101ProbeMu.Unlock()
		event := p.qq101ProbeWithPatch(probeCtx, champion, championID, tier, position, patch, patchErr, started)
		if event != nil {
			p.diag(event)
		}
		p.qq101ProbeMu.Lock()
		delete(p.qq101ProbeRunning, baseKey)
		p.qq101ProbeAt[key] = time.Now()
		p.qq101ProbeMu.Unlock()
	}()
}

func (p *championProvider) qq101Probe(ctx context.Context, champion string, championID int, tier, position string) map[string]any {
	started := time.Now()
	patch, err := p.loadQQ101LatestPatch(ctx)
	return p.qq101ProbeWithPatch(ctx, champion, championID, tier, position, patch, err, started)
}

func (p *championProvider) qq101ProbeWithPatch(ctx context.Context, champion string, championID int, tier, position, patch string, err error, started time.Time) map[string]any {
	if err != nil {
		if isCancellation(err) {
			return nil
		}
		return map[string]any{"event": "qq101_probe", "champion": champion, "tier": tier, "patch": "", "ok": false, "ms": time.Since(started).Milliseconds(), "bytes": 0, "errorKind": championProviderErrorKind(err)}
	}
	query, err := qq101RiftQuery(patch, tier, position, championID)
	if err != nil {
		return map[string]any{"event": "qq101_probe", "champion": champion, "tier": tier, "patch": patch, "ok": false, "ms": time.Since(started).Milliseconds(), "bytes": 0, "errorKind": championProviderErrorKind(err)}
	}
	type request struct {
		name  string
		path  string
		parse func([]byte) (int, error)
	}
	requests := []request{
		{name: "build", path: qq101RiftPath + "_build", parse: func(data []byte) (int, error) {
			fourth, fifth, sixth, err := parseQQ101BuildCounts(data)
			return fourth + fifth + sixth, err
		}},
		{name: "runes", path: qq101RiftPath + "_runeinfo", parse: parseQQ101RuneCount},
		{name: "spells", path: qq101RiftPath + "_skill", parse: parseQQ101SpellCount},
	}
	parts := make([]qq101ProbePart, len(requests))
	var fourthN, fifthN, sixthN int
	var wait sync.WaitGroup
	for index := range requests {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			item := requests[index]
			data, loadErr := p.fetchQQ101(ctx, item.path, query)
			part := qq101ProbePart{Name: item.name, Bytes: len(data)}
			if loadErr == nil {
				part.Count, loadErr = item.parse(data)
			}
			if loadErr != nil {
				part.Outcome = "failed"
				part.ErrorKind = championProviderErrorKind(loadErr)
			} else {
				part.Outcome = "success"
			}
			if item.name == "build" && loadErr == nil {
				fourthN, fifthN, sixthN, _ = parseQQ101BuildCounts(data)
			}
			parts[index] = part
		}(index)
	}
	wait.Wait()
	ok, totalBytes := true, 0
	for _, part := range parts {
		totalBytes += part.Bytes
		ok = ok && part.Outcome == "success"
	}
	return map[string]any{
		"event": "qq101_probe", "champion": champion, "tier": tier, "patch": patch, "ok": ok,
		"forth_n": fourthN, "fifth_n": fifthN, "sixth_n": sixthN,
		"rune_pages_n": parts[1].Count, "spells_n": parts[2].Count,
		"ms": time.Since(started).Milliseconds(), "bytes": totalBytes, "attempts": parts,
	}
}
