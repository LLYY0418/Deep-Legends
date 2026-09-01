package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	featureGateOPGG        = "opgg"
	featureGateHexdata     = "hexdata"
	featureGateLolalytics  = "lolalytics"
	featureGateQQ101       = "qq101"
	featureGateResponseMax = 64 << 10
)

var defaultFeatureGateValues = map[string]bool{
	featureGateOPGG: true, featureGateHexdata: true, featureGateLolalytics: true, featureGateQQ101: true,
}

type featureGates struct {
	mu     sync.RWMutex
	values map[string]bool
}

func newFeatureGates() *featureGates {
	values := make(map[string]bool, len(defaultFeatureGateValues))
	for key, enabled := range defaultFeatureGateValues {
		values[key] = enabled
	}
	return &featureGates{values: values}
}

func (g *featureGates) enabled(key string) bool {
	if g == nil {
		return defaultFeatureGateValues[key]
	}
	g.mu.RLock()
	enabled, known := g.values[key]
	g.mu.RUnlock()
	if !known {
		return false
	}
	return enabled
}

func (g *featureGates) apply(values map[string]bool) {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, enabled := range values {
		if _, known := defaultFeatureGateValues[key]; known {
			g.values[key] = enabled
		}
	}
}

// refresh accepts only a small boolean document over HTTPS. Failure leaves
// every built-in default intact, so remote configuration can never become a
// startup dependency.
func (g *featureGates) refresh(ctx context.Context, client *http.Client, rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(parsed.Scheme, "https") || parsed.Host == "" || parsed.User != nil {
		return errors.New("feature gate URL must be HTTPS")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return errors.New("feature gate service unavailable")
	}
	data, err := readLimited(response.Body, featureGateResponseMax)
	if err != nil {
		return err
	}
	var values map[string]bool
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	g.apply(values)
	return nil
}

func featureGateForChampionHost(host string) string {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case opggChampionHost, opggWebAPIHost, opggPageHost:
		return featureGateOPGG
	case hexdataHost:
		return featureGateHexdata
	case qq101Host:
		return featureGateQQ101
	default:
		return ""
	}
}
