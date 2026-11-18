package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	xhtml "golang.org/x/net/html"
)

const (
	ladderFreshFor       = 10 * time.Minute
	ladderMaxAge         = 24 * time.Hour
	ladderRetryAfter     = time.Minute
	ladderMaxCachedPages = 256
	ladderDirectory      = "kr-solo-ladder-v1"
	ladderPageSize       = 100
	ladderMaxPage        = 100
	ladderParseVersion   = 1
	ladderMaxSkippedPct  = 5
	ladderMinFullRows    = 90
	ladderEndTolerance   = 5
)

var ladderTiers = map[string]string{
	"challenger": "王者", "grandmaster": "宗师", "master": "大师", "diamond": "钻石",
	"emerald": "翡翠", "platinum": "铂金", "gold": "黄金", "silver": "白银",
	"bronze": "青铜", "iron": "黑铁",
}

//go:embed data/ladder_champion_slugs.json
var ladderChampionSlugData []byte

var ladderChampionIDs = sync.OnceValue(func() map[string]int {
	var result map[string]int
	_ = json.Unmarshal(ladderChampionSlugData, &result)
	return result
})

type ladderChampion struct {
	ChampionID int    `json:"championId"`
	Name       string `json:"name"`
}

type ladderRow struct {
	Rank          int              `json:"rank"`
	GameName      string           `json:"gameName"`
	TagLine       string           `json:"tagLine"`
	ProfileIconID int              `json:"profileIconId"`
	Tier          string           `json:"tier"`
	Division      int              `json:"division,omitempty"`
	LP            int              `json:"lp"`
	Level         int              `json:"level"`
	Wins          int              `json:"wins"`
	Losses        int              `json:"losses"`
	WinRate       int              `json:"winRate"`
	Champions     []ladderChampion `json:"champions"`
}

type ladderSourceRow struct {
	ladderRow
	OPGGPro bool `json:"opggPro,omitempty"`
}

type ladderProPlayer struct {
	Team string `json:"team"`
	Name string `json:"name"`
}

type ladderDisplayRow struct {
	ladderRow
	ProMark   bool             `json:"proMark"`
	ProPlayer *ladderProPlayer `json:"proPlayer,omitempty"`
}

type ladderMetadata struct {
	ChallengerCutoffLP  int       `json:"challengerCutoffLP"`
	ChallengerCount     int       `json:"challengerCount"`
	GrandmasterCutoffLP int       `json:"grandmasterCutoffLP"`
	GrandmasterCount    int       `json:"grandmasterCount"`
	TotalSummoners      int       `json:"totalSummoners"`
	Page                int       `json:"page"`
	Tier                string    `json:"tier"`
	FetchedAt           time.Time `json:"fetchedAt"`
}

type ladderSnapshot struct {
	ladderMetadata
	Rows  []ladderSourceRow `json:"rows"`
	stale bool
}

type ladderKey struct {
	page int
	tier string
}

func validLadderTier(tier string) bool { return tier == "" || ladderTiers[tier] != "" }

func validLadderPage(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n >= 1 && n <= ladderMaxPage && strconv.Itoa(n) == value
}

func validLadderRiotID(name, tag string) bool {
	return strings.TrimSpace(name) != "" && strings.TrimSpace(tag) != "" &&
		len(name) <= 128 && len(tag) <= 32 && !strings.ContainsAny(name+tag, "#/\\") &&
		strings.IndexFunc(name+tag, unicode.IsControl) == -1
}

