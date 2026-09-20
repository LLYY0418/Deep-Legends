package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const proSupplementHost = "www.trackingthepros.com"

// Reviewed identity-page slugs, not guessed account names. The source's team
// labels may be historical; require nickname + real name against our roster.
// Accounts (including hidden inactive rows) are read fresh, never hardcoded.
var proSupplementPlayers = []string{"TheShy", "Assum", "JiaQi"}

func proSupplementPlayer(name string) (proRosterTeam, proRosterPlayer, bool) {
	for _, allowed := range proSupplementPlayers {
		if name != allowed {
			continue
		}
		for _, team := range proRoster {
			for _, player := range team.Players {
				if player.Name == name {
					return team, player, true
				}
			}
		}
	}
	return proRosterTeam{}, proRosterPlayer{}, false
}

// This is deliberately separate from the generic asset allowlist/cache: only
// these reviewed public identity pages can be fetched. No cookies, Riot keys,
// source IDs or local client data are sent, and redirects stay on this origin.
func fetchProSupplement(ctx context.Context, provider *championProvider, name string) ([]byte, error) {
	if _, _, ok := proSupplementPlayer(name); !ok {
		return nil, errors.New("unreviewed pro identity page")
	}
	client := *provider.httpClient()
	client.Jar = nil
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 3 || r.URL.Scheme != "https" || r.URL.Host != proSupplementHost || r.URL.User != nil || r.URL.RawQuery != "" || strings.TrimSuffix(r.URL.Path, "/") != "/player/"+name {
			return errors.New("pro identity redirect rejected")
		}
		return nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+proSupplementHost+"/player/"+name, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pro identity HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, 1<<20)
}

func pendingProSupplements() []opggProTeam {
	teams := make([]opggProTeam, len(proSupplementPlayers))
	for i, name := range proSupplementPlayers {
		team, player, _ := proSupplementPlayer(name)
		member := opggProMember{TeamID: team.OPGGID, Nickname: player.Name, RealName: player.Names[0], Authority: "PROGAMER", Incomplete: true, Supplement: true}
		teams[i] = opggProTeam{ID: team.OPGGID, Name: team.Name, ShortName: team.Code, Members: []opggProMember{member}}
	}
	return teams
}

// Publish each verified identity as soon as it finishes. One slow supplemental
// host response must not hide accounts that have already passed verification.
func loadProSupplements(ctx context.Context, provider *championProvider, publish ...func([]opggProTeam)) []opggProTeam {
	teams := pendingProSupplements()
	type result struct {
		index  int
		member opggProMember
	}
	results := make(chan result, len(proSupplementPlayers))
	var wg sync.WaitGroup
	for i, name := range proSupplementPlayers {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			team, player, _ := proSupplementPlayer(name)
			member := opggProMember{TeamID: team.OPGGID, Nickname: player.Name, RealName: player.Names[0], Authority: "PROGAMER", Incomplete: true, Supplement: true}
			for attempt := 1; attempt <= 2; attempt++ {
				started := time.Now()
				requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
				body, err := fetchProSupplement(requestCtx, provider, name)
				cancel()
				stage := "request"
				if err == nil {
					stage = "identity"
					var parsed opggProMember
					parsed, err = parseProSupplement(body, player)
					if err == nil {
						member = parsed
						member.TeamID = team.OPGGID
						member.Supplement = true
						member.FetchedAt = time.Now()
					}
				}
				failure, retry := proSupplementFailure(err)
				if provider.diag != nil {
					provider.diag(map[string]any{"event": "pro_supplement_result", "roster_slot": i, "attempt": attempt, "stage": stage, "duration_ms": time.Since(started).Milliseconds(), "failure": failure, "account_count": len(member.Summoners), "partial": member.Incomplete})
				}
				if err == nil || stage == "identity" || !retry || attempt == 2 || ctx.Err() != nil {
					break
				}
				timer := time.NewTimer(300 * time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
				case <-timer.C:
				}
			}
			results <- result{i, member}
		}(i, name)
	}
	go func() { wg.Wait(); close(results) }()
	for row := range results {
		teams[row.index].Members = []opggProMember{row.member}
		for _, fn := range publish {
			if fn != nil {
				fn(cloneProTeams(teams))
			}
		}
	}
	return teams
}

func proSupplementFailure(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	if errors.Is(err, context.Canceled) {
		return "canceled", false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout", true
	}
	var status int
	if _, scanErr := fmt.Sscanf(err.Error(), "pro identity HTTP %d", &status); scanErr == nil {
		return fmt.Sprintf("http-%d", status), status == 429 || status >= 500
	}
	var networkError *url.Error
	if errors.As(err, &networkError) {
		return "network", true
	}
	return "validation", false
}

// A transient supplemental failure must not silently erase verified accounts.
// Reuse only that identity's last successful snapshot for at most 24 hours,
// without advancing its timestamp. Every reused row is explicitly stale.
func retainProSupplements(current, previous []opggProTeam, now time.Time) []opggProTeam {
	for ti := range current {
		for mi := range current[ti].Members {
			member := &current[ti].Members[mi]
			if !member.Supplement || !member.Incomplete || len(member.Summoners) != 0 {
				continue
			}
			for _, team := range previous {
				for _, old := range team.Members {
					if !old.Supplement || old.TeamID != member.TeamID || old.Nickname != member.Nickname || !proIdentityMatches(old, proRosterPlayer{Name: member.Nickname, Names: []string{member.RealName}}) || old.FetchedAt.IsZero() || now.Sub(old.FetchedAt) > proPlayersMaxStale {
						continue
					}
					member.FetchedAt = old.FetchedAt
					member.Summoners = append([]opggProAccount(nil), old.Summoners...)
					for i := range member.Summoners {
						member.Summoners[i].Stale = true
					}
				}
			}
		}
	}
	return current
}

