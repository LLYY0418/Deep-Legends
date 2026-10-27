package main

import (
	"context"
	"os"
)

type verifiedUpdateFile struct {
	info   os.FileInfo
	digest string
}

func verifiedUpdateFileInfo(path string) (os.FileInfo, error) {
	entry, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !entry.Mode().IsRegular() {
		return nil, errUpdateChecksum
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// On Windows path Stat/Lstat loads its file ID lazily, potentially after
	// replacement. File.Stat captures identity from this open handle now.
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || entry.Size() != info.Size() || !entry.ModTime().Equal(info.ModTime()) {
		return nil, errUpdateChecksum
	}
	return info, nil
}

func (u *updateManager) rememberVerifiedUpdateFile(path string, asset updateAsset) {
	info, err := verifiedUpdateFileInfo(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != asset.Size {
		return
	}
	u.mu.Lock()
	u.verifiedFile = &verifiedUpdateFile{info: info, digest: asset.SHA256}
	u.mu.Unlock()
}
func (u *updateManager) verifyReadyUpdateFile(ctx context.Context, path string, asset updateAsset) (bool, error) {
	info, err := verifiedUpdateFileInfo(path)
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
	after, err := verifiedUpdateFileInfo(path)
	if err != nil {
		return false, err
	}
	if !sameVerifiedUpdateFile(info, after) {
		return false, errUpdateChecksum
	}
	u.mu.Lock()
	u.verifiedFile = &verifiedUpdateFile{info: after, digest: asset.SHA256}
	u.mu.Unlock()
	return false, nil
}
func sameVerifiedUpdateFile(before, after os.FileInfo) bool {
	return before != nil && after != nil && after.Mode().IsRegular() && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime()) && os.SameFile(before, after)
}
