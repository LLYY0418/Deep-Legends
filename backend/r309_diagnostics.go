package main

import (
	"encoding/json"
	"math"
	"net/http"
)

func (a *app) recordR309Diagnostic(w http.ResponseWriter, r clientDiagnosticRequest) {
	var fields map[string]any
	if len(r.R278) > 2048 || json.Unmarshal(r.R278, &fields) != nil {
		http.Error(w, "invalid diagnostic", 400)
		return
	}
	clean := map[string]any{"event": r.Event, "reason": r.Reason}
	if r.Event == "mayhem_augment_detail_client" {
		id, idOK := fields["augment_id"].(float64)
		ms, msOK := fields["ms"].(float64)
		source, _ := fields["source"].(string)
		result, _ := fields["result"].(string)
		if !idOK || id <= 0 || id > 1000000 || id != math.Trunc(id) || !msOK || ms < 0 || ms > 3600000 || (source != "cache" && source != "network") || (result != "ok" && result != "error" && result != "stale") {
			http.Error(w, "invalid detail diagnostic", 400)
			return
		}
		clean["augment_id"] = int(id)
		clean["ms"] = ms
		clean["source"] = source
		clean["result"] = result
	}
	if r.Event == "filter_action_client" {
		action, _ := fields["action"].(string)
		if action != r.Reason || (action != "clear" && action != "remove" && action != "stop" && action != "change") {
			http.Error(w, "invalid action", 400)
			return
		}
		clean["action"] = action
		for _, key := range []string{"rows_loaded", "rows_rendered", "click_to_paint_ms", "longtask_ms"} {
			value, ok := fields[key].(float64)
			if !ok || value < 0 || value > 3600000 {
				http.Error(w, "invalid action metric", 400)
				return
			}
			clean[key] = value
		}
	}
	a.recordDiagnostic(clean)
	w.WriteHeader(http.StatusNoContent)
}
