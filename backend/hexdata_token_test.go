package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func r147Response(request *http.Request, status int, body, cookie string, maxAge int) *http.Response {
	header := make(http.Header)
	if cookie != "" {
		header.Set("Set-Cookie", fmt.Sprintf("hexpage=%s; Path=/; Max-Age=%d; Secure; HttpOnly", cookie, maxAge))
	}
	if status == http.StatusForbidden {
		header.Set("Content-Type", "application/json")
	}
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}
}

func r147HeroContext() context.Context {
	return context.WithValue(context.Background(), hexdataTokenPageKey{}, "/hero/887-gwen")
}

func TestHexdataPageTokenAcquiredBeforeJSONAndRefreshedBeforeExpiry(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	pageRequests, jsonRequests := 0, 0
	provider := newHexdataBudgetProvider(t, root, championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		switch request.URL.Path {
		case "/hero/887-gwen":
			pageRequests++
			return r147Response(request, 200, "<html></html>", fmt.Sprintf("FAKE-TOKEN-%d", pageRequests), 10), nil
		case "/api/hexdata/heroes/887":
			jsonRequests++
			if len(request.Cookies()) != 1 || request.Cookies()[0].Value != fmt.Sprintf("FAKE-TOKEN-%d", pageRequests) {
				return r147Response(request, 403, `{"error":"page_token_required"}`, "", 0), nil
			}
			return r147Response(request, 200, `{"ok":true}`, "", 0), nil
		}
		return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
	}))
	now := time.Now()
	provider.hexdata.now = func() time.Time { return now }
	for i := 0; i < 2; i++ {
		if _, err := provider.hexdata.fetch(r147HeroContext(), "hero-json", "/api/hexdata/heroes/887", "application/json", nil, false); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			now = now.Add(10 * time.Second)
		}
	}
	if pageRequests != 2 || jsonRequests != 2 {
		t.Fatalf("requests page/json = %d/%d, want 2/2", pageRequests, jsonRequests)
	}
}

func TestHexdataPageTokenRefreshesOnceAndFinalFailureCountsOnce(t *testing.T) {
	var mu sync.Mutex
	pageRequests, jsonRequests := 0, 0
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/hero/887-gwen" {
			pageRequests++
			return r147Response(request, 200, "<html></html>", fmt.Sprintf("FAKE-TOKEN-%d", pageRequests), 86400), nil
		}
		if request.URL.Path == "/api/hexdata/heroes/887" {
			jsonRequests++
			return r147Response(request, 403, `{"error":"page_token_required"}`, "", 0), nil
		}
		return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
	}))
	_, err := provider.hexdata.load(r147HeroContext(), "hero-json", "887", "/api/hexdata/heroes/887", "application/json", false)
	if err == nil {
		t.Fatal("token rejection should fail after one refresh")
	}
	if pageRequests != 2 || jsonRequests != 2 {
		t.Fatalf("requests page/json = %d/%d, want 2/2", pageRequests, jsonRequests)
	}
	if got := provider.hexdata.snapshot().Circuits["hero-json"].Failures; got != 1 {
		t.Fatalf("circuit failures = %d, want 1", got)
	}
}

func TestHexdataPageTokenRecoveredRequestDoesNotCountCircuitFailure(t *testing.T) {
	var mu sync.Mutex
	pageRequests, jsonRequests := 0, 0
	fixture := string(r116aFixture(t, "hexdata-hero-157.json"))
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/hero/887-gwen" {
			pageRequests++
			return r147Response(request, 200, "<html></html>", fmt.Sprintf("FAKE-TOKEN-%d", pageRequests), 86400), nil
		}
		if request.URL.Path == "/api/hexdata/heroes/887" {
			jsonRequests++
			cookie, err := request.Cookie("hexpage")
			if err != nil || cookie.Value != "FAKE-TOKEN-2" {
				return r147Response(request, 403, `{"error":"page_token_required"}`, "", 0), nil
			}
			return r147Response(request, 200, fixture, "", 0), nil
		}
		return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
	}))
	if _, err := provider.hexdata.load(r147HeroContext(), "hero-json", "887", "/api/hexdata/heroes/887", "application/json", false); err != nil {
		t.Fatal(err)
	}
	if pageRequests != 2 || jsonRequests != 2 {
		t.Fatalf("requests page/json = %d/%d, want 2/2", pageRequests, jsonRequests)
	}
	if got := provider.hexdata.snapshot().Circuits["hero-json"].Failures; got != 0 {
		t.Fatalf("recovered request counted %d circuit failures", got)
	}
}

