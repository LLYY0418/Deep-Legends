package main

import "context"

func (a *app) displayChampionNames(ctx context.Context, endpoint string) map[int64]string {
	names := a.overviewChampionNames(ctx)
	if len(names) == 0 {
		if _, loaded := a.championNameEmptyEndpoints.LoadOrStore(endpoint, true); !loaded {
			a.recordDiagnostic(map[string]any{"event": "champion_names_empty", "endpoint": endpoint, "source": "overview-catalog"})
		}
	}
	return names
}

func sanitizeCopyErrorName(name string) string {
	switch name {
	case "NotAllowedError", "SecurityError", "NotFoundError", "AbortError", "TypeError", "Error":
		return name
	}
	return ""
}
