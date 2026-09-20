package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
)

type proProfileCache struct {
	mu      sync.Mutex
	disk    *championDataCache
	entries map[string]opggProAccount
}

func proProfileTTL(row opggProAccount) time.Duration {
	if row.CheckFailed || row.ActivityFailed {
		return 30 * time.Second
	}
	account, _ := normalizeProAccount(row)
	if account.RankStatus == "unranked" {
		return 72 * time.Hour
	}
	return 15 * time.Minute
}

func (a *app) readProProfile(ctx context.Context, old opggProAccount) opggProAccount {
	key := "pro-profile-v2|" + proLadderAccountKey(old.GameName, old.TagLine)
	// Directory rows may have a rank but no timestamp/PUUID and therefore do
	// not qualify as Riot anchors. Keep that exact account's known rank if the
	// independent public-page read fails.
	if len(old.Rank) == 0 {
		a.proPlayers.mu.Lock()
		for _, team := range a.proPlayers.teams {
			for _, member := range team.Members {
				for _, row := range member.Summoners {
					if proLadderAccountKey(row.GameName, row.TagLine) == proLadderAccountKey(old.GameName, old.TagLine) && len(row.Rank) > 0 {
						old.Rank, old.UpdatedAt = row.Rank, row.UpdatedAt
					}
				}
			}
		}
		a.proPlayers.mu.Unlock()
	}
	c := &a.proProfiles
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]opggProAccount{}
		c.disk = newPublicBinaryCache(a.storage, "pro-profiles", 64, 4<<20)
	}
	cached, found := c.entries[key]
	if !found && c.disk != nil {
		if entry, err := c.disk.readDisk(key); err == nil {
			var stored proProfileSnapshot
			found = json.Unmarshal(entry.Data, &stored) == nil
			cached = stored.Account
			cached.LastMatchAt, cached.LastMatchAtKnown = stored.LastMatchAt, stored.LastMatchAtKnown
		}
	}
	at, _ := time.Parse(time.RFC3339Nano, cached.CheckedAt)
	if found && !at.IsZero() && time.Since(at) >= 0 && time.Since(at) < proProfileTTL(cached) {
		c.entries[key] = cached
		c.mu.Unlock()
		cached.SeedKey, cached.Source, cached.PUUID = old.SeedKey, old.Source, old.PUUID
		if old.LastMatchAtKnown && old.LastMatchAt > cached.LastMatchAt {
			cached.LastMatchAt, cached.LastMatchAtKnown = old.LastMatchAt, true
		}
		return cached
	}
	c.mu.Unlock()
	if found && cached.CheckedAt > old.CheckedAt {
		cached.SeedKey, cached.Source, cached.PUUID = old.SeedKey, old.Source, old.PUUID
		if old.LastMatchAtKnown && old.LastMatchAt > cached.LastMatchAt {
			cached.LastMatchAt, cached.LastMatchAtKnown = old.LastMatchAt, true
		}
		old = cached
	}
	ref := gameplayReference{Region: "kr", GameName: old.GameName, TagLine: old.TagLine}
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	data, err := a.readOPGGPlayerPage(requestCtx, ref, http.MethodGet, "", nil)
	var next opggProAccount
	if err == nil {
		next, err = parseProProfile(data, ref)
	}
	if err == nil {
		old.Rank, old.UpdatedAt = next.Rank, next.UpdatedAt
		if next.LastMatchAtKnown && (!old.LastMatchAtKnown || next.LastMatchAt > old.LastMatchAt) {
			old.LastMatchAt, old.LastMatchAtKnown = next.LastMatchAt, true
		}
		old.ActivityFailed = false
		account, _ := normalizeProAccount(next)
		// Public pages omit older histories for some ranked accounts. Only
		// those missing histories use the shared, foreground-reserving Riot
		// limiter; never issue a second rank lookup or reuse OP.GG's PUUID.
		if !next.LastMatchAtKnown && account.RankStatus == "ranked" && a.riot != nil && riotKeyConfigured() {
			activityCtx, stop := context.WithTimeout(withRiotBackground(ctx), 6*time.Second)
			identity, activityErr := a.riot.accountByRiotID(activityCtx, old.GameName, old.TagLine)
			if activityErr == nil {
				at, known, readErr := a.riot.lastMatchStartCached(activityCtx, identity.PUUID, "proprofile-lastmatch:v2:", 15*time.Minute)
				activityErr = readErr
				if readErr == nil && known {
					setProLastMatch(&old, at, true)
				}
			}
			stop()
			old.ActivityFailed = activityErr != nil
		}
		if next.LadderRankKnown {
			old.LadderRank, old.LadderRankKnown = next.LadderRank, true
		}
		old.Stale = false
	} else {
		old.Stale = len(old.Rank) > 0
	}
	old.CheckedAt, old.CheckFailed = time.Now().UTC().Format(time.RFC3339Nano), err != nil
	c.mu.Lock()
	c.entries[key] = old
	// Only normalized public account facts are persisted, never full HTML or
	// account identifiers from the page, cookies, auth tokens or request headers.
	if c.disk != nil && err == nil {
		safe := old
		safe.PUUID = ""
		body, _ := json.Marshal(proProfileSnapshot{Account: safe, LastMatchAt: safe.LastMatchAt, LastMatchAtKnown: safe.LastMatchAtKnown})
		hash := sha256.Sum256(body)
		now := time.Now()
		_ = c.disk.writeDisk(championCacheEnvelope{Schema: championCacheSchema, Key: key, FetchedAt: now, ExpiresAt: now.Add(proProfileTTL(old)), StaleUntil: now.Add(7 * 24 * time.Hour), Hash: hex.EncodeToString(hash[:]), Data: body})
	}
	c.mu.Unlock()
	return old
}

