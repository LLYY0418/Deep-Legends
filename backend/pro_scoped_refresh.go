package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
)

const proProfileRefreshInterval = 3 * time.Hour

func proProfileRetry(failures int) time.Duration {
	switch failures {
	case 0, 1:
		return 5 * time.Minute
	case 2:
		return 15 * time.Minute
	case 3:
		return time.Hour
	default:
		return 3 * time.Hour
	}
}
func proProfileDue(row opggProAccount, now time.Time) bool {
	at, err := time.Parse(time.RFC3339Nano, row.CheckedAt)
	return err != nil || now.Sub(at) < 0 || now.Sub(at) >= proProfileTTL(row)
}
func (a *app) proProfileNow() time.Time {
	if a.proProfiles.now != nil {
		return a.proProfiles.now()
	}
	return time.Now()
}
func (a *app) initializeProProfileCache() {
	c := &a.proProfiles
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = map[string]opggProAccount{}
		c.disk = newPublicBinaryCache(a.storage, "pro-profiles", 64, 4<<20)
	}
	if c.inflight == nil {
		c.inflight = map[string]bool{}
		c.manual = map[string]time.Time{}
		c.slots = make(chan struct{}, 2)
	}
	if c.identities == nil {
		c.identities = map[string]string{}
		if a.storage != nil {
			if raw, err := readLocalStoreFile(a.storage, "pro-account-identities.json"); err == nil && len(raw) <= 128<<10 {
				_ = json.Unmarshal(raw, &c.identities)
				if c.identities == nil {
					c.identities = map[string]string{}
				}
			}
		}
	}
}
func (a *app) proOfficialIdentity(ctx context.Context, row opggProAccount) (string, error) {
	a.initializeProProfileCache()
	key := proLadderAccountKey(row.GameName, row.TagLine)
	c := &a.proProfiles
	c.mu.Lock()
	identity := c.identities[key]
	c.mu.Unlock()
	if identity != "" {
		return identity, nil
	}
	account, err := a.riot.forPlatform("kr").accountByRiotID(ctx, row.GameName, row.TagLine)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.identities[key] = account.PUUID
	if a.storage != nil {
		raw, _ := json.Marshal(c.identities)
		if err = writeLocalStoreFile(a.storage, "pro-account-identities.json", raw); err != nil {
			return "", err
		}
	}
	return account.PUUID, nil
}

type proFailureKey struct{}
type proFailureBatch struct {
	mu     sync.Mutex
	counts map[proFailure]int
}
type proFailure struct {
	Stage, Kind string
	Status      int
}

