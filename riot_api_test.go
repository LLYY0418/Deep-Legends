package main

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestRiotAccountRequestDoesNotDoubleEncodeKoreanNames(t *testing.T) {
	t.Setenv("RIOT_API_KEY", "RGAPI-test")
	var gotURL string
	champions := newChampionProvider()
	champions.clientMu.Lock()
	champions.client = &http.Client{Transport: gameplayRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		gotURL = request.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"puuid":"abc","gameName":"노 갱","tagLine":"0518"}`)),
			Request:    request,
		}, nil
	})}
	champions.clientMu.Unlock()
	account, err := newRiotProvider(champions).accountByRiotID(context.Background(), "노 갱", "0518")
	if err != nil {
		t.Fatal(err)
	}
	if account.GameName != "노 갱" || account.TagLine != "0518" {
		t.Fatalf("account = %+v", account)
	}
	escaped := url.PathEscape("노 갱")
	if escaped == "" || strings.Contains(gotURL, "%25") {
		t.Fatalf("path was double-encoded: %s (once-escaped name %q)", gotURL, escaped)
	}
	if !strings.Contains(gotURL, escaped) {
		t.Fatalf("request URL %s missing once-escaped name %q", gotURL, escaped)
	}
}

func TestOpggParseSearchResultReadsSummoners(t *testing.T) {
	payload := "0:{\"a\":1}\n1:{\"summoners\":[{\"gameName\":\"Hide on bush\",\"tagline\":\"KR1\"},{\"gameName\":\"노 갱\",\"tagline\":\"0518\"}]}\n"
	got := opggParseSearchResult([]byte(payload))
	if len(got) != 2 || got[0].GameName != "Hide on bush" || got[0].TagLine != "KR1" || got[1].GameName != "노 갱" || got[1].TagLine != "0518" {
		t.Fatalf("parsed %+v", got)
	}
	if opggParseSearchResult([]byte("not-json")) != nil {
		t.Fatal("expected empty result for invalid payload")
	}
}