func (a *app) enrichProProfiles(ctx context.Context, seeds []opggProTeam) []opggProTeam {
	ctx, cost := newProRefreshCost(ctx)
	result := cloneProTeams(seeds)
	jobs := make(chan *opggProAccount)
	var workers sync.WaitGroup
	started := time.Now()
	for i := 0; i < 6; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for row := range jobs {
				*row = a.readProProfile(ctx, *row)
			}
		}()
	}
	for ti := range result {
		for mi := range result[ti].Members {
			for ai := range result[ti].Members[mi].Summoners {
				jobs <- &result[ti].Members[mi].Summoners[ai]
			}
		}
	}
	close(jobs)
	workers.Wait()
	total, failed := 0, 0
	for _, team := range result {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				total++
				if row.CheckFailed {
					failed++
				}
			}
		}
	}
	a.recordDiagnostic(map[string]any{"event": "pro_profile_batch", "accounts_total": total, "accounts_checked": total, "failed": failed, "duration_ms": time.Since(started).Milliseconds(), "riot_requests": cost.requests.Load(), "skipped_by_budget": cost.skipped.Load()})
	return result
}

// The KR page must prove its exact Riot ID before any rank is accepted. Current
// solo rank is read only from its explicit section, never season tables, peak
// ranks, other accounts or metadata snippets.
func parseProProfile(data []byte, ref gameplayReference) (opggProAccount, error) {
	row := opggProAccount{GameName: ref.GameName, TagLine: ref.TagLine, Region: "kr"}
	page, err := parseOPGGPlayerPage(data, ref)
	if err != nil {
		return row, err
	}
	for _, value := range page.rows {
		walkOPGGPage(value, 0, func(node map[string]any) {
			if node["puuid"] != page.puuid {
				return
			}
			if raw, ok := node["initUpdatedAt"].(string); ok {
				if at, err := time.Parse(time.RFC3339Nano, raw); err == nil {
					row.UpdatedAt = at.UTC().Format(time.RFC3339)
				}
			}
		})
	}
	doc, err := xhtml.Parse(bytes.NewReader(data))
	if err != nil {
		return row, err
	}
	lastMatch, known := proProfileLastMatch(doc, ref)
	setProLastMatch(&row, lastMatch, known)
	var sections []*xhtml.Node
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "span" && proNodeText(n) == "单排/双排" {
			for p := n.Parent; p != nil; p = p.Parent {
				if p.Data == "a" {
					break
				}
				if p.Data == "section" {
					sections = append(sections, p)
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	if len(sections) != 1 {
		return row, errors.New("pro-profile-solo-section")
	}
	var current *xhtml.Node
	unranked := false
	var scan func(*xhtml.Node)
	scan = func(n *xhtml.Node) {
		if n.Data == "table" {
			return
		}
		if n.Type == xhtml.ElementNode && n.Data == "strong" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(" "+attr.Val+" ", " text-xl ") {
					current = n
				}
			}
		}
		if n.Type == xhtml.ElementNode && (n.Data == "span" || n.Data == "strong") && proNodeText(n) == "未定级" {
			unranked = true
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			scan(c)
		}
	}
	scan(sections[0])
	if current == nil {
		if unranked {
			row.Rank = json.RawMessage(`{"tier":"UNRANKED"}`)
			return row, nil
		}
		return row, errors.New("pro-profile-current-rank")
	}
	labels := map[string]string{"黑铁": "IRON", "青铜": "BRONZE", "白银": "SILVER", "黄金": "GOLD", "铂金": "PLATINUM", "翡翠": "EMERALD", "钻石": "DIAMOND", "大师": "MASTER", "宗师": "GRANDMASTER", "王者": "CHALLENGER"}
	fields := strings.Fields(proNodeText(current))
	if len(fields) == 0 {
		return row, errors.New("pro-profile-tier")
	}
	tier := labels[fields[0]]
	if tier == "" {
		return row, errors.New("pro-profile-tier")
	}
	division := 1
	if proTierOrder[tier] < proTierOrder["MASTER"] {
		if len(fields) != 2 {
			return row, errors.New("pro-profile-division")
		}
		division, err = strconv.Atoi(fields[1])
		if err != nil || division < 1 || division > 4 {
			return row, errors.New("pro-profile-division")
		}
	}
	lp := -1
	for sibling := current.NextSibling; sibling != nil; sibling = sibling.NextSibling {
		value := proNodeText(sibling)
		if strings.HasSuffix(value, " LP") {
			lp, err = strconv.Atoi(strings.ReplaceAll(strings.TrimSuffix(value, " LP"), ",", ""))
			break
		}
	}
	if err != nil || lp < 0 {
		return row, errors.New("pro-profile-lp")
	}
	row.Rank, _ = json.Marshal(map[string]any{"tier": tier, "division": division, "lp": lp})
	return row, nil
}

func proNodeText(n *xhtml.Node) string {
	var b strings.Builder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}
