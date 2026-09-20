package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	xhtml "golang.org/x/net/html"
)

const (
	proLadderPath        = "/zh-cn/lol/leaderboards/tier"
	proLadderMaxBytes    = 2 << 20
	proLadderConcurrency = 8
	proLadderRequestTTL  = 7 * time.Second
	proLadderMaxRank     = 100_000_000
)

func proLadderAccountKey(gameName, tagLine string) string {
	return strings.ToLower(strings.TrimSpace(gameName) + "#" + strings.TrimSpace(tagLine))
}

// Only query accounts that survived the reviewed first-team identity merge and
// currently have an explicit ranked-solo tier. This avoids touching historical,
// academy, coach, collision or unranked rows from the full upstream directory.
func proRankedLadderAccounts(teams []opggProTeam) []proAccount {
	response := new(app).buildProPlayers(teams, proRoster)
	seen := make(map[string]bool)
	accounts := make([]proAccount, 0, response.AccountCount)
	for _, team := range response.Teams {
		for _, player := range team.Players {
			for _, account := range player.Accounts {
				key := proLadderAccountKey(account.GameName, account.TagLine)
				if account.RankStatus != "ranked" || strings.ContainsAny(account.GameName, "- ") || key == "#" || seen[key] {
					continue
				}
				seen[key] = true
				accounts = append(accounts, account)
			}
		}
	}
	return accounts
}

// OP.GG's pro directory exposes tier and LP but not the server-wide ladder
// position. Its public KR leaderboard can locate one Riot ID and renders the
// authoritative position in the first cell of that account's highlighted row.
func enrichProLadderRanks(ctx context.Context, provider *championProvider, teams []opggProTeam) {
	pipeline := newProLadderPipeline(ctx, provider)
	pipeline.submit(teams)
	pipeline.finish()
	pipeline.apply(teams)
}

// The directory and each completed supplement can discover new accounts. Queue
// only reviewed identities, without waiting for unrelated supplements. Workers
// keep results separate from the immutable snapshots published to readers.
type proLadderPipeline struct {
	mu      sync.Mutex
	ready   *sync.Cond
	workers sync.WaitGroup
	pending []proAccount
	seen    map[string]bool
	known   map[string]int
	closed  bool
}

func newProLadderPipeline(ctx context.Context, provider *championProvider) *proLadderPipeline {
	p := &proLadderPipeline{seen: make(map[string]bool), known: make(map[string]int)}
	p.ready = sync.NewCond(&p.mu)
	p.workers.Add(proLadderConcurrency)
	for i := 0; i < proLadderConcurrency; i++ {
		go func() {
			defer p.workers.Done()
			for {
				p.mu.Lock()
				for len(p.pending) == 0 && !p.closed {
					p.ready.Wait()
				}
				if len(p.pending) == 0 {
					p.mu.Unlock()
					return
				}
				account := p.pending[0]
				p.pending[0] = proAccount{}
				p.pending = p.pending[1:]
				p.mu.Unlock()
				if ctx.Err() != nil {
					continue
				}
				rank, err := fetchOPGGLadderRank(ctx, provider, account.GameName, account.TagLine)
				if err == nil && rank > 0 {
					p.mu.Lock()
					p.known[proLadderAccountKey(account.GameName, account.TagLine)] = rank
					p.mu.Unlock()
				}
			}
		}()
	}
	return p
}

func (p *proLadderPipeline) submit(teams []opggProTeam) {
	accounts := proRankedLadderAccounts(teams)
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, account := range accounts {
		key := proLadderAccountKey(account.GameName, account.TagLine)
		if !p.seen[key] {
			p.seen[key] = true
			p.pending = append(p.pending, account)
		}
	}
	p.ready.Broadcast()
}

func (p *proLadderPipeline) finish() {
	p.mu.Lock()
	p.closed = true
	p.ready.Broadcast()
	p.mu.Unlock()
	p.workers.Wait()
}

// Call only with a private snapshot; never mutate an already-published slice.
func (p *proLadderPipeline) apply(teams []opggProTeam) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for teamIndex := range teams {
		for memberIndex := range teams[teamIndex].Members {
			for accountIndex := range teams[teamIndex].Members[memberIndex].Summoners {
				account := &teams[teamIndex].Members[memberIndex].Summoners[accountIndex]
				if rank := p.known[proLadderAccountKey(account.GameName, account.TagLine)]; rank > 0 {
					account.LadderRank = rank
					account.LadderRankKnown = true
				}
			}
		}
	}
}

