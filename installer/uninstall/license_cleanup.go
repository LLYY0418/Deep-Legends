//go:build license

package main

import "path/filepath"

func cleanupLicenseCredentials(deleteData bool, removeAll func(string) error, emit func(string, any), roamingAppData []string) {
	if deleteData && len(roamingAppData) > 0 {
		root := roamingAppData[0]
		if root == "" {
			emit("progress", progressMessage{Percent: 99, Stage: stageCacheWarning})
		} else if err := removeAll(filepath.Join(root, "Deep Legends", "license")); err != nil {
			emit("progress", progressMessage{Percent: 99, Stage: stageCacheWarning})
		}
	}
}
