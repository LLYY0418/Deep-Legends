package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type aramkitRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn aramkitRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func newAramkitTestClient(roundTrip http.RoundTripper) *aramkitRatingClient {
	return &aramkitRatingClient{
		cache: make(map[string]aramkitRatingCacheEntry), flights: make(map[string]*aramkitRatingFlight),
		featureGates: newFeatureGates(),
		httpClient:   func() *http.Client { return &http.Client{Transport: roundTrip} },
		identityHash: func(string) string { return "redacted-player" },
		now:          time.Now, sleep: sleepWithContext, jitter: func(time.Duration) time.Duration { return 0 },
		baseURL: "https://api.aramkit.test", timeout: time.Second,
	}
}

func aramkitJSONResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestAramkitRatingSuccessUsesExactIdentityAndOmitsRoster(t *testing.T) {
	var requests atomic.Int32
	client := newAramkitTestClient(aramkitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		if got := request.URL.Query().Get("gameName"); got != "测试 玩家" {
			t.Fatalf("gameName = %q", got)
		}
		if got := request.URL.Query().Get("tagLine"); got != "12345" {
			t.Fatalf("tagLine = %q", got)
		}
		if got := request.Header.Get("User-Agent"); !strings.HasPrefix(got, "Deep-Legends/") {
			t.Fatalf("User-Agent = %q", got)
		}
		return aramkitJSONResponse(http.StatusOK, `{"code":200,"data":{"gameName":"测试 玩家","tagLine":"12345","platformId":"HN1","rating":2176,"ratingType":1,"marginOfError":42,"matches":[{"gameCreationTime":"2026-09-19T10:00:00Z","championId":99,"kills":9,"deaths":3,"assists":20,"participants":[{"gameName":"不应返回","tagLine":"9999"}]}]}}`), nil
	}))

	first := client.lookup(t.Context(), " 测试 玩家 ", "12345")
	if !first.Available || first.Rating != 2176 || first.RatingType != 1 || first.MarginOfError != 42 || first.PlatformID != "HN1" {
		t.Fatalf("rating response = %#v", first)
	}
	if len(first.Matches) != 1 || first.Matches[0].ChampionID != 99 {
		t.Fatalf("matches = %#v", first.Matches)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "不应返回") || strings.Contains(string(encoded), "participants") {
		t.Fatalf("third-party roster leaked to renderer: %s", encoded)
	}
	second := client.lookup(t.Context(), "测试 玩家", "12345")
	if !second.Available || requests.Load() != 1 {
		t.Fatalf("10 minute cache missed: requests=%d response=%#v", requests.Load(), second)
	}
}

func TestAramkitRatingBodyCodeAndTransportFailuresDegrade(t *testing.T) {
	tests := []struct {
		name, body, want string
		status           int
	}{
		{name: "not recorded", status: http.StatusOK, body: `{"code":400,"message":"not found"}`, want: "未收录"},
		{name: "nil data", status: http.StatusOK, body: `{"code":200,"data":null}`, want: "暂不可用"},
		{name: "invalid rating", status: http.StatusOK, body: `{"code":200,"data":{"rating":0}}`, want: "暂不可用"},
		{name: "upstream status", status: http.StatusTooManyRequests, body: `{"code":429}`, want: "暂不可用"},
		{name: "invalid json", status: http.StatusOK, body: `{`, want: "暂不可用"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return aramkitJSONResponse(tt.status, tt.body), nil
			}))
			got := client.lookup(t.Context(), "玩家", "1000")
			if got.Available || got.UnavailableReason != tt.want || got.Rating != 0 {
				t.Fatalf("response = %#v", got)
			}
		})
	}
}

