package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	specialistRuneCacheTTL     = 6 * time.Hour
	specialistRunePlayerLimit  = 3
	specialistRuneMatchIDCount = 30
	specialistRuneMatchScanMax = 10
	specialistRunePerPlayerMax = 3
	// Account, filtered match list, and up to ten details for each of three players.
	specialistRuneRequestBudget    = specialistRunePlayerLimit * (2 + specialistRuneMatchScanMax)
	specialistRuneRequestTimeout   = 25 * time.Second
	specialistRunePartialCacheTTL  = 30 * time.Minute
	specialistRuneEmptyCacheTTL    = 30 * time.Minute
	specialistRuneNegativeCacheTTL = 90 * time.Second
	specialistAccountTimeout       = 5 * time.Second
	specialistMatchIDsTimeout      = 5 * time.Second
	specialistMatchDetailTimeout   = 4 * time.Second
	specialistRiotQueueTimeout     = 3 * time.Second
	specialistSlotQueueTimeout     = 3 * time.Second
	specialistMatchConcurrency     = 3
	specialistRecentSummaryTTL     = 30 * time.Minute
	specialistMatchFailureLimit    = 3
)

type specialistOutcome string

const (
	specialistOutcomeSuccess          specialistOutcome = ""
	specialistOutcomeNoPositionSample specialistOutcome = "no-position-sample"
	specialistOutcomeTimeout          specialistOutcome = "upstream-timeout"
	specialistOutcomeThrottled        specialistOutcome = "upstream-throttled"
	specialistOutcomeError            specialistOutcome = "upstream-error"
)

type specialistRuneCacheEntry struct {
	expiresAt time.Time
	position  string
	runes     []gameplayRecommendationRune
	outcome   specialistOutcome
}

type specialistRuneFlight struct {
	done     chan struct{}
	position string
	runes    []gameplayRecommendationRune
	outcome  specialistOutcome
	waiters  int
}

type specialistMatchSummary struct {
	matchID     string
	championID  int64
	position    string
	gameCreated int64
}

type specialistRecentSummaryCacheEntry struct {
	expiresAt time.Time
	summaries []specialistMatchSummary
}

type specialistFailureTracker struct {
	mu        sync.Mutex
	timedOut  bool
	throttled bool
	failed    bool
}

type specialistMatchDetailResult struct {
	matchID string
	match   *riotMatch
	err     error
}

type specialistRequestBudget struct {
	remaining int
}

type specialistOpponentRankBatch struct {
	ctx       context.Context
	lookup    func(context.Context, string) rankScoreEntry
	semaphore chan struct{}
	mu        sync.Mutex
	requested map[string]struct{}
	entries   map[string]rankScoreEntry
	wait      sync.WaitGroup
}

func (b *specialistRequestBudget) take() bool {
	if b.remaining <= 0 {
		return false
	}
	b.remaining--
	return true
}

// Keep cache and single-flight entries independent per champion and lane.
// A delimited string avoids collisions between champion IDs and lane ordinals.
func specialistRuneKey(championID int64, position string) string {
	return fmt.Sprintf("%d|%s", championID, canonicalSpecialistPosition(position))
}

func canonicalSpecialistPosition(position string) string {
	switch strings.ToLower(strings.TrimSpace(position)) {
	case "middle":
		return "mid"
	case "bottom":
		return "adc"
	case "utility":
		return "support"
	default:
		return strings.ToLower(strings.TrimSpace(position))
	}
}

