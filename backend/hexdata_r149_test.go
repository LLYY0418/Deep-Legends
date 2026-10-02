package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestR149AnswerAndTokenPageStartTogether(t *testing.T) {
	answer, _ := hexdataRankingFixtures(t)
	meta := r116aMetaFixture(t, hexdataTestBuild)
	insights := r116aInsightsFixture(t, 173, 211)
	postmatch := r116aFixture(t, "hexdata-postmatch.json")
	entered := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != hexdataHost {
			return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody, Request: request}, nil
		}
		var body []byte
		switch request.URL.Path {
		case hexdataAnswerPath:
			body = answer
		case "/":
			body = []byte("<html></html>")
		case hexdataMetaPath:
			body = meta
		case hexdataHextechInsightsPath:
			body = insights
		case hexdataPostmatchPath:
			body = postmatch
		default:
			t.Fatalf("unexpected Hexdata path %s", request.URL.Path)
		}
		if request.URL.Path == hexdataAnswerPath || request.URL.Path == "/" {
			entered <- request.URL.Path
			<-release
		}
		return hexdataResponse(request, body), nil
	})
	provider := newHexdataBudgetProvider(t, t.TempDir(), transport)
	done := make(chan error, 1)
	go func() { _, err := provider.loadHexdataRankings(context.Background()); done <- err }()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case path := <-entered:
			seen[path] = true
		case <-time.After(3 * time.Second):
			unblock()
			t.Fatalf("answer and token page did not overlap: %v", seen)
		}
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	provider.hexdata.prefetchWait.Wait()
	provider.mayhemWarmWait.Wait()
}

func TestR149MayhemCatalogsWarmConcurrentlyAndRestartFromDisk(t *testing.T) {
	root := t.TempDir()
	var requests atomic.Int32
	entered := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	transport := championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests.Add(1)
		var body string
		switch request.URL.Path {
		case "/cdn/16.19.1/data/zh_CN/item.json":
			body = `{"data":{"1001":{"name":"测试装备","image":{"full":"1001.png"}}}}`
		case "/cdn/16.19.1/data/zh_CN/summoner.json":
			body = `{"data":{}}`
		case "/cdn/16.19.1/data/zh_CN/runesReforged.json":
			body = `[]`
		case "/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json":
			body = `[{"id":93,"nameTRA":"测试海克斯","rarity":"kSilver"}]`
		case "/latest/cdragon/arena/zh_cn.json":
			body = `{"augments":[]}`
		default:
			t.Fatalf("unexpected catalog path %s", request.URL.Path)
		}
		if strings.HasSuffix(request.URL.Path, "/item.json") || strings.HasSuffix(request.URL.Path, "/cherry-augments.json") {
			entered <- request.URL.Path
			<-release
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: request}, nil
	})
	provider := newHexdataBudgetProvider(t, root, transport)
	provider.patch = "16.19.1"
	provider.prefetchMayhemCatalogs()
	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case path := <-entered:
			seen[path] = true
		case <-time.After(3 * time.Second):
			unblock()
			t.Fatalf("Data Dragon and CommunityDragon prewarm did not overlap: %v", seen)
		}
	}
	unblock()
	provider.mayhemWarmWait.Wait()
	if got := requests.Load(); got != 5 {
		t.Fatalf("cold catalog requests = %d, want 5", got)
	}
	restarted := newHexdataBudgetProvider(t, root, transport)
	restarted.patch = "16.19.1"
	restarted.prefetchMayhemCatalogs()
	restarted.mayhemWarmWait.Wait()
	if got := requests.Load(); got != 5 {
		t.Fatalf("restart fetched %d additional catalog requests, want disk hits", got-5)
	}
}

func TestR149PrimaryDecorationStartsBeforeRSCFinishes(t *testing.T) {
	recorder := &r116aRecorder{}
	provider, _ := r116aMayhemProvider(t, recorder, r116aFixture(t, "hexdata-hero-157.json"))
	base := provider.client.Transport
	rscEntered := make(chan struct{}, 1)
	decorateEntered := make(chan struct{}, 1)
	provider.gameplayAugments = func(context.Context) ([]gameplayAugment, error) {
		decorateEntered <- struct{}{}
		return nil, nil
	}
	releaseRSC := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(releaseRSC) }) }
	defer unblock()
	provider.client.Transport = championRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == opggPageHost && strings.Contains(request.URL.Path, "/aram-mayhem/") {
			rscEntered <- struct{}{}
			<-releaseRSC
		}
		return base.RoundTrip(request)
	})
	done := make(chan error, 1)
	go func() { _, err := provider.loadMayhemDetail(context.Background(), "157"); done <- err }()
	select {
	case <-rscEntered:
	case <-time.After(3 * time.Second):
		t.Fatal("RSC request never started")
	}
	select {
	case <-decorateEntered:
	case <-time.After(3 * time.Second):
		unblock()
		t.Fatal("hero-json decoration waited for RSC")
	}
	unblock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
