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
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const licenseBuildEnabled = true

const licenseLifetime = 2 * time.Hour
const licenseHeartbeat = time.Minute
const licenseTimeout = 15 * time.Second
const licenseActivationTimeout = 35 * time.Second
const licenseClockTolerance = 300 * time.Second

var errLicenseLocked = errors.New("软件尚未激活")

type signedLicenseEnvelope struct {
	Algorithm string `json:"algorithm"`
	Kid       string `json:"kid"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}
type licensePayload struct {
	Version          int    `json:"version"`
	RequestID        string `json:"request_id"`
	DeviceHash       string `json:"device_pub_hash"`
	ServerTime       int64  `json:"server_time"`
	Status           string `json:"status"`
	Error            string `json:"error,omitempty"`
	LicenseID        string `json:"license_id,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Revision         int64  `json:"revision,omitempty"`
	IssuedAt         int64  `json:"issued_at,omitempty"`
	ExpiresAt        int64  `json:"expires_at,omitempty"`
	LicenseExpiresAt int64  `json:"license_expires_at,omitempty"`
}
type licenseRequest struct {
	Version   int    `json:"version"`
	RequestID string `json:"request_id"`
	DevicePub string `json:"device_pub"`
	TS        int64  `json:"ts"`
	Counter   string `json:"counter"`
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}
type licenseDiskState struct {
	PrivateKey     []byte                 `json:"private_key"`
	Counter        uint64                 `json:"counter"`
	Lease          *signedLicenseEnvelope `json:"lease,omitempty"`
	LastServerTime int64                  `json:"last_server_time"`
	LastWallTime   int64                  `json:"last_wall_time"`
	Terminal       string                 `json:"terminal,omitempty"`
	TerminalReason string                 `json:"terminal_reason,omitempty"`
	LeaseWallTime  int64                  `json:"lease_wall_time,omitempty"`
	LeaseRemaining int64                  `json:"lease_remaining,omitempty"`
}
type licenseStore interface {
	Load() (licenseDiskState, error)
	Save(licenseDiskState) error
}
type licenseOptions struct {
	Store   licenseStore
	Client  *http.Client
	Origin  string
	Keys    map[string]ed25519.PublicKey
	Now     func() time.Time
	Elapsed func() time.Time
	Observe func(map[string]any)
}
type licenseSnapshot struct {
	State            string `json:"state"`
	Message          string `json:"message"`
	Generation       uint64 `json:"generation"`
	LicenseExpiresAt int64  `json:"license_expires_at,omitempty"`
}
type licenseManager struct {
	mu                sync.Mutex
	opMu              sync.Mutex
	options           licenseOptions
	disk              licenseDiskState
	lease             licensePayload
	state             string
	deadline          time.Time
	wallDeadline      int64
	generation        uint64
	business          context.Context
	cancel            context.CancelFunc
	changed           chan struct{}
	clockOffset       int64
	nextRenew         time.Time
	failures          int
	wake              chan struct{}
	storeDirty        bool
	storeRetryAt      time.Time
	startupRenew      bool
	firstRenewPending bool
	storeFailures     int
}

