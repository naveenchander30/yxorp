# Testing Guide for Yxorp WAF

## Setup Test Environment

### 1. Start Backend Servers (in separate terminals)

```powershell
# Terminal 1 - Backend Server 1
go run test/backend1.go 3001

# Terminal 2 - Backend Server 2
go run test/backend1.go 3002

# Terminal 3 - Backend Server 3
go run test/backend1.go 3003
```

### 2. Start WAF (in another terminal)

```powershell
# Terminal 4 - WAF Server
go run cmd/waf/main.go
```

---

## Test Scenarios

### **Test 1: Load Balancing**
Verify that requests are distributed across backend servers.

```powershell
# Send multiple requests
for ($i=1; $i -le 10; $i++) {
    curl http://localhost:8080/ | jq .server
}
```

**Expected**: You should see responses from different backend servers (port 3001, 3002, 3003) in rotation.

---

### **Test 2: Health Checks**
Test automatic failover when a backend goes down.

```powershell
# Stop one backend server (Ctrl+C in Terminal 1)
# Then send requests
curl http://localhost:8080/ | jq
```

**Expected**: Traffic continues to the healthy backends. Check WAF logs for "Backend down" messages.

---

### **Test 3: WAF Rules (SQL Injection)**

```powershell
# This should be blocked
curl "http://localhost:8080/api/users?id=1' OR 1=1--"
```

**Expected**: HTTP 403 Forbidden

---

### **Test 4: Rate Limiting**

```powershell
# Send rapid requests
for ($i=1; $i -le 110; $i++) {
    curl -s http://localhost:8080/ | Select-String "429"
    Write-Host "Request $i"
}
```

**Expected**: After 100 requests, you'll get HTTP 429 (Too Many Requests).

---

### **Test 5: User-Agent Blocking**

```powershell
# Should be blocked
curl -A "curl/7.0" http://localhost:8080/
```

**Expected**: HTTP 403 Forbidden

---

### **Test 6: Security Headers**

```powershell
curl -I http://localhost:8080/
```

**Expected Output**:
```
X-Request-ID: <unique-uuid>
X-Content-Type-Options: nosniff
X-Frame-Options: DENY
X-XSS-Protection: 1; mode=block
Strict-Transport-Security: max-age=31536000; includeSubDomains
```

---

### **Test 7: Gzip Compression**

```powershell
curl -H "Accept-Encoding: gzip" -I http://localhost:8080/
```

**Expected**: `Content-Encoding: gzip` header present.

---

### **Test 8: Circuit Breaker**

```powershell
# Stop all backend servers
# Send requests
for ($i=1; $i -le 10; $i++) {
    curl http://localhost:8080/
}
```

**Expected**: After 5 failures, circuit breaker opens. Requests return 503 instantly for 30 seconds.

---

### **Test 9: Dashboard**

Open browser: `http://localhost:8081/dashboard`

**Expected**: 
- Real-time traffic logs
- System metrics (CPU, Memory, Goroutines)
- Active WAF rules list
- Charts updating every 2 seconds

---

### **Test 10: Request Tracing**

```powershell
curl -H "X-Request-ID: my-custom-id" -I http://localhost:8080/
```

**Expected**: Response includes `X-Request-ID: my-custom-id`

---

## Monitoring

### View Real-Time Logs
```powershell
# The WAF logs are JSON formatted
go run cmd/waf/main.go | jq
```

### Check Metrics API
```powershell
curl http://localhost:8081/api/stats | jq
curl http://localhost:8081/api/logs | jq
curl http://localhost:8081/api/rules | jq
```

---

## Load Testing (Optional)

Install Apache Bench or use `hey`:

```powershell
# Install hey
go install github.com/rakyll/hey@latest

# Run load test: 10,000 requests with 50 concurrent workers
hey -n 10000 -c 50 http://localhost:8080/
```

**Monitor**:
- Dashboard for real-time metrics
- Rate limiter activation
- Load balancing distribution
- Circuit breaker behavior under load
