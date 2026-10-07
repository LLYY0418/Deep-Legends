package main

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const updateRepo = "LLYY0418/Deep-Legends"
const updateReleasePage = "https://github.com/" + updateRepo + "/releases"
const updateManifestPath = updateReleasePage + "/latest/download/latest.json"
const updateCacheTTL = 6 * time.Hour
const updateSourceTimeout = 8 * time.Second

var updateMirrors = []string{"", "https://ghfast.top/", "https://gh-proxy.com/", "https://ghproxy.net/"}
var updateVersionPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)
var updateHashPattern = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)
var updateFingerprintPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

type updateVersion struct {
	parts [3]uint64
	pre   string
}

func parseUpdateVersion(text string) (updateVersion, bool) {
	matches := updateVersionPattern.FindStringSubmatch(text)
	var value updateVersion
	if matches == nil {
		return value, false
	}
	for i := range value.parts {
		n, err := strconv.ParseUint(matches[i+1], 10, 64)
		if err != nil {
			return value, false
		}
		value.parts[i] = n
	}
	value.pre = matches[4]
	return value, true
}

func compareVersions(a, b string) int {
	left, leftOK := parseUpdateVersion(a)
	right, rightOK := parseUpdateVersion(b)
	if !leftOK || !rightOK {
		return 0
	}
	for i := range left.parts {
		if left.parts[i] < right.parts[i] {
			return -1
		}
		if left.parts[i] > right.parts[i] {
			return 1
		}
	}
	if left.pre == right.pre {
		return 0
	}
	if left.pre == "" {
		return 1
	}
	if right.pre == "" {
		return -1
	}
	ls, rs := strings.Split(left.pre, "."), strings.Split(right.pre, ".")
	for i := 0; i < min(len(ls), len(rs)); i++ {
		if ls[i] == rs[i] {
			continue
		}
		ln, le := strconv.ParseUint(ls[i], 10, 64)
		rn, re := strconv.ParseUint(rs[i], 10, 64)
		if le == nil && re == nil {
			if ln < rn {
				return -1
			}
			return 1
		}
		if le == nil {
			return -1
		}
		if re == nil {
			return 1
		}
		if ls[i] < rs[i] {
			return -1
		}
		return 1
	}
	if len(ls) < len(rs) {
		return -1
	}
	return 1
}

type updateAsset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
	URL    string `json:"url"`
}
type updateManifest struct {
	SignedManifest *signedLicenseEnvelope `json:"signed_manifest,omitempty"`
	Schema         int                    `json:"schema"`
	Version        string                 `json:"version"`
	Fingerprint    string                 `json:"fingerprint"`
	PublishedAt    time.Time              `json:"publishedAt"`
	MinSupported   string                 `json:"minSupported"`
	Notes          string                 `json:"notes"`
	Asset          updateAsset            `json:"asset"`
}
type updateProgress struct {
	ReceivedBytes   int64   `json:"receivedBytes"`
	TotalBytes      int64   `json:"totalBytes"`
	BytesPerSecond  float64 `json:"bytesPerSecond"`
	ETASeconds      int64   `json:"etaSeconds"`
	SelectingSource bool    `json:"selectingSource,omitempty"`
}
type updateStatus struct {
	Supported   bool           `json:"supported"`
	Checking    bool           `json:"checking"`
	Current     string         `json:"current"`
	Latest      string         `json:"latest"`
	Notes       string         `json:"notes"`
	SizeBytes   int64          `json:"sizeBytes"`
	PublishedAt time.Time      `json:"publishedAt"`
	State       string         `json:"state"`
	Progress    updateProgress `json:"progress"`
	Error       string         `json:"error"`
	Portable    bool           `json:"portable"`
	ManualOnly  bool           `json:"manualOnly"`
	ReleaseURL  string         `json:"releaseUrl"`
}
type updateCache struct {
	CheckedAt time.Time      `json:"checkedAt"`
	Current   string         `json:"current"`
	Manifest  updateManifest `json:"manifest"`
}
type updateSettings struct {
	Mirrors         []string `json:"mirrors"`
	PreferredSource string   `json:"preferredSource"`
}

