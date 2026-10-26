package main

import (
	"encoding/json"
	"net/http"
	"regexp"
)

var resizeDiagnosticColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}([0-9a-fA-F]{2})?$`)

func validResizeDiagnosticTheme(theme string) bool {
	switch theme {
	case "", "auto", "light", "dark", "azure", "emerald", "violet", "crimson", "aurora", "oled":
		return true
	default:
		return false
	}
}

type resizeBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (a *app) recordWindowResizeTrace(w http.ResponseWriter, r clientDiagnosticRequest) {
	var v struct {
		NativeBackground string        `json:"nativeBackground"`
		Theme            string        `json:"theme"`
		ResizingClass    bool          `json:"resizingClass"`
		ZoomChanges      int           `json:"zoomChanges"`
		SidebarChanges   int           `json:"sidebarChanges"`
		Reason           string        `json:"-"`
		Drag             int64         `json:"drag"`
		Sample           int           `json:"sample"`
		Bounds           resizeBounds  `json:"bounds"`
		ContentBounds    resizeBounds  `json:"contentBounds"`
		NextBounds       *resizeBounds `json:"nextBounds,omitempty"`
		Minimum          [2]int        `json:"minimum"`
		Zoom             float64       `json:"zoom"`
		InnerWidth       int           `json:"innerWidth"`
		InnerHeight      int           `json:"innerHeight"`
		NextFrameMS      float64       `json:"nextFrameMs"`
		LongtaskCount    int           `json:"longtaskCount"`
		LongtaskMaxMS    float64       `json:"longtaskMaxMs"`
	}
	if len(r.ResizeTrace) == 0 || json.Unmarshal(r.ResizeTrace, &v) != nil || v.Sample < 1 || v.Sample > 60 || v.Drag <= 0 || v.Zoom < .1 || v.Zoom > 8 || v.InnerWidth < 0 || v.InnerWidth > 100000 || v.NextFrameMS < 0 || v.NextFrameMS > 60000 || v.ZoomChanges < 0 || v.SidebarChanges < 0 || v.LongtaskCount < 0 || v.LongtaskMaxMS < 0 || v.LongtaskMaxMS > 60000 || v.NativeBackground != "" && !resizeDiagnosticColor.MatchString(v.NativeBackground) || !validResizeDiagnosticTheme(v.Theme) {
		http.Error(w, "invalid resize trace", 400)
		return
	}
	a.recordDiagnostic(map[string]any{"event": "window_resize_trace", "reason": r.Reason, "trace": v})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) recordChampionDetailRestore(w http.ResponseWriter, r clientDiagnosticRequest) {
	var v struct {
		Source     string `json:"source"`
		Mode       string `json:"mode"`
		ChampionID int    `json:"championId"`
	}
	if json.Unmarshal(r.ChampionRestore, &v) != nil || v.ChampionID <= 0 || v.ChampionID > 100000 || (v.Source != "session" && v.Source != "open-champion" && v.Source != "user") || (v.Mode != "ranked" && v.Mode != "aram-mayhem" && v.Mode != "arena") {
		http.Error(w, "invalid champion restore", 400)
		return
	}
	a.recordDiagnostic(map[string]any{"event": "champion_detail_restore", "source": v.Source, "mode": v.Mode, "champion_id": v.ChampionID})
	w.WriteHeader(http.StatusNoContent)
}