func (a *app) handleGameplaySpecialistRunes(w http.ResponseWriter, r *http.Request) {
	championID, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("championId")), 10, 64)
	if err != nil || championID <= 0 || championID > 10000 {
		http.Error(w, "推荐英雄无效", http.StatusBadRequest)
		return
	}
	position, err := normalizeOPGGPosition(r.URL.Query().Get("position"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	provider := a.championDataProvider()
	recordHandler := func(outcome, reason string) {
		if provider.diag != nil {
			provider.diag(map[string]any{"event": "specialist_runes_handler", "champion_id": championID, "position": canonicalSpecialistPosition(position), "outcome": outcome, "reason": reason})
		}
	}
	if !riotKeyConfigured() || a.riot == nil {
		recordHandler("skipped", "riot-key-missing")
		respondJSON(w, map[string]any{"reason": "riot-key-missing", "runes": []gameplayRecommendationRune{}})
		return
	}
	recordHandler("accepted", "")
	ctx, cancel := context.WithTimeout(r.Context(), specialistRuneRequestTimeout)
	defer cancel()
	metadata, err := provider.championMetadataByID(ctx, int(championID))
	if err != nil {
		recordHandler("failed", "champion-metadata")
		http.Error(w, "暂时无法识别当前英雄", http.StatusNotFound)
		return
	}
	runes, outcome := a.riot.specialistRunes(ctx, championID, metadata.Slug, metadata.NameZH, position)
	if len(runes) == 0 {
		reason := string(outcome)
		if reason == "" {
			reason = string(specialistOutcomeNoPositionSample)
		}
		recordHandler("empty", reason)
		respondJSON(w, map[string]any{"reason": reason, "runes": runes})
		return
	}
	respondJSON(w, runes)
}

func (p *riotProvider) specialistRunes(ctx context.Context, championID int64, championSlug, championName string, positions ...string) ([]gameplayRecommendationRune, specialistOutcome) {
	if p == nil || p.champions == nil || !riotKeyConfigured() || championID <= 0 || strings.TrimSpace(championSlug) == "" {
		return []gameplayRecommendationRune{}, specialistOutcomeError
	}
	now := time.Now()
	position := ""
	if len(positions) > 0 {
		position = strings.TrimSpace(positions[0])
	}
	position = canonicalSpecialistPosition(position)
	key := specialistRuneKey(championID, position)
	p.specialistMu.Lock()
	if cached, ok := p.specialistCache[key]; ok && cached.position == position && now.Before(cached.expiresAt) {
		result := cloneSpecialistRunes(cached.runes)
		p.specialistMu.Unlock()
		return result, cached.outcome
	}
	if flight := p.specialistFlights[key]; flight != nil {
		flight.waiters++
		done := flight.done
		p.specialistMu.Unlock()
		select {
		case <-done:
			return cloneSpecialistRunes(flight.runes), flight.outcome
		case <-ctx.Done():
			return []gameplayRecommendationRune{}, specialistOutcomeTimeout
		}
	}
	flight := &specialistRuneFlight{done: make(chan struct{}), position: position}
	p.specialistFlights[key] = flight
	p.specialistMu.Unlock()
	if ctx.Err() != nil {
		p.finishSpecialistRuneFlight(key, flight, nil, specialistOutcomeTimeout, time.Now())
		return []gameplayRecommendationRune{}, specialistOutcomeTimeout
	}
	if err := acquireSpecialistSlot(ctx, p.specialistSlots, specialistSlotQueueTimeout); err != nil {
		if errors.Is(err, errThrottled) {
			p.recordSpecialistDiagnostic(map[string]any{"event": "specialist_runes_step_failed", "step": "flight_slot", "errorKind": "rate-limit", "budget_remaining": specialistRuneRequestBudget})
			p.finishSpecialistRuneFlight(key, flight, nil, specialistOutcomeThrottled, time.Now())
			return []gameplayRecommendationRune{}, specialistOutcomeThrottled
		}
		p.finishSpecialistRuneFlight(key, flight, nil, specialistOutcomeTimeout, time.Now())
		return []gameplayRecommendationRune{}, specialistOutcomeTimeout
	}
	runes, outcome := p.loadSpecialistRunes(ctx, championID, championSlug, championName, position)
	<-p.specialistSlots
	p.finishSpecialistRuneFlight(key, flight, runes, outcome, time.Now())
	return cloneSpecialistRunes(runes), outcome
}

func (p *riotProvider) finishSpecialistRuneFlight(key string, flight *specialistRuneFlight, runes []gameplayRecommendationRune, outcome specialistOutcome, fetchedAt time.Time) {
	result := cloneSpecialistRunes(runes)
	p.specialistMu.Lock()
	if len(result) > 0 || outcome == specialistOutcomeNoPositionSample || outcome == specialistOutcomeThrottled || outcome == specialistOutcomeTimeout {
		ttl := specialistRuneCacheTTL
		if len(result) == 0 {
			ttl = specialistRuneEmptyCacheTTL
			if outcome == specialistOutcomeThrottled || outcome == specialistOutcomeTimeout {
				ttl = specialistRuneNegativeCacheTTL
			}
		} else if len(result) < specialistRunePlayerLimit*specialistRunePerPlayerMax {
			ttl = specialistRunePartialCacheTTL
		}
		p.specialistCache[key] = specialistRuneCacheEntry{expiresAt: fetchedAt.Add(ttl), position: flight.position, runes: result, outcome: outcome}
	}
	flight.runes = cloneSpecialistRunes(result)
	flight.outcome = outcome
	delete(p.specialistFlights, key)
	close(flight.done)
	p.specialistMu.Unlock()
}

func (p *riotProvider) loadSpecialistRunes(ctx context.Context, championID int64, championSlug, championName, position string) (result []gameplayRecommendationRune, outcome specialistOutcome) {
	started := time.Now()
	players := p.champions.loadTopPlayersForPosition(ctx, championSlug, position)
	if len(players) > specialistRunePlayerLimit {
		players = players[:specialistRunePlayerLimit]
	}
	result = make([]gameplayRecommendationRune, 0, len(players)*specialistRunePerPlayerMax)
	budget := specialistRequestBudget{remaining: specialistRuneRequestBudget}
	var budgetMu sync.Mutex
	failures := &specialistFailureTracker{}
	p.recordSpecialistDiagnostic(map[string]any{"event": "specialist_runes_start", "champion_id": championID, "position": canonicalSpecialistPosition(position), "players_parsed": len(players)})
	defer func() {
		budgetMu.Lock()
		remaining := budget.remaining
		budgetMu.Unlock()
		p.recordSpecialistDiagnostic(map[string]any{"event": "specialist_runes_done", "champion_id": championID, "position": canonicalSpecialistPosition(position), "runes_returned": len(result), "outcome": outcome, "budget_used": specialistRuneRequestBudget - remaining, "duration_ms": time.Since(started).Milliseconds()})
	}()
	playerResults := make([][]gameplayRecommendationRune, len(players))
	var workers sync.WaitGroup
	for playerIndex, player := range players {
		playerIndex, player := playerIndex, player
		workers.Add(1)
		go func() {
			defer workers.Done()
			take := func() bool { budgetMu.Lock(); defer budgetMu.Unlock(); return budget.take() }
			remaining := func() int { budgetMu.Lock(); defer budgetMu.Unlock(); return budget.remaining }
			recordFailure := func(step string, err error) {
				failures.record(err)
				p.recordSpecialistDiagnostic(map[string]any{"event": "specialist_runes_step_failed", "step": step, "errorKind": specialistErrorKind(err), "budget_remaining": remaining()})
			}
			if strings.TrimSpace(player.Name) == "" || strings.TrimSpace(player.Tagline) == "" {
				recordFailure("player", errors.New("specialist player identity is incomplete"))
				return
			}
			if !take() {
				recordFailure("account_budget", errThrottled)
				return
			}
			accountCtx, accountCancel := specialistStepContext(ctx, specialistAccountTimeout)
			account, err := p.accountByRiotID(accountCtx, player.Name, player.Tagline)
			accountCancel()
			if err != nil {
				recordFailure("account", err)
				return
			}
			summaries, summaryHit := p.specialistRecentSummaries(account.PUUID, time.Now())
			matchIDs := specialistSummaryCandidates(summaries, championID, position)
			if !summaryHit {
				if !take() {
					recordFailure("match_ids_budget", errThrottled)
					return
				}
				idsCtx, idsCancel := specialistStepContext(ctx, specialistMatchIDsTimeout)
				matchIDs, err = p.matchIDsFiltered(idsCtx, account.PUUID, 0, specialistRuneMatchIDCount, 420, "ranked")
				idsCancel()
				if err != nil {
					recordFailure("match_ids", err)
					return
				}
				if len(matchIDs) > specialistRuneMatchScanMax {
					matchIDs = matchIDs[:specialistRuneMatchScanMax]
				}
			}
			matches := make([]gameplayRecommendationRune, 0, specialistRunePerPlayerMax)
			consecutiveDetailFailures := 0
			details := make([]specialistMatchDetailResult, 0, len(matchIDs))
			scanComplete := true
			for batchStart := 0; batchStart < len(matchIDs); batchStart += specialistMatchConcurrency {
				batchEnd := batchStart + specialistMatchConcurrency
				if batchEnd > len(matchIDs) {
					batchEnd = len(matchIDs)
				}
				batch := make([]specialistMatchDetailResult, batchEnd-batchStart)
				var detailWorkers sync.WaitGroup
				for offset, matchID := range matchIDs[batchStart:batchEnd] {
					if !take() {
						batch[offset] = specialistMatchDetailResult{matchID: matchID, err: errThrottled}
						continue
					}
					detailWorkers.Add(1)
					go func(offset int, matchID string) {
						defer detailWorkers.Done()
						detailCtx, detailCancel := specialistStepContext(ctx, specialistMatchDetailTimeout)
						match, detailErr := p.matchByID(detailCtx, matchID)
						detailCancel()
						batch[offset] = specialistMatchDetailResult{matchID: matchID, match: match, err: detailErr}
					}(offset, matchID)
				}
				detailWorkers.Wait()
				for _, detail := range batch {
					details = append(details, detail)
					if detail.err != nil {
						recordFailure("match_detail", detail.err)
						consecutiveDetailFailures++
						scanComplete = false
						continue
					}
					consecutiveDetailFailures = 0
				}
				if consecutiveDetailFailures >= specialistMatchFailureLimit || ctx.Err() != nil {
					scanComplete = false
					break
				}
			}
			freshSummaries := make([]specialistMatchSummary, 0, len(details))
			for _, detail := range details {
				if detail.err != nil || detail.match == nil {
					continue
				}
				match := detail.match
				participant, found := specialistParticipant(match, account.PUUID)
				if !found {
					recordFailure("match_participant", errors.New("Riot match detail is missing the requested participant"))
					scanComplete = false
					continue
				}
				positionKey := canonicalSpecialistPosition(riotPositionKey(participant))
				freshSummaries = append(freshSummaries, specialistMatchSummary{matchID: detail.matchID, championID: participant.ChampionID, position: positionKey, gameCreated: match.Info.GameCreation})
				if participant.ChampionID != championID || positionKey != canonicalSpecialistPosition(position) || len(matches) >= specialistRunePerPlayerMax {
					continue
				}
				rune, ok := specialistRuneFromParticipant(championID, championName, player, match, participant, 0)
				if !ok {
					continue
				}
				rune.Position = positionKey
				if opponent := specialistOpponent(match, participant, positionKey); opponent.ChampionID > 0 {
					rune.OpponentChampionID = opponent.ChampionID
					rune.OpponentChampionName = p.specialistChampionName(opponent.ChampionID)
					rune.OpponentPlayerName = firstNonEmpty(strings.TrimSpace(opponent.RiotIDGameName), strings.TrimSpace(opponent.SummonerName))
					rune.OpponentTagLine = strings.TrimSpace(opponent.RiotIDTagline)
					rune.opponentPUUID = strings.TrimSpace(opponent.PUUID)
				}
				matches = append(matches, rune)
			}
			if !summaryHit && scanComplete && len(freshSummaries) == len(matchIDs) {
				p.cacheSpecialistRecentSummaries(account.PUUID, freshSummaries, time.Now())
			}
			playerResults[playerIndex] = matches
		}()
	}
	workers.Wait()
	// Start rank enrichment after the scan finishes. The lookup has its own
	// deadline and must not inherit the nearly-expired scan context.
	rankCtx, rankCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	opponentRanks := newSpecialistOpponentRankBatch(rankCtx, p.opponentRankScore)
	for _, matches := range playerResults {
		for _, rune := range matches {
			rune.Key = "specialist-" + strconv.Itoa(len(result))
			result = append(result, rune)
			opponentRanks.add(rune.opponentPUUID)
		}
	}
	opponentRanks.apply(result)
	rankCancel()
	if len(result) > 0 {
		return result, specialistOutcomeSuccess
	}
	return result, failures.outcome(ctx.Err())
}

func specialistStepContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	stepCtx, cancel := context.WithTimeout(ctx, timeout)
	return withRiotQueueLimit(stepCtx, specialistRiotQueueTimeout), cancel
}