func recordProProfileFailure(ctx context.Context, stage string, err error) {
	batch, _ := ctx.Value(proFailureKey{}).(*proFailureBatch)
	if batch == nil || err == nil {
		return
	}
	kind := "parse"
	status := 0
	var statusError *riotStatusError
	if errors.As(err, &statusError) {
		status = statusError.status
	}
	if errors.Is(err, context.DeadlineExceeded) {
		kind = "timeout"
	} else if status > 0 {
		kind = "http_status"
	} else if diagnosticErrorKind(err) == "timeout" {
		kind = "timeout"
	}
	batch.mu.Lock()
	batch.counts[proFailure{stage, kind, status}]++
	batch.mu.Unlock()
}
func (a *app) handleProScopedRefresh(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Team     string   `json:"team"`
		Accounts []string `json:"accounts"`
		Manual   bool     `json:"manual"`
	}
	if decodeJSONRequest(r, &body, 8192) != nil || len(body.Accounts) > 64 {
		http.Error(w, "invalid refresh", 400)
		return
	}
	allowed := body.Team == "all"
	for _, team := range proRoster {
		if strings.EqualFold(team.Code, body.Team) {
			allowed = true
		}
	}
	if !allowed {
		http.Error(w, "invalid team", 400)
		return
	}
	a.initializeProProfileCache()
	teams, _, _ := a.proPlayersFirstScreen(false)
	reviewed := a.buildReviewedProPlayers(teams)
	selected := map[string]bool{}
	for _, team := range reviewed.Teams {
		if body.Team != "all" && !strings.EqualFold(team.Code, body.Team) {
			continue
		}
		for _, player := range team.Players {
			for _, account := range player.Accounts {
				selected[proLadderAccountKey(account.GameName, account.TagLine)] = true
			}
		}
	}
	if body.Team == "all" {
		visible := map[string]bool{}
		for _, key := range body.Accounts {
			visible[strings.ToLower(key)] = true
		}
		for key := range selected {
			if !visible[key] {
				delete(selected, key)
			}
		}
	}
	c := &a.proProfiles
	now := a.proProfileNow()
	c.mu.Lock()
	if body.Manual && !c.manual["all"].IsZero() && now.Sub(c.manual["all"]) < time.Minute {
		c.mu.Unlock()
		http.Error(w, "refresh cooldown", 429)
		return
	}
	if body.Manual {
		c.manual["all"] = now
	}
	rows := map[string]opggProAccount{}
	skipped := 0
	for _, team := range teams {
		for _, member := range team.Members {
			for _, row := range member.Summoners {
				id := proLadderAccountKey(row.GameName, row.TagLine)
				if !selected[id] {
					continue
				}
				key := "pro-profile-v2|" + id
				if cached, ok := c.entries[key]; ok {
					row = cached
				}
				if c.inflight[key] || !body.Manual && !proProfileDue(row, now) {
					skipped++
					continue
				}
				c.inflight[key] = true
				rows[key] = row
			}
		}
	}
	c.mu.Unlock()
	a.recordDiagnostic(map[string]any{"event": "pro_refresh_plan", "due": len(rows), "skipped_fresh": skipped, "visible": len(selected), "manual": body.Manual})
	if len(rows) > 0 {
		a.goSafe("pro-scoped-refresh", func() { a.refreshProScope(rows, body.Manual) })
	}
	respondJSON(w, map[string]any{"updating": len(rows) > 0, "due": len(rows)})
}

type proManualRefreshKey struct{}

func (a *app) refreshProScope(rows map[string]opggProAccount, manual bool) {
	background := a.proBusinessContext()
	if background == nil {
		background = context.Background()
	}
	ctx, cancel := context.WithTimeout(withRiotBackground(background), 60*time.Second)
	defer cancel()
	batch := &proFailureBatch{counts: map[proFailure]int{}}
	ctx = context.WithValue(ctx, proFailureKey{}, batch)
	ctx = context.WithValue(ctx, proManualRefreshKey{}, manual)
	jobs := make(chan struct {
		key string
		row opggProAccount
	})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer a.recoverPanic("pro-scoped-worker")
			for job := range jobs {
				func() {
					defer func() { a.proProfiles.mu.Lock(); delete(a.proProfiles.inflight, job.key); a.proProfiles.mu.Unlock() }()
					select {
					case a.proProfiles.slots <- struct{}{}:
					case <-ctx.Done():
						return
					}
					defer func() { <-a.proProfiles.slots }()
					if ctx.Err() != nil {
						return
					}
					row := a.readProProfile(ctx, job.row)
					if row.ActivityPending {
						cancel()
					}
					a.proPlayers.mu.Lock()
					snapshot := cloneProTeams(a.proPlayers.teams)
					for ti := range snapshot {
						for mi := range snapshot[ti].Members {
							for ai := range snapshot[ti].Members[mi].Summoners {
								r := &snapshot[ti].Members[mi].Summoners[ai]
								if proLadderAccountKey(r.GameName, r.TagLine) == proLadderAccountKey(row.GameName, row.TagLine) {
									*r = row
								}
							}
						}
					}
					a.proPlayers.teams = snapshot
					a.proPlayers.fetchedAt = a.proProfileNow()
					a.persistProSnapshotLocked()
					a.proPlayers.mu.Unlock()
				}()
			}
		}()
	}
sendJobs:
	for key, row := range rows {
		select {
		case jobs <- struct {
			key string
			row opggProAccount
		}{key, row}:
		case <-ctx.Done():
			break sendJobs
		}
	}
	close(jobs)
	wg.Wait()
	a.proProfiles.mu.Lock()
	for key := range rows {
		delete(a.proProfiles.inflight, key)
	}
	a.proProfiles.mu.Unlock()
	for failure, count := range batch.counts {
		a.recordDiagnostic(map[string]any{"event": "pro_profile_failure", "stage": failure.Stage, "kind": failure.Kind, "http_status": failure.Status, "count": count})
	}
}
