package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Probe the same credential on a platform endpoint, never through a relay.
// A request-specific 403 does not say anything about the credential's validity.
func (p *riotProvider) confirmRiotCredential(ctx context.Context, key, source string) bool {
	s := riotUserKeys
	s.mu.Lock()
	if done := s.authProbeDone; done != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return false
		case <-done:
		}
		s.mu.Lock()
		current, currentSource := s.effectiveLocked()
		healthy := current == key && currentSource == source && !s.embeddedRejected && s.invalidKey != key
		s.mu.Unlock()
		return healthy
	}
	if source == "embedded" && s.embeddedRejected && s.authNowLocked().Before(s.authUntil) {
		s.mu.Unlock()
		return false
	}
	done := make(chan struct{})
	s.authProbeDone = done
	s.mu.Unlock()
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	probe = context.WithValue(probe, riotPinnedCredentialKey{}, riotPinnedCredential{key: key, source: source})
	probe = context.WithValue(probe, riotAuthProbeKey{}, true)
	probe = context.WithValue(probe, riotRequestSourceKey{}, "key-confirmation")
	probe = context.WithValue(probe, riotRouteKey{}, "direct")
	var data any
	err := p.getLimitedRoute(probe, p.platformHost(), "/lol/status/v4/platform-data", nil, &data, riotResponseMax)
	cancel()
	var status *riotStatusError
	rejected := errors.As(err, &status) && (status.status == 401 || status.status == 403)
	s.mu.Lock()
	current, currentSource := s.effectiveLocked()
	same := current == key && currentSource == source || source == "embedded" && s.key == "" && strings.TrimSpace(os.Getenv("RIOT_API_KEY")) == "" && riotEmbeddedKey() == key
	if same {
		if source == "embedded" {
			if rejected {
				s.embeddedRejected = true
				s.authUntil = s.authNowLocked().Add(10 * time.Minute)
			} else if err == nil {
				s.embeddedRejected = false
				s.authUntil = time.Time{}
			} else if s.embeddedRejected {
				s.authUntil = s.authNowLocked().Add(10 * time.Second)
			}
		} else if rejected {
			s.invalidKey = key
		}
	}
	s.authProbeDone = nil
	close(done)
	s.mu.Unlock()
	if rejected && same {
		p.routeRecord(map[string]any{"event": "riot_key_rejected", "source": source, "status": status.status, "confirmed": true, "cooldown_s": 600})
	}
	if err == nil && same {
		s.observe(key, 200)
		p.routeRecord(map[string]any{"event": "riot_key_confirmation", "source": source, "result": "accepted"})
	}
	return err == nil && same
}

func riotPathShape(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) >= 5 && parts[0] == "lol" && parts[1] == "match" && parts[2] == "v5" && parts[3] == "matches" && parts[4] != "by-puuid" {
		id := strings.SplitN(parts[4], "_", 2)
		if len(id) == 2 {
			platform := strings.ToUpper(id[0])
			allowed := false
			for _, v := range []string{"BR1", "EUN1", "EUW1", "JP1", "KR", "LA1", "LA2", "ME1", "NA1", "OC1", "RU", "TR1", "SG2", "TW2", "VN2", "PH2", "TH2"} {
				if platform == v {
					allowed = true
				}
			}
			digits := true
			for _, c := range id[1] {
				if c < '0' || c > '9' {
					digits = false
				}
			}
			if allowed && digits && len(id[1]) > 0 {
				suffix := ""
				if len(parts) > 5 && parts[5] == "timeline" {
					suffix = "/timeline"
				}
				return fmt.Sprintf("match/v5/matches/%s_{digits:%d}%s", platform, len(id[1]), suffix)
			}
		}
		return "match/v5/matches/{invalid-id}"
	}
	if strings.Contains(path, "/accounts/") {
		return "account/v1/accounts/{identity}"
	}
	if strings.HasSuffix(path, "/ids") {
		return "match/v5/matches/by-puuid/{identity}/ids"
	}
	if strings.Contains(path, "/summoners/") {
		return "summoner/v4/summoners/{identity}"
	}
	if path == "/lol/status/v4/platform-data" {
		return "status/v4/platform-data"
	}
	return "other"
}

func riotPublicMatchEligible(match gameplayMatch) bool {
	return match.GameID > 0 && match.QueueID > 0 && !isCustomGameplayMatch(match) && !strings.Contains(strings.ToUpper(match.GameMode), "PRACTICE") && !strings.Contains(strings.ToUpper(match.GameMode), "TUTORIAL") && !strings.Contains(strings.ToUpper(match.GameType), "TUTORIAL")
}