func validOPGGLadderURL(target *url.URL) bool {
	if target == nil || target.Scheme != "https" || target.Host != opggPageHost ||
		target.User != nil || target.EscapedPath() != proLadderPath || target.Fragment != "" {
		return false
	}
	query, err := url.ParseQuery(target.RawQuery)
	if err != nil || query.Get("type") != "ladder" || query.Get("region") != "kr" {
		return false
	}
	for k, v := range query {
		if len(v) != 1 {
			return false
		}
		switch k {
		case "type", "region":
		case "page":
			if !validLadderPage(v[0]) {
				return false
			}
		case "tier":
			if v[0] == "" || !validLadderTier(v[0]) {
				return false
			}
		case "summoner":
			i := strings.LastIndexByte(v[0], '-')
			if i <= 0 || !validLadderRiotID(v[0][:i], v[0][i+1:]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func fetchOPGGLadderPage(ctx context.Context, provider *championProvider, key ladderKey, summoner string) ([]byte, error) {
	if provider == nil {
		return nil, errors.New("ladder source unavailable")
	}
	if gate := featureGateForChampionHost(opggPageHost); gate != "" && !provider.featureGates.enabled(gate) {
		return nil, errors.New("ladder source disabled")
	}
	query := url.Values{"type": {"ladder"}, "region": {"kr"}}
	if key.page > 0 {
		query.Set("page", strconv.Itoa(key.page))
	}
	if key.tier != "" {
		query.Set("tier", key.tier)
	}
	if summoner != "" {
		query.Set("summoner", summoner)
	}
	target := &url.URL{Scheme: "https", Host: opggPageHost, Path: proLadderPath, RawQuery: query.Encode()}
	if !validOPGGLadderURL(target) {
		return nil, errors.New("invalid ladder URL")
	}
	ctx, cancel := context.WithTimeout(ctx, proLadderRequestTTL)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.6")
	request.Header.Set("User-Agent", "Deep-Legends/"+version)

	client := *provider.httpClient()
	client.Jar = nil
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 3 || !validOPGGLadderURL(request.URL) {
			return errors.New("ladder redirect rejected")
		}
		return nil
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ladder HTTP %d", response.StatusCode)
	}
	return readLimited(response.Body, proLadderMaxBytes)
}

var (
	errLadderPrefetchBusy = errors.New("ladder prefetch in progress")
	ladderIconPattern     = regexp.MustCompile(`/profile_icons/profileIcon([0-9]+)\.`)
	ladderRecordPattern   = regexp.MustCompile(`([\d,]+)\s*胜\s*([\d,]+)\s*败\s*([\d,]+)%`)
	ladderDigitsPattern   = regexp.MustCompile(`[0-9][0-9,]*`)
	ladderTotalPattern    = regexp.MustCompile(`总计\s*([\d,]+)\s*召唤师`)
	ladderRangePattern    = regexp.MustCompile(`#([\d,]+)\s*~\s*#[\d,]+`)
)

func ladderNumber(value string) int {
	n, err := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(value), ",", ""))
	if err != nil || n < 0 || n > proLadderMaxRank {
		return -1
	}
	return n
}

func ladderAttr(token xhtml.Token, key string) string {
	for _, attr := range token.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

type ladderRowParser struct {
	id        string
	row       ladderSourceRow
	cell      int
	text      [8]strings.Builder
	tag       strings.Builder
	inTag     bool
	slug      string
	badgeTier string
}

func ladderBadgeTier(token xhtml.Token) string {
	src, _, _ := strings.Cut(ladderAttr(token, "src"), "?")
	src, _, _ = strings.Cut(src, "#")
	stem := strings.TrimSuffix(path.Base(src), path.Ext(src))
	for _, label := range []string{stem, ladderAttr(token, "alt")} {
		for _, key := range strings.FieldsFunc(strings.ToLower(label), func(r rune) bool { return r < 'a' || r > 'z' }) {
			if ladderTiers[key] != "" {
				return key
			}
		}
	}
	return ""
}

func (p *ladderRowParser) start(token xhtml.Token, championIDs map[string]int) {
	if token.Data == "td" {
		p.cell++
	}
	switch p.cell {
	case 2:
		switch token.Data {
		case "strong":
			p.row.OPGGPro = p.row.OPGGPro || ladderAttr(token, "data-tooltip-content") == "PROGAMER"
		case "span":
			p.inTag = ladderAttr(token, "dir") == "auto"
		case "img":
			if match := ladderIconPattern.FindStringSubmatch(ladderAttr(token, "src")); match != nil {
				p.row.ProfileIconID = ladderNumber(match[1])
			}
		}
	case 3:
		if token.Data == "img" && p.badgeTier == "" {
			p.badgeTier = ladderBadgeTier(token)
		}
		if token.Data == "span" && p.text[3].Len() != 0 && !strings.HasSuffix(p.text[3].String(), "\n") {
			p.text[3].WriteByte('\n')
		}
	case 5:
		switch token.Data {
		case "a":
			_, suffix, ok := strings.Cut(ladderAttr(token, "href"), "/champions/")
			if ok && strings.HasSuffix(suffix, "/build") {
				p.slug = strings.TrimSuffix(suffix, "/build")
			}
		case "img":
			name := strings.TrimSpace(ladderAttr(token, "alt"))
			id := championIDs[p.slug]
			if id == 0 {
				id = ladderChampionIDs()[p.slug]
			}
			if id > 0 && name != "" && len(p.row.Champions) < 3 {
				p.row.Champions = append(p.row.Champions, ladderChampion{ChampionID: id, Name: name})
			}
		}
	}
}

func (p *ladderRowParser) finish() (ladderSourceRow, bool) {
	row := p.row
	tag := strings.TrimPrefix(strings.TrimSpace(p.tag.String()), "#")
	name, ok := strings.CutSuffix(p.id, "-"+tag)
	row.GameName, row.TagLine = strings.TrimSpace(name), strings.TrimSpace(tag)
	if !ok || p.cell != 7 || !validLadderRiotID(row.GameName, row.TagLine) || row.ProfileIconID <= 0 {
		return row, false
	}
	row.Rank = ladderNumber(p.text[1].String())
	row.LP = ladderNumber(p.text[4].String())
	row.Level = ladderNumber(p.text[6].String())
	tierText, short, _ := strings.Cut(p.text[3].String(), "\n")
	label, division, _ := strings.Cut(strings.TrimSpace(tierText), " ")
	row.Tier = p.badgeTier
	if row.Tier == "" {
		for key, tier := range ladderTiers {
			if label == tier || strings.EqualFold(label, key) {
				row.Tier = key
				break
			}
		}
	}
	if division == "" {
		// The compact OP.GG label uses a tier initial and division (e.g. D1).
		short = strings.TrimSpace(short)
		if short == "" && p.badgeTier != "" {
			short = label
		}
		if len(short) > 1 && short[0] >= 'A' && short[0] <= 'Z' && short[1] >= '0' && short[1] <= '9' {
			division = short[1:]
		}
	}
	if division != "" {
		row.Division = ladderNumber(division)
		if row.Division == 0 {
			row.Division = -1
		}
	}
	record := ladderRecordPattern.FindStringSubmatch(p.text[7].String())
	if row.Rank < 0 || row.Tier == "" || record == nil {
		return row, false
	}
	row.Wins, row.Losses, row.WinRate = ladderNumber(record[1]), ladderNumber(record[2]), ladderNumber(record[3])
	if row.Champions == nil {
		row.Champions = []ladderChampion{}
	}
	return row, true
}

func parseOPGGLadderPage(body []byte, championIDs map[string]int) (ladderSnapshot, int, error) {
	tokenizer := xhtml.NewTokenizer(bytes.NewReader(body))
	result := ladderSnapshot{ladderMetadata: ladderMetadata{TotalSummoners: -1}, Rows: make([]ladderSourceRow, 0, ladderPageSize)}
	skipped, tbody, sawBody := 0, false, false
	var row *ladderRowParser
	var cutoffTier string
	var cutoffLP, cutoffCount strings.Builder
	var cutoffDepth, cutoffGroup, cutoffBadgeGroup int
	inCutoff, inStrong := false, false
	var footer strings.Builder
	for {
		kind := tokenizer.Next()
		switch kind {
		case xhtml.ErrorToken:
			if errors.Is(tokenizer.Err(), io.EOF) {
				return result, skipped, nil
			}
			return result, skipped, tokenizer.Err()
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			switch {
			case token.Data == "tbody" && !sawBody:
				tbody, sawBody = true, true
			case tbody && token.Data == "tr":
				if row != nil {
					skipped++
				}
				row = &ladderRowParser{id: ladderAttr(token, "id")}
			case row != nil:
				row.start(token, championIDs)
			case !sawBody && token.Data == "li":
				inCutoff, inStrong, cutoffTier = true, false, ""
				cutoffLP.Reset()
				cutoffCount.Reset()
				cutoffDepth, cutoffGroup, cutoffBadgeGroup = 0, 0, 0
			case inCutoff && token.Data == "div":
				if cutoffDepth == 0 {
					cutoffGroup++
				}
				cutoffDepth++
			case inCutoff && token.Data == "img":
				switch alt := strings.ToLower(strings.TrimSpace(ladderAttr(token, "alt"))); alt {
				case "challenger", "grandmaster":
					cutoffTier, cutoffBadgeGroup = alt, cutoffGroup
				}
			case cutoffTier != "" && token.Data == "strong":
				inStrong = true
			case sawBody && !tbody && token.Data == "p":
				footer.Reset()
			}
		case xhtml.TextToken:
			value := tokenizer.Text()
			switch {
			case row != nil && row.cell > 0 && row.cell <= 7:
				if row.inTag {
					row.tag.Write(value)
				}
				if row.cell != 2 && row.cell != 5 {
					row.text[row.cell].Write(value)
				}
			case cutoffTier != "":
				if inStrong && cutoffGroup == cutoffBadgeGroup {
					cutoffLP.Write(value)
				} else if cutoffGroup > cutoffBadgeGroup || cutoffDepth == 0 {
					// The badge's block also holds the LP delta. The count is
					// in the next block, with a locale-dependent label.
					cutoffCount.Write(value)
				}
			case sawBody && !tbody && footer.Len() < 128:
				if footer.Len() > 0 || bytes.HasPrefix(bytes.TrimSpace(value), []byte("#")) {
					footer.Write(value)
				}
			}
		case xhtml.EndTagToken:
			switch tokenizer.Token().Data {
			case "span":
				if row != nil {
					row.inTag = false
				}
			case "a":
				if row != nil {
					row.slug = ""
				}
			case "tr":
				if row != nil {
					if parsed, ok := row.finish(); ok && len(result.Rows) < ladderPageSize {
						result.Rows = append(result.Rows, parsed)
					} else {
						skipped++
					}
					row = nil
				}
			case "tbody":
				tbody = false
				if row != nil {
					skipped++
					row = nil
				}
			case "div":
				if inCutoff && cutoffDepth > 0 {
					cutoffDepth--
				}
			case "strong":
				inStrong = false
			case "li":
				if cutoffTier != "" {
					lp := ladderNumber(ladderDigitsPattern.FindString(cutoffLP.String()))
					count := ladderNumber(ladderDigitsPattern.FindString(cutoffCount.String()))
					switch cutoffTier {
					case "challenger":
						result.ChallengerCutoffLP, result.ChallengerCount = lp, count
					case "grandmaster":
						result.GrandmasterCutoffLP, result.GrandmasterCount = lp, count
					}
				}
				inCutoff, cutoffTier = false, ""
			case "p":
				if match := ladderTotalPattern.FindStringSubmatch(footer.String()); match != nil {
					result.TotalSummoners = ladderNumber(match[1])
					if span := ladderRangePattern.FindStringSubmatch(footer.String()); span != nil {
						if start := ladderNumber(span[1]); start > 0 {
							result.Page = (start-1)/ladderPageSize + 1
						}
					}
					return result, skipped, nil
				}
			}
		}
	}
}

type ladderParseRejection string

func (reason ladderParseRejection) Error() string { return "ladder parse rejected: " + string(reason) }

const (
	ladderRejectedSkipped ladderParseRejection = "skipped_ratio"
	ladderRejectedCount   ladderParseRejection = "row_count"
	ladderRejectedRank    ladderParseRejection = "rank_order"
	ladderRejectedFields  ladderParseRejection = "field_sanity"
	ladderRejectedFooter  ladderParseRejection = "footer"
	ladderRejectedCutoff  ladderParseRejection = "cutoff"
)

func saneLadderFields(row ladderSourceRow) bool {
	if row.LP < 0 || row.LP > 5000 || row.Level < 1 || row.Level > 5000 {
		return false
	}
	if row.Wins < 0 || row.Losses < 0 || row.WinRate < 0 || row.WinRate > 100 {
		return false
	}
	switch row.Tier {
	case "challenger", "grandmaster", "master":
		if row.Division != 0 {
			return false
		}
	default:
		if row.Division < 1 || row.Division > 4 {
			return false
		}
	}
	wins, games := int64(row.Wins), int64(row.Wins)+int64(row.Losses)
	if games > 0 {
		rounded := int((100*wins + games/2) / games)
		return row.WinRate >= rounded-1 && row.WinRate <= rounded+1
	}
	return true
}

// validateLadderPage removes isolated field outliers before any caller can
// publish the snapshot. Large departures from the upstream layout fail closed.
func validateLadderPage(key ladderKey, page *ladderSnapshot, skipped int) error {
	if page.TotalSummoners < 0 || (key.page > 0 && page.Page > 0 && key.page != page.Page) {
		return ladderRejectedFooter
	}
	if key.page == 0 {
		key.page = page.Page
	}
	if key.page < 1 {
		return ladderRejectedFooter
	}
	if key.page == 1 && key.tier == "" &&
		(page.ChallengerCutoffLP <= 0 || page.ChallengerCount <= 0 || page.GrandmasterCutoffLP <= 0 || page.GrandmasterCount <= 0) {
		return ladderRejectedCutoff
	}
	totalRows := len(page.Rows) + skipped
	valid := page.Rows[:0]
	for _, row := range page.Rows {
		if saneLadderFields(row) {
			valid = append(valid, row)
		}
	}
	fieldInvalid := len(page.Rows) - len(valid)
	page.Rows = valid
	if fieldInvalid*100 > totalRows*ladderMaxSkippedPct {
		return ladderRejectedFields
	}
	if (skipped+fieldInvalid)*100 > totalRows*ladderMaxSkippedPct {
		return ladderRejectedSkipped
	}
	start := (key.page-1)*ladderPageSize + 1
	if key.tier == "" {
		expected := page.TotalSummoners - start + 1
		switch {
		case expected < 0:
			return ladderRejectedCount
		case expected <= ladderPageSize:
			if len(valid) < expected-ladderEndTolerance || len(valid) > expected {
				return ladderRejectedCount
			}
		case key.page < ladderMaxPage && len(valid) < ladderMinFullRows:
			return ladderRejectedCount
		}
	}
	if len(valid) == 0 && page.TotalSummoners > 0 && (key.tier != "" || page.TotalSummoners-start+1 > ladderEndTolerance) {
		return ladderRejectedCount
	}
	previous := 0
	for i, row := range valid {
		if row.Rank <= previous {
			return ladderRejectedRank
		}
		if key.tier == "" && (row.Rank < start || row.Rank >= start+ladderPageSize) {
			return ladderRejectedRank
		}
		if key.tier != "" && i == 0 && (row.Rank < start-ladderEndTolerance || row.Rank > start+ladderEndTolerance) {
			return ladderRejectedRank
		}
		previous = row.Rank
	}
	return nil
}

type ladderFlight struct {
	done chan struct{}
	page ladderSnapshot
	err  error
}

type ladderFailure struct {
	at  time.Time
	err error
}

type ladderRankPage struct {
	fetchedAt time.Time
	rows      map[string]int
}

type ladderCachedRank struct {
	location  ladderLocation
	fetchedAt time.Time
}

type ladderProvider struct {
	store         *localStore
	champions     *championProvider
	now           func() time.Time
	fetch         func(context.Context, ladderKey, string) ([]byte, error)
	index         func() proIdentityIndex
	observe       func(map[string]any)
	updated       func(ladderKey)
	mu            sync.Mutex
	pages         map[ladderKey]ladderSnapshot
	flights       map[ladderKey]*ladderFlight
	diskRanksOnce sync.Once
	failures      map[ladderKey]ladderFailure
	rankPages     map[int]ladderRankPage
	ranks         map[string]ladderCachedRank
	prewarmGate   chan struct{}
	prefetchGate  chan struct{}
	diskMu        sync.Mutex
}

func newLadderProvider(a *app) *ladderProvider {
	return &ladderProvider{
		store: a.storage, champions: a.champions, now: time.Now, index: a.proIdentitySnapshot, observe: a.recordDiagnostic,
		fetch: func(ctx context.Context, key ladderKey, summoner string) ([]byte, error) {
			return fetchOPGGLadderPage(ctx, a.champions, key, summoner)
		},
		updated: func(key ladderKey) {
			data, _ := json.Marshal(map[string]any{"type": "ladder-updated", "page": key.page, "tier": key.tier})
			a.broadcastEvent(string(data))
		},
		pages: make(map[ladderKey]ladderSnapshot), flights: make(map[ladderKey]*ladderFlight),
		failures: make(map[ladderKey]ladderFailure), rankPages: make(map[int]ladderRankPage), ranks: make(map[string]ladderCachedRank),
		prewarmGate: make(chan struct{}, 1), prefetchGate: make(chan struct{}, 1),
	}
}

func (k ladderKey) file() string {
	return filepath.Join(ladderDirectory, fmt.Sprintf("%03d-%s.json", k.page, k.tier))
}

func (p *ladderProvider) read(key ladderKey) ladderSnapshot {
	if p.store == nil {
		return ladderSnapshot{}
	}
	file, err := os.Open(filepath.Join(p.store.root, key.file()))
	if err != nil {
		return ladderSnapshot{}
	}
	defer file.Close()
	var cached ladderSnapshot
	data, err := readLimited(file, proLadderMaxBytes)
	if err != nil || json.Unmarshal(data, &cached) != nil || cached.Page != key.page || cached.Tier != key.tier ||
		cached.FetchedAt.IsZero() || cached.FetchedAt.After(p.now()) || len(cached.Rows) > ladderPageSize {
		return ladderSnapshot{}
	}
	return cached
}

func (p *ladderProvider) load(ctx context.Context, key ladderKey, refresh, prefetch bool) (ladderSnapshot, error) {
	p.mu.Lock()
	page, known := p.pages[key]
	if !known {
		p.mu.Unlock()
		disk := p.read(key)
		ranks := indexLadderPage(key, disk)
		p.mu.Lock()
		if page, known = p.pages[key]; !known {
			page = disk
			if !disk.FetchedAt.IsZero() {
				p.cachePageLocked(key, disk)
				if ranks.fetchedAt.After(p.rankPages[key.page].fetchedAt) {
					p.updateRanksLocked(key.page, ranks)
				}
			}
		}
	}
	now := p.now()
	age := now.Sub(page.FetchedAt)
	failure := p.failures[key]
	hadFailure := failure.err != nil
	page.stale = hadFailure
	if !refresh && age < ladderFreshFor {
		p.mu.Unlock()
		p.report(page, "hit", 0, 0, 0, 0, nil)
		return page, nil
	}
	usable := !page.FetchedAt.IsZero() && age < ladderMaxAge
	stale := !refresh && usable
	flight := p.flights[key]
	if flight == nil {
		if !refresh && failure.err != nil && now.Sub(failure.at) < ladderRetryAfter {
			p.mu.Unlock()
			if stale {
				p.report(page, "stale", 0, 0, 0, 0, nil)
				return page, nil
			}
			return ladderSnapshot{}, failure.err
		}
		if prefetch {
			select {
			case p.prefetchGate <- struct{}{}:
			default:
				p.mu.Unlock()
				return ladderSnapshot{}, errLadderPrefetchBusy
			}
		}
		flight = &ladderFlight{done: make(chan struct{})}
		p.flights[key] = flight
		goSafe("ladder.fetch", func() {
			if prefetch {
				defer func() { <-p.prefetchGate }()
			}
			fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), proLadderRequestTTL)
			defer cancel()
			var fetched ladderSnapshot
			fetchErr := errors.New("ladder fetch interrupted")
			defer func() {
				p.mu.Lock()
				flight.page, flight.err = fetched, fetchErr
				if fetchErr != nil {
					p.failures[key] = ladderFailure{p.now(), fetchErr}
				}
				delete(p.flights, key)
				close(flight.done)
				p.mu.Unlock()
				if usable && ((fetchErr == nil && (age >= ladderFreshFor || hadFailure)) ||
					(fetchErr != nil && stale && !hadFailure)) {
					p.updated(key)
				}
			}()
			fetched, fetchErr = p.fetchPage(fetchCtx, key, "")
			if fetchErr == nil {
				p.save(key, fetched)
			}
		})
	}
	p.mu.Unlock()
	if stale {
		p.report(page, "stale", 0, 0, 0, 0, nil)
		return page, nil
	}
	select {
	case <-ctx.Done():
		return ladderSnapshot{}, ctx.Err()
	case <-flight.done:
		if flight.err != nil && usable {
			page.stale = true
			return page, nil
		}
		return flight.page, flight.err
	}
}

