package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const prestigeArtworkHost = "game.gtimg.cn"

//go:embed prestige_chromas.json
var prestigeCatalogJSON []byte

type prestigeChromaMetadata struct {
	SkinID     int64  `json:"skinId"`
	InstanceID string `json:"instanceId"`
	Name       string `json:"name"`
	TagID      int    `json:"tagId"`
}

type prestigeCatalogFile struct {
	Source        string                   `json:"source"`
	SourceUpdated string                   `json:"sourceUpdated"`
	Entries       []prestigeChromaMetadata `json:"entries"`
}

var (
	prestigeCatalogOnce sync.Once
	prestigeCatalog     map[int64]prestigeChromaMetadata
	prestigeCatalogErr  error
	prestigeUUID        = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	prestigeHTTPClient  = &http.Client{
		Timeout: 8 * time.Second,
		CheckRedirect: func(request *http.Request, _ []*http.Request) error {
			if !strings.EqualFold(request.URL.Scheme, "https") || !strings.EqualFold(request.URL.Hostname(), prestigeArtworkHost) {
				return errors.New("prestige artwork redirect rejected")
			}
			return nil
		},
	}
)

func loadPrestigeCatalog() (map[int64]prestigeChromaMetadata, error) {
	prestigeCatalogOnce.Do(func() {
		var file prestigeCatalogFile
		if err := json.Unmarshal(prestigeCatalogJSON, &file); err != nil {
			prestigeCatalogErr = fmt.Errorf("prestige catalog decode: %w", err)
			return
		}
		if len(file.Entries) < 100 || strings.TrimSpace(file.Source) == "" || strings.TrimSpace(file.SourceUpdated) == "" {
			prestigeCatalogErr = errors.New("prestige catalog is incomplete")
			return
		}
		catalog := make(map[int64]prestigeChromaMetadata, len(file.Entries))
		for _, entry := range file.Entries {
			entry.InstanceID = strings.ToLower(strings.TrimSpace(entry.InstanceID))
			entry.Name = strings.TrimSpace(entry.Name)
			if entry.SkinID <= 0 || entry.Name == "" || !prestigeUUID.MatchString(entry.InstanceID) {
				prestigeCatalogErr = fmt.Errorf("prestige catalog contains invalid entry %d", entry.SkinID)
				return
			}
			if _, exists := catalog[entry.SkinID]; exists {
				prestigeCatalogErr = fmt.Errorf("prestige catalog contains duplicate ID %d", entry.SkinID)
				return
			}
			catalog[entry.SkinID] = entry
		}
		prestigeCatalog = catalog
	})
	return prestigeCatalog, prestigeCatalogErr
}

func enrichPrestigeChroma(chroma Chroma) Chroma {
	catalog, err := loadPrestigeCatalog()
	if err != nil {
		return chroma
	}
	metadata, ok := catalog[chroma.ID]
	if !ok {
		return chroma
	}
	chroma.Name = metadata.Name
	chroma.IsPrestige = true
	chroma.PrestigeImageID = metadata.InstanceID
	return chroma
}

func prestigeImageURL(instanceID string) (string, bool) {
	instanceID = strings.ToLower(strings.TrimSpace(instanceID))
	if !prestigeUUID.MatchString(instanceID) {
		return "", false
	}
	return "https://" + prestigeArtworkHost + "/images/lol/act/a20230715chromahub/skin/site3-" + url.PathEscape(instanceID) + ".jpg", true
}

func (a *app) handlePrestigeImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid prestige chroma ID", http.StatusBadRequest)
		return
	}
	catalog, err := loadPrestigeCatalog()
	if err != nil {
		http.Error(w, "prestige catalog unavailable", http.StatusServiceUnavailable)
		return
	}
	metadata, ok := catalog[id]
	if !ok {
		http.NotFound(w, r)
		return
	}
	artworkURL, ok := prestigeImageURL(metadata.InstanceID)
	if !ok {
		http.NotFound(w, r)
		return
	}

	cacheKey := "prestige:" + metadata.InstanceID
	data, err := a.loadAsset(r.Context(), cacheKey, prestigeArtworkMaxEntryBytes, 90*time.Second, func(ctx context.Context) ([]byte, error) {
		if data, ok := a.storage.loadPrestigeArtwork(metadata.InstanceID); ok {
			return data, nil
		}
		data, err := fetchTencentArtwork(ctx, artworkURL)
		if err != nil {
			return nil, err
		}
		if a.storage != nil {
			go func() { _ = a.storage.storePrestigeArtwork(metadata.InstanceID, data) }()
		}
		return data, nil
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeTencentArtwork(w, data)
}

func fetchTencentArtwork(ctx context.Context, artworkURL string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, artworkURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "image/avif,image/webp,image/jpeg,image/*;q=0.8")
	response, err := prestigeHTTPClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tencent artwork returned HTTP %d", response.StatusCode)
	}
	data, err := readLimited(response.Body, prestigeArtworkMaxEntryBytes)
	if err != nil {
		return nil, err
	}
	if !validPrestigeArtwork(data) {
		return nil, errors.New("tencent artwork is not a supported image")
	}
	return data, nil
}

func writeTencentArtwork(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", http.DetectContentType(data))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}

// skinArtworkURL points at the public horizontal splash for a skin ID, used
// only as a fallback when the logged-in client cannot serve the image itself.
func skinArtworkURL(skinID int64) (string, bool) {
	if skinID < 1000 || skinID > 9_999_999 {
		return "", false
	}
	return "https://" + prestigeArtworkHost + "/images/lol/act/img/skin/big" + strconv.FormatInt(skinID, 10) + ".jpg", true
}

func (a *app) handleSkinArt(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid skin ID", http.StatusBadRequest)
		return
	}
	artworkURL, ok := skinArtworkURL(id)
	if !ok {
		http.Error(w, "invalid skin ID", http.StatusBadRequest)
		return
	}
	data, err := a.loadAsset(r.Context(), "skin-art:"+strconv.FormatInt(id, 10), prestigeArtworkMaxEntryBytes, 90*time.Second, func(ctx context.Context) ([]byte, error) {
		return fetchTencentArtwork(ctx, artworkURL)
	})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	writeTencentArtwork(w, data)
}
