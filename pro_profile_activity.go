package main

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
)

// The directory DTO intentionally omits local activity metadata. Persist it
// explicitly beside the public profile so restarting preserves account order.
type proProfileSnapshot struct {
	Account          opggProAccount `json:"account"`
	LastMatchAt      string         `json:"lastMatchAt,omitempty"`
	LastMatchAtKnown bool           `json:"lastMatchAtKnown"`
}

// OP.GG's JSON-LD ItemList describes historical PlayGameActions. Its startTime
// is a game start, unlike initUpdatedAt (the page's last profile refresh).
// parseProProfile has already verified the Flight identity; also bind each
// action's agent URL to that exact KR Riot ID before accepting its timestamp.
func proProfileLastMatch(doc *xhtml.Node, ref gameplayReference) (time.Time, bool) {
	var latest time.Time
	var visit func(*xhtml.Node)
	visit = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && n.Data == "script" {
			for _, attr := range n.Attr {
				if attr.Key != "type" || attr.Val != "application/ld+json" {
					continue
				}
				var value any
				if json.Unmarshal([]byte(proNodeText(n)), &value) != nil {
					continue
				}
				walkOPGGPage(value, 0, func(node map[string]any) {
					if node["@type"] != "PlayGameAction" {
						return
					}
					agent, _ := node["agent"].(map[string]any)
					link, _ := agent["@id"].(string)
					u, err := url.Parse(link)
					if err != nil || u.Scheme != "https" || u.Host != "op.gg" || u.RawQuery != "" {
						return
					}
					parts := strings.Split(strings.Trim(u.Path, "/"), "/")
					if len(parts) != 5 || parts[1] != "lol" || parts[2] != "summoners" || parts[3] != "kr" {
						return
					}
					slug := parts[4]
					wanted := ref.GameName + "-" + ref.TagLine
					// The public JSON-LD double-escapes spaces/Korean characters
					// (e.g. Hide%2520on%2520bush). Decode only this slug, then
					// compare the complete Riot ID; never accept a prefix match.
					if !strings.EqualFold(slug, wanted) {
						slug, err = url.PathUnescape(slug)
						if err != nil || !strings.EqualFold(slug, wanted) {
							return
						}
					}
					raw, _ := node["startTime"].(string)
					at, err := time.Parse(time.RFC3339Nano, raw)
					if err == nil && at.Year() >= 2009 && !at.After(time.Now().Add(5*time.Minute)) && at.After(latest) {
						latest = at.UTC()
					}
				})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)
	return latest, !latest.IsZero()
}