// cachePageLocked leaves in-flight entries alone. If all slots are pinned,
// the incoming snapshot is still available through the flight and disk.
func (p *ladderProvider) cachePageLocked(key ladderKey, page ladderSnapshot) {
	if _, exists := p.pages[key]; !exists && len(p.pages) >= ladderMaxCachedPages {
		var oldest ladderKey
		var oldestAt time.Time
		found := false
		for candidate, cached := range p.pages {
			if p.flights[candidate] == nil && (!found || cached.FetchedAt.Before(oldestAt)) {
				oldest, oldestAt, found = candidate, cached.FetchedAt, true
			}
		}
		if !found || (p.flights[key] == nil && page.FetchedAt.Before(oldestAt)) {
			return
		}
		delete(p.pages, oldest)
	}
	p.pages[key] = page
}

func (p *ladderProvider) save(key ladderKey, page ladderSnapshot) {
	// Serialize disk writes so an older lookup cannot replace a newer refresh.
	p.diskMu.Lock()
	defer p.diskMu.Unlock()
	p.mu.Lock()
	previous, cached := p.pages[key]
	p.mu.Unlock()
	if !cached {
		previous = p.read(key)
	}
	ranks := indexLadderPage(key, page)
	p.mu.Lock()
	delete(p.failures, key)
	if previous.FetchedAt.After(page.FetchedAt) || p.pages[key].FetchedAt.After(page.FetchedAt) {
		p.mu.Unlock()
		return
	}
	p.cachePageLocked(key, page)
	p.updateRanksLocked(key.page, ranks)
	p.mu.Unlock()
	if p.store == nil {
		return
	}
	data, err := json.Marshal(page)
	if err == nil {
		err = writeLocalStoreFile(p.store, key.file(), data)
	}
	if err != nil {
		p.observe(map[string]any{"event": "ladder_cache_write_failed", "page": key.page, "tier": key.tier})
	}
}