func acquireSpecialistSlot(ctx context.Context, slots chan struct{}, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case slots <- struct{}{}:
		return nil
	case <-timer.C:
		return errThrottled
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *specialistFailureTracker) record(err error) {
	if f == nil || err == nil {
		return
	}
	kind := specialistErrorKind(err)
	f.mu.Lock()
	switch kind {
	case "timeout", "canceled":
		f.timedOut = true
	case "rate-limit":
		f.throttled = true
	default:
		f.failed = true
	}
	f.mu.Unlock()
}

func (f *specialistFailureTracker) outcome(ctxErr error) specialistOutcome {
	if f == nil {
		return specialistOutcomeNoPositionSample
	}
	f.mu.Lock()
	timedOut, throttled, failed := f.timedOut, f.throttled, f.failed
	f.mu.Unlock()
	if errors.Is(ctxErr, context.DeadlineExceeded) || errors.Is(ctxErr, context.Canceled) || timedOut {
		return specialistOutcomeTimeout
	}
	if throttled {
		return specialistOutcomeThrottled
	}
	if failed {
		return specialistOutcomeError
	}
	return specialistOutcomeNoPositionSample
}

func (p *riotProvider) specialistRecentSummaries(puuid string, now time.Time) ([]specialistMatchSummary, bool) {
	puuid = strings.TrimSpace(puuid)
	if p == nil || puuid == "" {
		return nil, false
	}
	p.specialistMu.Lock()
	entry, ok := p.specialistRecent[puuid]
	if ok && !now.Before(entry.expiresAt) {
		delete(p.specialistRecent, puuid)
		ok = false
	}
	p.specialistMu.Unlock()
	if !ok {
		return nil, false
	}
	return append([]specialistMatchSummary(nil), entry.summaries...), true
}

