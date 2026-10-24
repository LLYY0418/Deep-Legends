package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const aramkitDataCN = "data.aramkit.cn"
const aramkitDataCOM = "data.aramkit.com"

type aramkitVersion struct {
	Version      string `json:"version"`
	DataPath     string `json:"dataPath"`
	ResourcePath string `json:"resourcePath"`
	DataDate     string `json:"dataDate"`
	AllMatches   int    `json:"allMatches"`
}
type aramkitVersions struct {
	Latest   string           `json:"latest"`
	Versions []aramkitVersion `json:"versions"`
}
type aramkitVersionCache struct {
	Payload        aramkitVersions
	ETag, Modified string
}
type aramkitClient struct {
	provider *championProvider
	mu       sync.Mutex
	slots    chan struct{}
	flights  map[string]chan struct{}
	memory   map[string][]byte
	retry    map[string]time.Time
	dir      string
	versions aramkitVersionCache
	checked  time.Time
}

func (p *championProvider) aramkitClient() *aramkitClient {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.aramkit == nil {
		dir := ""
		if p.cache != nil {
			dir = filepath.Join(filepath.Dir(p.cache.dir), "aramkit")
		}
		p.aramkit = &aramkitClient{provider: p, slots: make(chan struct{}, 2), flights: map[string]chan struct{}{}, memory: map[string][]byte{}, retry: map[string]time.Time{}, dir: dir}
		if dir != "" {
			if b, e := os.ReadFile(filepath.Join(dir, "versions-cache.json")); e == nil {
				_ = json.Unmarshal(b, &p.aramkit.versions)
			}
		}
	}
	return p.aramkit
}
func (c *aramkitClient) diagnostic(event string, fields map[string]any) {
	if c.provider.diag != nil {
		fields["event"] = event
		c.provider.diag(fields)
	}
}
func (c *aramkitClient) persist(name string, b []byte) {
	if c.dir == "" {
		return
	}
	target := filepath.Join(c.dir, filepath.FromSlash(name))
	if os.MkdirAll(filepath.Dir(target), 0700) != nil {
		return
	}
	f, e := os.CreateTemp(filepath.Dir(target), ".write-*")
	if e != nil {
		return
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return
	}
	if f.Close() == nil {
		_ = os.Rename(tmp, target)
	}
}
func (c *aramkitClient) request(ctx context.Context, requestPath string, conditional *aramkitVersionCache) ([]byte, http.Header, int, error) {
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return nil, nil, 0, ctx.Err()
	}
	var last error
	for _, host := range []string{aramkitDataCN, aramkitDataCOM} {
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+host+"/"+requestPath, nil)
		if e != nil {
			return nil, nil, 0, e
		}
		req.Header.Set("User-Agent", "Deep-Legends/"+version)
		if strings.HasPrefix(requestPath, "assets/") {
			req.Header.Set("Accept", "image/webp,image/*")
		} else {
			req.Header.Set("Accept", "application/json")
		}
		if conditional != nil {
			req.Header.Set("If-None-Match", conditional.ETag)
			req.Header.Set("If-Modified-Since", conditional.Modified)
		}
		c.provider.clientMu.RLock()
		client := c.provider.client
		c.provider.clientMu.RUnlock()
		resp, e := client.Do(req)
		if e != nil {
			last = e
			continue
		}
		if resp.StatusCode == 304 {
			resp.Body.Close()
			return nil, resp.Header, 304, nil
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			last = fmt.Errorf("ARAMKit HTTP %d", resp.StatusCode)
			continue
		}
		b, e := readLimited(resp.Body, 8<<20)
		resp.Body.Close()
		if e != nil {
			last = e
			continue
		}
		if !json.Valid(b) && !strings.HasPrefix(requestPath, "assets/") {
			last = errors.New("ARAMKit invalid JSON")
			continue
		}
		return b, resp.Header, 200, nil
	}
	c.diagnostic("aramkit_read_failed", map[string]any{"path": requestPath, "count": 1})
	return nil, nil, 0, last
}
func safeARAMKitPath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && !strings.ContainsAny(p, "\\?#") && path.Clean(p) == p && !strings.Contains(p, "..")
}
func (c *aramkitClient) immutable(ctx context.Context, p string) ([]byte, error) {
	if !safeARAMKitPath(p) {
		return nil, errors.New("invalid ARAMKit path")
	}
	for {
		c.mu.Lock()
		if b, ok := c.memory[p]; ok {
			c.mu.Unlock()
			return b, nil
		}
		if c.dir != "" {
			if b, e := os.ReadFile(filepath.Join(c.dir, filepath.FromSlash(p))); e == nil && len(b) > 0 {
				c.memory[p] = b
				c.mu.Unlock()
				return b, nil
			}
		}
		if done := c.flights[p]; done != nil {
			c.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if time.Now().Before(c.retry[p]) {
			c.mu.Unlock()
			return nil, errors.New("ARAMKit retry backoff")
		}
		done := make(chan struct{})
		c.flights[p] = done
		c.mu.Unlock()
		b, _, _, e := c.request(ctx, p, nil)
		if e == nil {
			c.persist(p, b)
		}
		c.mu.Lock()
		if e == nil {
			c.memory[p] = b
		} else {
			c.retry[p] = time.Now().Add(2 * time.Minute)
		}
		delete(c.flights, p)
		close(done)
		c.mu.Unlock()
		return b, e
	}
}
func (c *aramkitClient) current(ctx context.Context) (aramkitVersion, error) {
	// Versions are mutable; serialize refreshes independently of immutable file reads.
	for {
		c.mu.Lock()
		if time.Since(c.checked) < 3*time.Hour && c.versions.Payload.Latest != "" {
			v := c.versions.Payload
			c.mu.Unlock()
			return selectARAMKitVersion(v)
		}
		if done := c.flights["versions"]; done != nil {
			c.mu.Unlock()
			select {
			case <-done:
				continue
			case <-ctx.Done():
				return aramkitVersion{}, ctx.Err()
			}
		}
		if time.Now().Before(c.retry["versions"]) {
			v := c.versions.Payload
			c.mu.Unlock()
			return selectARAMKitVersion(v)
		}
		done := make(chan struct{})
		c.flights["versions"] = done
		old := c.versions
		c.mu.Unlock()
		b, h, status, e := c.request(ctx, "data/versions.json", &old)
		next := old
		if e == nil && status == 200 {
			next.Payload = aramkitVersions{}
			e = json.Unmarshal(b, &next.Payload)
			if e == nil {
				_, e = selectARAMKitVersion(next.Payload)
			}
			next.ETag = h.Get("ETag")
			next.Modified = h.Get("Last-Modified")
		}
		if e == nil {
			if saved, err := json.Marshal(next); err == nil {
				c.persist("versions-cache.json", saved)
			}
		}
		c.mu.Lock()
		if e == nil {
			c.versions = next
			c.checked = time.Now()
		} else {
			c.retry["versions"] = time.Now().Add(2 * time.Minute)
		}
		v := c.versions.Payload
		delete(c.flights, "versions")
		close(done)
		c.mu.Unlock()
		if selected, err := selectARAMKitVersion(v); err == nil {
			return selected, nil
		}
		if e == nil {
			e = errors.New("ARAMKit versions unavailable")
		}
		return aramkitVersion{}, e
	}
}
func selectARAMKitVersion(v aramkitVersions) (aramkitVersion, error) {
	for _, r := range v.Versions {
		if r.Version == v.Latest && aramkitPatch(r.Version) != "" && strings.HasPrefix(r.DataPath, "data/") && strings.HasPrefix(r.ResourcePath, "resources/") && safeARAMKitPath(r.DataPath) && safeARAMKitPath(r.ResourcePath) {
			return r, nil
		}
	}
	return aramkitVersion{}, errors.New("ARAMKit versions changed")
}
func aramkitPatch(v string) string {
	parts := strings.Split(v, ".")
	if len(parts) != 2 {
		return ""
	}
	major, e := strconv.Atoi(parts[0])
	minor, e2 := strconv.Atoi(parts[1])
	if e != nil || e2 != nil || major < 10 || major > 19 || minor < 1 || minor > 30 {
		return ""
	}
	return fmt.Sprintf("%d.%d", major+10, minor)
}

type aramkitStat struct {
	ID              int     `json:"id"`
	ChampionID      int     `json:"championId"`
	Rank            int     `json:"rank"`
	Tier            string  `json:"tier"`
	SampleCount     int     `json:"sampleCount"`
	WinRate         float64 `json:"winRate"`
	PickRate        float64 `json:"pickRate"`
	BlueWinRate     float64 `json:"blueWinRate"`
	RedWinRate      float64 `json:"redWinRate"`
	ChampionWinRate float64 `json:"championWinRate"`
	AvailableStages []int   `json:"availableStages"`
	StageAgnostic   bool    `json:"stageAgnostic"`
}
type aramkitTable struct {
	All    []aramkitStat            `json:"all"`
	Stages map[string][]aramkitStat `json:"stages"`
}
type aramkitTooltip struct {
	Blocks []struct {
		Spans []struct{ Type, Text, Label string } `json:"spans"`
	} `json:"blocks"`
}

func (t aramkitTooltip) text() string {
	var lines []string
	for _, b := range t.Blocks {
		var s strings.Builder
		for _, v := range b.Spans {
			if v.Type == "text" {
				s.WriteString(v.Text)
			} else if v.Type == "icon" {
				s.WriteString(v.Label)
			}
		}
		if s.Len() > 0 {
			lines = append(lines, s.String())
		}
	}
	return strings.Join(lines, "\n")
}

type aramkitModifier struct {
	Key   string  `json:"key"`
	Unit  string  `json:"unit"`
	Value float64 `json:"value"`
}
type aramkitAbilityChange struct {
	Ability     string   `json:"ability"`
	Key         string   `json:"key"`
	Description string   `json:"description"`
	Lines       []string `json:"lines"`
}
type aramkitBalance struct {
	Modifiers      []aramkitModifier      `json:"modifiers,omitempty"`
	AbilityChanges []aramkitAbilityChange `json:"abilityChanges,omitempty"`
}
type aramkitResource struct {
	Name       string         `json:"name"`
	Title      string         `json:"title"`
	RiotID     string         `json:"riotId"`
	Icon       string         `json:"icon"`
	Slug       string         `json:"slug"`
	Rarity     string         `json:"rarity"`
	Tooltip    aramkitTooltip `json:"tooltip"`
	PriceTotal int            `json:"priceTotal"`
	IsFinal    bool           `json:"isFinal"`
	Balance    aramkitBalance `json:"balance"`
}
type aramkitResources struct {
	Champions, Augments, Items, Spells map[string]aramkitResource
	Archetypes                         map[string]aramkitResource
	Version                            aramkitVersion
	Metadata                           map[int]championMetadata
}

func verifyARAMKitChampions(rows map[string]aramkitResource, meta map[int]championMetadata) (map[string]aramkitResource, []int, []int) {
	valid := map[string]aramkitResource{}
	var cases, mismatch []int
	for key, r := range rows {
		id, e := strconv.Atoi(key)
		m, ok := meta[id]
		if e != nil || !ok || m.Key == "" || !strings.EqualFold(m.Key, r.RiotID) {
			mismatch = append(mismatch, id)
			continue
		}
		if m.Key != r.RiotID {
			cases = append(cases, id)
		}
		valid[key] = r
	}
	sort.Ints(cases)
	sort.Ints(mismatch)
	return valid, cases, mismatch
}
func (p *championProvider) loadARAMKitResources(ctx context.Context, v aramkitVersion) (aramkitResources, error) {
	c := p.aramkitClient()
	r := aramkitResources{Version: v}
	for _, target := range []struct {
		name string
		out  any
	}{{"champions", &r.Champions}, {"augments", &r.Augments}, {"items", &r.Items}, {"summoner-spells", &r.Spells}, {"archetypes", &r.Archetypes}} {
		b, e := c.immutable(ctx, v.ResourcePath+"/zh-CN/resources/"+target.name+".json")
		if e != nil {
			return r, e
		}
		if e = json.Unmarshal(b, target.out); e != nil {
			return r, e
		}
	}
	p.mu.Lock()
	meta := make(map[int]championMetadata, len(p.championMeta))
	for id, m := range p.championMeta {
		meta[id] = m
	}
	p.mu.Unlock()
	if len(meta) == 0 {
		if _, e := p.loadCatalog(ctx); e != nil {
			return r, e
		}
		p.mu.Lock()
		for id, m := range p.championMeta {
			meta[id] = m
		}
		p.mu.Unlock()
	}
	r.Metadata = meta
	var cases, mismatch []int
	r.Champions, cases, mismatch = verifyARAMKitChampions(r.Champions, meta)
	if len(cases) > 0 {
		c.diagnostic("aramkit_champion_key_case_diff", map[string]any{"count": len(cases), "ids": cases})
	}
	if len(mismatch) > 0 {
		c.diagnostic("aramkit_champion_id_mismatch", map[string]any{"count": len(mismatch), "ids": mismatch})
	}
	return r, nil
}
func (r aramkitResources) asset(kind string, id int) (championAsset, bool) {
	rows := r.Items
	switch kind {
	case "augment":
		rows = r.Augments
	case "spell":
		rows = r.Spells
	}
	m, ok := rows[strconv.Itoa(id)]
	if !ok || m.Name == "" || !strings.HasPrefix(m.Icon, "assets/") || !safeARAMKitPath(m.Icon) {
		return championAsset{}, false
	}
	return championAsset{ID: id, Kind: kind, Rarity: m.Rarity, Name: m.Name, Description: m.Tooltip.text(), Source: "aramkit", Path: "/" + m.Icon}, true
}
func validARAMKitRates(s aramkitStat) bool {
	return s.SampleCount > 0 && s.WinRate >= 0 && s.WinRate <= 1 && s.PickRate >= 0 && s.PickRate <= 1
}
func validARAMKitStat(s aramkitStat) bool { return s.ID > 0 && validARAMKitRates(s) }
func (r aramkitResources) metric(s aramkitStat, kind string) (championMetricRow, bool) {
	a, ok := r.asset(kind, s.ID)
	if !ok || !validARAMKitStat(s) {
		return championMetricRow{}, false
	}
	return championMetricRow{Assets: []championAsset{a}, Rarity: a.Rarity, Grade: s.Tier, WinRate: s.WinRate * 100, PickRate: s.PickRate * 100, Games: s.SampleCount}, true
}
func (r aramkitResources) metrics(rows []aramkitStat, kind string) []championMetricRow {
	var out []championMetricRow
	for _, s := range rows {
		if row, ok := r.metric(s, kind); ok {
			out = append(out, row)
		}
	}
	return out
}
