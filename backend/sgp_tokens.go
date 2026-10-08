package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type sgpTokenFlightKey struct {
	client *LCUClient
	kind   sgpTokenKind
}
type sgpTokenFlight struct {
	done  chan struct{}
	token string
	err   error
}

func (p *sgpProvider) tokenLocked(client *LCUClient, kind sgpTokenKind) string {
	if kind == sgpTokenLeagueSession {
		if p.sessionOwner == client {
			return p.sessionToken
		}
	} else if p.tokenClient == client {
		return p.token
	}
	return ""
}

func (p *sgpProvider) setTokenLocked(client *LCUClient, kind sgpTokenKind, token string) {
	if kind == sgpTokenLeagueSession {
		p.sessionOwner, p.sessionToken, p.sessionAt = client, token, time.Now()
	} else {
		p.tokenClient, p.token, p.tokenAt = client, token, time.Now()
	}
}

// Tokens remain hot until an event or an authentication rejection invalidates them.
// In-flight readers share one LCU read; a newer event always wins over that read.
func (p *sgpProvider) cachedToken(ctx context.Context, client *LCUClient, kind sgpTokenKind, force bool) (string, error) {
	if p == nil || client == nil {
		return "", errors.New("SGP 令牌暂不可用")
	}
	if overviewCostFromContext(ctx) != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 1500*time.Millisecond)
		defer cancel()
	}
	key := sgpTokenFlightKey{client, kind}
	p.mu.Lock()
	if token := p.tokenLocked(client, kind); token != "" && !force {
		p.mu.Unlock()
		return token, nil
	}
	if flight := p.tokenFlights[key]; flight != nil {
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-flight.done:
			return flight.token, flight.err
		}
	}
	if p.tokenFlights == nil {
		p.tokenFlights = map[sgpTokenFlightKey]*sgpTokenFlight{}
	}
	flight := &sgpTokenFlight{done: make(chan struct{})}
	p.tokenFlights[key] = flight
	p.tokenEpoch[kind]++
	epoch := p.tokenEpoch[kind]
	if force {
		p.setTokenLocked(client, kind, "")
	}
	p.mu.Unlock()
	var token string
	var err error
	if kind == sgpTokenLeagueSession {
		err = p.readTokenJSON(ctx, client, "/lol-league-session/v1/league-session-token", &token)
	} else {
		var payload struct {
			AccessToken string `json:"accessToken"`
		}
		err = p.readTokenJSON(ctx, client, "/entitlements/v1/token", &payload)
		token = payload.AccessToken
	}
	if err == nil && strings.TrimSpace(token) == "" {
		p.recordEmptyToken(ctx, kind.diagnosticName())
		if kind == sgpTokenEntitlements {
			err = errors.New("客户端 SGP 令牌端点响应成功，但 accessToken 字段为空")
		} else {
			err = errors.New("客户端返回的 league-session 令牌为空")
		}
	}
	p.mu.Lock()
	if p.tokenEpoch[kind] != epoch {
		token = p.tokenLocked(client, kind)
		if token == "" {
			err = errors.New("SGP 令牌读取期间身份发生变化")
		} else {
			err = nil
		}
	} else if err == nil {
		p.setTokenLocked(client, kind, token)
	}
	flight.token, flight.err = token, err
	delete(p.tokenFlights, key)
	close(flight.done)
	p.mu.Unlock()
	return token, err
}

func (p *sgpProvider) observeTokenEvent(client *LCUClient, event LCUEvent) {
	if p == nil {
		return
	}
	kind := sgpTokenEntitlements
	var token string
	switch strings.ToLower(event.URI) {
	case "/entitlements/v1/token":
		var payload struct {
			AccessToken string `json:"accessToken"`
		}
		if !strings.EqualFold(event.EventType, "Delete") && json.Unmarshal(event.Data, &payload) != nil {
			return
		}
		token = payload.AccessToken
	case "/lol-league-session/v1/league-session-token":
		kind = sgpTokenLeagueSession
		if !strings.EqualFold(event.EventType, "Delete") && json.Unmarshal(event.Data, &token) != nil {
			return
		}
	default:
		return
	}
	p.mu.Lock()
	p.tokenEpoch[kind]++
	p.setTokenLocked(client, kind, token)
	p.mu.Unlock()
}

func (a *app) warmSGPTokens(ctx context.Context, client *LCUClient) {
	if a.sgp == nil {
		return
	}
	for _, kind := range []sgpTokenKind{sgpTokenEntitlements, sgpTokenLeagueSession} {
		a.goSafe("sgp-token-warm", func() {
			warmCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			for warmCtx.Err() == nil {
				readCtx, stop := context.WithTimeout(warmCtx, 1500*time.Millisecond)
				_, err := a.sgp.cachedToken(readCtx, client, kind, false)
				stop()
				if err == nil {
					a.sgp.mu.Lock()
					ready := a.sgp.tokenLocked(client, sgpTokenEntitlements) != "" && a.sgp.tokenLocked(client, sgpTokenLeagueSession) != ""
					a.sgp.mu.Unlock()
					if ready {
						a.observeColdLaunchMilestone("sgp_token_ms", time.Now())
					}
					return
				}
				a.recordDiagnostic(map[string]any{"event": "sgp_token_warm", "kind": kind.diagnosticName(), "result": "retry", "error_kind": diagnosticErrorKind(err)})
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-warmCtx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		})
	}
}
