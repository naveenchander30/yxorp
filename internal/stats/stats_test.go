package stats

import (
	"fmt"
	"testing"
)

func TestAddLogAndGetRecentLogs(t *testing.T) {
	// Reset buffer
	logMu.Lock()
	logBuffer = make([]LogEntry, 0, maxLogs)
	logMu.Unlock()

	// 1. Add log entry
	entry1 := LogEntry{
		Timestamp:  "2026-06-12T10:00:00Z",
		ClientIP:   "192.168.1.1",
		Method:     "GET",
		Path:       "/home",
		StatusCode: 200,
		Latency:    "1.5ms",
		Action:     "ALLOWED",
	}
	AddLog(entry1)

	logs := GetRecentLogs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].ClientIP != "192.168.1.1" {
		t.Errorf("expected client IP 192.168.1.1, got %s", logs[0].ClientIP)
	}

	// 2. Add second log, check prepend order (newest first)
	entry2 := LogEntry{
		ClientIP: "192.168.1.2",
	}
	AddLog(entry2)

	logs = GetRecentLogs()
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].ClientIP != "192.168.1.2" {
		t.Errorf("expected newest log first, got client IP %s", logs[0].ClientIP)
	}
	if logs[1].ClientIP != "192.168.1.1" {
		t.Errorf("expected oldest log second, got client IP %s", logs[1].ClientIP)
	}

	// 3. Add more than 50 logs, verify limit is enforced
	for i := 0; i < 60; i++ {
		AddLog(LogEntry{
			ClientIP: fmt.Sprintf("10.0.0.%d", i),
		})
	}

	logs = GetRecentLogs()
	if len(logs) != 50 {
		t.Errorf("expected maxLogs 50, got %d", len(logs))
	}
	// The newest log should be 10.0.0.59
	if logs[0].ClientIP != "10.0.0.59" {
		t.Errorf("expected newest client IP 10.0.0.59, got %s", logs[0].ClientIP)
	}
}

func TestGetSystemStats(t *testing.T) {
	stats := GetSystemStats()
	
	if stats.Goroutines <= 0 {
		t.Errorf("expected >0 goroutines, got %d", stats.Goroutines)
	}
	
	if stats.HeapAlloc == 0 {
		t.Error("expected non-zero heap alloc bytes")
	}

	if stats.Uptime == "" {
		t.Error("expected non-empty uptime string")
	}
}
