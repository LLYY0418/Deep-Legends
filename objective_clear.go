package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Matches LeagueAkari's MissionsHttpApi.putPlayer (IdsDTO), NOT the
// /player/{missionId} rewardGroups selection API. Only explicit unseen records
// are submitted; never submit an empty body or guess IDs from diagnostic hashes.
type objectiveReadRecord struct {
	ID         string `json:"id"`
	Viewed     *bool  `json:"viewed"`
	IsNew      bool   `json:"isNew"`
	Objectives []struct {
		Progress struct {
			Current *float64 `json:"currentProgress"`
			Viewed  *float64 `json:"lastViewedProgress"`
		} `json:"progress"`
	} `json:"objectives"`
}

func objectiveUnreadIDs(raw []byte) ([]string, error) {
	var rows []objectiveReadRecord
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, fmt.Errorf("客户端未返回目标列表，未执行清空")
	}
	if len(rows) > 4096 {
		return nil, fmt.Errorf("目标列表过大，未执行清空")
	}
	ids := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		unread := (row.Viewed != nil && !*row.Viewed) || row.IsNew
		for _, objective := range row.Objectives {
			progress := objective.Progress
			if progress.Current != nil && progress.Viewed != nil && *progress.Current > *progress.Viewed {
				unread = true
			}
		}
		if unread {
			if !safeLCUIdentifier(row.ID) {
				return nil, fmt.Errorf("目标标识异常，未执行清空")
			}
			if !seen[row.ID] {
				ids = append(ids, row.ID)
				seen[row.ID] = true
			}
		}
	}
	return ids, nil
}

func (a *app) clearObjectiveBadge(ctx context.Context, client *LCUClient) error {
	// Serialize manual read-mark-read attempts independently of diagnostic GETs.
	client.objectiveClearMu.Lock()
	defer client.objectiveClearMu.Unlock()
	d := &client.objectiveDiagnostics
	d.mu.Lock()
	if d.trace == "" {
		nonce := make([]byte, 12)
		if _, err := rand.Read(nonce); err != nil {
			d.mu.Unlock()
			return err
		}
		d.trace = hex.EncodeToString(nonce)
	}
	if d.last == nil {
		d.last = map[string]string{}
	}
	d.until = time.Now().Add(2 * time.Minute)
	d.events = 0
	trace := d.trace
	d.mu.Unlock()
	probe := func(phase, method, path string, err error) {
		event := map[string]any{"event": "objective_badge_clear_probe", "trace_id": trace, "phase": phase, "method": method, "path": path, "ok": err == nil}
		var httpErr *LCUHTTPError
		if errors.As(err, &httpErr) {
			event["status_code"] = httpErr.StatusCode
		}
		a.recordDiagnostic(event)
	}
	a.recordDiagnostic(map[string]any{"event": "objective_badge_capture", "trace_id": trace, "source": "clear", "mode": "mark-read-and-verify", "event_window_seconds": 120})
	paths := []string{"/lol-missions/v1/missions", "/lol-missions/v1/series"}
	read := func(phase string) ([][]string, error) {
		ids := make([][]string, 2)
		for i, path := range paths {
			raw, err := client.getBytes(ctx, path, 2<<20, "application/json")
			probe(phase, http.MethodGet, path, err)
			if err != nil {
				return nil, err
			}
			a.recordObjectiveState(client, path, phase, "snapshot", raw)
			ids[i], err = objectiveUnreadIDs(raw)
			if err != nil {
				return nil, err
			}
		}
		a.recordDiagnostic(map[string]any{"event": "objective_badge_clear", "phase": phase, "mission_count": len(ids[0]), "series_count": len(ids[1])})
		return ids, nil
	}
	ids, err := read("before")
	if err != nil {
		return err
	}
	if len(ids[0])+len(ids[1]) == 0 {
		return nil
	}
	body := map[string]any{"missionIds": ids[0], "seriesIds": ids[1]}
	err = client.RequestJSON(ctx, http.MethodPut, "/lol-missions/v1/player", body, nil)
	probe("write", http.MethodPut, "/lol-missions/v1/player", err)
	a.recordDiagnostic(map[string]any{"event": "objective_badge_clear", "phase": "write", "method": http.MethodPut, "path": "/lol-missions/v1/player", "body_fields": []string{"missionIds", "seriesIds"}, "mission_count": len(ids[0]), "series_count": len(ids[1]), "ok": err == nil})
	if err != nil {
		return err
	}
	// Poll readback only, never repeat a write just because client state lags.
	targets := []map[string]bool{{}, {}}
	for i, rows := range ids {
		for _, id := range rows {
			targets[i][id] = true
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			timer := time.NewTimer(350 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		after, readErr := read("after")
		if readErr != nil {
			return fmt.Errorf("已提交已读请求，但复核失败：%w", readErr)
		}
		remaining := 0
		for i, rows := range after {
			for _, id := range rows {
				if targets[i][id] {
					remaining++
				}
			}
		}
		if remaining == 0 {
			return nil
		}
	}
	return fmt.Errorf("已提交已读请求，但客户端仍返回未读状态；请打开目标面板确认并导出诊断日志，不会自动重复写入")
}