type updateManager struct {
	trustKeys          map[string]ed25519.PublicKey
	migrationDirectory func() (string, error)
	startSourceTimer   func(time.Duration, func()) func()
	progressTicks      func() (<-chan time.Time, func())
	downloadClock      func() time.Time
	sourceDefaults     []string
	mu                 sync.Mutex
	status             updateStatus
	manifest           *updateManifest
	cache              updateCache
	lastManual         time.Time
	store              *localStore
	directory          string
	installDir         string
	installDetection   updateInstallDetection
	migrate            func() error
	portableDirectory  func() (string, error)
	mirrors            []string
	preferredSource    string
	client             *http.Client
	now                func() time.Time
	freeBytes          func(string) (int64, error)
	launch             func(string, string) error
	notify             func(string, any)
	diagnostic         func(map[string]any)
	ctx                context.Context
	stop               context.CancelFunc
	downloadCancel     context.CancelFunc
	downloadDone       chan struct{}
	checkDone          chan struct{}
}

type updateCheckFailure struct {
	stage      string
	errorKind  string
	safeError  string
	httpStatus int
	cause      error
}

func (e *updateCheckFailure) Error() string { return e.cause.Error() }
func (e *updateCheckFailure) Unwrap() error { return e.cause }

func updateDiagnosticMirrorPrefix(prefix string) string {
	if prefix == "" {
		return "direct"
	}
	parsed, err := url.Parse(prefix)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || len(parsed.Host) > 128 {
		return "custom"
	}
	// Custom mirror paths may contain tokens; log only the scheme and host.
	return "https://" + parsed.Host + "/"
}

func (u *updateManager) recordUpdateCheck(event map[string]any) {
	if u.diagnostic == nil {
		return
	}
	event["current_version"] = u.status.Current
	event["build_fingerprint"] = buildFingerprint
	u.diagnostic(event)
}

func newUpdateManager(current string, store *localStore, notify func(string, any)) *updateManager {
	return newUpdateManagerWithTrust(current, store, notify, updateTrustKeys())
}
func newUpdateManagerWithTrust(current string, store *localStore, notify func(string, any), keys map[string]ed25519.PublicKey) *updateManager {
	ctx, cancel := context.WithCancel(context.Background())
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = updateSourceTimeout
	transport.TLSHandshakeTimeout = updateSourceTimeout
	u := &updateManager{trustKeys: keys, store: store, mirrors: append([]string(nil), updateMirrors...), now: time.Now,
		client: &http.Client{Transport: transport}, freeBytes: updateDiskFreeBytes, launch: launchUpdateInstaller,
		notify: notify, ctx: ctx, stop: cancel,
		status: updateStatus{Current: current, State: "idle", Portable: true, ReleaseURL: updateReleasePage},
	}
	// R236: staging never reads a release cache or follows the production Latest.
	if !licenseOnlineUpdates {
		u.status.ReleaseURL = ""
		u.mirrors = nil
		return u
	}
	if current == "dev" {
		return u
	}
	if store == nil {
		return u
	}
	u.status.Supported = true
	u.directory = filepath.Join(store.root, "updates")
	u.installDetection, _ = detectUpdateInstallation()
	u.installDir = u.installDetection.directory
	u.status.Portable = u.installDir == ""
	u.migrate = u.preparePortableData
	u.portableDirectory = defaultPortableInstallDirectory
	if data, err := readLocalStoreFile(store, "update-settings.json"); err == nil && len(data) < 16384 {
		var settings updateSettings
		if json.Unmarshal(data, &settings) == nil && validUpdateMirrors(settings.Mirrors) == nil {
			u.mirrors = settings.Mirrors
			if updateSourcePresent(u.sourcesLocked(), settings.PreferredSource) {
				u.preferredSource = settings.PreferredSource
			}
		}
	}
	if data, err := readLocalStoreFile(store, "update-manifest.json"); err == nil && len(data) <= 256*1024 {
		var cached updateCache
		if strictLicenseJSON(data, &cached) == nil && verifyUpdateManifestTrust(cached.Manifest, u.trustKeys) == nil {
			u.cache = cached
		}
	}
	return u
}

