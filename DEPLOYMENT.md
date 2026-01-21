# Yxorp WAF - Professional Deployment Guide

## Table of Contents
1. [Pre-Deployment Checklist](#pre-deployment-checklist)
2. [Deployment Architectures](#deployment-architectures)
3. [Configuration Best Practices](#configuration-best-practices)
4. [Production Deployment Steps](#production-deployment-steps)
5. [Monitoring & Observability](#monitoring--observability)
6. [Security Hardening](#security-hardening)
7. [High Availability Setup](#high-availability-setup)
8. [Troubleshooting](#troubleshooting)

---

## Pre-Deployment Checklist

### Code Improvements Needed (Minor Issues)

Before deploying to production, address these issues:

#### 1. Fix Graceful Shutdown (Priority: HIGH)
```go
// In cmd/waf/main.go, update shutdown section:

// Add context for config watcher
configWatcherCtx, cancelConfigWatcher := context.WithCancel(context.Background())

// Config Watcher (line 82)
go func() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    var lastMod time.Time
    for {
        select {
        case <-ticker.C:
            // ... existing reload logic
        case <-configWatcherCtx.Done():
            logger.Info("Config watcher stopped")
            return
        }
    }
}()

// Graceful Shutdown (line 266)
<-quit
logger.Info("Shutting down server...")

ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

// Stop accepting new requests
if err := srv.Shutdown(ctx); err != nil {
    logger.Error("Server forced to shutdown", "error", err)
}

// Cleanup background goroutines
rateLimiter.Stop()
cancelConfigWatcher()

logger.Info("Server exited properly")
```

#### 2. Remove or Integrate AdvancedRateLimiter (Priority: MEDIUM)
```bash
# Option A: Remove if not needed
rm internal/middleware/advanced_ratelimit.go

# Option B: Integrate if needed for advanced features
# Replace NewRateLimiter with NewAdvancedRateLimiter in main.go
```

#### 3. Add Backend Metrics Endpoint (Priority: LOW)
```go
// In cmd/waf/main.go, add this endpoint:
http.Handle("/api/backends", apiAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    backends := make([]map[string]interface{}, 0)
    for i, b := range rp.GetBackends() {
        backends = append(backends, map[string]interface{}{
            "id":     i,
            "url":    b.URL.String(),
            "alive":  b.IsAlive(),
            "cb_open": b.CB.IsOpen(),
        })
    }
    json.NewEncoder(w).Encode(map[string]interface{}{
        "backends": backends,
        "total":    len(backends),
    })
})))
```

---

## Deployment Architectures

### 1. Single Instance (Small-Scale)
```
┌─────────────┐
│   Internet  │
└──────┬──────┘
       │
┌──────▼──────────────┐
│   Yxorp WAF         │
│   Port 8080 (HTTP)  │
│   Port 8081 (Metrics)│
└──────┬──────────────┘
       │
┌──────▼──────────────┐
│  Backend Servers    │
└─────────────────────┘
```

**Use Case:** Development, testing, small production workloads
**Requirements:** 2 CPU cores, 4GB RAM

### 2. Load-Balanced (Medium-Scale)
```
┌─────────────┐
│   Internet  │
└──────┬──────┘
       │
┌──────▼──────────────┐
│   Load Balancer     │
│   (nginx/HAProxy)   │
└──┬─────────┬────────┘
   │         │
┌──▼────┐ ┌──▼────┐
│ WAF-1 │ │ WAF-2 │
└──┬────┘ └──┬────┘
   │         │
   └────┬────┘
        │
┌───────▼────────────┐
│  Backend Servers   │
└────────────────────┘
```

**Use Case:** Medium traffic, high availability needed
**Requirements:** 2-4 WAF instances, 2 CPU / 4GB RAM each

### 3. Kubernetes (Large-Scale)
```
┌─────────────────────────────────────┐
│         Kubernetes Cluster          │
│  ┌─────────────────────────────┐   │
│  │  Ingress Controller         │   │
│  └───────────┬─────────────────┘   │
│              │                      │
│  ┌───────────▼─────────────────┐   │
│  │  Yxorp WAF Deployment       │   │
│  │  (3 replicas, autoscale)    │   │
│  └───────────┬─────────────────┘   │
│              │                      │
│  ┌───────────▼─────────────────┐   │
│  │  Backend Services           │   │
│  └─────────────────────────────┘   │
│                                     │
│  ┌─────────────────────────────┐   │
│  │  Prometheus + Grafana       │   │
│  │  (Monitoring Stack)         │   │
│  └─────────────────────────────┘   │
└─────────────────────────────────────┘
```

**Use Case:** High traffic, enterprise production
**Requirements:** K8s cluster with autoscaling

---

## Configuration Best Practices

### 1. Environment-Specific Configurations

#### Development (configs/dev.yaml)
```yaml
server:
  port: "8080"
  read_timeout: 5s
  write_timeout: 10s
  # No TLS in dev

proxy:
  targets:
    - "http://localhost:3000"  # Local backend
  max_request_size: 10485760  # 10MB

security:
  block_user_agents: []  # Relaxed for testing
  rate_limit:
    enabled: false  # Disabled in dev
    requests_per_minute: 60
  max_body_size: 10485760  # 10MB
  max_decompressed_size: 10485760  # 10MB
```

#### Staging (configs/staging.yaml)
```yaml
server:
  port: "8080"
  read_timeout: 10s
  write_timeout: 20s
  cert_file: "/etc/yxorp/tls/cert.pem"
  key_file: "/etc/yxorp/tls/key.pem"
  api_key: "${WAF_API_KEY}"  # From env

proxy:
  targets:
    - "https://staging-backend-1.example.com"
    - "https://staging-backend-2.example.com"
  max_request_size: 5242880  # 5MB

security:
  block_user_agents:
    - "Nikto"
    - "sqlmap"
    - "curl"  # Block in staging
  rate_limit:
    enabled: true
    requests_per_minute: 100
  max_body_size: 5242880  # 5MB
  max_decompressed_size: 10485760  # 10MB
  rules:  # Include all 36 production rules
```

#### Production (configs/prod.yaml)
```yaml
server:
  port: "8080"
  read_timeout: 30s
  write_timeout: 60s
  cert_file: "/etc/yxorp/tls/cert.pem"
  key_file: "/etc/yxorp/tls/key.pem"
  api_key: "${WAF_API_KEY}"  # MUST use env variable

proxy:
  targets:
    - "https://prod-backend-1.example.com"
    - "https://prod-backend-2.example.com"
    - "https://prod-backend-3.example.com"
  max_request_size: 2097152  # 2MB (strict)

security:
  block_user_agents:
    - "Nikto"
    - "sqlmap"
    - "Havij"
    - "Acunetix"
    - "curl"
    - "wget"
    - "python-requests"
  rate_limit:
    enabled: true
    requests_per_minute: 60  # Strict rate limiting
  max_body_size: 2097152  # 2MB
  max_decompressed_size: 5242880  # 5MB
  rules:  # All 36 production rules
```

### 2. Security Configuration

#### Generate Strong API Key
```bash
# Generate a secure random API key
export WAF_API_KEY=$(openssl rand -base64 32)
echo "WAF_API_KEY=${WAF_API_KEY}" >> /etc/yxorp/.env

# Or use a password manager/vault
```

#### TLS Certificate Setup
```bash
# Option 1: Let's Encrypt
certbot certonly --standalone -d waf.example.com

# Option 2: Self-signed (dev only)
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout /etc/yxorp/tls/key.pem \
  -out /etc/yxorp/tls/cert.pem \
  -days 365 \
  -subj "/CN=waf.example.com"
```

---

## Production Deployment Steps

### Option 1: Systemd Service (Traditional VMs)

#### 1. Build the Binary
```bash
# Build for production
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o yxorp-waf \
  -ldflags="-w -s -X main.version=1.0.0" \
  ./cmd/waf

# Verify binary
./yxorp-waf --version
```

#### 2. Create System User
```bash
sudo useradd -r -s /bin/false yxorp
sudo mkdir -p /opt/yxorp/{bin,configs,logs}
sudo chown -R yxorp:yxorp /opt/yxorp
```

#### 3. Install Files
```bash
sudo cp yxorp-waf /opt/yxorp/bin/
sudo cp configs/prod.yaml /opt/yxorp/configs/rules.yaml
sudo chmod 600 /opt/yxorp/configs/rules.yaml
sudo chown yxorp:yxorp /opt/yxorp/configs/rules.yaml
```

#### 4. Create Systemd Service
```bash
sudo tee /etc/systemd/system/yxorp-waf.service <<EOF
[Unit]
Description=Yxorp Web Application Firewall
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=yxorp
Group=yxorp
WorkingDirectory=/opt/yxorp
ExecStart=/opt/yxorp/bin/yxorp-waf
ExecReload=/bin/kill -HUP \$MAINPID
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal
SyslogIdentifier=yxorp-waf

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/opt/yxorp/logs
ReadOnlyPaths=/opt/yxorp/configs

# Environment
Environment="WAF_API_KEY=your-api-key-here"
EnvironmentFile=-/etc/yxorp/.env

# Resource limits
LimitNOFILE=65536
LimitNPROC=512

[Install]
WantedBy=multi-user.target
EOF
```

#### 5. Start and Enable Service
```bash
sudo systemctl daemon-reload
sudo systemctl enable yxorp-waf
sudo systemctl start yxorp-waf
sudo systemctl status yxorp-waf

# View logs
sudo journalctl -u yxorp-waf -f
```

---

### Option 2: Docker Deployment

#### 1. Create Dockerfile
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o yxorp-waf \
    -ldflags="-w -s" ./cmd/waf

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app
COPY --from=builder /build/yxorp-waf .
COPY configs/rules.yaml configs/

# Create non-root user
RUN addgroup -S yxorp && adduser -S yxorp -G yxorp && \
    chown -R yxorp:yxorp /app

USER yxorp

EXPOSE 8080 8081

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8081/api/stats || exit 1

CMD ["./yxorp-waf"]
```

#### 2. Create docker-compose.yml
```yaml
version: '3.8'

services:
  yxorp-waf:
    build: .
    image: yxorp-waf:1.0.0
    container_name: yxorp-waf
    restart: unless-stopped
    ports:
      - "8080:8080"  # Main traffic
      - "8081:8081"  # Metrics & API
    environment:
      - WAF_API_KEY=${WAF_API_KEY}
    volumes:
      - ./configs/rules.yaml:/app/configs/rules.yaml:ro
      - ./logs:/app/logs
    networks:
      - yxorp-network
    healthcheck:
      test: ["CMD", "wget", "--spider", "http://localhost:8081/api/stats"]
      interval: 30s
      timeout: 3s
      retries: 3
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '1'
          memory: 1G

  prometheus:
    image: prom/prometheus:latest
    container_name: yxorp-prometheus
    restart: unless-stopped
    ports:
      - "9090:9090"
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml:ro
      - prometheus-data:/prometheus
    networks:
      - yxorp-network
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'

  grafana:
    image: grafana/grafana:latest
    container_name: yxorp-grafana
    restart: unless-stopped
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=${GRAFANA_PASSWORD}
    volumes:
      - grafana-data:/var/lib/grafana
      - ./monitoring/grafana-dashboards:/etc/grafana/provisioning/dashboards:ro
    networks:
      - yxorp-network

networks:
  yxorp-network:
    driver: bridge

volumes:
  prometheus-data:
  grafana-data:
```

#### 3. Deploy with Docker
```bash
# Build
docker build -t yxorp-waf:1.0.0 .

# Run
export WAF_API_KEY=$(openssl rand -base64 32)
export GRAFANA_PASSWORD=your-secure-password
docker-compose up -d

# View logs
docker-compose logs -f yxorp-waf

# Check health
curl http://localhost:8081/api/stats
```

---

### Option 3: Kubernetes Deployment

#### 1. Create Kubernetes Manifests

**deployment.yaml**
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: yxorp-waf
  namespace: yxorp
  labels:
    app: yxorp-waf
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  selector:
    matchLabels:
      app: yxorp-waf
  template:
    metadata:
      labels:
        app: yxorp-waf
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8081"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: yxorp-waf
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: waf
        image: your-registry.com/yxorp-waf:1.0.0
        imagePullPolicy: Always
        ports:
        - name: http
          containerPort: 8080
          protocol: TCP
        - name: metrics
          containerPort: 8081
          protocol: TCP
        env:
        - name: WAF_API_KEY
          valueFrom:
            secretKeyRef:
              name: yxorp-secrets
              key: api-key
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
        livenessProbe:
          httpGet:
            path: /api/stats
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /api/stats
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 2
          failureThreshold: 2
        volumeMounts:
        - name: config
          mountPath: /app/configs
          readOnly: true
        - name: tls
          mountPath: /app/tls
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: yxorp-config
      - name: tls
        secret:
          secretName: yxorp-tls
---
apiVersion: v1
kind: Service
metadata:
  name: yxorp-waf
  namespace: yxorp
spec:
  type: ClusterIP
  selector:
    app: yxorp-waf
  ports:
  - name: http
    port: 80
    targetPort: 8080
    protocol: TCP
  - name: metrics
    port: 8081
    targetPort: 8081
    protocol: TCP
---
apiVersion: v1
kind: Service
metadata:
  name: yxorp-waf-metrics
  namespace: yxorp
  labels:
    app: yxorp-waf
spec:
  type: ClusterIP
  selector:
    app: yxorp-waf
  ports:
  - name: metrics
    port: 8081
    targetPort: 8081
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: yxorp-waf-hpa
  namespace: yxorp
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: yxorp-waf
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
```

**configmap.yaml**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: yxorp-config
  namespace: yxorp
data:
  rules.yaml: |
    # Your production config here
    server:
      port: "8080"
      read_timeout: 30s
      write_timeout: 60s
    # ... rest of config
```

**secrets.yaml**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: yxorp-secrets
  namespace: yxorp
type: Opaque
stringData:
  api-key: "your-base64-encoded-api-key"
```

#### 2. Deploy to Kubernetes
```bash
# Create namespace
kubectl create namespace yxorp

# Create secrets
kubectl create secret generic yxorp-secrets \
  --from-literal=api-key=$(openssl rand -base64 32) \
  -n yxorp

# Apply manifests
kubectl apply -f k8s/

# Check status
kubectl get pods -n yxorp
kubectl logs -f deployment/yxorp-waf -n yxorp

# Port forward for testing
kubectl port-forward svc/yxorp-waf 8080:80 -n yxorp
```

---

## Monitoring & Observability

### 1. Prometheus Configuration

**prometheus.yml**
```yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'yxorp-waf'
    static_configs:
      - targets: ['yxorp-waf:8081']
    metrics_path: /metrics

  - job_name: 'yxorp-api'
    static_configs:
      - targets: ['yxorp-waf:8081']
    metrics_path: /api/stats
    bearer_token: 'your-api-key'

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

rule_files:
  - 'alerts.yml'
```

**alerts.yml**
```yaml
groups:
  - name: yxorp_waf
    interval: 30s
    rules:
      - alert: HighRateLimitViolations
        expr: rate(rate_limit_exceeded_total[5m]) > 10
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "High rate limit violations detected"
          description: "Rate limit exceeded more than 10 times per second in the last 5 minutes"

      - alert: WAFBlockedRequests
        expr: rate(waf_blocked_requests_total[5m]) > 5
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High number of blocked requests"
          description: "WAF is blocking more than 5 requests per second"

      - alert: BackendDown
        expr: backend_health_status == 0
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Backend server is down"
          description: "Backend {{ $labels.backend }} has been down for 1 minute"

      - alert: CircuitBreakerOpen
        expr: circuit_breaker_state == 1
        for: 2m
        labels:
          severity: warning
        annotations:
          summary: "Circuit breaker is open"
          description: "Circuit breaker for {{ $labels.backend }} has been open for 2 minutes"

      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) / rate(http_requests_total[5m]) > 0.05
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "High 5xx error rate"
          description: "More than 5% of requests are returning 5xx errors"
```

### 2. Grafana Dashboard

Import this JSON dashboard for comprehensive monitoring:

**Key Metrics to Monitor:**
- Request rate (RPS)
- Error rate (4xx, 5xx)
- Response time (p50, p95, p99)
- Rate limit violations
- WAF blocks by rule
- Backend health status
- Circuit breaker state
- Memory/CPU usage
- Config reload success/failures

### 3. Logging Setup

#### Structured Logging with ELK/Loki

**Fluent Bit Configuration** (sidecar for Kubernetes)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: fluent-bit-config
  namespace: yxorp
data:
  fluent-bit.conf: |
    [SERVICE]
        Flush        5
        Daemon       Off
        Log_Level    info

    [INPUT]
        Name              tail
        Path              /var/log/yxorp/*.log
        Parser            json
        Tag               yxorp.*
        Refresh_Interval  5

    [FILTER]
        Name    kubernetes
        Match   kube.*
        Merge_Log On
        Keep_Log Off

    [OUTPUT]
        Name  loki
        Match *
        Host  loki.monitoring.svc.cluster.local
        Port  3100
        Labels job=yxorp-waf
```

---

## Security Hardening

### 1. Network Security

#### Firewall Rules (iptables)
```bash
# Allow incoming on 8080 (main traffic)
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT

# Allow 8081 only from monitoring network
iptables -A INPUT -p tcp --dport 8081 -s 10.0.1.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 8081 -j DROP

# Drop all other incoming
iptables -A INPUT -j DROP
```

#### Security Groups (AWS)
```terraform
resource "aws_security_group" "yxorp_waf" {
  name        = "yxorp-waf-sg"
  description = "Security group for Yxorp WAF"

  ingress {
    description = "HTTP traffic"
    from_port   = 8080
    to_port     = 8080
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  ingress {
    description     = "Metrics from monitoring"
    from_port       = 8081
    to_port         = 8081
    protocol        = "tcp"
    security_groups = [aws_security_group.monitoring.id]
  }

  egress {
    description = "To backend servers"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["10.0.0.0/16"]
  }
}
```

### 2. Rate Limiting Best Practices

```yaml
# Conservative defaults for production
security:
  rate_limit:
    enabled: true
    requests_per_minute: 60  # 1 req/sec per IP

# For APIs with authenticated users
# Increase limits for known good IPs via whitelist

# For public websites
# May need higher limits: 120-300 rpm depending on traffic
```

### 3. WAF Rules Tuning

#### Initial Deployment (Monitor Mode)
```yaml
# Week 1: Log only, don't block
# Monitor false positives

# Week 2: Block obvious attacks
# SQLi, XSS, Path Traversal

# Week 3: Enable all rules
# Tune based on false positives
```

#### Rule Customization
```yaml
# Add custom rules for your application
security:
  rules:
    - name: "block_admin_access"
      pattern: "^/admin"
      location: "uri"

    - name: "api_key_required"
      pattern: "^X-API-Key:\\s*$"
      location: "headers"
```

### 4. TLS Best Practices

```yaml
server:
  # Use strong cipher suites
  cert_file: "/etc/yxorp/tls/fullchain.pem"
  key_file: "/etc/yxorp/tls/privkey.pem"

# Add to nginx/HAProxy in front:
# ssl_protocols TLSv1.2 TLSv1.3;
# ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256;
```

---

## High Availability Setup

### Architecture Overview
```
                      Internet
                         │
                    ┌────▼────┐
                    │  DNS    │
                    │ Round   │
                    │ Robin   │
                    └────┬────┘
                         │
        ┌────────────────┼────────────────┐
        │                │                │
   ┌────▼────┐     ┌────▼────┐     ┌────▼────┐
   │ WAF-1   │     │ WAF-2   │     │ WAF-3   │
   │ US-East │     │ US-West │     │ EU-West │
   └────┬────┘     └────┬────┘     └────┬────┘
        │                │                │
        └────────────────┼────────────────┘
                         │
                ┌────────▼────────┐
                │  Backend Pool   │
                │  (Multi-Region) │
                └─────────────────┘
```

### Multi-Region Deployment

#### Using AWS
```terraform
# Multi-region with Route53
resource "aws_route53_health_check" "waf" {
  fqdn              = "waf.example.com"
  port              = 8080
  type              = "HTTPS"
  resource_path     = "/health"
  failure_threshold = 3
  request_interval  = 30
}

resource "aws_route53_record" "waf" {
  zone_id = aws_route53_zone.main.zone_id
  name    = "waf.example.com"
  type    = "A"

  set_identifier = "waf-us-east-1"
  
  failover_routing_policy {
    type = "PRIMARY"
  }

  alias {
    name                   = aws_lb.waf_us_east.dns_name
    zone_id                = aws_lb.waf_us_east.zone_id
    evaluate_target_health = true
  }

  health_check_id = aws_route53_health_check.waf.id
}
```

### Load Balancer Configuration

#### Nginx as LB (in front of multiple WAF instances)
```nginx
upstream yxorp_waf {
    least_conn;
    
    server waf-1.internal:8080 max_fails=3 fail_timeout=30s;
    server waf-2.internal:8080 max_fails=3 fail_timeout=30s;
    server waf-3.internal:8080 max_fails=3 fail_timeout=30s;
    
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name waf.example.com;

    ssl_certificate     /etc/nginx/ssl/cert.pem;
    ssl_certificate_key /etc/nginx/ssl/key.pem;
    ssl_protocols       TLSv1.2 TLSv1.3;

    location / {
        proxy_pass http://yxorp_waf;
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts
        proxy_connect_timeout 5s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;
    }

    # Health check endpoint
    location /health {
        access_log off;
        proxy_pass http://yxorp_waf/api/stats;
    }
}
```

---

## Troubleshooting

### Common Issues

#### 1. High Memory Usage
```bash
# Check memory
ps aux | grep yxorp-waf

# Analyze with pprof (add import _ "net/http/pprof")
go tool pprof http://localhost:8081/debug/pprof/heap

# Solutions:
# - Increase cleanup interval for rate limiter
# - Reduce max_decompressed_size
# - Add memory limits in systemd/k8s
```

#### 2. Configuration Not Reloading
```bash
# Check file watcher
tail -f /var/log/yxorp-waf.log | grep "Configuration change"

# Manually trigger reload by touching file
touch /opt/yxorp/configs/rules.yaml

# Check validation errors
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8081/api/config
```

#### 3. Backend Connection Failures
```bash
# Check circuit breaker status
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8081/api/backends

# Check backend health
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8081/api/stats

# Test backend directly
curl -v https://backend.example.com
```

#### 4. Rate Limiting Issues
```bash
# Check if client is being rate limited
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8081/metrics | grep rate_limit_exceeded

# Whitelist an IP (requires code modification)
# Or increase rate limits in config
```

### Debug Mode

Add to systemd service:
```bash
Environment="LOG_LEVEL=debug"
```

### Performance Tuning

```yaml
# Increase worker capacity
server:
  read_timeout: 60s
  write_timeout: 120s

# OS limits
ulimit -n 65536  # File descriptors

# Kernel tuning
sysctl -w net.core.somaxconn=65535
sysctl -w net.ipv4.tcp_max_syn_backlog=65535
```

---

## Maintenance

### Backup Configuration
```bash
# Automated backup
0 2 * * * /usr/local/bin/backup-yxorp-config.sh

# Script
#!/bin/bash
DATE=$(date +%Y%m%d)
cp /opt/yxorp/configs/rules.yaml \
   /backup/yxorp-config-$DATE.yaml
```

### Upgrade Procedure
```bash
# 1. Backup current config
cp configs/rules.yaml configs/rules.yaml.bak

# 2. Build new version
go build -o yxorp-waf-new ./cmd/waf

# 3. Test with new binary
./yxorp-waf-new &
# Run smoke tests

# 4. Rolling upgrade (zero downtime)
# Update one instance at a time
systemctl stop yxorp-waf
cp yxorp-waf-new /opt/yxorp/bin/yxorp-waf
systemctl start yxorp-waf

# 5. Monitor for issues
journalctl -u yxorp-waf -f
```

---

## Production Checklist

Before going live:

- [ ] Code improvements implemented (graceful shutdown)
- [ ] Configuration validated for production
- [ ] TLS certificates installed and valid
- [ ] API key generated and stored securely
- [ ] Monitoring configured (Prometheus + Grafana)
- [ ] Alerting rules configured
- [ ] Logging centralized (ELK/Loki)
- [ ] Load testing completed
- [ ] Security audit performed
- [ ] Backup strategy implemented
- [ ] Disaster recovery plan documented
- [ ] On-call rotation established
- [ ] Documentation complete

---

## Support & Resources

- **Logs:** `journalctl -u yxorp-waf -f`
- **Metrics:** `http://localhost:8081/metrics`
- **Health Check:** `http://localhost:8081/api/stats`
- **API Docs:** See `/api/*` endpoints with Authorization header

For issues, check degradation status first:
```bash
curl -H "Authorization: Bearer YOUR_API_KEY" \
  http://localhost:8081/api/degradation
```
