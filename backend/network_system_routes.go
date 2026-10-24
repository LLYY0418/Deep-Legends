package main

import (
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

type hostNetworkRoute struct {
	address   string
	transport *automaticNetworkTransport
}

// Chromium resolves PAC for each actual source, never by borrowing another site's rule.
func systemProxyTargets() map[string]string {
	targets := map[string]string{}
	add := func(raw string) {
		if u, err := url.Parse(raw); err == nil && u.Hostname() != "" {
			targets[strings.ToLower(u.Hostname())] = u.Scheme + "://" + u.Host + "/"
		}
	}
	for _, host := range []string{opggChampionHost, opggWebAPIHost, opggPageHost, esportsOPGGHost, qq101Host, communityDragonHost, yourGGArenaHost, opggAssetHost, dataDragonHost, aramkitRatingHost, aramkitRatingCNHost, aramkitDataCN, aramkitDataCOM, prestigeArtworkHost, "github.com", "api.github.com", "raw.githubusercontent.com", "objects.githubusercontent.com", "release-assets.githubusercontent.com"} {
		add("https://" + host)
	}
	for _, region := range strings.Fields("br1 eun1 euw1 jp1 kr la1 la2 me1 na1 oc1 ru tr1 ph2 sg2 th2 tw2 vn2 americas asia europe sea") {
		add("https://" + region + ".api.riotgames.com")
	}
	for _, address := range riotRelayAddresses {
		add(address)
	}
	if config, ok := parseRiotRelayEntries(os.Getenv("DEEP_LEGENDS_RELAY_ENTRIES")); ok {
		for _, entry := range config.Entries {
			add(entry.origin())
		}
	}
	for _, mirror := range updateMirrors {
		add(mirror)
	}
	for _, target := range additionalSystemProxyTargets() {
		add(target)
	}
	return targets
}

func (a *app) handleSystemProxyTargets(w http.ResponseWriter, _ *http.Request) {
	respondJSON(w, map[string]any{"targets": systemProxyTargets()})
}

func (t *automaticNetworkTransport) refreshHostRoutes(routes map[string]string) {
	t.mu.Lock()
	changed := map[*automaticNetworkTransport]string{}
	for host, route := range t.hostRoutes {
		address, exists := routes[host]
		if !exists || address != route.address {
			changed[route.transport] = address
			if !exists {
				delete(t.hostRoutes, host)
			}
		}
	}
	t.mu.Unlock()
	for route, address := range changed {
		route.refreshProxy(address)
		route.CloseIdleConnections()
	}
}

func (t *automaticNetworkTransport) forRequest(r *http.Request) *automaticNetworkTransport {
	p := t.routeProvider
	if t.settings.Mode != "auto" || p == nil {
		return t
	}
	host := strings.ToLower(r.URL.Hostname())
	p.clientMu.RLock()
	address, known := p.systemProxyRoutes[host]
	p.clientMu.RUnlock()
	if !known {
		return t
	} // Older desktop/standalone backends retain their single-proxy contract.
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hostRoutes == nil {
		t.hostRoutes = map[string]hostNetworkRoute{}
	}
	previous, exists := t.hostRoutes[host]
	if exists && previous.address == address {
		return previous.transport
	}
	child := newAutomaticNetworkTransportForProxy(t.settings, p, func() string { return address })
	child.targetHost = host
	child.now = t.now
	child.probes = map[string]string{}
	if len(t.probes) > 0 {
		child.probes[networkGroup(host)] = systemProxyTargets()[host]
	}
	if exists {
		previous.transport.mu.Lock()
		child.recoveryPending = previous.transport.failed || previous.transport.recoveryPending
		previous.transport.generation++ // Late probes from the old PAC route cannot win.
		previous.transport.mu.Unlock()
		previous.transport.CloseIdleConnections()
	}
	t.hostRoutes[host] = hostNetworkRoute{address, child}
	return child
}

// Preserve the caller's timeout/redirect/Jar policy while following live app routing.
func (a *app) externalHTTPClient(base *http.Client) *http.Client {
	if a.champions == nil {
		return base
	}
	copy := *base
	copy.Transport = &providerNetworkTransport{provider: a.champions, template: base.Transport}
	return &copy
}

type providerNetworkTransport struct {
	provider *championProvider
	template http.RoundTripper
	mu       sync.Mutex
	client   *http.Client
	route    http.RoundTripper
}

func (t *providerNetworkTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	client := t.provider.httpClient()
	t.mu.Lock()
	if t.client != client {
		if closer, ok := t.route.(interface{ CloseIdleConnections() }); ok {
			closer.CloseIdleConnections()
		}
		t.client, t.route = client, client.Transport
		if owner, ok := client.Transport.(*automaticNetworkTransport); ok {
			if template, ok := t.template.(*http.Transport); ok {
				t.route = &mappedNetworkTransport{owner: owner, mapRoute: func(raw http.RoundTripper) http.RoundTripper {
					base, ok := raw.(*http.Transport)
					if !ok {
						return raw
					}
					transport := base.Clone()
					transport.TLSHandshakeTimeout = template.TLSHandshakeTimeout
					transport.ResponseHeaderTimeout = template.ResponseHeaderTimeout
					return transport
				}}
			}
		}
	}
	route := t.route
	t.mu.Unlock()
	if route == nil {
		route = http.DefaultTransport
	}
	return route.RoundTrip(r)
}

func (t *providerNetworkTransport) CloseIdleConnections() {
	t.mu.Lock()
	route := t.route
	t.mu.Unlock()
	if closer, ok := route.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