func validProLadderURL(target *url.URL) bool {
	if target == nil || target.Scheme != "https" || target.Host != opggPageHost || target.User != nil || target.Path != proLadderPath || target.Fragment != "" {
		return false
	}
	query := target.Query()
	return len(query) == 4 && len(query["type"]) == 1 && query.Get("type") == "ladder" &&
		len(query["region"]) == 1 && query.Get("region") == "kr" &&
		len(query["tier"]) == 1 && query.Get("tier") == "all" &&
		len(query["summoner"]) == 1 && strings.TrimSpace(query.Get("summoner")) != ""
}

func fetchOPGGLadderRank(ctx context.Context, provider *championProvider, gameName, tagLine string) (int, error) {
	if gate := featureGateForChampionHost(opggPageHost); gate != "" && !provider.featureGates.enabled(gate) {
		return 0, errors.New("pro ladder source disabled")
	}
	gameName, tagLine = strings.TrimSpace(gameName), strings.TrimSpace(tagLine)
	if gameName == "" || tagLine == "" || len(gameName) > 128 || len(tagLine) > 32 || strings.ContainsAny(gameName+tagLine, "#") {
		return 0, errors.New("invalid pro ladder account")
	}
	query := url.Values{"type": {"ladder"}, "region": {"kr"}, "summoner": {gameName + "-" + tagLine}, "tier": {"all"}}
	body, _, err := provider.fetchWithMetadataCacheKeyLoader(ctx, opggPageHost, proLadderPath, query, proLadderMaxBytes, "text/html,application/xhtml+xml", "", func(ctx context.Context) ([]byte, error) {
		target := &url.URL{Scheme: "https", Host: opggPageHost, Path: proLadderPath, RawQuery: query.Encode()}
		requestContext, cancel := context.WithTimeout(ctx, proLadderRequestTTL)
		defer cancel()
		request, err := http.NewRequestWithContext(requestContext, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, err
		}
		request.Header.Set("Accept", "text/html,application/xhtml+xml")
		request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.6")
		request.Header.Set("User-Agent", "Deep-Legends/"+version)

		client := *provider.httpClient()
		client.Jar = nil
		client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
			if len(via) >= 3 || !validProLadderURL(request.URL) {
				return errors.New("pro ladder redirect rejected")
			}
			return nil
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("pro ladder HTTP %d", response.StatusCode)
		}
		return readLimited(response.Body, proLadderMaxBytes)
	})
	if err != nil {
		return 0, err
	}
	return parseOPGGLadderRank(body, gameName, tagLine)
}

func parseOPGGLadderRank(body []byte, gameName, tagLine string) (int, error) {
	if len(body) == 0 || len(body) > proLadderMaxBytes {
		return 0, errors.New("invalid pro ladder response size")
	}
	target := strings.TrimSpace(gameName) + "-" + strings.TrimSpace(tagLine)
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(body))
	inTargetRow, inRankCell, sawRankCell := false, false, false
	var text strings.Builder
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			return 0, errors.New("pro ladder account row missing")
		case xhtml.StartTagToken:
			token := tokenizer.Token()
			if token.Data == "tr" && !inTargetRow {
				for _, attribute := range token.Attr {
					if attribute.Key == "id" && strings.EqualFold(strings.TrimSpace(attribute.Val), target) {
						inTargetRow = true
						break
					}
				}
			} else if token.Data == "td" && inTargetRow && !sawRankCell {
				inRankCell, sawRankCell = true, true
			}
		case xhtml.TextToken:
			if inRankCell {
				text.Write(tokenizer.Text())
			}
		case xhtml.EndTagToken:
			token := tokenizer.Token()
			if token.Data == "td" && inRankCell {
				value := strings.ReplaceAll(strings.TrimSpace(text.String()), ",", "")
				rank, err := strconv.Atoi(value)
				if err != nil || rank <= 0 || rank > proLadderMaxRank {
					return 0, errors.New("invalid pro ladder rank")
				}
				return rank, nil
			}
			if token.Data == "tr" && inTargetRow {
				return 0, errors.New("pro ladder rank cell missing")
			}
		}
	}
}
