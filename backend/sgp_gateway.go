package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const sgpEndpointConfig = "/lol-platform-config/v1/namespaces/PlayerPreferences/ServiceEndpoint"
const sgpPlatformConfig = "/lol-platform-config/v1/namespaces/LoginDataPacket/platformId"

var sgpOfficialHost = regexp.MustCompile(`^[a-z0-9]+(?:-k8s)?-sgp\.lol\.qq\.com$`)
var sgpPlatformCode = regexp.MustCompile(`^[A-Z0-9_]{2,16}$`)

type sgpGateway struct {
	Platform       string `json:"platform"`
	Base           string `json:"-"`
	Source         string `json:"source"`
	EndpointStatus int    `json:"endpoint_status"`
	PlatformStatus int    `json:"platform_status"`
	BuiltinKnown   bool   `json:"builtin_known"`
	BuiltinMatches bool   `json:"builtin_matches"`
}
type sgpGatewayCache struct {
	mu      sync.Mutex
	client  *LCUClient
	account string
	at      time.Time
	value   sgpGateway
}

// Client configuration is not permission to forward a credential to any URL.
// Allow new official shards, but never redirects, arbitrary hosts or URL paths.
func validatedSGPBase(value string) (string, bool) {
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || u.Path != "" && u.Path != "/" || !sgpOfficialHost.MatchString(u.Hostname()) || u.Port() != "" && u.Port() != "443" && u.Port() != "21019" {
		return "", false
	}
	return "https://" + u.Host, true
}

// Only the two gateway configuration GETs; local credentials never follow redirects.
func readGatewayConfig(ctx context.Context, c *LCUClient, path string, limit int64) ([]byte, int, error) {
	if c == nil || c.http == nil {
		return nil, 0, errors.New("client-unavailable")
	}
	origin, err := url.Parse(c.baseURL)
	if err != nil || !net.ParseIP(origin.Hostname()).IsLoopback() || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || origin.Path != "" || origin.Scheme != "https" && origin.Scheme != "http" {
		return nil, 0, errors.New("client-origin-not-loopback")
	}
	switch path {
	case sgpEndpointConfig, sgpPlatformConfig:
	default:
		return nil, 0, errors.New("gateway-config-path-not-allowed")
	}
	token, ok := c.credentials()
	if !ok {
		return nil, 0, errors.New("client-unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.SetBasicAuth("riot", token)
	req.Header.Set("Accept", "application/json")
	copy := *c.http
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copy.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, response.StatusCode, errors.New("lcu-http-error")
	}
	body, err := readLimited(response.Body, limit)
	return body, response.StatusCode, err
}

func (p *sgpProvider) discoverGateway(ctx context.Context, c *LCUClient, account string) sgpGateway {
	p.gateway.mu.Lock()
	defer p.gateway.mu.Unlock()
	if p.gateway.client == c && p.gateway.account == account && time.Since(p.gateway.at) < time.Minute {
		return p.gateway.value
	}
	// Do not run platformInfo's unscoped command-line probe while holding this
	// lock. These two bounded config GETs are the authority for discovery.
	c.mu.RLock()
	region, platform := c.region, c.rsoPlatform
	c.mu.RUnlock()
	result := sgpGateway{Platform: strings.ToUpper(platform), Source: "unavailable"}
	if !sgpPlatformCode.MatchString(result.Platform) {
		result.Platform = ""
	}
	if region != "" && !strings.EqualFold(region, "TENCENT") {
		return result
	}
	readString := func(path string) (string, int) {
		child, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		body, status, err := readGatewayConfig(child, c, path, 4096)
		var value string
		if err != nil || json.Unmarshal(body, &value) != nil {
			return "", status
		}
		return strings.TrimSpace(value), status
	}
	endpoint, endpointStatus := readString(sgpEndpointConfig)
	dynamicPlatform, platformStatus := readString(sgpPlatformConfig)
	result.EndpointStatus, result.PlatformStatus = endpointStatus, platformStatus
	dynamicPlatform = strings.ToUpper(dynamicPlatform)
	if sgpPlatformCode.MatchString(dynamicPlatform) {
		result.Platform = dynamicPlatform
	}
	base, valid := validatedSGPBase(endpoint)
	if valid && sgpPlatformCode.MatchString(dynamicPlatform) {
		result.Platform, result.Base, result.Source = dynamicPlatform, base, "client"
	}
	builtin, known := p.serverBase(result.Platform)
	result.BuiltinKnown, result.BuiltinMatches = known, known && builtin == result.Base
	if result.Base == "" && known {
		result.Base, result.Source = builtin, "builtin-fallback"
	}
	if p.gatewayAccount != nil && p.gatewayAccount(c) != account {
		return sgpGateway{Source: "connection-changed"}
	}
	if result.Source == "client" {
		c.mu.Lock()
		c.region, c.rsoPlatform = "TENCENT", result.Platform
		c.mu.Unlock()
	}
	p.gateway.client, p.gateway.account, p.gateway.at, p.gateway.value = c, account, time.Now(), result
	fields := map[string]any{"event": "sgp_gateway_resolution", "source": result.Source, "server_id": result.Platform, "endpoint_status": endpointStatus, "platform_status": platformStatus, "endpoint_valid": valid, "platform_valid": sgpPlatformCode.MatchString(dynamicPlatform), "builtin_known": known, "builtin_matches": result.BuiltinMatches, "builtin_comparison_available": known && result.Source == "client"}
	if u, err := url.Parse(result.Base); err == nil && valid {
		fields["upstream_host"], fields["upstream_port"] = u.Hostname(), u.Port()
	}
	p.recordObservation(fields)
	return result
}

func (p *sgpProvider) serverBaseOn(ctx context.Context, c *LCUClient, server string) (string, bool) {
	if p == nil {
		return "", false
	}
	if p.gatewayAccount != nil && c != nil {
		account := p.gatewayAccount(c)
		if account == "" {
			return "", false
		}
		gateway := p.discoverGateway(ctx, c, account)
		if gateway.Source == "connection-changed" {
			return "", false
		}
		if server == "" || strings.EqualFold(server, gateway.Platform) {
			return gateway.Base, gateway.Base != ""
		}
	}
	return p.serverBase(server)
}
