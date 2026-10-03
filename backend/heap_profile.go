package main

import (
	"runtime"
	"sort"
	"strings"
)

type heapProfilePosition struct {
	Function string  `json:"function"`
	MB       float64 `json:"mb"`
}

// MemProfile is sampled and may lag reclamation by two GC cycles. Do not force
// a stop-the-world collection during normal play. No stack paths or data leave here.
func heapProfileTop(records []runtime.MemProfileRecord) []heapProfilePosition {
	sort.Slice(records, func(i, j int) bool { return records[i].InUseBytes() > records[j].InUseBytes() })
	top := make([]heapProfilePosition, 0, 15)
	for _, record := range records {
		if record.InUseBytes() <= 0 {
			continue
		}
		name := "unknown"
		frames := runtime.CallersFrames(record.Stack())
		for {
			frame, more := frames.Next()
			if frame.Function != "" && !strings.HasPrefix(frame.Function, "runtime.") {
				name = frame.Function
				break
			}
			if !more {
				break
			}
		}
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		top = append(top, heapProfilePosition{Function: name, MB: float64(record.InUseBytes()) / (1024 * 1024)})
		if len(top) == 15 {
			break
		}
	}
	return top
}

func (a *app) recordHeapProfileTop() {
	n, _ := runtime.MemProfile(nil, true)
	records := make([]runtime.MemProfileRecord, n+32)
	n, ok := runtime.MemProfile(records, true)
	if !ok {
		a.recordDiagnostic(map[string]any{"event": "heap_profile_top", "result": "sample-grew", "top": []heapProfilePosition{}})
		return
	}
	a.recordDiagnostic(map[string]any{"event": "heap_profile_top", "top": heapProfileTop(records[:n])})
}
