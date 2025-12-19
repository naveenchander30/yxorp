# Screenshot Placeholders

This directory contains screenshots for the README documentation.

## Required Screenshots

1. **logo.png** - Yxorp logo (200x200px recommended)
2. **dashboard.png** - Main dashboard overview showing all panels
3. **dashboard-overview.png** - Full dashboard with traffic, charts, and metrics
4. **logs.png** - Live traffic logs table with color-coded status
5. **metrics.png** - System metrics panel (CPU, Memory, Goroutines)

## How to Take Screenshots

1. Start the WAF: `go run cmd/waf/main.go`
2. Open dashboard: `http://localhost:8081/dashboard`
3. Generate some traffic (legitimate and blocked requests)
4. Take screenshots using your preferred tool
5. Save them in this directory with the names listed above

## Screenshot Guidelines

- Use a clean browser window (no extensions/dev tools visible)
- Capture at least 1920x1080 resolution
- Show real data (not empty tables)
- Include both successful and blocked requests for contrast
- Dark theme should be visible

## Example Test Commands to Generate Traffic

```powershell
# Legitimate requests
curl http://localhost:8080/
curl http://localhost:8080/api/users

# Blocked requests (will show in red)
curl "http://localhost:8080/?id=1' OR 1=1--"
curl "http://localhost:8080/?q=<script>alert(1)</script>"
curl -A "sqlmap" http://localhost:8080/
```

Wait a few seconds for the dashboard to update, then take your screenshots!
