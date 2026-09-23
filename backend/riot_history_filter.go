package main

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func riotOverviewFilter(filters []string) string {
	if len(filters) == 0 {
		return "all"
	}
	return normalizeGameplayMatchFilter(filters[0])
}

func riotOverviewPagination(start, loaded, requested int, filter string) gameplayPagination {
	return gameplayPagination{BegIndex: start, Count: loaded, HasMore: loaded == requested,
		Filter: filter, ServerFiltered: filter != "all" && riotDirectMatchFilter(filter),
		FilterFallback: filter != "all" && !riotDirectMatchFilter(filter)}
}

// Ask Match v5 for IDs in the requested queues, never download unrelated
// matches to discover whether a mode has any history. Each queue prefix is
// cached by matchIDsFiltered; only the final page's details are downloaded.
// KR match IDs share a monotonically increasing platform game ID, allowing
// multiple queue streams to merge before fetching any match payloads.
const riotOverviewMaxIDRequests = 12

// riotOverviewMaxConcurrentIDRequests 限制一次分模式查询同时在途的 Riot ID 请求数。
// 具名而不是字面量：并发上界是工单 P1-1 的验收对象，测试要能直接引用同一个常量，
// 否则把容量改大（等于去掉限流）不会有任何测试察觉。
const riotOverviewMaxConcurrentIDRequests = 4

// KR match IDs share a monotonically increasing platform game ID, allowing
// multiple queue streams to merge before fetching any match payloads.
func (p *riotProvider) matchIDsForOverview(ctx context.Context, puuid string, start, count int, filter string) ([]string, error) {
	filter = normalizeGameplayMatchFilter(filter)
	if filter == "all" || !riotDirectMatchFilter(filter) {
		return p.matchIDs(ctx, puuid, start, count)
	}
	var queues []int64
	for _, definition := range supportedQueueDefinitions {
		if definition.Filter == filter || (filter == "ranked" && (definition.ID == 420 || definition.ID == 440)) {
			queues = append(queues, definition.ID)
		}
	}
	// Unknown queue families retain the bounded client-side fallback.
	if len(queues) == 0 {
		return p.matchIDs(ctx, puuid, start, count)
	}
	if len(queues) == 1 {
		return p.matchIDsFiltered(ctx, puuid, start, count, queues[0], "")
	}
	ids := make(map[string]struct{})
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var requestMu sync.Mutex
	issued := 0
	var idsMu sync.Mutex
	var firstErr error
	var errOnce sync.Once
	semaphore := make(chan struct{}, riotOverviewMaxConcurrentIDRequests)
	var workers sync.WaitGroup
	for _, queue := range queues {
		queue := queue
		workers.Add(1)
		go func() {
			defer workers.Done()
			for offset := 0; offset < start+count; offset += 100 {
				if workerCtx.Err() != nil {
					return
				}
				requestMu.Lock()
				if issued >= riotOverviewMaxIDRequests {
					requestMu.Unlock()
					return
				}
				issued++
				requestMu.Unlock()
				select {
				case semaphore <- struct{}{}:
				case <-workerCtx.Done():
					return
				}
				page, err := p.matchIDsFiltered(workerCtx, puuid, offset, 100, queue, "")
				<-semaphore
				if err != nil {
					errOnce.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
				idsMu.Lock()
				for _, id := range page {
					ids[id] = struct{}{}
				}
				idsMu.Unlock()
				if len(page) < 100 {
					return
				}
			}
		}()
	}
	workers.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	merged := make([]string, 0, len(ids))
	for id := range ids {
		merged = append(merged, id)
	}
	sort.Slice(merged, func(i, j int) bool { return riotGameID(merged[i]) > riotGameID(merged[j]) })
	if start >= len(merged) {
		return []string{}, nil
	}
	end := start + count
	if end > len(merged) {
		end = len(merged)
	}
	return merged[start:end], nil
}

func riotGameID(id string) int64 {
	_, number, _ := strings.Cut(id, "_")
	value, _ := strconv.ParseInt(number, 10, 64)
	return value
}

func riotDirectMatchFilter(filter string) bool {
	return filter != "more:hextech-classic" && filter != "more:hextech-qualifier" && filter != "more:special"
}
