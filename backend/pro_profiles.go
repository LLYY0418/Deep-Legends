package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
)

type proProfileCache struct {
	mu         sync.Mutex
	disk       *championDataCache
	entries    map[string]opggProAccount
	now        func() time.Time
	inflight   map[string]bool
	manual     map[string]time.Time
	slots      chan struct{}
	identities map[string]string
}

func proProfileTTL(row opggProAccount) time.Duration {
	if row.CheckFailed || row.ActivityFailed || row.ActivityPending {
		return proProfileRetry(row.Failures)
	}
	return proProfileRefreshInterval
}

func (a *app) readProProfile(ctx context.Context, old opggProAccount) opggProAccount {
	return a.readScopedProProfile(ctx, old)
}

func (a *app) enrichProProfiles(ctx context.Context, seeds []opggProTeam) []opggProTeam {
	ctx, cost := newProRefreshCost(ctx)
	result := cloneProTeams(seeds)
	jobs := make(chan *opggProAccount)
	var workers sync.WaitGroup
	started := time.Now()
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer a.recoverPanic("pro_profiles.enrichProProfiles.1")

			defer workers.Done()
			for row := range jobs {
				func() { defer a.recoverPanic("enrichProProfiles.job"); *row = a.readProProfile(ctx, *row) }()
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

func expireProProfileFacts(row opggProAccount, now time.Time) opggProAccount {
	at, _ := time.Parse(time.RFC3339Nano, row.LastSuccessAt)
	if at.IsZero() && !row.CheckFailed {
		at, _ = time.Parse(time.RFC3339Nano, row.CheckedAt)
	}
	if !at.IsZero() && !now.Before(at.Add(7*24*time.Hour)) {
		row.Rank, row.LastMatchAt, row.LastMatchID = nil, "", ""
		row.LastMatchAtKnown, row.LadderRankKnown = false, false
		row.LadderRank, row.LastSuccessAt = 0, ""
	}
	return row
}

func proProfileRetainedUntil(row opggProAccount, now time.Time) time.Time {
	at, _ := time.Parse(time.RFC3339Nano, row.LastSuccessAt)
	if at.IsZero() {
		at, _ = time.Parse(time.RFC3339Nano, row.CheckedAt)
	}
	if at.IsZero() {
		at = now
	}
	return at.Add(7 * 24 * time.Hour)
}

func pruneProProfilesLocked(c *proProfileCache, now time.Time) {
	for key, row := range c.entries {
		if !now.Before(proProfileRetainedUntil(row, now)) {
			delete(c.entries, key)
		}
	}
	for len(c.entries) > 128 {
		var oldest string
		var at string
		for key, row := range c.entries {
			if oldest == "" || row.CheckedAt < at {
				oldest = key
				at = row.CheckedAt
			}
		}
		delete(c.entries, oldest)
	}
}
