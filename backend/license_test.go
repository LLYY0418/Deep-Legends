//go:build license

package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type licenseTestStore struct {
	mu       sync.Mutex
	data     []byte
	fail     bool
	failNext int
	saves    int
	failAt   map[int]bool
}

func (s *licenseTestStore) Load() (licenseDiskState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var state licenseDiskState
	if len(s.data) > 0 {
		json.Unmarshal(s.data, &state)
	}
	return state, nil
}
func (s *licenseTestStore) Save(state licenseDiskState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.fail || s.failNext > 0 || s.failAt[s.saves] {
		if s.failNext > 0 {
			s.failNext--
		}
		return errors.New("test persistence unavailable")
	}
	s.data, _ = json.Marshal(state)
	return nil
}

type licenseTestClock struct {
	mu        sync.Mutex
	now       time.Time
	wallShift time.Duration
}

func (c *licenseTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now.Add(c.wallShift)
}
func (c *licenseTestClock) Elapsed() time.Time       { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *licenseTestClock) StepWall(d time.Duration) { c.mu.Lock(); c.wallShift += d; c.mu.Unlock() }
func (c *licenseTestClock) Advance(d time.Duration)  { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

type licenseTestIssuer struct {
	mu        sync.Mutex
	clock     *licenseTestClock
	key       ed25519.PrivateKey
	pub       ed25519.PublicKey
	owner     string
	revision  int64
	counters  map[string]uint64
	requests  atomic.Int64
	offline   atomic.Bool
	malformed atomic.Bool
	revoked   atomic.Bool
	client    *http.Client
}

func newLicenseTestIssuer(t *testing.T) *licenseTestIssuer {
	t.Helper()
	pub, key, _ := ed25519.GenerateKey(rand.Reader)
	s := &licenseTestIssuer{clock: &licenseTestClock{now: time.Now().Truncate(time.Second)}, key: key, pub: pub, counters: map[string]uint64{}, revision: 1}
	s.client = &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w.Result(), nil
	})}
	return s
}
func testSignedEnvelope(key ed25519.PrivateKey, kid, domain string, payload any) signedLicenseEnvelope {
	data, _ := json.Marshal(payload)
	encoded := base64.RawURLEncoding.EncodeToString(data)
	sig := ed25519.Sign(key, []byte(domain+"\n"+kid+"\n"+encoded))
	return signedLicenseEnvelope{"Ed25519", kid, encoded, base64.RawURLEncoding.EncodeToString(sig)}
}
func (s *licenseTestIssuer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.requests.Add(1)
	if s.offline.Load() {
		http.Error(w, "offline", 503)
		return
	}
	var request licenseRequest
	data, _ := io.ReadAll(r.Body)
	if strictLicenseJSON(data, &request) != nil {
		http.Error(w, "invalid", 400)
		return
	}
	body, _ := rawURLDecode(request.Payload, -1)
	pub, _ := rawURLDecode(request.DevicePub, 32)
	sig, _ := rawURLDecode(request.Signature, 64)
	digest := sha256.Sum256(body)
	message := strings.Join([]string{"DL-LICENSE-REQUEST-V1", "POST", r.URL.Path, request.RequestID, request.DevicePub, strconv.FormatInt(request.TS, 10), request.Counter, hex.EncodeToString(digest[:])}, "\n")
	if !ed25519.Verify(pub, []byte(message), sig) {
		http.Error(w, "bad proof", 403)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := licensePayload{Version: 1, RequestID: request.RequestID, DeviceHash: licenseDeviceHash(pub), ServerTime: s.clock.Now().Unix(), Status: "ACTIVE", LicenseID: "test-license", Kind: "standard", Revision: s.revision}
	var input struct {
		Code          string `json:"code"`
		ClientVersion string `json:"client_version"`
		LicenseID     string `json:"license_id"`
		Revision      int64  `json:"revision"`
	}
	json.Unmarshal(body, &input)
	kind := "standard"
	if input.Code == strings.Repeat("1", 20) || input.LicenseID == "test-admin" {
		kind = "admin"
		p.Kind = kind
		p.LicenseID = "test-admin"
	}
	counter, _ := strconv.ParseUint(request.Counter, 10, 64)
	counterKey := p.LicenseID + request.DevicePub
	switch {
	case s.revoked.Load():
		p.Status, p.Error = "ERROR", "REVOKED"
	case request.Version != 1 || counter <= s.counters[counterKey]:
		p.Status = "ERROR"
		p.Error = "REPLAY"
	case request.TS < p.ServerTime-300 || request.TS > p.ServerTime+300:
		p.Status = "ERROR"
		p.Error = "TIMESTAMP_INVALID"
	case r.URL.Path == "/v1/activate":
		if input.Code != strings.Repeat("0", 20) && input.Code != strings.Repeat("1", 20) {
			p.Status = "ERROR"
			p.Error = "INVALID_CODE"
		} else if kind == "standard" && s.owner != request.DevicePub {
			s.owner = request.DevicePub
			s.revision++
			p.Revision = s.revision
		}
	case r.URL.Path == "/v1/renew":
		if kind == "standard" && (s.owner != request.DevicePub || input.Revision != s.revision) {
			p.Status = "ERROR"
			p.Error = "REPLACED"
		}
	default:
		p.Status = "ERROR"
		p.Error = "UNSUPPORTED_VERSION"
	}
	if p.Status == "ACTIVE" {
		s.counters[counterKey] = counter
		p.IssuedAt = p.ServerTime
		p.ExpiresAt = p.ServerTime + int64(licenseLifetime/time.Second)
	}
	envelope := testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", p)
	if s.malformed.Load() {
		envelope.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64))
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(envelope)
}
func (s *licenseTestIssuer) Manager(store licenseStore) *licenseManager {
	return newLicenseManager(licenseOptions{Store: store, Client: s.client, Origin: "https://license.test", Keys: map[string]ed25519.PublicKey{"test-issuer": s.pub}, Now: s.clock.Now, Elapsed: s.clock.Elapsed})
}
func testActivate(t *testing.T, m *licenseManager, admin bool) {
	t.Helper()
	code := strings.Repeat("0", 20)
	if admin {
		code = strings.Repeat("1", 20)
	}
	if err := m.Activate(context.Background(), code); err != nil {
		t.Fatal(err)
	}
	if !m.Allowed() {
		t.Fatal("valid lease did not open gate")
	}
}