func (p *ladderProvider) fetchPage(ctx context.Context, key ladderKey, summoner string) (ladderSnapshot, error) {
	body, err := p.fetch(ctx, key, summoner)
	if err != nil {
		p.report(ladderSnapshot{ladderMetadata: ladderMetadata{Page: key.page, Tier: key.tier}}, "miss", 0, 0, 0, 0, err)
		return ladderSnapshot{}, err
	}
	started := time.Now()
	// The live catalog publishes a fresh immutable map when it changes. The
	// bundled IDs cover cold starts without joining a catalog network request.
	var ids map[string]int
	if p.champions != nil {
		p.champions.mu.Lock()
		ids = p.champions.championIDs
		p.champions.mu.Unlock()
	}
	page, skipped, err := parseOPGGLadderPage(body, ids)
	parsedRows := len(page.Rows)
	if err == nil {
		err = validateLadderPage(key, &page, skipped)
	}
	fieldInvalid := parsedRows - len(page.Rows)
	if key.page > 0 {
		page.Page = key.page
	}
	page.Tier, page.FetchedAt = key.tier, p.now().UTC()
	p.report(page, "miss", len(body), time.Since(started), skipped+fieldInvalid, fieldInvalid, err)
	if err != nil {
		reason := ladderRejectedFooter
		_ = errors.As(err, &reason)
		p.observe(map[string]any{"event": "ladder_parse_rejected", "page": page.Page, "tier": key.tier,
			"reason": string(reason), "rows": len(page.Rows), "skipped": skipped + fieldInvalid, "bytes": len(body)})
	}
	return page, err
}

