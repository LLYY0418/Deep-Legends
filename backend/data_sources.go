package main

import "strings"

const (
	dataSourceLCU     = "lcu"
	dataSourceSGP     = "sgp"
	dataSourceRiot    = "riot"
	dataSourceOPGG    = "opgg"
	dataSourceQQ101   = "qq101"
	dataSourceARAMKit = "aramkit"

	dataSourceSuccess         = "success"
	dataSourceFailed          = "failed"
	dataSourceDisabled        = "disabled"
	dataSourceModeUnsupported = "mode-unsupported"
)

// DataSourceAttempt keeps cross-source fallback explicit. Different sources
// may expose different datasets, so callers must record why each compatible
// source was attempted instead of treating source changes as transparent retry.
type DataSourceAttempt struct {
	Source  string `json:"source"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
}

type rankDataSourceInput struct {
	PlayerReferenceValid bool
	LCUConnected         bool
	RemoteServer         bool
	SGPAvailable         bool
}

type matchHistoryDataSourceInput struct {
	PlayerReferenceValid bool
	LCUConnected         bool
	RemoteServer         bool
	SGPAvailable         bool
}

type timelineDataSourceInput struct {
	LCUConnected bool
	RemoteServer bool
	SGPAvailable bool
}

type dataSourceDecision struct {
	Sources []string
	Reason  string
}

func resolveRankDataSources(input rankDataSourceInput) dataSourceDecision {
	if !input.PlayerReferenceValid {
		return dataSourceDecision{Reason: "invalid-player-reference"}
	}
	sources := make([]string, 0, 2)
	if input.LCUConnected && !input.RemoteServer {
		sources = append(sources, dataSourceLCU)
	}
	if input.SGPAvailable {
		sources = append(sources, dataSourceSGP)
	}
	reason := "no-compatible-source"
	switch {
	case len(sources) > 0 && sources[0] == dataSourceLCU:
		reason = "local-client-connected"
	case input.RemoteServer && input.SGPAvailable:
		reason = "cross-server-lcu-unavailable"
	case input.SGPAvailable:
		reason = "lcu-unavailable"
	}
	return dataSourceDecision{Sources: sources, Reason: reason}
}

// Match history prefers SGP because its SUMMARY response contains the complete
// roster. LCU is a compatible fallback only for the currently connected server;
// switching sources is therefore an explicit decision, not an HTTP retry.
func resolveMatchHistoryDataSources(input matchHistoryDataSourceInput) dataSourceDecision {
	if !input.PlayerReferenceValid {
		return dataSourceDecision{Reason: "invalid-player-reference"}
	}
	sources := make([]string, 0, 2)
	if input.SGPAvailable {
		sources = append(sources, dataSourceSGP)
	}
	if input.LCUConnected && !input.RemoteServer {
		sources = append(sources, dataSourceLCU)
	}
	reason := "no-compatible-source"
	switch {
	case len(sources) > 0 && sources[0] == dataSourceSGP && input.RemoteServer:
		reason = "cross-server-sgp-only"
	case len(sources) > 0 && sources[0] == dataSourceSGP:
		reason = "sgp-complete-roster"
	case len(sources) > 0 && sources[0] == dataSourceLCU:
		reason = "sgp-unavailable"
	}
	return dataSourceDecision{Sources: sources, Reason: reason}
}

func resolveTimelineDataSources(input timelineDataSourceInput) dataSourceDecision {
	sources := make([]string, 0, 2)
	if input.LCUConnected && !input.RemoteServer {
		sources = append(sources, dataSourceLCU)
	}
	if input.SGPAvailable {
		sources = append(sources, dataSourceSGP)
	}
	reason := "no-compatible-source"
	switch {
	case len(sources) > 0 && sources[0] == dataSourceLCU:
		reason = "local-client-connected"
	case input.RemoteServer && input.SGPAvailable:
		reason = "cross-server-lcu-unavailable"
	case input.SGPAvailable:
		reason = "lcu-unavailable"
	}
	return dataSourceDecision{Sources: sources, Reason: reason}
}

func sourceScopedKey(source, key string) string {
	return strings.ToLower(strings.TrimSpace(source)) + ":" + key
}

func capabilitySource(capability EndpointCapability) string {
	path := strings.ToLower(strings.TrimSpace(capability.Path))
	switch {
	case strings.HasPrefix(path, dataSourceLCU+":"), strings.HasPrefix(path, "/lol-"):
		return dataSourceLCU
	case strings.HasPrefix(path, dataSourceSGP+":"):
		return dataSourceSGP
	case strings.HasPrefix(path, dataSourceRiot+":"):
		return dataSourceRiot
	default:
		return "unknown"
	}
}