func TestR232NormalizeAndFailClosed(t *testing.T) {
	code, err := normalizeLicenseCode("DL-OOOOO-00000-00000-00000")
	if err != nil || code != strings.Repeat("0", 20) {
		t.Fatal(code, err)
	}
	if code, err = normalizeLicenseCode("dl 00000 00000 00000 00000"); err != nil || code != strings.Repeat("0", 20) {
		t.Fatal("pasted code disagrees with protocol", code, err)
	}
	for _, code := range []string{"", strings.Repeat("U", 20), "DL-abc"} {
		if _, err := normalizeLicenseCode(code); err == nil {
			t.Fatal("invalid code accepted")
		}
	}
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	m.options.Keys = map[string]ed25519.PublicKey{}
	if m.Activate(context.Background(), strings.Repeat("0", 20)) == nil || m.Allowed() {
		t.Fatal("unconfigured production trust allowed activation")
	}
}

func TestR232LocalSideEffectsCheckAgain(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	a := &app{license: m}
	recorder := httptest.NewRecorder()
	a.handleCameraModePreference(recorder, httptest.NewRequest("POST", "/api/rig/camera", strings.NewReader(`{"mode":"free"}`)))
	if recorder.Code != http.StatusForbidden {
		t.Fatal("locked settings write was attempted", recorder.Code)
	}
	testActivate(t, m, false)
	if a.licenseSideEffect(context.Background()) != nil {
		t.Fatal("active operation rejected")
	}
	business, generation, _ := m.admission()
	old := context.WithValue(context.Background(), licenseContextKey{}, licenseAdmission{m, business, generation})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if a.licenseSideEffect(ctx) == nil {
		t.Fatal("queued canceled work can execute")
	}
	s.clock.Advance(licenseLifetime + time.Second)
	if a.licenseSideEffect(context.Background()) == nil || a.licenseBusinessContext().Err() == nil {
		t.Fatal("expiry did not cancel detached business work")
	}
	testActivate(t, m, false)
	if a.licenseSideEffect(old) == nil {
		t.Fatal("a new activation revived an old admitted operation")
	}
}
func TestR232LatestActivationAndTerminalRestart(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	a, b := s.Manager(store), s.Manager(&licenseTestStore{})
	testActivate(t, a, false)
	testActivate(t, b, false)
	if !b.Allowed() {
		t.Fatal("latest device was made to wait")
	}
	if err := a.Renew(context.Background()); err == nil || a.Snapshot().State != "REPLACED" || a.Allowed() {
		t.Fatal("old device not replaced", err, a.Snapshot())
	}
	count := s.requests.Load()
	restored := s.Manager(store)
	if restored.Snapshot().State != "REPLACED" {
		t.Fatal("terminal state lost on restart")
	}
	_ = restored.Renew(context.Background())
	if s.requests.Load() != count {
		t.Fatal("replaced device automatically sent a request")
	}
	testActivate(t, restored, false)
	if b.Renew(context.Background()) == nil || b.Allowed() {
		t.Fatal("explicit takeover did not replace B")
	}
}
func TestR232AdminAndIdempotentSameDevice(t *testing.T) {
	s := newLicenseTestIssuer(t)
	a, b := s.Manager(&licenseTestStore{}), s.Manager(&licenseTestStore{})
	testActivate(t, a, true)
	testActivate(t, b, true)
	for _, m := range []*licenseManager{a, b} {
		testActivate(t, m, true)
		if err := m.Renew(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	s2 := newLicenseTestIssuer(t)
	m := s2.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	revision := m.lease.Revision
	testActivate(t, m, false)
	if m.lease.Revision != revision {
		t.Fatal("same device kicked itself")
	}
}
func TestR232OfflineExpiryCacheAndRollback(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	testActivate(t, m, false)
	if m.nextRenew.Sub(s.clock.Now()) != time.Minute {
		t.Fatal("heartbeat differs from confirmed rule")
	}
	s.offline.Store(true)
	if m.Renew(context.Background()) == nil || !m.Allowed() {
		t.Fatal("transient outage discarded valid lease")
	}
	restarted := s.Manager(store)
	if !restarted.Allowed() {
		t.Fatal("valid cached lease did not survive restart")
	}
	s.clock.Advance(licenseLifetime + time.Second)
	if m.Allowed() || m.Snapshot().State != "NETWORK_LOCKED" {
		t.Fatal("lease was extended by failed request")
	}
	s.offline.Store(false)
	if err := m.Renew(context.Background()); err != nil || !m.Allowed() {
		t.Fatal("expired current device cannot recover", err)
	}
	rollback := 0
	for _, seconds := range []int{1, 299, 1} {
		rollback += seconds
		s.clock.StepWall(-time.Duration(seconds) * time.Second)
		allowed := s.Manager(store).Allowed()
		if allowed != (rollback <= 300) {
			t.Fatal("cached rollback must allow up to 300 seconds, rejecting larger steps", s.clock.Now(), allowed)
		}
	}
}
func TestR232SignatureAndPayloadTampering(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	testActivate(t, m, false)
	envelope := *m.disk.Lease
	for _, mutate := range []func(*signedLicenseEnvelope){func(e *signedLicenseEnvelope) { e.Kid = "unknown" }, func(e *signedLicenseEnvelope) { e.Algorithm = "none" }, func(e *signedLicenseEnvelope) { e.Payload += "=" }, func(e *signedLicenseEnvelope) { e.Signature = base64.RawURLEncoding.EncodeToString(make([]byte, 64)) }} {
		bad := envelope
		mutate(&bad)
		if _, err := m.verify(bad, ""); err == nil {
			t.Fatal("tampering accepted")
		}
	}
	for _, mutate := range []func(*licensePayload){func(p *licensePayload) { p.Kind = "superadmin" }, func(p *licensePayload) { p.DeviceHash = strings.Repeat("f", 64) }, func(p *licensePayload) { p.ExpiresAt = p.IssuedAt + int64(licenseLifetime/time.Second) + 1 }, func(p *licensePayload) { p.Revision = 0 }, func(p *licensePayload) { p.Version = 2 }} {
		bad := m.lease
		mutate(&bad)
		e := testSignedEnvelope(s.key, "test-issuer", "DL-LICENSE-RESPONSE-V1", bad)
		if _, err := m.verify(e, ""); err == nil {
			t.Fatal("invalid signed lease accepted", bad)
		}
	}
	s.malformed.Store(true)
	if m.Renew(context.Background()) == nil || !m.Allowed() {
		t.Fatal("invalid new response either authorized or destroyed valid cached lease")
	}
	if strictLicenseJSON([]byte(`{"version":1,"version":2}`), &licensePayload{}) == nil {
		t.Fatal("duplicate JSON accepted")
	}
}
func TestR232CounterAndStorageFailure(t *testing.T) {
	s := newLicenseTestIssuer(t)
	store := &licenseTestStore{}
	m := s.Manager(store)
	testActivate(t, m, false)
	before := m.disk.Counter
	restarted := s.Manager(store)
	if err := restarted.Renew(context.Background()); err != nil || restarted.disk.Counter <= before {
		t.Fatal("counter did not survive restart", err)
	}
	count := s.requests.Load()
	store.mu.Lock()
	store.fail = true
	store.mu.Unlock()
	if restarted.Renew(context.Background()) == nil || !restarted.Allowed() || s.requests.Load() != count {
		t.Fatal("counter failure sent a request or destroyed a valid lease")
	}
}
func TestR232HTTPAndLateResponseGate(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	a := &app{license: m, token: "local-test"}
	var calls int
	handler := a.withLicenseProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.Write([]byte("business")) }))
	request := httptest.NewRequest("GET", "/api/gameplay/overview", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Code != 403 || calls != 0 {
		t.Fatal("locked HTTP gate invoked business")
	}
	testActivate(t, m, false)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	if w.Body.String() != "business" {
		t.Fatal(w.Body.String())
	}
	b := s.Manager(&licenseTestStore{})
	late := a.withLicenseProtection(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testActivate(t, b, false)
		_ = m.Renew(context.Background())
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			t.Fatal("admitted request not canceled")
		}
		w.Write([]byte("private late result"))
	}))
	w = httptest.NewRecorder()
	late.ServeHTTP(w, request)
	if w.Body.Len() != 0 {
		t.Fatal("late business data escaped")
	}
	for _, path := range []string{"/api/status", "/api/events", "/api/champions/network", "/api/watch/rules", "/api/claim/execute", "/api/update/unknown", "/api/license/unknown"} {
		if licensePublicRoute(httptest.NewRequest("GET", path, nil)) {
			t.Fatal("unexpected whitelist", path)
		}
	}
}
func TestR232LCUWriteAndInFlightCancellation(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	var calls atomic.Int64
	client := &LCUClient{license: m, baseURL: "http://127.0.0.1:2999", token: "test", http: &http.Client{Transport: updateRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}}
	if client.RequestJSON(context.Background(), "POST", "/lol-matchmaking/v1/ready-check/accept", nil, nil) == nil || calls.Load() != 0 {
		t.Fatal("locked background write reached LCU")
	}
	testActivate(t, m, false)
	done := make(chan error, 1)
	go func() {
		done <- client.RequestJSON(context.Background(), "POST", "/lol-matchmaking/v1/ready-check/accept", nil, nil)
	}()
	deadline := time.After(time.Second)
	for calls.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("write never started")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	b := s.Manager(&licenseTestStore{})
	testActivate(t, b, false)
	_ = m.Renew(context.Background())
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("inflight write not canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("inflight write hung")
	}
	before := calls.Load()
	_ = client.RequestJSON(context.Background(), "PATCH", "/lol-champ-select/v1/session/actions/1", nil, nil)
	if calls.Load() != before {
		t.Fatal("queued old write executed after revocation")
	}
}
func TestR232RequestBindingAndDiagnosticRedaction(t *testing.T) {
	s := newLicenseTestIssuer(t)
	m := s.Manager(&licenseTestStore{})
	var events []map[string]any
	m.options.Observe = func(event map[string]any) { events = append(events, event) }
	testActivate(t, m, false)
	if _, err := m.verify(*m.disk.Lease, strings.Repeat("a", 32)); err == nil {
		t.Fatal("response for different request accepted")
	}
	encoded, _ := json.Marshal(events)
	if bytes.Contains(encoded, []byte(strings.Repeat("0", 20))) || bytes.Contains(encoded, []byte(m.disk.Lease.Signature)) || bytes.Contains(encoded, []byte("private_key")) {
		t.Fatal("secret in diagnostics")
	}
}
