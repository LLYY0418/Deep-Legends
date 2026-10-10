package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const riotUserKeyFile = "riot-user-key.dat"

// Credentials never enter a response, diagnostic, cache key, or error string.
// An empty file remembers an explicit clear, so upgrade migration cannot undo it.
type riotKeyStore struct {
	mu               sync.Mutex
	store            *localStore
	key              string
	invalidKey       string
	rejected         bool
	exists           bool
	embeddedRejected bool
	networkFailures  int
	routeUntil       time.Time
	quotaWindowAt    time.Time
	probing          bool
}

var riotUserKeys = &riotKeyStore{}

func loadRiotKeyStore(store *localStore) *riotKeyStore {
	s := &riotKeyStore{store: store}
	if store == nil {
		return s
	}
	path := filepath.Join(store.root, riotUserKeyFile)
	info, err := os.Lstat(path)
	if err != nil {
		return s
	}
	s.exists = true
	if !info.Mode().IsRegular() || info.Size() > 16384 {
		s.rejected = true
		return s
	}
	data, err := os.ReadFile(path)
	if err != nil {
		s.rejected = true
		return s
	}
	if len(data) == 0 {
		return s
	}
	plain, err := unprotectRiotUserKey(data)
	if err != nil {
		s.rejected = true
		return s
	}
	s.key = strings.TrimSpace(string(plain))
	return s
}

func (s *riotKeyStore) effective() (string, string) {
	if value := strings.TrimSpace(os.Getenv("RIOT_API_KEY")); value != "" {
		return value, "env"
	}
	s.mu.Lock()
	value := s.key
	rejected := s.embeddedRejected
	s.mu.Unlock()
	if value != "" {
		return value, "user"
	}
	if value := riotEmbeddedKey(); value != "" && !rejected {
		return value, "embedded"
	}
	if len(configuredRiotRelays()) > 0 {
		return "", "relay"
	}
	return "", "none"
}

func riotKeySource() string { _, source := riotUserKeys.effective(); return source }
func riotActiveKeyInvalid() bool {
	key, _ := riotUserKeys.effective()
	riotUserKeys.mu.Lock()
	defer riotUserKeys.mu.Unlock()
	return key != "" && key == riotUserKeys.invalidKey
}
func riotKeyState() string {
	key, source := riotUserKeys.effective()
	riotUserKeys.mu.Lock()
	defer riotUserKeys.mu.Unlock()
	if riotUserKeys.rejected || key != "" && key == riotUserKeys.invalidKey {
		return "invalid"
	}
	if key == "" && source != "relay" {
		return "unconfigured"
	}
	return "configured"
}

func (s *riotKeyStore) observe(key string, status int) {
	current, _ := s.effective()
	if key == "" || current != key {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		s.invalidKey = key
	}
	if status == http.StatusOK && s.invalidKey == key {
		s.invalidKey = ""
	}
}

func (s *riotKeyStore) writeLocked(key string) error {
	if s.store == nil {
		return errors.New("本地存储不可用")
	}
	var data []byte
	var err error
	if key != "" {
		data, err = protectRiotUserKey([]byte(key))
		if err != nil {
			return errors.New("Riot Key 保存失败")
		}
	}
	if err = writeLocalStoreFile(s.store, riotUserKeyFile, data); err != nil {
		return errors.New("Riot Key 保存失败")
	}
	s.key, s.exists, s.rejected, s.invalidKey = key, true, false, ""
	return nil
}

func (s *riotKeyStore) save(ctx context.Context, client *http.Client, key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 512 || strings.ContainsAny(key, "\r\n\x00") {
		return "", errors.New("请输入有效的 Riot Key")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://kr.api.riotgames.com/lol/status/v4/platform-data", nil)
	if err != nil {
		return "", errors.New("Riot Key 验证失败")
	}
	request.Header.Set("X-Riot-Token", key)
	validationClient := *client
	validationClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, requestErr := validationClient.Do(request)
	result := "ok"
	if requestErr != nil {
		result = "network_error"
	}
	if response != nil {
		response.Body.Close()
		if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
			s.mu.Lock()
			s.rejected = true
			s.mu.Unlock()
			return "invalid", nil
		}
		if response.StatusCode != http.StatusOK {
			result = "network_error"
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return result, s.writeLocked(key)
}

func (s *riotKeyStore) migrateEmbedded(record func(map[string]any)) error {
	s.mu.Lock()
	result := "skipped_no_embedded"
	var err error
	if s.exists {
		result = "skipped_exists"
	} else if key := riotEmbeddedKey(); key != "" {
		err = s.writeLocked(key)
		result = "ok"
		if err != nil {
			result = "failed"
		}
	}
	s.mu.Unlock()
	if record != nil {
		record(map[string]any{"event": "riot_key_migrated", "result": result})
	}
	return err
}

func (a *app) handleRiotKeySettings(w http.ResponseWriter, r *http.Request) {
	result := ""
	var err error
	switch r.Method {
	case http.MethodPost:
		var input struct {
			Key string `json:"key"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			http.Error(w, "Riot Key 请求无效", http.StatusBadRequest)
			return
		}
		result, err = riotUserKeys.save(r.Context(), a.championDataProvider().httpClient(), input.Key)
		if result != "" {
			a.recordDiagnostic(map[string]any{"event": "riot_key_saved", "result": result})
		}
	case http.MethodDelete:
		riotUserKeys.mu.Lock()
		err = riotUserKeys.writeLocked("")
		riotUserKeys.mu.Unlock()
	case http.MethodGet:
	default:
		http.Error(w, "方法不支持", http.StatusMethodNotAllowed)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if r.Method != http.MethodGet && result != "invalid" {
		a.clearRiotCredentialFailures()
		a.broadcastEvent("riot-key-updated")
	}
	respondJSON(w, map[string]any{"status": riotKeyState(), "source": riotKeySource(), "result": result})
}

func (a *app) clearRiotCredentialFailures() {
	if p := a.riot; p != nil {
		p.accountMu.Lock()
		p.accountCache = make(map[string]riotAccountCacheEntry)
		p.accountFlights = make(map[string]*riotAccountFlight)
		p.accountMu.Unlock()
		p.specialistMu.Lock()
		p.specialistCache = make(map[string]specialistRuneCacheEntry)
		p.specialistFlights = make(map[string]*specialistRuneFlight)
		p.specialistRecent = make(map[string]specialistRecentSummaryCacheEntry)
		p.specialistMu.Unlock()
	}
}
