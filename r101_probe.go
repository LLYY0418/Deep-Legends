package main

import (
	"context"
	"net/http"
	"sort"
	"time"
)

type facadeIconInventoryEntry struct {
	ItemID int64 `json:"itemId"`
	Owned  bool  `json:"owned"`
}

// Each attempt owns its restoration, including rejected writes and cancellation.
func r101IconAttempt(ctx context.Context, client *LCUClient, record func(map[string]any), label, method, path string, original, target int64) (status int, restored bool) {
	body := func(id int64) map[string]any {
		if method == http.MethodPost {
			return map[string]any{"key": "profileIconId", "value": id}
		}
		return map[string]any{"profileIconId": id}
	}
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		code, err := r99Request(restoreCtx, client, method, path, body(original), nil)
		if err != nil && method == http.MethodPost {
			code, err = r99Request(restoreCtx, client, http.MethodPut, "/lol-summoner/v1/current-summoner/icon", map[string]any{"profileIconId": original}, nil)
		}
		restored = err == nil
		record(map[string]any{"probe": label + "-restore", "status": code, "error_code": r99ProbeErrorCode(err), "restored": restored, "ok": restored})
	}()
	code, err := r99Request(ctx, client, method, path, body(target), nil)
	record(map[string]any{"probe": label, "status": code, "error_code": r99ProbeErrorCode(err), "ok": err == nil, "ownership": "owned", "scope": "profile"})
	return code, false
}

func (a *app) r101ProbeIcon(ctx context.Context, client *LCUClient, record func(map[string]any)) {
	var current struct {
		Icon *int64 `json:"profileIconId"`
	}
	var inventory []facadeIconInventoryEntry
	_, err := r99Request(ctx, client, http.MethodGet, "/lol-summoner/v1/current-summoner", nil, &current)
	if err != nil || current.Icon == nil {
		record(map[string]any{"probe": "W5", "skipped": true})
		return
	}
	_, err = r99Request(ctx, client, http.MethodGet, "/lol-inventory/v2/inventory/SUMMONER_ICON", nil, &inventory)
	sort.Slice(inventory, func(i, j int) bool { return inventory[i].ItemID < inventory[j].ItemID })
	var target int64
	for _, item := range inventory {
		if item.Owned && item.ItemID > 0 && item.ItemID != *current.Icon {
			target = item.ItemID
			break
		}
	}
	if err != nil || target == 0 {
		record(map[string]any{"probe": "W5", "skipped": true})
		return
	}
	status, restored := r101IconAttempt(ctx, client, record, "W5", http.MethodPut, "/lol-summoner/v1/current-summoner/icon", *current.Icon, target)
	if status != http.StatusUnauthorized || !restored {
		return
	}
	status, restored = r101IconAttempt(ctx, client, record, "W5-b", http.MethodPost, "/lol-summoner/v1/current-summoner/summoner-profile", *current.Icon, target)
	if status >= 200 && status < 300 || !restored {
		return
	}
	// W5-c is intentionally read-only until a real token response establishes its
	// schema and binding to the selected item. Never guess or log a token value.
	for _, version := range []string{"v2", "v1"} {
		var response map[string]any
		code, _ := r99Request(ctx, client, http.MethodGet, "/lol-inventory/"+version+"/signedInventory", nil, &response)
		a.recordDiagnostic(map[string]any{"event": "r99_read_probe", "probe": "W5-c-token-" + version, "status": code, "fields": r99Keys(response)})
	}
	record(map[string]any{"probe": "W5-c", "skipped": true})
}

func (a *app) r101ProbeBanner(ctx context.Context, client *LCUClient, record func(map[string]any)) {
	original, err := facadeBannerPreferences(ctx, client)
	if err != nil {
		record(map[string]any{"probe": "W6", "skipped": true})
		return
	}
	var inventory map[string]facadeBannerInventoryEntry
	if client.RequestJSON(ctx, http.MethodGet, facadeBannerInventoryPath, nil, &inventory) != nil {
		record(map[string]any{"probe": "W6", "skipped": true})
		return
	}
	var catalog []facadeRegalia
	if client.RequestJSON(ctx, http.MethodGet, "/lol-game-data/assets/v1/regalia.json", nil, &catalog) != nil {
		record(map[string]any{"probe": "W6", "skipped": true})
		return
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].ID < catalog[j].ID })
	target := ""
	for _, item := range catalog {
		if inventory[item.ID].Owned && item.Type == "kBanner" && item.Selectable && item.ID != "1" && item.ID != "2" && item.ID != "" && item.ID != facadeBannerIdentity(original["bannerAccent"]) {
			target = item.ID
			break
		}
	}
	if target == "" {
		record(map[string]any{"probe": "W6", "skipped": true})
		return
	}
	variant := make(map[string]any, len(original))
	for key, value := range original {
		variant[key] = value
	}
	variant["bannerAccent"] = target
	defer func() {
		restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		status, err := writeFacadeBannerPreferencesResult(restoreCtx, client, original)
		record(map[string]any{"probe": "W6-restore", "ok": err == nil, "restored": err == nil, "error_code": r99ProbeErrorCode(err), "status": status})
	}()
	status, err := writeFacadeBannerPreferencesResult(ctx, client, variant)
	record(map[string]any{"probe": "W6", "method": "POST", "surface": "challenge-preferences", "banner_id": target, "contract": "catalog-id", "ownership": "owned", "changed": err == nil, "ok": err == nil, "error_code": r99ProbeErrorCode(err), "status": status})
}