func ladderDisplay(row ladderSourceRow, index proIdentityIndex) ladderDisplayRow {
	out := ladderDisplayRow{ladderRow: row.ladderRow, ProMark: row.OPGGPro}
	if identity := index.byKey["name:"+proLadderAccountKey(row.GameName, row.TagLine)]; identity != nil {
		out.ProMark = true
		out.ProPlayer = &ladderProPlayer{Team: identity.TeamCode, Name: identity.PlayerName}
	}
	return out
}

func (p *ladderProvider) report(page ladderSnapshot, cache string, bytes int, elapsed time.Duration, skipped, fieldInvalid int, err error) {
	index := p.index()
	opgg, roster := 0, 0
	for _, row := range page.Rows {
		if row.OPGGPro {
			opgg++
		}
		if ladderDisplay(row, index).ProPlayer != nil {
			roster++
		}
	}
	event := map[string]any{"event": "ladder_fetch", "page": page.Page, "tier": page.Tier, "cache": cache,
		"bytes": bytes, "parse_ms": elapsed.Milliseconds(), "rows": len(page.Rows),
		"rows_skipped": skipped, "pro_mark_opgg": opgg, "pro_mark_roster": roster}
	if bytes > 0 {
		event["parse_version"] = ladderParseVersion
		event["field_invalid_rows"] = fieldInvalid
	}
	if err != nil {
		event["outcome"] = "failed"
	}
	p.observe(event)
}

