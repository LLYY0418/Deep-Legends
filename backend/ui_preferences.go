package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

const uiPreferencesLimit = 128 << 10

var uiPreferenceKey = regexp.MustCompile(`^lol-loot-[a-z0-9-]{1,64}$`)

func validUIPreference(key, value string) bool {
	return (uiPreferenceKey.MatchString(key) && !strings.HasPrefix(key, "lol-loot-demo") || key == "deep-legends-announcements-read-v1" || key == "deep-legends-announcements-dismissed-v1") && len(value) <= 8192
}

type uiPreferences struct {
	Schema int               `json:"schema"`
	Values map[string]string `json:"values"`
}

func (s *localStore) readUIPreferencesLocked() (uiPreferences, error) {
	value := uiPreferences{Schema: 1, Values: map[string]string{}}
	raw, err := readLocalStoreFile(s, "ui-preferences.json")
	if errors.Is(err, os.ErrNotExist) {
		return value, nil
	}
	if err != nil {
		return value, err
	}
	if len(raw) > uiPreferencesLimit || json.Unmarshal(raw, &value) != nil || value.Schema != 1 || len(value.Values) > 256 {
		return uiPreferences{}, errors.New("invalid UI preferences")
	}
	for key, v := range value.Values {
		if !validUIPreference(key, v) {
			return uiPreferences{}, errors.New("invalid UI preference")
		}
	}
	if value.Values == nil {
		value.Values = map[string]string{}
	}
	return value, nil
}
func (a *app) handleUIPreferences(w http.ResponseWriter, r *http.Request) {
	if a.storage == nil {
		http.Error(w, "本地偏好暂不可用", 503)
		return
	}
	var req struct {
		Mode   string             `json:"mode"`
		Values map[string]*string `json:"values"`
	}
	if r.Method == http.MethodPost {
		r.Body = http.MaxBytesReader(w, r.Body, uiPreferencesLimit)
		defer r.Body.Close()
		d := json.NewDecoder(r.Body)
		d.DisallowUnknownFields()
		var extra any
		if d.Decode(&req) != nil || d.Decode(&extra) != io.EOF || (req.Mode != "update" && req.Mode != "migrate") || len(req.Values) > 256 {
			http.Error(w, "invalid preferences", 400)
			return
		}
		for k, v := range req.Values {
			if !validUIPreference(k, "") || v != nil && !validUIPreference(k, *v) {
				http.Error(w, "invalid preference", 400)
				return
			}
		}
	}
	s := a.storage
	s.mu.Lock()
	defer s.mu.Unlock()
	value, err := s.readUIPreferencesLocked()
	if err != nil {
		http.Error(w, "本地偏好读取失败", 500)
		return
	}
	if r.Method == http.MethodPost {
		for k, v := range req.Values {
			if _, exists := value.Values[k]; req.Mode == "migrate" && exists {
				continue
			}
			if v == nil {
				delete(value.Values, k)
			} else {
				value.Values[k] = *v
			}
		}
		raw, _ := json.Marshal(value)
		if len(raw) > uiPreferencesLimit || len(value.Values) > 256 {
			http.Error(w, "preferences limit", 400)
			return
		}
		if writeLocalStoreFile(s, "ui-preferences.json", raw) != nil {
			http.Error(w, "本地偏好保存失败", 500)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	respondJSON(w, value)
}
