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

type proLadderResult struct {
	key  string
	rank int
	err  error
}

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
	accounts := proRankedLadderAccounts(teams)
	if len(accounts) == 0 || ctx.Err() != nil {
		return
	}

	jobs := make(chan proAccount)
	results := make(chan proLadderResult, len(accounts))
	workerCount := proLadderConcurrency
	if len(accounts) < workerCount {
		workerCount = len(accounts)
	}
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for i := 0; i < workerCount; i++ {
		go func() {
			defer workers.Done()
			for account := range jobs {
				rank, err := fetchOPGGLadderRank(ctx, provider, account.GameName, account.TagLine)
				result := proLadderResult{key: proLadderAccountKey(account.GameName, account.TagLine), rank: rank, err: err}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, account := range accounts {
			select {
			case jobs <- account:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()

	known := make(map[string]int, len(accounts))
	for result := range results {
		if result.err == nil && result.rank > 0 {
			known[result.key] = result.rank
		}
	}
	for teamIndex := range teams {
		for memberIndex := range teams[teamIndex].Members {
			for accountIndex := range teams[teamIndex].Members[memberIndex].Summoners {
				account := &teams[teamIndex].Members[memberIndex].Summoners[accountIndex]
				if rank := known[proLadderAccountKey(account.GameName, account.TagLine)]; rank > 0 {
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
	target := &url.URL{Scheme: "https", Host: opggPageHost, Path: proLadderPath, RawQuery: query.Encode()}
	requestContext, cancel := context.WithTimeout(ctx, proLadderRequestTTL)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("pro ladder HTTP %d", response.StatusCode)
	}
	body, err := readLimited(response.Body, proLadderMaxBytes)
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
