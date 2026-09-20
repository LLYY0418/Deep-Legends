package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var arenaRiotMatchIDPattern = regexp.MustCompile(`^KR_[0-9]+$`)

func validArenaRiotMatchID(matchID string) bool {
	return arenaRiotMatchIDPattern.MatchString(matchID)
}

func (a *app) handleArenaMatchDetail(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	provider := a.championDataProvider()
	report := func(status int, cache, outcome, errorKind string) {
		provider.reportArenaMatchDetail(status, time.Since(started), cache, outcome, errorKind)
	}

	matchID := r.PathValue("matchId")
	if !validArenaRiotMatchID(matchID) {
		report(http.StatusBadRequest, "miss", "rejected", "invalid-match-id")
		http.Error(w, "对局编号无效", http.StatusBadRequest)
		return
	}
	if a.riot == nil {
		report(http.StatusServiceUnavailable, "miss", "error", "unavailable")
		http.Error(w, "Riot 对局详情服务暂不可用", http.StatusServiceUnavailable)
		return
	}

	raw, cacheState, err := a.riot.matchByIDWithCache(r.Context(), matchID)
	if err != nil {
		status, message, errorKind := arenaMatchDetailFailure(err)
		report(status, cacheState, "error", errorKind)
		http.Error(w, message, status)
		return
	}
	if raw == nil || raw.Metadata.MatchID != matchID {
		report(http.StatusBadGateway, cacheState, "error", "invalid-response")
		http.Error(w, "Riot 返回的对局详情无法验证", http.StatusBadGateway)
		return
	}

	a.checkArenaRiotMatchTruth(raw)
	match := riotConvertMatch(raw, "", a.riotChampionNames(r.Context()), a.riotQueueLabels(r.Context()))
	if match.GameID <= 0 || len(match.Participants) < 2 || (match.ModeGroup != "arena" && !strings.EqualFold(match.GameMode, "CHERRY")) {
		report(http.StatusBadGateway, cacheState, "error", "invalid-response")
		http.Error(w, "Riot 返回的竞技场详情不完整", http.StatusBadGateway)
		return
	}
	match.ModeGroup = "arena"
	if strings.TrimSpace(match.QueueLabel) == "" || match.QueueLabel == match.GameMode {
		match.QueueLabel = "斗魂竞技场"
	}
	a.publicizeMatchReferences(&match)
	report(http.StatusOK, cacheState, "success", "")
	respondJSON(w, match)
}

func arenaMatchDetailFailure(err error) (int, string, string) {
	value := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, context.Canceled):
		return http.StatusRequestTimeout, "Riot 对局详情请求已取消", "canceled"
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(value, "timeout") || strings.Contains(value, "deadline"):
		return http.StatusGatewayTimeout, "Riot 对局详情请求超时，请稍后重试", "timeout"
	case errors.Is(err, errRiotNotFound):
		return http.StatusNotFound, "Riot 未找到这场对局", "not-found"
	case strings.Contains(value, "429") || strings.Contains(value, "限流"):
		return http.StatusTooManyRequests, "Riot 接口限流中，请稍后重试", "rate-limited"
	case strings.Contains(value, "尚未配置 riot api key"):
		return http.StatusServiceUnavailable, "Riot 对局详情未启用：当前安装包没有注入 Riot API Key，重新构建时带上密钥即可展开韩服高手对局", "not-configured"
	case strings.Contains(value, "api key"):
		return http.StatusBadGateway, "Riot 对局详情服务认证失败", "forbidden"
	case strings.Contains(value, "无法解析") || strings.Contains(value, "缺少必要字段"):
		return http.StatusBadGateway, "Riot 返回的对局详情无法解析", "invalid-response"
	default:
		return http.StatusBadGateway, "Riot 对局详情读取失败，请稍后重试", "unavailable"
	}
}

func (p *championProvider) reportArenaMatchDetail(status int, duration time.Duration, cache, outcome, errorKind string) {
	if p == nil || p.diag == nil {
		return
	}
	if cache != "hit" {
		cache = "miss"
	}
	p.diag(map[string]any{
		"event": "arena_match_detail", "source": "riot", "status": status,
		"duration_ms": duration.Milliseconds(), "cache": cache, "outcome": outcome, "errorKind": errorKind,
	})
}
