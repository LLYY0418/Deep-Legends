package main

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (a *app) loadClientIcon(ctx context.Context, client *LCUClient, assetPath string) ([]byte, error) {
	const prefix = "/lol-game-data/assets/v1/profile-icons/"
	id := strings.TrimSuffix(strings.TrimPrefix(assetPath, prefix), ".jpg")
	n, err := strconv.ParseInt(id, 10, 64)
	if !strings.HasPrefix(assetPath, prefix) || !strings.HasSuffix(assetPath, ".jpg") || err != nil || n < 0 || strconv.FormatInt(n, 10) != id {
		return client.GetBytesContext(ctx, assetPath)
	}
	a.mu.Lock()
	if a.profileIconImages == nil {
		a.profileIconImages = newPublicBinaryCache(a.storage, "profile-icons", 2048, 64<<20)
	}
	cache := a.profileIconImages
	a.mu.Unlock()
	result, err := cache.loadWithStatus(ctx, "public-profile-icon|"+id, 7*24*time.Hour, 30*24*time.Hour, true, func(ctx context.Context) ([]byte, error) {
		data, err := client.GetBytesContext(ctx, assetPath)
		if err == nil && !strings.HasPrefix(http.DetectContentType(data), "image/") {
			err = errors.New("invalid profile icon")
		}
		return data, err
	})
	return result.data, err
}