func newLicenseManager(options licenseOptions) *licenseManager {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Elapsed == nil {
		options.Elapsed = time.Now
	}
	m := &licenseManager{options: options, state: "LOCKED", generation: 1, changed: make(chan struct{}, 1), wake: make(chan struct{}, 1)}
	m.business, m.cancel = context.WithCancel(context.Background())
	m.cancel()
	if options.Store == nil || options.Client == nil {
		m.state = "DEVICE_ERROR"
		return m
	}
	disk, err := options.Store.Load()
	if err != nil {
		m.state = "DEVICE_ERROR"
		return m
	}
	if len(disk.PrivateKey) == 0 {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			m.state = "DEVICE_ERROR"
			return m
		}
		disk.PrivateKey = key
		if options.Store.Save(disk) != nil {
			m.state = "DEVICE_ERROR"
			return m
		}
	}
	if len(disk.PrivateKey) != ed25519.PrivateKeySize || !bytes.Equal(ed25519.NewKeyFromSeed(disk.PrivateKey[:32]), disk.PrivateKey) {
		m.state = "DEVICE_ERROR"
		return m
	}
	m.disk = disk
	if disk.Terminal == "REPLACED" || disk.Terminal == "REVOKED" {
		m.state = disk.Terminal
		return m
	}
	if disk.Lease != nil {
		payload, err := m.verify(*disk.Lease, "")
		if err == nil && payload.Status == "ACTIVE" {
			m.lease = payload
			now := options.Now()
			remaining := min(int64(licenseLifetime/time.Second), payload.ExpiresAt-max(now.Unix(), payload.ServerTime))
			if disk.LeaseWallTime > 0 && disk.LeaseRemaining > 0 {
				// Separate lease anchor from the counter's wall high-water mark:
				// failed renew attempts must not refresh the cached lease lifetime.
				remaining = min(int64(licenseLifetime/time.Second), payload.ExpiresAt-payload.ServerTime, disk.LeaseRemaining) - max(int64(0), now.Unix()-disk.LeaseWallTime)
			}
			if !licenseClockSteppedBack(now.Unix(), disk.LastServerTime) && !licenseClockSteppedBack(now.Unix(), disk.LastWallTime) && remaining > 0 {
				m.deadline = options.Elapsed().Add(time.Duration(remaining) * time.Second)
				m.wallDeadline = now.Unix() + remaining
				m.setStateLocked("ACTIVE")
			} else {
				m.state = "NETWORK_LOCKED"
				m.startupRenew, m.firstRenewPending = true, true
			}
		}
	}
	return m
}

func normalizeLicenseCode(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, value)
	if len(value) == 22 && strings.HasPrefix(value, "DL") {
		value = value[2:]
	}
	value = strings.Map(func(r rune) rune {
		if r == '-' || r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		if r == 'O' {
			return '0'
		}
		if r == 'I' || r == 'L' {
			return '1'
		}
		return r
	}, value)
	if len(value) != 20 {
		return "", errors.New("INVALID_CODE")
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", r) {
			return "", errors.New("INVALID_CODE")
		}
	}
	return value, nil
}
func licenseDeviceHash(key []byte) string {
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:])
}
func rawURLDecode(value string, size int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(value)
	if err != nil || (size >= 0 && len(decoded) != size) || base64.RawURLEncoding.EncodeToString(decoded) != value {
		return nil, errors.New("invalid encoding")
	}
	return decoded, nil
}