func (p *riotProvider) cacheSpecialistRecentSummaries(puuid string, summaries []specialistMatchSummary, fetchedAt time.Time) {
	puuid = strings.TrimSpace(puuid)
	if p == nil || puuid == "" {
		return
	}
	p.specialistMu.Lock()
	p.specialistRecent[puuid] = specialistRecentSummaryCacheEntry{
		expiresAt: fetchedAt.Add(specialistRecentSummaryTTL),
		summaries: append([]specialistMatchSummary(nil), summaries...),
	}
	p.specialistMu.Unlock()
}

func specialistSummaryCandidates(summaries []specialistMatchSummary, championID int64, position string) []string {
	position = canonicalSpecialistPosition(position)
	result := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary.championID == championID && canonicalSpecialistPosition(summary.position) == position && strings.TrimSpace(summary.matchID) != "" {
			result = append(result, summary.matchID)
		}
	}
	return result
}

func newSpecialistOpponentRankBatch(ctx context.Context, lookup func(context.Context, string) rankScoreEntry) *specialistOpponentRankBatch {
	return &specialistOpponentRankBatch{
		ctx:       ctx,
		lookup:    lookup,
		semaphore: make(chan struct{}, matchTiersRankConcurrency),
		requested: make(map[string]struct{}),
		entries:   make(map[string]rankScoreEntry),
	}
}

