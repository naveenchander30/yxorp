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

	// Prepend (newest first)
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

	// Format uptime as HH:MM:SS
	duration := time.Since(startTime)
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60
	uptime := ""
	if hours > 0 {
		uptime = time.Duration(hours*int(time.Hour) + minutes*int(time.Minute) + seconds*int(time.Second)).String()
		// Remove microseconds/nanoseconds
		if idx := len(uptime) - 1; uptime[idx] == 's' {
			// Find last digit before 's'
			for i := idx - 1; i >= 0; i-- {
				if uptime[i] == '.' {
					uptime = uptime[:i] + "s"
					break
				}
			}
		}
	} else {
		uptime = time.Duration(minutes*int(time.Minute) + seconds*int(time.Second)).String()
		// Remove microseconds/nanoseconds
		if idx := len(uptime) - 1; idx > 0 && uptime[idx] == 's' {
			for i := idx - 1; i >= 0; i-- {
				if uptime[i] == '.' {
					uptime = uptime[:i] + "s"
					break
				}
			}
		}
	}

	return SystemStats{
		Goroutines: runtime.NumGoroutine(),
		HeapAlloc:  m.HeapAlloc,
		SysMem:     m.Sys,
		Uptime:     uptime,
	}
}