func (u *updateManager) Status() updateStatus {
	if u == nil {
		if !licenseOnlineUpdates {
			return updateStatus{Current: version, State: "idle", Portable: true}
		}
		return updateStatus{Current: version, State: "idle", Portable: true, ReleaseURL: updateReleasePage}
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

func (u *updateManager) publish() {
	if u.notify != nil {
		u.notify("update:status", u.Status())
	}
}

func (u *updateManager) Start() {
	if !licenseOnlineUpdates || !u.Status().Supported {
		return
	}
	if u.diagnostic != nil {
		d := u.installDetection
		u.diagnostic(d.diagnosticEvent())
	}
	goSafe("update.Start.1", func() {
		u.Check(false)
		ticker := time.NewTicker(updateCacheTTL)
		defer ticker.Stop()
		for {
			select {
			case <-u.ctx.Done():
				return
			case <-ticker.C:
				u.Check(false)
			}
		}
	})
}

func (u *updateManager) Close() {
	if u != nil {
		u.stop()
	}
}

func validateUpdateManifest(m updateManifest) error {
	_, versionOK := parseUpdateVersion(m.Version)
	_, minimumOK := parseUpdateVersion(m.MinSupported)
	base := "Deep-Legends-Setup-" + strings.TrimPrefix(m.Version, "v")
	expected := base + ".exe"
	// The published public package and both older names are an exact allowlist.
	for _, name := range []string{base + "-" + m.Fingerprint + ".exe", base + "-public.exe"} {
		if m.Asset.Name == name {
			expected = name
		}
	}
	parsed, err := url.Parse(m.Asset.URL)
	if m.Schema != 1 || !versionOK || (m.MinSupported != "" && !minimumOK) ||
		!updateFingerprintPattern.MatchString(m.Fingerprint) || m.Asset.Name != expected ||
		m.Asset.Size <= 0 || m.Asset.Size > 2<<30 || !updateHashPattern.MatchString(m.Asset.SHA256) ||
		m.PublishedAt.IsZero() || len(m.Notes) > 128*1024 || err != nil ||
		parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		parsed.Path != "/"+updateRepo+"/releases/download/v"+strings.TrimPrefix(m.Version, "v")+"/"+expected {
		return errors.New("更新清单格式不正确，请从发布页手动下载")
	}
	return nil
}

func validUpdateMirrors(mirrors []string) error {
	if len(mirrors) == 0 || len(mirrors) > 8 {
		return errors.New("请提供 1 至 8 个下载线路")
	}
	for _, prefix := range mirrors {
		if prefix == "" {
			continue
		}
		parsed, err := url.Parse(prefix)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasSuffix(prefix, "/") {
			return errors.New("镜像前缀必须是以 / 结尾的 HTTPS 地址；空字符串表示直连")
		}
	}
	return nil
}

func (u *updateManager) Settings() updateSettings {
	u.mu.Lock()
	defer u.mu.Unlock()
	return updateSettings{Mirrors: append([]string(nil), u.mirrors...), PreferredSource: u.preferredSource}
}

func (u *updateManager) SetSettings(settings updateSettings) error {
	if !licenseOnlineUpdates {
		return errors.New("当前构建不支持更新")
	}
	if err := validUpdateMirrors(settings.Mirrors); err != nil {
		return err
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.busyLocked() {
		return errors.New("更新任务进行中，请稍后修改线路")
	}
	configured := append(append([]string(nil), settings.Mirrors...), u.defaultUpdateSources()...)
	if !updateSourcePresent(configured, settings.PreferredSource) {
		settings.PreferredSource = ""
	}
	if settings.PreferredSource == "" && updateSourcePresent(configured, u.preferredSource) {
		settings.PreferredSource = u.preferredSource
	}
	data, _ := json.Marshal(settings)
	if err := writeLocalStoreFile(u.store, "update-settings.json", data); err != nil {
		return err
	}
	u.mirrors = append([]string(nil), settings.Mirrors...)
	u.preferredSource = settings.PreferredSource
	u.lastManual = time.Time{}
	return nil
}

func (u *updateManager) busyLocked() bool {
	switch u.status.State {
	case "checking", "downloading", "verifying", "applying":
		return true
	}
	return false
}

// Manual checks bypass the six-hour cache, but clicks in one five-second burst
// share the first request. A running check is always single-flight.
func (u *updateManager) Check(force bool) bool {
	if !licenseOnlineUpdates {
		return false
	}
	u.mu.Lock()
	now := u.now()
	if !u.status.Supported || u.busyLocked() || (force && !u.lastManual.IsZero() && now.Sub(u.lastManual) < 5*time.Second) {
		u.mu.Unlock()
		return false
	}
	if force {
		u.lastManual = now
	}
	cached := !force && !u.cache.CheckedAt.IsZero() && now.Sub(u.cache.CheckedAt) >= 0 && now.Sub(u.cache.CheckedAt) < updateCacheTTL
	u.status.Checking, u.status.State, u.status.Error = true, "checking", ""
	previous := u.manifest
	done := make(chan struct{})
	u.checkDone = done
	mirrors := u.sourcesLocked()
	manifest := u.cache.Manifest
	upgraded := u.cache.Current != "" && compareVersions(u.status.Current, u.cache.Current) > 0
	u.mu.Unlock()
	u.publish()
	go func() {
		defer recoverPanic("update.Check.1")

		defer close(done)
		var err error
		checkedSource := "cached"
		// Reserve the check before cleanup so a manual download cannot race it.
		if upgraded {
			err = u.cleanDownloads("")
			if err == nil {
				u.mu.Lock()
				u.cache.Current = u.status.Current
				data, _ := json.Marshal(u.cache)
				_ = writeLocalStoreFile(u.store, "update-manifest.json", data)
				u.mu.Unlock()
			}
		}
		if err == nil && !cached {
			manifest, checkedSource, err = u.fetchManifest(u.ctx, mirrors)
		}
		if err == nil && u.ctx.Err() == nil {
			keep := ""
			if compareVersions(manifest.Version, u.status.Current) > 0 {
				keep = manifest.Asset.Name
			}
			err = u.cleanDownloads(keep)
		}
		ready := false
		if err == nil && u.ctx.Err() == nil && compareVersions(manifest.Version, u.status.Current) > 0 {
			ready = verifyUpdateFile(u.ctx, filepath.Join(u.directory, manifest.Asset.Name), manifest.Asset) == nil
		}
		u.mu.Lock()
		u.status.Checking = false
		if err != nil || u.ctx.Err() != nil {
			u.status.State = "idle"
			if previous != nil && compareVersions(previous.Version, u.status.Current) > 0 {
				u.status.State = "available"
			}
			if err != nil {
				u.status.Error = "无法检查更新：" + err.Error()
			}
		} else {
			u.manifest = &manifest
			u.status.Latest, u.status.Notes, u.status.SizeBytes, u.status.PublishedAt = manifest.Version, manifest.Notes, manifest.Asset.Size, manifest.PublishedAt
			u.status.ManualOnly = manifest.MinSupported != "" && compareVersions(u.status.Current, manifest.MinSupported) < 0
			u.status.Progress = updateProgress{}
			u.status.State = "idle"
			if compareVersions(manifest.Version, u.status.Current) > 0 {
				u.status.State = "available"
				if ready {
					u.status.State = "ready"
				}
			}
			if !cached {
				u.cache = updateCache{CheckedAt: u.now(), Current: u.status.Current, Manifest: manifest}
			}
			data, _ := json.Marshal(u.cache)
			_ = writeLocalStoreFile(u.store, "update-manifest.json", data)
		}
		resultState := u.status.State
		u.mu.Unlock()
		if err == nil && u.ctx.Err() == nil {
			u.recordUpdateCheck(map[string]any{
				"event": "update_check_succeeded", "latest_version": manifest.Version,
				"state": resultState, "cached": cached, "mirror_prefix": func() string {
					if cached {
						return "cached"
					}
					return updateDiagnosticMirrorPrefix(checkedSource)
				}(),
			})
		} else if err != nil {
			var sourceFailure *updateCheckFailure
			if !errors.As(err, &sourceFailure) {
				u.recordUpdateCheck(map[string]any{
					"event": "update_check_failed", "stage": "cleanup",
					"mirror_prefix": "none", "http_status": 0,
					"error_kind": diagnosticErrorKind(err), "error": "local-cleanup-failed",
				})
			}
		}
		u.publish()
	}()
	return true
}

func (u *updateManager) fetchManifest(ctx context.Context, mirrors []string) (updateManifest, string, error) {
	var last error
	for _, prefix := range mirrors {
		if ctx.Err() != nil {
			return updateManifest{}, "", ctx.Err()
		}
		manifest, err := u.fetchManifestSource(ctx, prefix+updateManifestPath)
		if err == nil {
			u.rememberUpdateSource(prefix)
			return manifest, prefix, nil
		}
		if !isCancellation(err) {
			var failure *updateCheckFailure
			if errors.As(err, &failure) {
				u.recordUpdateCheck(map[string]any{
					"event": "update_check_failed", "stage": failure.stage,
					"mirror_prefix": updateDiagnosticMirrorPrefix(prefix),
					"http_status":   failure.httpStatus, "error_kind": failure.errorKind, "error": failure.safeError,
				})
			}
		}
		last = err
	}
	if last == nil {
		last = errors.New("没有可用下载线路")
	}
	return updateManifest{}, "", last
}

func (u *updateManager) fetchManifestSource(ctx context.Context, target string) (updateManifest, error) {
	ctx, cancel := context.WithTimeout(ctx, updateSourceTimeout)
	defer cancel()
	var manifest updateManifest
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return manifest, &updateCheckFailure{stage: "fetch", errorKind: "request", safeError: "request-invalid", cause: err}
	}
	request.Header.Set("User-Agent", "Deep-Legends/"+u.status.Current)
	request.Header.Set("Cache-Control", "no-cache")
	response, err := u.client.Do(request)
	if err != nil {
		return manifest, &updateCheckFailure{stage: "fetch", errorKind: diagnosticErrorKind(err), safeError: "request-failed", cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return manifest, &updateCheckFailure{stage: "fetch", errorKind: "http", safeError: "http-error", httpStatus: response.StatusCode, cause: fmt.Errorf("%s：HTTP %d", target, response.StatusCode)}
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 256*1024+1))
	if err != nil {
		return manifest, &updateCheckFailure{stage: "fetch", errorKind: diagnosticErrorKind(err), safeError: "body-read-failed", httpStatus: response.StatusCode, cause: err}
	}
	if len(data) > 256*1024 {
		return manifest, &updateCheckFailure{stage: "parse", errorKind: "response-too-large", safeError: "manifest-too-large", httpStatus: response.StatusCode, cause: errors.New("更新清单无法读取")}
	}
	if err := strictLicenseJSON(data, &manifest); err != nil {
		return manifest, &updateCheckFailure{stage: "parse", errorKind: "decode", safeError: "manifest-decode-failed", httpStatus: response.StatusCode, cause: errors.New("更新清单无法读取")}
	}
	if err := verifyUpdateManifestTrust(manifest, u.trustKeys); err != nil {
		return manifest, &updateCheckFailure{stage: "validate", errorKind: "invalid-manifest", safeError: "manifest-invalid", httpStatus: response.StatusCode, cause: err}
	}
	return manifest, nil
}

func (u *updateManager) cleanDownloads(keep string) error {
	entries, err := os.ReadDir(u.directory)
	if os.IsNotExist(err) {
		return os.MkdirAll(u.directory, 0700)
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if keep != "" && (entry.Name() == keep || entry.Name() == keep+".part") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(u.directory, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