type ladderLocation struct {
	Page int `json:"page"`
	Rank int `json:"rank"`
	// Search can return a page beyond the upstream page=100 request limit.
	Data *ladderDisplayPage `json:"data,omitempty"`
}

func indexLadderPage(key ladderKey, snapshot ladderSnapshot) ladderRankPage {
	// Filtered OP.GG pages renumber players within the tier.
	if key.tier != "" || snapshot.FetchedAt.IsZero() {
		return ladderRankPage{}
	}
	rows := make(map[string]int, len(snapshot.Rows))
	for _, row := range snapshot.Rows {
		id := proLadderAccountKey(row.GameName, row.TagLine)
		if _, exists := rows[id]; !exists {
			rows[id] = row.Rank
		}
	}
	return ladderRankPage{snapshot.FetchedAt, rows}
}

func (p *ladderProvider) updateRanksLocked(page int, next ladderRankPage) {
	if next.fetchedAt.IsZero() {
		return
	}
	previous := p.rankPages[page]
	if next.fetchedAt.Before(previous.fetchedAt) {
		return
	}
	p.rankPages[page] = next
	for id := range previous.rows {
		if _, retained := next.rows[id]; retained || p.ranks[id].location.Page != page {
			continue
		}
		delete(p.ranks, id)
		for otherPage, other := range p.rankPages {
			if rank, exists := other.rows[id]; exists {
				p.recordRankLocked(id, otherPage, rank, other.fetchedAt)
			}
		}
	}
	for id, rank := range next.rows {
		p.recordRankLocked(id, page, rank, next.fetchedAt)
	}
}