// Check exact tagged names before encoding/json's case-insensitive matching.
// Dynamic map keys remain data; their values still use the declared schema.
func strictLicenseJSON(data []byte, target any) error {
	targetType := reflect.TypeOf(target)
	if targetType == nil || targetType.Kind() != reflect.Pointer || reflect.ValueOf(target).IsNil() {
		return errors.New("invalid JSON target")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func(reflect.Type) error
	walk = func(expected reflect.Type) error {
		for expected.Kind() == reflect.Pointer {
			expected = expected.Elem()
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			var fields map[string]reflect.Type
			switch expected.Kind() {
			case reflect.Struct:
				fields = licenseJSONFields(expected)
			case reflect.Map:
				if expected.Key().Kind() != reflect.String {
					return errors.New("invalid JSON map schema")
				}
			default:
				return errors.New("invalid JSON object schema")
			}
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok || seen[name] {
					return errors.New("duplicate JSON field")
				}
				seen[name] = true
				var child reflect.Type
				if expected.Kind() == reflect.Map {
					child = expected.Elem()
				} else {
					child = fields[name]
					if child == nil {
						return errors.New("unknown JSON field")
					}
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		} else if delim == '[' {
			if expected.Kind() != reflect.Array && expected.Kind() != reflect.Slice {
				return errors.New("invalid JSON array schema")
			}
			for decoder.More() {
				if err := walk(expected.Elem()); err != nil {
					return err
				}
			}
		} else {
			return errors.New("invalid JSON")
		}
		_, err = decoder.Token()
		return err
	}
	if err := walk(targetType.Elem()); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// Anonymous structs promote their fields unless explicitly named by a tag.
// Match encoding/json's depth/tag precedence, excluding ambiguous names.
func licenseJSONFields(root reflect.Type) map[string]reflect.Type {
	type candidate struct {
		typeOf reflect.Type
		depth  int
		tagged bool
		count  int
	}
	selected := map[string]candidate{}
	visiting := map[reflect.Type]bool{}
	var collect func(reflect.Type, int)
	collect = func(t reflect.Type, depth int) {
		if visiting[t] {
			return
		}
		visiting[t] = true
		defer delete(visiting, t)
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			base := field.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if field.Anonymous && name == "" && base.Kind() == reflect.Struct {
				collect(base, depth+1)
				continue
			}
			if field.PkgPath != "" {
				continue
			}
			tagged := name != ""
			if !tagged {
				name = field.Name
			}
			old, exists := selected[name]
			if !exists || depth < old.depth || depth == old.depth && tagged && !old.tagged {
				selected[name] = candidate{field.Type, depth, tagged, 1}
			} else if depth == old.depth && tagged == old.tagged {
				old.count++
				selected[name] = old
			}
		}
	}
	collect(root, 0)
	fields := map[string]reflect.Type{}
	for name, field := range selected {
		if field.count == 1 {
			fields[name] = field.typeOf
		}
	}
	return fields
}
func verifySignedEnvelope(envelope signedLicenseEnvelope, keys map[string]ed25519.PublicKey, domain string) ([]byte, error) {
	if envelope.Algorithm != "Ed25519" || len(envelope.Kid) == 0 || len(envelope.Kid) > 64 || strings.ContainsAny(envelope.Kid, "\r\n") {
		return nil, errors.New("invalid signature metadata")
	}
	for _, r := range envelope.Kid {
		if r < 0x21 || r > 0x7e {
			return nil, errors.New("invalid signing key ID")
		}
	}
	key := keys[envelope.Kid]
	if len(key) != ed25519.PublicKeySize {
		return nil, errors.New("unknown signing key")
	}
	payload, err := rawURLDecode(envelope.Payload, -1)
	if err != nil {
		return nil, err
	}
	sig, err := rawURLDecode(envelope.Signature, ed25519.SignatureSize)
	if err != nil {
		return nil, err
	}
	if !ed25519.Verify(key, []byte(domain+"\n"+envelope.Kid+"\n"+envelope.Payload), sig) {
		return nil, errors.New("invalid signature")
	}
	return payload, nil
}
func (m *licenseManager) verify(envelope signedLicenseEnvelope, requestID string) (licensePayload, error) {
	var p licensePayload
	data, err := verifySignedEnvelope(envelope, m.options.Keys, "DL-LICENSE-RESPONSE-V1")
	if err != nil {
		return p, err
	}
	if len(data) > 24*1024 {
		return p, errors.New("large payload")
	}
	if err := strictLicenseJSON(data, &p); err != nil {
		return p, err
	}
	pub := ed25519.PrivateKey(m.disk.PrivateKey).Public().(ed25519.PublicKey)
	requestBytes, requestErr := hex.DecodeString(p.RequestID)
	if p.Version != 1 || requestErr != nil || len(requestBytes) != 16 || hex.EncodeToString(requestBytes) != p.RequestID || (requestID != "" && p.RequestID != requestID) || p.DeviceHash != licenseDeviceHash(pub) || p.ServerTime <= 0 {
		return p, errors.New("unbound response")
	}
	if p.Status == "ERROR" {
		switch p.Error {
		case "INVALID_CODE", "REPLACED", "REVOKED", "EXPIRED", "REPLAY", "TIMESTAMP_INVALID", "RATE_LIMITED", "SERVICE_UNAVAILABLE", "UNSUPPORTED_VERSION":
			return p, nil
		}
		return p, errors.New("unknown error")
	}
	if p.Status != "ACTIVE" || p.Error != "" || len(p.LicenseID) == 0 || len(p.LicenseID) > 128 || (p.Kind != "standard" && p.Kind != "admin") || p.Revision < 1 || p.Revision > 9007199254740991 || p.IssuedAt <= 0 || p.IssuedAt > p.ServerTime || p.ExpiresAt <= p.ServerTime || p.ExpiresAt-p.IssuedAt > int64(licenseLifetime/time.Second) {
		return p, errors.New("invalid lease")
	}
	// Presence matters: an omitted field means permanent; null and zero do not.
	// Integer decoding above already rejects strings, fractions and overflow.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return p, err
	}
	if _, present := fields["license_expires_at"]; present && (p.LicenseExpiresAt <= 0 || p.LicenseExpiresAt < p.IssuedAt || p.ExpiresAt > p.LicenseExpiresAt) {
		return p, errors.New("invalid license expiry")
	}
	return p, nil
}
func (m *licenseManager) setStateLocked(state string) {
	if m.state == state {
		return
	}
	m.cancel()
	m.state = state
	m.generation++
	if state == "ACTIVE" {
		m.business, m.cancel = context.WithCancel(context.Background())
	}
	select {
	case m.changed <- struct{}{}:
	default:
	}
	if m.options.Observe != nil {
		m.options.Observe(map[string]any{"event": "license_state", "state": state, "generation": m.generation})
	}
}
func (m *licenseManager) expireLocked() {
	if m.state != "ACTIVE" {
		return
	}
	now := m.options.Now().Unix()
	rollback := licenseClockSteppedBack(now, m.disk.LastWallTime)
	if !m.options.Elapsed().Before(m.deadline) || now >= m.wallDeadline || rollback {
		m.setStateLocked("NETWORK_LOCKED")
		if rollback {
			m.nextRenew = m.options.Elapsed()
			m.observeStoreLocked("license_clock", "rollback", "wall_clock")
			select {
			case m.wake <- struct{}{}:
			default:
			}
		}
	}
}

func licenseClockSteppedBack(now, reference int64) bool {
	return now < reference-int64(licenseClockTolerance/time.Second)
}
func (m *licenseManager) Allowed() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	return m.state == "ACTIVE"
}
func (m *licenseManager) admission() (context.Context, uint64, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	return m.business, m.generation, m.state == "ACTIVE"
}
func licenseMessage(state string) string {
	switch state {
	case "ACTIVE":
		return ""
	case "REPLACED":
		return "注册码已在其他设备使用或已被重置"
	case "REVOKED":
		return "注册码已停用"
	case "EXPIRED":
		return "注册码已过期"
	case "NETWORK_LOCKED":
		return "无法连接激活服务，请联网后重试"
	case "DEVICE_ERROR":
		return "设备授权信息无法读取，请重试"
	}
	return "请输入注册码激活软件"
}
func (m *licenseManager) Snapshot() licenseSnapshot {
	if m == nil {
		return licenseSnapshot{State: "LOCKED", Message: licenseMessage("LOCKED")}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	message := licenseMessage(m.state)
	if m.state == "NETWORK_LOCKED" && m.firstRenewPending {
		message = "正在连接激活服务…"
	}
	if m.state == "REVOKED" && m.disk.TerminalReason == "EXPIRED" {
		message = licenseMessage("EXPIRED")
	}
	return licenseSnapshot{State: m.state, Message: message, Generation: m.generation, LicenseExpiresAt: m.lease.LicenseExpiresAt}
}
func (m *licenseManager) Activate(ctx context.Context, code string) error {
	code, err := normalizeLicenseCode(code)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, licenseActivationTimeout)
	defer cancel()
	payload := map[string]any{"code": code, "client_version": version}
	err = m.exchange(ctx, "/v1/activate", payload, true)
	if err != nil && ctx.Err() == nil && (err.Error() == "TIMESTAMP_INVALID" || retryLicenseActivation(err)) {
		return m.exchange(ctx, "/v1/activate", payload, true)
	}
	return err
}
func (m *licenseManager) Renew(ctx context.Context) error {
	defer func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.firstRenewPending {
			m.firstRenewPending = false
			select {
			case m.changed <- struct{}{}:
			default:
			}
		}
	}()
	m.mu.Lock()
	id, revision, state := m.lease.LicenseID, m.lease.Revision, m.state
	m.mu.Unlock()
	if id == "" || state == "REPLACED" || state == "REVOKED" || state == "DEVICE_ERROR" {
		return errLicenseLocked
	}
	payload := map[string]any{"license_id": id, "revision": revision, "client_version": version}
	err := m.exchange(ctx, "/v1/renew", payload, false)
	if err != nil && err.Error() == "TIMESTAMP_INVALID" {
		return m.exchange(ctx, "/v1/renew", payload, false)
	}
	return err
}
func (m *licenseManager) exchange(ctx context.Context, path string, payload any, activate bool) (exchangeErr error) {
	if !m.lockOperation(ctx) {
		return errLicenseLocked
	}
	defer m.opMu.Unlock()
	if ctx.Err() != nil {
		return errLicenseLocked
	}
	// A queued renew must not revive a terminal result received by an activation.
	m.mu.Lock()
	if len(m.disk.PrivateKey) != ed25519.PrivateKeySize || (!activate && (m.state == "REPLACED" || m.state == "REVOKED")) {
		m.mu.Unlock()
		return errLicenseLocked
	}
	if !activate {
		payload = map[string]any{"license_id": m.lease.LicenseID, "revision": m.lease.Revision, "client_version": version}
	}
	if m.disk.Counter == ^uint64(0) {
		m.setStateLocked("DEVICE_ERROR")
		m.mu.Unlock()
		return errLicenseLocked
	}
	m.disk.Counter++
	m.disk.LastWallTime = max(m.disk.LastWallTime, m.options.Now().Unix())
	if m.options.Store.Save(m.disk) != nil {
		m.storeFailedLocked("counter")
		m.scheduleRetryLocked()
		m.mu.Unlock()
		return errLicenseLocked
	}
	m.storeSavedLocked("counter")
	key := append(ed25519.PrivateKey(nil), m.disk.PrivateKey...)
	counter := m.disk.Counter
	ts := m.options.Now().Unix() + m.clockOffset
	m.mu.Unlock()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req := licenseRequest{Version: 1, RequestID: hex.EncodeToString(id), DevicePub: base64.RawURLEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), TS: ts, Counter: strconv.FormatUint(counter, 10), Payload: base64.RawURLEncoding.EncodeToString(body)}
	message, _ := licenseRequestSignatureText(path, req)
	req.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(message)))
	encoded, _ := json.Marshal(req)
	ctx, cancel := context.WithTimeout(ctx, licenseTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, m.options.Origin+path, bytes.NewReader(encoded))
	if err != nil {
		return errLicenseLocked
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cache-Control", "no-store")
	started := m.options.Elapsed()
	timings := newLicenseNetworkTimings()
	request = request.WithContext(httptrace.WithClientTrace(request.Context(), timings.trace()))
	failure, reason, status, serverError, ray := "parse", "body", 0, "", ""
	retryable := false
	defer func() {
		if exchangeErr != nil {
			m.observeExchangeFailure(activationKind(activate), failure, reason, status, serverError, ray, started, timings)
			if retryable {
				exchangeErr = &licenseExchangeFailure{retryable: true, cause: exchangeErr}
			}
		}
	}()
	response, err := m.options.Client.Do(request)
	if err != nil {
		failure, reason = licenseNetworkFailure(err), "network"
		retryable = failure != "tls" && !errors.Is(err, context.Canceled)
		return m.failed("network")
	}
	defer response.Body.Close()
	status, ray = response.StatusCode, licenseCFRay(response.Header.Get("CF-Ray"))
	retryable = transientLicenseStatus(status)
	timings.begin("body_ms")
	data, err := io.ReadAll(io.LimitReader(response.Body, 32*1024+1))
	timings.end("body_ms")
	if err != nil || len(data) > 32*1024 {
		if err != nil {
			failure = licenseNetworkFailure(err)
			retryable = retryable || failure == "timeout" || failure == "connect"
		}
		return m.failed("body")
	}
	var envelope signedLicenseEnvelope
	if strictLicenseJSON(data, &envelope) != nil {
		if code, unsigned := licenseUnsignedError(data); unsigned {
			failure, serverError, retryable = "unsigned_error", code, true
		} else if status != http.StatusOK {
			failure = "http_status"
		}
		return m.failed("envelope")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, signatureErr := verifySignedEnvelope(envelope, m.options.Keys, "DL-LICENSE-RESPONSE-V1"); signatureErr != nil {
		failure = "bad_signature"
		m.scheduleRetryLocked()
		return errLicenseLocked
	}
	p, err := m.verify(envelope, req.RequestID)
	if err != nil {
		m.scheduleRetryLocked()
		return errLicenseLocked
	}
	m.clockOffset = p.ServerTime - m.options.Now().Unix()
	if p.Status == "ERROR" {
		failure, reason, serverError = "http_status", "server", p.Error
		retryable = p.Error == "SERVICE_UNAVAILABLE"
		if p.Error == "REPLACED" || p.Error == "REVOKED" || p.Error == "EXPIRED" {
			state := p.Error
			if state == "EXPIRED" {
				state = "REVOKED"
			}
			m.disk.Terminal = state
			m.disk.TerminalReason = p.Error
			m.disk.Lease = nil
			m.setStateLocked(state)
			if m.options.Store.Save(m.disk) != nil {
				m.storeFailedLocked("terminal")
			} else {
				m.storeSavedLocked("terminal")
			}
		} else {
			m.scheduleRetryLocked()
		}
		return errors.New(p.Error)
	}
	if response.StatusCode != http.StatusOK {
		failure, reason = "http_status", "http_status"
	}
	if response.StatusCode != http.StatusOK || (!activate && (p.LicenseID != m.lease.LicenseID || p.Revision < m.lease.Revision)) {
		m.scheduleRetryLocked()
		return errLicenseLocked
	}
	deadline := started.Add(time.Duration(p.ExpiresAt-p.ServerTime) * time.Second)
	if !m.options.Elapsed().Before(deadline) {
		m.scheduleRetryLocked()
		return errLicenseLocked
	}
	// A newly signed business expiry may shorten an existing lease. Other
	// renewals still cannot roll its deadline backwards.
	if !activate && m.state == "ACTIVE" && deadline.Before(m.deadline) && (p.LicenseExpiresAt == 0 || p.ExpiresAt != p.LicenseExpiresAt) {
		m.scheduleRetryLocked()
		return errLicenseLocked
	}
	disk := m.disk
	disk.Lease = &envelope
	disk.Terminal = ""
	disk.TerminalReason = ""
	disk.LastServerTime = p.ServerTime
	disk.LastWallTime = m.options.Now().Unix()
	disk.LeaseWallTime = disk.LastWallTime
	disk.LeaseRemaining = int64(deadline.Sub(m.options.Elapsed()) / time.Second)
	if m.state == "ACTIVE" && (m.lease.LicenseID != p.LicenseID || m.lease.Revision != p.Revision) {
		m.setStateLocked("LOCKED")
	}
	m.disk = disk
	m.lease = p
	m.deadline = deadline
	// Wall-clock bound includes the measured server offset; never grants more
	// than the 2-hour contract even if the local clock is behind the server.
	m.wallDeadline = m.options.Now().Unix() + int64(deadline.Sub(m.options.Elapsed())/time.Second)
	if m.options.Store.Save(m.disk) != nil {
		m.storeFailedLocked("lease")
	} else {
		m.storeSavedLocked("lease")
	}
	m.failures = 0
	m.startupRenew, m.firstRenewPending = false, false
	m.nextRenew = m.options.Elapsed().Add(licenseHeartbeat)
	m.setStateLocked("ACTIVE")
	if m.options.Observe != nil {
		row := map[string]any{"event": "license_renew", "kind": activationKind(activate), "result": "ok", "request_id": req.RequestID, "revision": p.Revision, "duration_ms": m.options.Elapsed().Sub(started).Milliseconds(), "http_status": status, "elapsed_ms": max(int64(0), m.options.Elapsed().Sub(started).Milliseconds())}
		if ray != "" {
			row["cf_ray"] = ray
		}
		timings.add(row)
		m.options.Observe(row)
	}
	return nil
}
func (m *licenseManager) scheduleRetryLocked() {
	m.failures++
	delays := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute}
	if m.startupRenew {
		delays = []time.Duration{3 * time.Second, 6 * time.Second, 12 * time.Second, 15 * time.Second, 30 * time.Second, time.Minute}
	}
	delay := delays[min(m.failures-1, len(delays)-1)]
	m.nextRenew = m.options.Elapsed().Add(delay)
	m.expireLocked()
}