func proSupplementsIncomplete(teams []opggProTeam) bool {
	for _, team := range teams {
		for _, member := range team.Members {
			if member.Supplement && member.Incomplete {
				return true
			}
		}
	}
	return false
}

var proSupplementRankPattern = regexp.MustCompile(`(?i)^(Challenger|Grandmaster|Master|GM|Diamond|Emerald|Platinum|Gold|Silver|Bronze|Iron)(?:\s+(IV|III|II|I))?(?:\s+([0-9][0-9,]*)\s*LP)?$`)

func proSupplementRank(text string) json.RawMessage {
	text = strings.TrimSpace(text)
	if strings.EqualFold(text, "Unranked") {
		return json.RawMessage(`{"tier":"UNRANKED"}`)
	}
	match := proSupplementRankPattern.FindStringSubmatch(text)
	if match == nil {
		return nil
	}
	tier := strings.ToUpper(match[1])
	if tier == "GM" {
		tier = "GRANDMASTER"
	}
	division := map[string]int{"I": 1, "II": 2, "III": 3, "IV": 4}[strings.ToUpper(match[2])]
	if proTierOrder[tier] >= proTierOrder["MASTER"] {
		division = 1
	}
	rank := map[string]any{"tier": tier, "division": division}
	if match[3] != "" {
		if lp, err := strconv.Atoi(strings.ReplaceAll(match[3], ",", "")); err == nil {
			rank["lp"] = lp
		}
	}
	data, _ := json.Marshal(rank)
	return data
}

func parseProSupplement(body []byte, player proRosterPlayer) (opggProMember, error) {
	member := opggProMember{Nickname: player.Name, Authority: "PROGAMER"}
	if len(body) == 0 || len(body) > 1<<20 {
		return member, errors.New("invalid pro identity size")
	}
	root, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return member, err
	}
	var text func(*html.Node) string
	text = func(n *html.Node) string {
		if n.Type == html.TextNode {
			return n.Data
		}
		var b strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			b.WriteString(text(c))
		}
		return b.String()
	}
	var walk func(*html.Node, int) error
	headerMatch, accountsFound, rows := false, false, 0
	walk = func(n *html.Node, depth int) error {
		if depth > 80 {
			return errors.New("pro identity nesting limit")
		}
		if n.Type == html.ElementNode && n.Data == "h1" {
			// Club name is inside <a><small>; nickname is the heading's direct text.
			var nickname strings.Builder
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					nickname.WriteString(c.Data)
				}
			}
			headerMatch = strings.EqualFold(strings.TrimSpace(nickname.String()), player.Name)
		}
		if n.Type == html.ElementNode && n.Data == "tr" {
			cells := []*html.Node{}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "td" {
					cells = append(cells, c)
				}
			}
			if len(cells) == 2 && strings.TrimSpace(text(cells[0])) == "Name" {
				member.RealName = strings.TrimSpace(text(cells[1]))
			}
		}
		if n.Type == html.ElementNode && n.Data == "div" {
			isAccounts := false
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "h4" && strings.TrimSpace(text(c)) == "Accounts" {
					isAccounts = true
				}
			}
			if isAccounts {
				if accountsFound {
					return errors.New("duplicate pro accounts section")
				}
				accountsFound = true
				var accountRows func(*html.Node, int) error
				accountRows = func(row *html.Node, d int) error {
					if d > 30 {
						return errors.New("pro account nesting limit")
					}
					if row.Type == html.ElementNode && row.Data == "tr" {
						rows++
						if rows > 200 {
							return errors.New("pro account row limit")
						}
						cells := []*html.Node{}
						for c := row.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.ElementNode && c.Data == "td" {
								cells = append(cells, c)
							}
						}
						if len(cells) != 2 {
							return nil
						} // e.g. the "Show Inactive" toggle
						identity := strings.TrimSpace(text(cells[0]))
						if !strings.HasPrefix(identity, "[KR]") {
							return nil
						}
						name, tag, ok := strings.Cut(strings.TrimSpace(strings.TrimPrefix(identity, "[KR]")), "#")
						if !ok {
							member.Incomplete = true
							return nil
						}
						raw := opggProAccount{GameName: name, TagLine: tag, Region: "kr", Rank: proSupplementRank(text(cells[1])), Source: "TrackingThePros"}
						for _, attr := range row.Attr {
							if attr.Key == "class" && strings.Contains(" "+attr.Val+" ", " inactive_account ") {
								raw.Inactive = true
							}
						}
						if _, valid := normalizeProAccount(raw); !valid {
							member.Incomplete = true
							return nil
						}
						member.Summoners = append(member.Summoners, raw)
						return nil
					}
					for c := row.FirstChild; c != nil; c = c.NextSibling {
						if err := accountRows(c, d+1); err != nil {
							return err
						}
					}
					return nil
				}
				if err := accountRows(n, 0); err != nil {
					return err
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if err := walk(c, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return member, err
	}
	if !headerMatch || !accountsFound || !proIdentityMatches(member, player) {
		return member, errors.New("unverified pro identity page")
	}
	return member, nil
}
