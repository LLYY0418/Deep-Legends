package main

import (
	"golang.org/x/sys/windows"
	"time"
)

func processCreatedToMainMillis() int64 {
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user) != nil {
		return -1
	}
	return max(0, time.Since(time.Unix(0, created.Nanoseconds())).Milliseconds())
}