func licenseRequestSignatureText(path string, request licenseRequest) (string, error) {
	body, err := rawURLDecode(request.Payload, -1)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(body)
	return strings.Join([]string{"DL-LICENSE-REQUEST-V1", "POST", path, request.RequestID, request.DevicePub, strconv.FormatInt(request.TS, 10), request.Counter, hex.EncodeToString(hash[:])}, "\n"), nil
}

func (m *licenseManager) observeStoreLocked(event, result, stage string) {
	if m.options.Observe != nil {
		m.options.Observe(map[string]any{"event": event, "result": result, "stage": stage})
	}
}
func (m *licenseManager) storeFailedLocked(stage string) {
	m.storeDirty = true
	m.storeFailures++
	delay := []time.Duration{15 * time.Second, 30 * time.Second, time.Minute}[min(m.storeFailures-1, 2)]
	m.storeRetryAt = m.options.Elapsed().Add(delay)
	m.observeStoreLocked("license_store_write", "failed", stage)
}
func (m *licenseManager) storeSavedLocked(stage string) {
	if m.storeDirty {
		m.observeStoreLocked("license_store_write", "recovered", stage)
	}
	m.storeDirty = false
	m.storeFailures = 0
}
func (m *licenseManager) maintain(ctx context.Context) {
	m.opMu.Lock()
	m.mu.Lock()
	terminal := m.state == "REPLACED" || m.state == "REVOKED"
	if terminal && m.storeDirty && !m.options.Elapsed().Before(m.storeRetryAt) {
		if m.options.Store.Save(m.disk) != nil {
			m.storeFailedLocked("terminal_retry")
		} else {
			m.storeSavedLocked("terminal_retry")
		}
	}
	m.mu.Unlock()
	m.opMu.Unlock()
	if !terminal {
		_ = m.Renew(ctx)
	}
}
func (m *licenseManager) failed(kind string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.scheduleRetryLocked()
	return errLicenseLocked
}
func (m *licenseManager) Run(ctx context.Context) {
	// Expiration is independent of a hung network exchange; every admission also
	// checks expiry, including when a machine resumes before this ticker runs.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	requests := make(chan struct{}, 1)
	// Admission remains occupied until maintain finishes, not merely until its
	// channel token is consumed. A slow initial cached-lease renew used to leave
	// nextRenew zero at the first tick and enqueue a second immediate exchange.
	queued := true // All accesses below are protected by m.mu.
	requests <- struct{}{}
	go func() {
		defer recoverPanic("license.Run.renew")
		for {
			select {
			case <-ctx.Done():
				return
			case <-requests:
				m.maintain(ctx)
				m.mu.Lock()
				queued = false
				m.mu.Unlock()
			}
		}
	}()
	for {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.cancel()
			m.mu.Unlock()
			return
		case <-ticker.C:
		case <-m.wake:
		}
		m.mu.Lock()
		m.expireLocked()
		terminal := m.state == "REPLACED" || m.state == "REVOKED"
		due := !queued && (terminal && m.storeDirty && !m.options.Elapsed().Before(m.storeRetryAt) || !terminal && !m.options.Elapsed().Before(m.nextRenew) && m.lease.LicenseID != "" && m.state != "DEVICE_ERROR")
		if due {
			queued = true
		}
		if due && !terminal {
			m.nextRenew = m.options.Elapsed().Add(licenseHeartbeat)
		}
		m.mu.Unlock()
		if due {
			select {
			case requests <- struct{}{}:
			default:
				m.mu.Lock()
				queued = false
				m.mu.Unlock()
			}
		}
	}
}

