package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
)

// Resolve the client identity before public history requests. The overview
// retains its LCU profile and season snapshot while public statistics share
// the same identity resolver.
func (a *app) loadClientRiotHistory(ctx context.Context, region, puuid string, begIndex, count int, filter string, names, labels map[int64]string, references ...gameplayReference) ([]gameplayMatch, gameplayPagination, error) {
	if a.riot == nil {
		return nil, gameplayPagination{}, errRiotKeyMissing
	}
	ref := gameplayReference{}
	if len(references) > 0 {
		ref = references[0]
	}
	clientPUUID := puuid
	a.mu.RLock()
	client := a.lcu
	a.mu.RUnlock()
	public, resolveErr := a.resolveClientRiotPUUID(ctx, client, region, puuid, ref)
	if resolveErr != nil {
		return nil, gameplayPagination{}, resolveErr
	}
	puuid = public
	p := a.riot.forPlatform(region)
	ids, err := p.matchIDsForOverview(ctx, puuid, begIndex, count, filter)
	if err != nil {
		return nil, gameplayPagination{}, err
	}
	if begIndex == 0 && normalizeGameplayMatchFilter(filter) == "all" {
		recordOverviewAllHistory(ctx, ids)
	}
	rows := make([]*riotMatch, len(ids))
	var firstErr error
	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, p.detailConcurrency())
	for i, id := range ids {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			defer a.recoverPanic("riot.clientHistory")
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-slots }()
			raw, err := p.matchByID(ctx, id)
			mu.Lock()
			defer mu.Unlock()
			rows[i] = raw
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}(i, id)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, gameplayPagination{}, ctx.Err()
	}
	// Failed detail pages fall back as a whole; never claim complete roster data
	// for a sparse list or advance a pagination cursor over missing matches.
	if firstErr != nil {
		return nil, gameplayPagination{}, firstErr
	}
	result := make([]gameplayMatch, 0, len(rows))
	for _, raw := range rows {
		if raw == nil {
			return nil, gameplayPagination{}, errRiotNotFound
		}
		m := riotConvertMatch(raw, puuid, names, labels)
		for i := range m.Participants {
			if m.Participants[i].PlayerRef == puuid {
				m.Participants[i].PlayerRef = clientPUUID
				m.Participants[i].reference.PlayerRef = clientPUUID
				m.Participants[i].reference.AlternatePlayerRef = puuid
				m.Participants[i].reference.ClientIdentity = true
			}
		}
		a.recordMatchScores(dataSourceRiot, m)
		if !isCustomGameplayMatch(m) {
			result = append(result, m)
		}
	}
	return result, riotOverviewPagination(begIndex, len(ids), count, filter), nil
}
func transientLCUHistoryError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	var status *LCUHTTPError
	if errors.As(err, &status) {
		return status.StatusCode >= 500 && status.StatusCode <= 599
	}
	var network net.Error
	return errors.As(err, &network) && network.Timeout() || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, context.DeadlineExceeded)
}
func getLCUHistoryWithRetry(ctx context.Context, client *LCUClient, path string, out any) error {
	delays := []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second}
	region, _ := clientRegionInfo(client)
	external := region != "TENCENT"
	for attempt := 0; ; attempt++ {
		err := client.GetJSONContext(ctx, path, out)
		if !external || attempt == len(delays) || !transientLCUHistoryError(ctx, err) {
			return err
		}
		client.recordRequestHistoryRetry(attempt+1, delays[attempt], err)
		sleep := client.historyRetrySleep
		if sleep == nil {
			sleep = waitRiotDelay
		}
		if err := sleep(ctx, delays[attempt]); err != nil {
			return err
		}
	}
}
func (c *LCUClient) recordRequestHistoryRetry(attempt int, delay time.Duration, err error) {
	c.diagnosticMu.Lock()
	observe := c.diagnosticObserve
	c.diagnosticMu.Unlock()
	if observe != nil {
		observe(map[string]any{"event": "lcu_history_retry", "client_region": clientRiotPlatform(c), "attempt": attempt, "delay_ms": delay.Milliseconds(), "reason": safeDiagnosticReason(err)})
	}
}

// Stable failure categories for source decisions. Never retain URL/account
// arguments from a network error in a history attempt.
func riotHistoryFailureMessage(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, errRiotKeyMissing):
		return "未配置 Riot Key"
	case errors.Is(err, errRiotRelayQuotaExhausted):
		return "Riot 中转当日额度已用尽"
	case errors.Is(err, errRiotNotFound):
		return "Riot 上游未找到数据（HTTP 404）"
	case errors.Is(err, context.DeadlineExceeded):
		return "Riot 接口响应超时"
	case riotErrorStatus(err) == http.StatusTooManyRequests:
		return "Riot 查询暂时限流（HTTP 429）"
	}
	var network net.Error
	if errors.As(err, &network) {
		if network.Timeout() {
			return "Riot 接口响应超时"
		}
		return "Riot 接口连接失败"
	}
	return safeDiagnosticReason(err)
}