func TestAramkitRatingTimeoutFeatureGateSingleFlightAndPacing(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		client := newAramkitTestClient(aramkitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			<-request.Context().Done()
			return nil, request.Context().Err()
		}))
		client.timeout = 10 * time.Millisecond
		got := client.lookup(t.Context(), "超时", "1")
		if got.Available || got.UnavailableReason != "暂不可用" {
			t.Fatalf("timeout response = %#v", got)
		}
	})

	t.Run("feature gate", func(t *testing.T) {
		var requests atomic.Int32
		client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
			requests.Add(1)
			return nil, errors.New("must not run")
		}))
		client.featureGates.apply(map[string]bool{featureGateAramkit: false})
		got := client.lookup(t.Context(), "关闭", "1")
		if got.Available || got.UnavailableReason != "功能暂不可用" || requests.Load() != 0 {
			t.Fatalf("disabled response=%#v requests=%d", got, requests.Load())
		}
	})

	t.Run("single flight", func(t *testing.T) {
		var requests atomic.Int32
		started, release := make(chan struct{}), make(chan struct{})
		client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
			if requests.Add(1) == 1 {
				close(started)
			}
			<-release
			return aramkitJSONResponse(http.StatusOK, `{"code":200,"data":{"rating":1999}}`), nil
		}))
		results := make(chan mayhemRatingResponse, 2)
		go func() { results <- client.lookup(context.Background(), "同一玩家", "1") }()
		<-started
		go func() { results <- client.lookup(context.Background(), "同一玩家", "1") }()
		close(release)
		for range 2 {
			if got := <-results; !got.Available || got.Rating != 1999 {
				t.Fatalf("singleflight response = %#v", got)
			}
		}
		if requests.Load() != 1 {
			t.Fatalf("requests = %d", requests.Load())
		}
	})

	t.Run("global pacing", func(t *testing.T) {
		var clockMu sync.Mutex
		now := time.Unix(1_800_000_000, 0)
		var slept []time.Duration
		client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return aramkitJSONResponse(http.StatusOK, `{"code":200,"data":{"rating":1888}}`), nil
		}))
		client.minimumDelay = 300 * time.Millisecond
		client.now = func() time.Time { clockMu.Lock(); defer clockMu.Unlock(); return now }
		client.sleep = func(_ context.Context, delay time.Duration) error {
			clockMu.Lock()
			slept = append(slept, delay)
			now = now.Add(delay)
			clockMu.Unlock()
			return nil
		}
		client.lookup(t.Context(), "玩家一", "1")
		client.lookup(t.Context(), "玩家二", "2")
		if len(slept) != 1 || slept[0] < 300*time.Millisecond {
			t.Fatalf("paced sleeps = %#v", slept)
		}
	})
}

func TestAramkitRatingHandlerUsesOpaqueReferenceAndRejectsKR(t *testing.T) {
	var requested atomic.Int32
	client := newAramkitTestClient(aramkitRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested.Add(1)
		if request.URL.Query().Get("gameName") != "国服玩家" || request.URL.Query().Get("tagLine") != "峡谷" {
			t.Fatalf("upstream query = %s", request.URL.RawQuery)
		}
		return aramkitJSONResponse(http.StatusOK, `{"code":200,"data":{"rating":2001}}`), nil
	}))
	a := &app{token: "test-token", gameplayRefs: make(map[string]string), gameplayRefDetails: make(map[string]gameplayReference), mayhemRatings: client}
	cnRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "cn-player-reference-0001", GameName: "国服玩家", TagLine: "峡谷", ServerID: "HN1"})
	recorder := httptest.NewRecorder()
	a.handleGameplayMayhemRating(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/mayhem-rating?playerRef="+cnRef, nil))
	if recorder.Code != http.StatusOK || requested.Load() != 1 || !strings.Contains(recorder.Body.String(), `"rating":2001`) {
		t.Fatalf("CN response status=%d requests=%d body=%s", recorder.Code, requested.Load(), recorder.Body.String())
	}

	krRef := a.registerGameplayReferenceDetails(gameplayReference{PlayerRef: "kr-player-reference-0001", GameName: "KR Player", TagLine: "KR1", Region: riotRegionKR})
	recorder = httptest.NewRecorder()
	a.handleGameplayMayhemRating(recorder, httptest.NewRequest(http.MethodGet, "/api/gameplay/mayhem-rating?playerRef="+krRef, nil))
	if recorder.Code != http.StatusOK || requested.Load() != 1 || !strings.Contains(recorder.Body.String(), "仅支持国服") {
		t.Fatalf("KR response status=%d requests=%d body=%s", recorder.Code, requested.Load(), recorder.Body.String())
	}
}

func TestAramkitDiagnosticsContainOnlyHashedIdentity(t *testing.T) {
	var events []map[string]any
	client := newAramkitTestClient(aramkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return aramkitJSONResponse(http.StatusOK, `{"code":200,"data":{"rating":2048}}`), nil
	}))
	client.observe = func(event map[string]any) { events = append(events, event) }
	client.lookup(t.Context(), "敏感名称", "私密编号")
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "敏感名称") || strings.Contains(string(encoded), "私密编号") || !strings.Contains(string(encoded), "redacted-player") {
		t.Fatalf("diagnostics identity boundary = %s", encoded)
	}
}