func productionLicenseManager(observe func(map[string]any)) *licenseManager {
	client := &http.Client{Timeout: licenseTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	store, err := newLicenseFileStore()
	if err != nil {
		return &licenseManager{state: "DEVICE_ERROR", options: licenseOptions{Now: time.Now, Elapsed: time.Now}, changed: make(chan struct{}, 1), wake: make(chan struct{}, 1), cancel: func() {}}
	}
	return newLicenseManager(licenseOptions{Store: store, Client: client, Origin: licenseOrigin, Keys: licenseTrustKeys(), Observe: observe})
}

func (a *app) handleLicenseStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, a.license.Snapshot())
}
func (a *app) handleLicenseActivate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var input struct {
		Code string `json:"code"`
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 257))
	if err != nil || len(data) > 256 || strictLicenseJSON(data, &input) != nil {
		http.Error(w, "注册码无效，请检查后重试", http.StatusBadRequest)
		return
	}
	if a.license == nil {
		http.Error(w, "激活服务暂不可用", http.StatusServiceUnavailable)
		return
	}
	if err := a.license.Activate(r.Context(), input.Code); err != nil {
		message := "无法连接激活服务，请联网后重试"
		if err.Error() == "INVALID_CODE" {
			message = "注册码无效，请检查后重试"
		} else if err.Error() == "REPLACED" || err.Error() == "REVOKED" || err.Error() == "EXPIRED" {
			message = a.license.Snapshot().Message
		}
		http.Error(w, message, http.StatusForbidden)
		return
	}
	respondJSON(w, a.license.Snapshot())
}

func (r licenseRequest) String() string { return fmt.Sprintf("license request %s", r.RequestID) }
