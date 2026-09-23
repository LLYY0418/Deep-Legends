package main

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"
)

// 一个 assetPath 最多扇出 6 个 CommunityDragon 候选（_large.png 缺图时逐个 404），
// 所以负结果必须按分钟级缓存：候选级 5 秒意味着同一个资源每 5 秒重放 4 次上游 404。
// 404 对固定 remotePath 是永久的，网络抖动最多让一张装饰图缺席一分钟。
const (
	communityImageCandidateNegativeTTL = 60 * time.Second
	communityImageResolveNegativeTTL   = 2 * time.Minute
)

// errCommunityImageCandidatesExhausted marks "every candidate for this assetPath
// failed" so loadAsset can negative-cache the whole fan-out under one key.
var errCommunityImageCandidatesExhausted = errors.New("community dragon asset unavailable")

// Measured 39 profile/spell/rune icons total 706 KB (about 942 KB encoded).
// 512 entries / 16 MiB accommodates roughly thirteen such sets independently
// of equipment images; strict accounting includes the JSON/base64 envelope.
func newCommunityImageCache(store *localStore) *championDataCache {
	return newPublicBinaryCache(store, "community-images", 512, 16<<20)
}

func (a *app) loadCommunityDragonAsset(ctx context.Context, remotePath string) ([]byte, error) {
	p := a.champions
	started := time.Now()
	state := "memory"
	data, err := a.loadAssetFromHost(ctx, communityDragonHost, "cdragon:"+remotePath, 2<<20, communityImageCandidateNegativeTTL, func(ctx context.Context) ([]byte, error) {
		state = "miss"
		loader := func(ctx context.Context) ([]byte, error) {
			data, err := p.fetchDirect(ctx, communityDragonHost, remotePath, nil, 2<<20, championImageAccept)
			if err == nil && !strings.HasPrefix(http.DetectContentType(data), "image/") {
				err = errors.New("invalid CommunityDragon image")
			}
			return data, err
		}
		if p.communityImageCache == nil {
			return loader(ctx)
		}
		key := championCacheKey(communityDragonHost, remotePath, "", championImageAccept)
		result, err := p.communityImageCache.loadWithStatus(ctx, key, 7*24*time.Hour, 30*24*time.Hour, true, loader)
		state = result.state
		return result.data, err
	})
	if err != nil && state == "memory" {
		state = "negative"
	}
	p.observeAssetFetch(communityDragonHost, state, time.Since(started), err)
	return data, err
}
