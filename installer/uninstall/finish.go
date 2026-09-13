package main

import "path/filepath"

// Used by both the silent and interactive runners; the Core exit code gates
// all destructive cache cleanup and every host.done notification.
func finishUninstall(code int, deleteData bool, localAppData string, removeAll func(string) error, emit func(string, any)) bool {
	if code != 0 {
		emit("failed", failureMessage{Message: "卸载没有完成，请关闭正在使用的程序后重试"})
		return false
	}
	if deleteData {
		if localAppData == "" {
			emit("progress", progressMessage{Percent: 99, Stage: stageCacheWarning})
		} else if err := removeAll(filepath.Join(localAppData, "LOLLootAssistant")); err != nil {
			emit("progress", progressMessage{Percent: 99, Stage: stageCacheWarning})
		}
	}
	emit("done", nil)
	return true
}