func (b *specialistOpponentRankBatch) add(puuid string) {
	puuid = strings.TrimSpace(puuid)
	if b == nil || b.lookup == nil || puuid == "" {
		return
	}
	b.mu.Lock()
	if _, exists := b.requested[puuid]; exists {
		b.mu.Unlock()
		return
	}
	b.requested[puuid] = struct{}{}
	b.wait.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wait.Done()
		select {
		case b.semaphore <- struct{}{}:
			defer func() { <-b.semaphore }()
		case <-b.ctx.Done():
			return
		}
		entry := b.lookup(b.ctx, puuid)
		b.mu.Lock()
		b.entries[puuid] = entry
		b.mu.Unlock()
	}()
}

func (b *specialistOpponentRankBatch) apply(runes []gameplayRecommendationRune) {
	if b == nil {
		return
	}
	b.wait.Wait()
	b.mu.Lock()
	defer b.mu.Unlock()
	for index := range runes {
		entry, ok := b.entries[strings.TrimSpace(runes[index].opponentPUUID)]
		if !ok || !entry.known {
			continue
		}
		runes[index].OpponentTier = entry.tier
		runes[index].OpponentDivision = entry.division
		if entry.winRateKnown {
			winRate := entry.winRate
			runes[index].OpponentWinRate = &winRate
		}
	}
}

