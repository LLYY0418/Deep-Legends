package main

import (
	"errors"
	"os"
	"path/filepath"
)

// Explicitly authorized on 2026-09-09, identified by the user's working Akari
// file as well as the screenshot. No title/prefix matching of third-party sets.
const authorizedOPGGUID = "akari1-13-ranked-kr-all-top-16.17"

func authorizedOPGGItemSet(uid string) bool { return uid == authorizedOPGGUID }
func authorizedOPGGMarker(location settingsLocation) (string, error) {
	dir, err := recommendedDirectory(location)
	if err != nil {
		return "", err
	}
	root := filepath.Join(filepath.Dir(filepath.Dir(dir)), "DeepLegendsItemSetBackups")
	if err := os.Mkdir(root, 0755); err != nil && !os.IsExist(err) {
		return "", err
	}
	stat, err := os.Lstat(root)
	if err != nil || !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("推荐备份目录不安全")
	}
	return filepath.Join(root, "authorized-opgg-0909.done"), nil
}
func authorizedOPGGRemovalPending(location settingsLocation) bool {
	path, err := authorizedOPGGMarker(location)
	if err != nil {
		return false
	}
	_, err = os.Lstat(path)
	return os.IsNotExist(err)
}
func markAuthorizedOPGGRemoval(location settingsLocation, guard func() error) error {
	path, err := authorizedOPGGMarker(location)
	if err != nil {
		return err
	}
	if guard != nil {
		if err := guard(); err != nil {
			return err
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = f.WriteString(authorizedOPGGUID + "\n"); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
