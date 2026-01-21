package stats

import (
	"runtime"
	"sync"
	"time"
)

// LogEntry represents a single request log for the dashboard
type LogEntry struct {
	Timestamp  string `json:"timestamp"`
	ClientIP   string `json:"client_ip"`
	Method     string `json:"method"`
	Path       string `json:"path"`
	StatusCode int    `json:"status_code"`
	Latency    string `json:"latency"`
	Action     string `json:"action"`
}

// SystemStats represents runtime statistics
type SystemStats struct {
	Goroutines int    `json:"goroutines"`
	HeapAlloc  uint64 `json:"heap_alloc_bytes"`
	SysMem     uint64 `json:"sys_mem_bytes"`
	Uptime     string `json:"uptime"`
}

var (
	logBuffer []LogEntry
	logMu     sync.RWMutex
	maxLogs   = 50
	startTime = time.Now()
)

func init() {
	logBuffer = make([]LogEntry, 0, maxLogs)
}

// AddLog adds a new log entry to the circular buffer
func AddLog(entry LogEntry) {
	logMu.Lock()
	defer logMu.Unlock()

	// Prepend (newest first) and maintain size
	logBuffer = append([]LogEntry{entry}, logBuffer...)
	if len(logBuffer) > maxLogs {
		logBuffer = logBuffer[:maxLogs]
	}
}

// GetRecentLogs returns the recent logs
func GetRecentLogs() []LogEntry {
	logMu.RLock()
	defer logMu.RUnlock()
	// Return a copy to be safe
	logs := make([]LogEntry, len(logBuffer))
	copy(logs, logBuffer)
	return logs
}

// GetSystemStats returns current runtime stats
func GetSystemStats() SystemStats {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Format uptime - truncate to seconds for simplicity
	duration := time.Since(startTime)
	uptime := duration.Truncate(time.Second).String()

	return SystemStats{
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		SysMem:     m.Sys,
		Uptime:     uptime,
	}
}