func (p *riotProvider) recordSpecialistDiagnostic(event map[string]any) {
	if p != nil && p.champions != nil && p.champions.diag != nil {
		p.champions.diag(event)
	}
}

func specialistErrorKind(err error) string {
	if err == nil {
		return "other"
	}
	if errors.Is(err, errThrottled) {
		return "rate-limit"
	}
	value := strings.ToLower(err.Error())
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout") || strings.Contains(value, "deadline") {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, errRiotNotFound) || strings.Contains(value, "404") || strings.Contains(value, "未找到") {
		return "404"
	}
	var statusErr *riotStatusError
	if errors.As(err, &statusErr) {
		switch {
		case statusErr.status == http.StatusTooManyRequests:
			return "rate-limit"
		case statusErr.status == http.StatusUnauthorized || statusErr.status == http.StatusForbidden:
			return "forbidden"
		case statusErr.status >= 500 && statusErr.status <= 599:
			return "5xx"
		}
	}
	if strings.Contains(value, "429") || strings.Contains(value, "限流") {
		return "rate-limit"
	}
	if strings.Contains(value, "403") || strings.Contains(value, "api key") || strings.Contains(value, "无权") {
		return "forbidden"
	}
	if strings.Contains(value, "无法解析") || strings.Contains(value, "缺少必要字段") || strings.Contains(value, "invalid") || strings.Contains(value, "json") {
		return "parse"
	}
	return "other"
}

func specialistParticipant(match *riotMatch, puuid string) (riotParticipant, bool) {
	if match == nil || strings.TrimSpace(puuid) == "" {
		return riotParticipant{}, false
	}
	for _, participant := range match.Info.Participants {
		if participant.PUUID == puuid {
			return participant, true
		}
	}
	return riotParticipant{}, false
}

func specialistOpponent(match *riotMatch, subject riotParticipant, requestedPosition string) riotParticipant {
	if match == nil {
		return riotParticipant{}
	}
	position := strings.ToLower(strings.TrimSpace(requestedPosition))
	if position == "mid" {
		position = "middle"
	}
	if position == "adc" {
		position = "bottom"
	}
	if position == "support" {
		position = "utility"
	}
	for _, candidate := range match.Info.Participants {
		if candidate.TeamID == subject.TeamID || candidate.ChampionID <= 0 {
			continue
		}
		candidatePosition := strings.ToLower(strings.TrimSpace(candidate.TeamPosition))
		if candidatePosition == "" {
			candidatePosition = strings.ToLower(strings.TrimSpace(candidate.IndividualPosition))
		}
		if position != "" && candidatePosition == position {
			return candidate
		}
	}
	return riotParticipant{}
}

func (p *riotProvider) specialistChampionName(championID int64) string {
	if p == nil || p.champions == nil {
		return ""
	}
	p.champions.mu.Lock()
	metadata := p.champions.championMeta[int(championID)]
	p.champions.mu.Unlock()
	if strings.TrimSpace(metadata.NameZH) != "" {
		return metadata.NameZH
	}
	if strings.TrimSpace(metadata.Key) != "" {
		return metadata.Key
	}
	if championID > 0 {
		return "英雄 " + strconv.FormatInt(championID, 10)
	}
	return ""
}

