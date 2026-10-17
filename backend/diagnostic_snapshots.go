package main

import (
	"encoding/json"
	"strings"
	"time"
)

type diagnosticSnapshotWindow struct {
	at              time.Time
	count           int
	name, signature string
	metrics         map[string][]int64
}

func snapshotDiagnosticEvent(name string) bool {
	switch name {
	case "build_spells_client", "augment_description_client", "overview_card_ready", "overview_load_cost", "season_stats_head_refresh", "overview_phases_ms", "loot_name_fallback", "loot_metadata_source", "loot_category_assigned", "lcu_discovery", "sgp_match_filter_fallback", "overview_rerender_client", "recent_ranked_sample_fallback", "sgp_gateway_resolution", "loot_map_shape", "overview_background_read", "recent_players_resolved", "pro_seed_skipped", "lp_chip_render", "season_ranked_snapshot", "opgg_match_tiers_result", "opgg_match_tiers_cost", "sgp_game_decode_failed", "riot_local_rate_limited", "overview_art_client", "overview_art_source", "facade_identity_shape", "match_history_data_source_decision", "riot_overview_cost", "current_game_client", "r99_read_probe", "sgp_participant_keys", "overview_loadout_shape", "recent_ranked_sample_resolved", "ranked_data_source_decision", "social_presence_resolved", "sgp_match_filter_resolved", "ddragon_item_purchasability_shape":
		return true
	}
	return false
}
func snapshotMetric(key string) bool {
	return strings.HasPrefix(key, "transport_") && (key == "transport_suppressed" || key == "transport_failed" || key == "transport_dropped") || strings.HasSuffix(key, "_ms") || key == "durationMs" || key == "load_phases_ms"
}

// Unchanged observations preserve the first body, all repeat counts and timing distributions.
// Upstream request errors are excluded; a semantic change always gets a distinct signature.
func (a *app) snapshotDiagnostics(event map[string]any, now time.Time, flush bool) ([]map[string]any, bool) {
	a.snapshotDiagnosticMu.Lock()
	defer a.snapshotDiagnosticMu.Unlock()
	out := []map[string]any{}
	for key, b := range a.snapshotDiagnosticWindows {
		if flush || now.Sub(b.at) >= time.Hour {
			if b.count > 0 {
				row := map[string]any{"event": "diagnostic_snapshot_summary", "snapshot_event": b.name, "snapshot_signature": b.signature, "count": b.count, "window_start": b.at.UTC().Format(time.RFC3339)}
				if len(b.metrics) > 0 {
					metrics := map[string]any{}
					for name, samples := range b.metrics {
						constant := true
						for _, value := range samples {
							if value != samples[0] {
								constant = false
								break
							}
						}
						// Constant metrics retain the value in the first full snapshot.
						if constant {
							continue
						}
						summary := lcuMetricSummary(samples)
						delete(summary, "p90")
						metrics[name] = []int64{summary["p50"], summary["p95"], summary["max"]}
					}
					row["timing_ms"] = metrics
					row["timing_order"] = []string{"p50", "p95", "max"}
				}
				out = append(out, row)
			}
			delete(a.snapshotDiagnosticWindows, key)
		}
	}
	if flush || event == nil {
		return out, false
	}
	name, _ := event["event"].(string)
	if !snapshotDiagnosticEvent(name) {
		return out, false
	}
	shape := map[string]any{}
	metrics := map[string]int64{}
	for key, value := range event {
		if snapshotMetric(key) {
			if typed, ok := value.(map[string]int64); ok {
				for k, v := range typed {
					metrics[key+"."+k] = v
				}
				continue
			}
			if nested, ok := value.(map[string]any); ok {
				for k, v := range nested {
					metrics[key+"."+k] = int64(diagnosticInteger(v))
				}
			} else {
				metrics[key] = int64(diagnosticInteger(value))
			}
		} else {
			shape[key] = value
		}
	}
	raw, _ := json.Marshal(shape)
	signature := riotIdentityKey(string(raw))
	signature = signature[len(signature)-16:]
	key := name + ":" + signature
	if a.snapshotDiagnosticWindows == nil {
		a.snapshotDiagnosticWindows = map[string]*diagnosticSnapshotWindow{}
	}
	b := a.snapshotDiagnosticWindows[key]
	if b == nil {
		if len(a.snapshotDiagnosticWindows) >= 512 {
			return out, false
		}
		b = &diagnosticSnapshotWindow{at: now, name: name, signature: signature, metrics: map[string][]int64{}}
		a.snapshotDiagnosticWindows[key] = b
		full := map[string]any{}
		for k, v := range event {
			full[k] = v
		}

		out = append(out, full)
	} else {
		b.count++
	}
	for k, v := range metrics {
		if len(b.metrics[k]) < 1024 {
			b.metrics[k] = append(b.metrics[k], v)
		}
	}
	return out, true
}
func (a *app) flushSnapshotDiagnostics() {
	rows, _ := a.snapshotDiagnostics(nil, time.Now(), true)
	for _, row := range rows {
		a.appendDiagnosticEvent(row)
	}
}