func (p *ladderProvider) recordRankLocked(id string, page, rank int, fetchedAt time.Time) {
	best := p.ranks[id]
	// As in the previous descending freshness selection, ties go to the higher page.
	if fetchedAt.After(best.fetchedAt) || (fetchedAt.Equal(best.fetchedAt) && page >= best.location.Page) {
		p.ranks[id] = ladderCachedRank{ladderLocation{Page: page, Rank: rank}, fetchedAt}
	}
}

func (p *ladderProvider) cachedRank(name, tag string) ladderLocation {
	p.diskRanksOnce.Do(func() {
		if p.store == nil {
			return
		}
		files, _ := filepath.Glob(filepath.Join(p.store.root, ladderDirectory, "*-.json"))
		for _, file := range files {
			number := strings.TrimSuffix(path.Base(file), "-.json")
			page, err := strconv.Atoi(number)
			if err == nil && page >= 1 && page <= ladderMaxPage {
				key := ladderKey{page: page}
				ranks := indexLadderPage(key, p.read(key))
				p.mu.Lock()
				if ranks.fetchedAt.After(p.rankPages[page].fetchedAt) {
					p.updateRanksLocked(page, ranks)
				}
				p.mu.Unlock()
			}
		}
	})
	id := proLadderAccountKey(name, tag)
	p.mu.Lock()
	cached := p.ranks[id]
	p.mu.Unlock()
	if p.now().Sub(cached.fetchedAt) >= ladderMaxAge {
		return ladderLocation{}
	}
	return cached.location
}

