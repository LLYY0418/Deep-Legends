package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A Riot API project's encrypted IDs are not interchangeable with OP.GG's.
// Bind the public page's own ID to its explicit KR + Riot-ID profile first.
type opggPlayerPage struct {
	rows  map[string]any
	puuid string
}

func walkOPGGPage(value any, depth int, visit func(map[string]any)) {
	if depth > 80 {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		visit(v)
		for _, child := range v {
			walkOPGGPage(child, depth+1, visit)
		}
	case []any:
		for _, child := range v {
			walkOPGGPage(child, depth+1, visit)
		}
	}
}

func parseOPGGPlayerPage(data []byte, ref gameplayReference) (*opggPlayerPage, error) {
	p := &opggPlayerPage{rows: map[string]any{}}
	for _, line := range strings.Split(decodeNextFlight(data), "\n") {
		id, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var value any
		if json.Unmarshal([]byte(raw), &value) == nil {
			p.rows[id] = value
		}
	}
	ids := map[string]bool{}
	for _, value := range p.rows {
		walkOPGGPage(value, 0, func(node map[string]any) {
			profile, ok := node["data"].(map[string]any)
			if !ok || node["region"] != "kr" {
				return
			}
			name, _ := profile["gameName"].(string)
			tag, _ := profile["tagline"].(string)
			id, _ := profile["puuid"].(string)
			if strings.EqualFold(name, ref.GameName) && strings.EqualFold(tag, ref.TagLine) && name != "" && tag != "" && validPlayerReference(id) {
				ids[id] = true
			}
		})
	}
	if len(ids) != 1 {
		return nil, errors.New("opgg-profile-identity")
	}
	for id := range ids {
		p.puuid = id
	}
	return p, nil
}

func (a *app) readOPGGPlayerPage(ctx context.Context, ref gameplayReference, method, action string, body []byte) ([]byte, error) {
	endpoint := "https://op.gg/zh-cn/lol/summoners/kr/" + url.PathEscape(ref.GameName+"-"+ref.TagLine)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Cache-Control", "no-cache")
	if action != "" {
		req.Header.Set("Next-Action", action)
		req.Header.Set("Accept", "text/x-component")
		req.Header.Set("Content-Type", "text/plain;charset=UTF-8")
	}
	client := *a.champions.httpClient()
	client.Jar = nil
	client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("opgg-profile-redirect") }
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OP.GG profile HTTP %d", resp.StatusCode)
	}
	return readLimited(resp.Body, opggGamesResponseMax)
}

// Resolve only the selected data object, not the whole component graph. Flight
// property paths use `props` for element tuple[3]. Bound cycles and expansion.
func (p *opggPlayerPage) resolve(value any, depth int, budget *int) (any, error) {
	*budget--
	if depth > 80 || *budget < 0 {
		return nil, errors.New("opgg-flight-limit")
	}
	switch v := value.(type) {
	case string:
		if v == "$undefined" {
			return nil, nil
		}
		if strings.HasPrefix(v, "$$") {
			return strings.TrimPrefix(v, "$"), nil
		}
		if !strings.HasPrefix(v, "$") {
			return v, nil
		}
		path := strings.Split(strings.TrimPrefix(v, "$"), ":")
		key := strings.TrimPrefix(path[0], "L")
		x, ok := p.rows[key]
		if !ok {
			return nil, errors.New("opgg-flight-reference")
		}
		for _, field := range path[1:] {
			switch node := x.(type) {
			case map[string]any:
				x, ok = node[field]
			case []any:
				i, err := strconv.Atoi(field)
				if field == "props" && len(node) == 4 && node[0] == "$" {
					i, err = 3, nil
				}
				ok = err == nil && i >= 0 && i < len(node)
				if ok {
					x = node[i]
				}
			default:
				ok = false
			}
			if !ok {
				return nil, errors.New("opgg-flight-reference")
			}
		}
		return p.resolve(x, depth+1, budget)
	case map[string]any:
		out := map[string]any{}
		for k, child := range v {
			r, err := p.resolve(child, depth+1, budget)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			r, err := p.resolve(child, depth+1, budget)
			if err != nil {
				return nil, err
			}
			out[i] = r
		}
		return out, nil
	}
	return value, nil
}

func (p *opggPlayerPage) currentGame(now time.Time) ([]byte, bool, error) {
	var initial map[string]any
	count := 0
	for _, value := range p.rows {
		walkOPGGPage(value, 0, func(node map[string]any) {
			if node["region"] != "kr" || node["puuid"] != p.puuid {
				return
			}
			if result, ok := node["initialResult"].(map[string]any); ok {
				initial = result
				count++
			}
		})
	}
	if count != 1 {
		return nil, false, nil
	}
	stamp, ok := initial["fetchedAt"].(float64)
	age := now.Sub(time.UnixMilli(int64(stamp)))
	// Same ten-second freshness rule as OP.GG's InGame provider. Never resurrect
	// a cached game simply because it started less than six hours ago.
	if !ok || age < -time.Minute || age >= 10*time.Second {
		return nil, false, nil
	}
	data, present := initial["data"]
	if !present {
		return nil, false, nil
	}
	if game, ok := data.(map[string]any); ok {
		selected := map[string]any{}
		for _, key := range []string{"game_id", "created_at", "game_map", "game_type", "summaryTeamsData"} {
			selected[key] = game[key]
		}
		if record, ok := game["record_info"].(map[string]any); ok {
			selected["record_info"] = map[string]any{"is_finished": record["is_finished"]}
		}
		data = selected // Never resolve or return spectate scripts/credentials.
	}
	budget := 50000
	resolved, err := p.resolve(data, 0, &budget)
	if err != nil {
		return nil, true, err
	}
	raw, err := json.Marshal(resolved)
	return append([]byte("0:{\"a\":\"$@1\"}\n1:"), raw...), true, err
}
