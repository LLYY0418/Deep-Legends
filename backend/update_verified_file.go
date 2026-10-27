package main

import (
	"context"
	"os"
)

type verifiedUpdateFile struct {
	info   os.FileInfo
	digest string
}

func (u *updateManager) rememberVerifiedUpdateFile(path string, asset updateAsset) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return
	}
	u.mu.Lock()
	u.verifiedFile = &verifiedUpdateFile{info: info, digest: asset.SHA256}
	u.mu.Unlock()
}
func (u *updateManager) verifyReadyUpdateFile(ctx context.Context, path string, asset updateAsset) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() || info.Size() != asset.Size {
		return false, errUpdateChecksum
	}
	u.mu.Lock()
	previous := u.verifiedFile
	u.mu.Unlock()
	if previous != nil && previous.digest == asset.SHA256 && sameVerifiedUpdateFile(previous.info, info) {
		return true, nil
	}
	if err = verifyUpdateFile(ctx, path, asset); err != nil {
		return false, err
	}
	after, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	if !sameVerifiedUpdateFile(info, after) {
		return false, errUpdateChecksum
	}
	u.rememberVerifiedUpdateFile(path, asset)
	return false, nil
}
func sameVerifiedUpdateFile(before, after os.FileInfo) bool {
	return before != nil && after != nil && after.Mode().IsRegular() && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && os.SameFile(before, after)
}
