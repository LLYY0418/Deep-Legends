package main

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProviderTransportPoolsReuseEightConnections(t *testing.T) {
	for _, kind := range []string{"champion", "sgp", "lcu"} {
		t.Run(kind, func(t *testing.T) {
			var handshakes atomic.Int32
			arrived := make(chan struct{}, 8)
			release := make(chan struct{})
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				arrived <- struct{}{}
				<-release
				_, _ = io.WriteString(w, "ok")
			}))
			server.TLS = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) { handshakes.Add(1); return nil, nil }}
			server.StartTLS()
			defer server.Close()
			var client *http.Client
			switch kind {
			case "champion":
				client = newChampionHTTPClient(nil)
			case "sgp":
				client = newSGPProvider().http
			case "lcu":
				client = newLCUClient(1, "test-token").http
			}
			defer client.CloseIdleConnections()
			transport := client.Transport.(*http.Transport)
			if transport.MaxIdleConns != 32 || transport.MaxIdleConnsPerHost != 8 || transport.IdleConnTimeout != 90*time.Second {
				t.Fatal("production pool configuration drifted")
			}
			if transport.TLSClientConfig == nil {
				transport.TLSClientConfig = &tls.Config{}
			}
			pool := x509.NewCertPool()
			pool.AddCert(server.Certificate())
			transport.TLSClientConfig.RootCAs = pool
			if kind != "lcu" && transport.TLSClientConfig.InsecureSkipVerify {
				t.Fatal("loopback TLS exception escaped to public transport")
			}
			for round := 0; round < 5; round++ {
				var wg sync.WaitGroup
				for i := 0; i < 8; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						response, err := client.Get(server.URL)
						if err != nil {
							t.Error(err)
							return
						}
						_, err = io.Copy(io.Discard, response.Body)
						response.Body.Close()
						if err != nil {
							t.Error(err)
						}
					}()
				}
				for i := 0; i < 8; i++ {
					select {
					case <-arrived:
					case <-time.After(15 * time.Second):
						close(release)
						t.Fatal("requests did not overlap")
					}
				}
				for i := 0; i < 8; i++ {
					release <- struct{}{}
				}
				wg.Wait()
			}
			if n := handshakes.Load(); n > 10 {
				t.Fatalf("40 requests required %d handshakes, want <=10", n)
			}
		})
	}
}

func TestChampionTransportNegotiatesHTTP2(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") }))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := newChampionHTTPClient(nil)
	defer client.CloseIdleConnections()
	pool := x509.NewCertPool()
	pool.AddCert(server.Certificate())
	client.Transport.(*http.Transport).TLSClientConfig.RootCAs = pool
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.Proto != "HTTP/2.0" {
		t.Fatalf("protocol = %s", response.Proto)
	}
}