func TestHexdataTokenJarIsHostLimited(t *testing.T) {
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected network request")
	}))
	jar := provider.hexdata.tokenJar
	hexdataURL, _ := url.Parse("https://hexdata.com.cn/hero/887-gwen")
	otherURL, _ := url.Parse("https://example.com/")
	jar.SetCookies(hexdataURL, []*http.Cookie{{Name: "hexpage", Value: "FAKE-HOST-TOKEN", Path: "/"}, {Name: "other", Value: "ignored", Path: "/"}})
	if got := jar.Cookies(otherURL); len(got) != 0 {
		t.Fatalf("Hexdata cookie leaked to another host: %v", got)
	}
	if got := jar.Cookies(hexdataURL); len(got) != 1 || got[0].Name != "hexpage" {
		t.Fatalf("Hexdata jar names = %v, want only hexpage", got)
	}
}

func TestHexdataPageTokenTenConcurrentRequestsUseOnePage(t *testing.T) {
	var mu sync.Mutex
	pageRequests, jsonRequests := 0, 0
	provider := newHexdataBudgetProvider(t, t.TempDir(), championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		if request.URL.Path == "/hero/887-gwen" {
			pageRequests++
			time.Sleep(20 * time.Millisecond) // Keep acquisition in flight while the other nine callers arrive.
			return r147Response(request, 200, "<html></html>", "FAKE-CONCURRENT-TOKEN", 86400), nil
		}
		if request.URL.Path == "/api/hexdata/heroes/887" {
			jsonRequests++
			if _, err := request.Cookie("hexpage"); err != nil {
				return r147Response(request, 403, `{"error":"page_token_required"}`, "", 0), nil
			}
			return r147Response(request, 200, `{"ok":true}`, "", 0), nil
		}
		return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
	}))
	start := make(chan struct{})
	var wg sync.WaitGroup
	errors := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := provider.hexdata.fetch(r147HeroContext(), "hero-json", "/api/hexdata/heroes/887", "application/json", nil, false)
			errors <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if pageRequests != 1 || jsonRequests != 10 {
		t.Fatalf("requests page/json = %d/%d, want 1/10", pageRequests, jsonRequests)
	}
}

func TestHexdataPageTokenNeverAppearsInDiagnosticsOrDisk(t *testing.T) {
	const secret = "FAKE-SECRET-COOKIE-VALUE"
	root := t.TempDir()
	var mu sync.Mutex
	var events []map[string]any
	provider := newHexdataBudgetProvider(t, root, championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/hero/887-gwen" {
			return r147Response(request, 200, "<html></html>", secret, 86400), nil
		}
		if request.URL.Path == "/api/hexdata/heroes/887" {
			return r147Response(request, 403, `{"error":"page_token_required","message":"`+secret+`"}`, "", 0), nil
		}
		return nil, fmt.Errorf("unexpected path %s", request.URL.Path)
	}))
	provider.diag = func(event map[string]any) { mu.Lock(); events = append(events, event); mu.Unlock() }
	_, _ = provider.hexdata.load(r147HeroContext(), "hero-json", "887", "/api/hexdata/heroes/887", "application/json", false)
	mu.Lock()
	requestEvents := 0
	for _, event := range events {
		if event["event"] == "hexdata_request" {
			requestEvents++
			for _, key := range []string{"kind", "path_kind", "status", "bytes", "pace_wait_ms", "wire_ms", "cache"} {
				if _, ok := event[key]; !ok {
					t.Fatalf("hexdata_request lacks %s: %v", key, event)
				}
			}
			for key, value := range event {
				if strings.Contains(strings.ToLower(key), "cookie") || strings.Contains(strings.ToLower(key), "url") || strings.Contains(fmt.Sprint(value), "?") {
					t.Fatalf("hexdata_request logged URL or cookie material: %v", event)
				}
			}
		}
		for _, value := range event {
			if strings.Contains(fmt.Sprint(value), secret) {
				t.Fatalf("cookie leaked into diagnostics: %v", event)
			}
		}
	}
	if requestEvents == 0 {
		t.Fatal("hexdata_request diagnostics were not emitted")
	}
	mu.Unlock()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), secret) {
			return fmt.Errorf("cookie leaked to %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