func specialistRuneFromParticipant(championID int64, championName string, player championTopPlayer, match *riotMatch, participant riotParticipant, index int) (gameplayRecommendationRune, bool) {
	selected := make([]int64, 0, 9)
	var primaryStyleID, subStyleID int64
	for styleIndex, style := range participant.Perks.Styles {
		description := strings.ToLower(strings.TrimSpace(style.Description))
		switch {
		case strings.Contains(description, "primary"):
			primaryStyleID = style.Style
		case strings.Contains(description, "sub") || strings.Contains(description, "secondary"):
			subStyleID = style.Style
		case styleIndex == 0 && primaryStyleID == 0:
			primaryStyleID = style.Style
		case subStyleID == 0:
			subStyleID = style.Style
		}
		for _, selection := range style.Selections {
			if selection.Perk > 0 {
				selected = append(selected, selection.Perk)
			}
		}
	}
	statMods := compactPositiveInt64(participant.Perks.StatPerks.Offense, participant.Perks.StatPerks.Flex, participant.Perks.StatPerks.Defense)
	items := itemSlots(participant.Item0, participant.Item1, participant.Item2, participant.Item3, participant.Item4, participant.Item5, participant.Item6)
	selected = append(selected, statMods...)
	request := gameplayRuneApplyRequest{
		ChampionName: championName, Source: "绝活哥", ChampionID: championID,
		PrimaryStyleID: primaryStyleID, SubStyleID: subStyleID, SelectedPerkIDs: selected,
	}
	if validateRuneApplyRequest(request) != nil {
		return gameplayRecommendationRune{}, false
	}
	tier, division := specialistTier(player.Tier)
	games := specialistGames(player.Games)
	riotID := strings.TrimSpace(player.Name) + "#" + strings.TrimSpace(player.Tagline)
	result := "loss"
	if riotMatchInfoIsRemake(&match.Info) {
		result = "remake"
	} else if participant.Win {
		result = "win"
	}
	winRate := player.WinRate
	gameCount := games
	return gameplayRecommendationRune{
		Key: "specialist-" + strconv.Itoa(index), Title: riotID, ChampionID: championID, ChampionName: championName,
		PrimaryStyleID: primaryStyleID, SubStyleID: subStyleID, SelectedPerkIDs: selected, StatModIDs: statMods, ItemIDs: items,
		Stats:      gameplayRecommendationStats{WinRate: &winRate, Games: &gameCount},
		PlayerName: strings.TrimSpace(player.Name), TagLine: strings.TrimSpace(player.Tagline), Tier: tier, Division: division,
		LeaguePoints: strings.TrimSpace(player.LP), ChampionGames: games, PlayedAt: normalizeEpochMillis(match.Info.GameCreation), Result: result, Region: riotRegionKR,
	}, true
}

func specialistTier(value string) (string, string) {
	parts := strings.Fields(strings.ToLower(strings.TrimSpace(value)))
	if len(parts) == 0 {
		return "", ""
	}
	division := ""
	if len(parts) > 1 {
		division = map[string]string{"1": "I", "2": "II", "3": "III", "4": "IV"}[parts[1]]
		if division == "" {
			division = strings.ToUpper(parts[1])
		}
	}
	return parts[0], division
}

func specialistGames(value string) int {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, value)
	games, _ := strconv.Atoi(digits)
	return games
}

func cloneSpecialistRunes(source []gameplayRecommendationRune) []gameplayRecommendationRune {
	result := make([]gameplayRecommendationRune, len(source))
	copy(result, source)
	for index := range result {
		result[index].SelectedPerkIDs = append([]int64(nil), source[index].SelectedPerkIDs...)
		result[index].StatModIDs = append([]int64(nil), source[index].StatModIDs...)
		result[index].ItemIDs = append([]int64(nil), source[index].ItemIDs...)
		if source[index].OpponentWinRate != nil {
			winRate := *source[index].OpponentWinRate
			result[index].OpponentWinRate = &winRate
		}
	}
	return result
}