func ladderRequestIdentity(r *http.Request) (string, string, bool) {
	name, tag, ok := strings.Cut(strings.TrimSpace(r.URL.Query().Get("riotId")), "#")
	name, tag = strings.TrimSpace(name), strings.TrimSpace(tag)
	return name, tag, ok && validLadderRiotID(name, tag)
}

func (a *app) handleLadder(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	page, tier := query.Get("page"), query.Get("tier")
	if page == "" {
		page = "1"
	}
	if !validLadderPage(page) || !validLadderTier(tier) {
		http.Error(w, "invalid ladder filter", http.StatusBadRequest)
		return
	}
	n, _ := strconv.Atoi(page)
	data, err := a.ladder.load(r.Context(), ladderKey{n, tier}, query.Get("refresh") == "1", query.Get("prefetch") == "1")
	if errors.Is(err, errLadderPrefetchBusy) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		http.Error(w, "ladder unavailable", http.StatusBadGateway)
		return
	}
	respondJSON(w, a.ladder.displayPage(data))
}

type ladderDisplayPage struct {
	ladderMetadata
	Rows        []ladderDisplayRow `json:"rows"`
	Stale       bool               `json:"stale"`
	StaleReason string             `json:"staleReason,omitempty"`
}

func (p *ladderProvider) displayPage(data ladderSnapshot) *ladderDisplayPage {
	result := &ladderDisplayPage{ladderMetadata: data.ladderMetadata, Rows: make([]ladderDisplayRow, 0, len(data.Rows)), Stale: data.stale}
	if data.stale {
		result.StaleReason = "upstream_unavailable"
	}
	index := p.index()
	for _, row := range data.Rows {
		result.Rows = append(result.Rows, ladderDisplay(row, index))
	}
	return result
}

func (a *app) handleLadderFind(w http.ResponseWriter, r *http.Request) {
	name, tag, ok := ladderRequestIdentity(r)
	if !ok {
		http.Error(w, "invalid Riot ID", http.StatusBadRequest)
		return
	}
	if cached := a.ladder.cachedRank(name, tag); cached.Rank > 0 {
		respondJSON(w, cached)
		return
	}
	page, err := a.ladder.fetchPage(r.Context(), ladderKey{}, name+"-"+tag)
	if err != nil {
		http.Error(w, "ladder unavailable", http.StatusBadGateway)
		return
	}
	for _, row := range page.Rows {
		if proLadderAccountKey(name, tag) == proLadderAccountKey(row.GameName, row.TagLine) {
			location := ladderLocation{Page: (row.Rank-1)/ladderPageSize + 1, Rank: row.Rank}
			if location.Page > ladderMaxPage {
				page.Page = location.Page
				location.Data = a.ladder.displayPage(page)
			} else if page.Page == location.Page {
				a.ladder.save(ladderKey{page: page.Page}, page)
			}
			respondJSON(w, location)
			return
		}
	}
	http.Error(w, "ladder player not found", http.StatusNotFound)
}

func (a *app) handleLadderPrewarm(w http.ResponseWriter, r *http.Request) {
	name, tag, ok := ladderRequestIdentity(r)
	if !ok {
		http.Error(w, "invalid Riot ID", http.StatusBadRequest)
		return
	}
	if a.riot == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	select {
	case a.ladder.prewarmGate <- struct{}{}:
		defer func() { <-a.ladder.prewarmGate }()
	default:
		w.WriteHeader(http.StatusNoContent)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), proLadderRequestTTL)
	defer cancel()
	// Pin the interactive credential first so the warmed PUUID and league
	// cache use the same scope as overview, while keeping background admission.
	ctx = withRiotBackground(riotPinnedIdentityContext(withRiotPlatform(ctx, "kr")))
	ctx = context.WithValue(ctx, riotPassiveAccountLookupKey{}, true)
	ctx = context.WithValue(ctx, riotRequestSourceKey{}, "ladder")
	riot := a.riot.forPlatform("kr")
	account, err := riot.accountByRiotID(ctx, name, tag)
	if err != nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if account.scope == "relay" {
		ctx = context.WithValue(ctx, riotRouteKey{}, "relay")
	}
	_, _ = riot.leagueEntries(ctx, account.PUUID)
	if ctx.Err() != nil {
		return
	}
	publicRef := a.registerGameplayReferenceDetails(gameplayReference{
		PlayerRef: account.PUUID, GameName: name, TagLine: tag, Region: "kr", RiotCredentialScope: account.scope,
	}, "ladder")
	respondJSON(w, map[string]string{"playerRef": publicRef})
}
