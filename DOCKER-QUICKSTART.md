# Yxorp WAF - Docker Quick Start

Get Yxorp WAF running in minutes with Docker Compose.

## Prerequisites

- Docker 20.10 or higher
- Docker Compose 2.0 or higher

## Quick Start

### 1. Clone and Configure

```bash
# Navigate to the project directory
cd yxorp

# Copy environment template
cp .env.example .env

# Edit .env and set your API key (required)
# Generate a secure key: openssl rand -hex 32
nano .env
```

### 2. Start the Stack

```bash
# Build and start all services
docker-compose up -d

# View logs
docker-compose logs -f waf
```

### 3. Access Services

- **WAF Protected Site**: http://localhost:8080
- **Grafana Dashboard**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **WAF Metrics**: http://localhost:8080/metrics

## What's Included

The Docker Compose stack includes:

1. **Yxorp WAF** (port 8080)
   - 36 active security rules
   - Rate limiting (100 req/min)
   - Request/response logging
   - API authentication

2. **Prometheus** (port 9090)
   - Metrics collection
   - Alerting rules
   - 15-second scrape interval

3. **Grafana** (port 3000)
   - Pre-configured WAF dashboard
   - Real-time metrics visualization
   - Alert management

4. **Demo Backend** (internal port 80)
   - Nginx serving test page
   - Protected by WAF
   - Interactive attack tests

## Testing the WAF

Open http://localhost:8080 in your browser. The demo page includes buttons to test:

- SQL Injection protection
- XSS attack blocking
- Path traversal prevention
- Large payload rejection
- Rate limiting

All malicious requests will be blocked and logged.

## Accessing Admin API

The WAF provides admin endpoints that require authentication:

```bash
# Set your API key
export WAF_API_KEY="your-key-from-env-file"

# View recent logs
curl -H "Authorization: Bearer $WAF_API_KEY" \
  http://localhost:8080/api/logs

# Get system stats
curl -H "Authorization: Bearer $WAF_API_KEY" \
  http://localhost:8080/api/stats

# View security rules
curl -H "Authorization: Bearer $WAF_API_KEY" \
  http://localhost:8080/api/rules

# Check backend health
curl -H "Authorization: Bearer $WAF_API_KEY" \
  http://localhost:8080/api/backends

# Get degradation status
curl -H "Authorization: Bearer $WAF_API_KEY" \
  http://localhost:8080/api/degradation
```

## Viewing Dashboards

### Grafana Dashboard

1. Navigate to http://localhost:3000
2. Login with `admin` / `admin` (or your configured password)
3. Go to Dashboards > Yxorp WAF Dashboard
4. View real-time metrics:
   - Request rate and latency
   - Blocked requests by rule
   - Backend health status
   - Network traffic
   - Component degradation

### Prometheus

1. Navigate to http://localhost:9090
2. Query metrics directly:
   - `rate(yxorp_requests_total[1m])` - Request rate
   - `yxorp_blocked_requests_total` - Blocked requests
   - `yxorp_backend_healthy` - Backend health
   - `yxorp_circuit_breaker_state` - Circuit breaker status

## Configuration

### Custom Rules

Edit `configs/rules.yaml` to customize:

```yaml
security:
  rate_limit:
    enabled: true
    requests_per_minute: 100
  max_body_size: 10485760  # 10MB
  rules:
    - name: "Custom Rule"
      pattern: "your-regex-pattern"
      location: "header|body|uri"
```

Restart the WAF to apply changes:

```bash
docker-compose restart waf
```

### Environment Variables

Available in `.env`:

- `WAF_API_KEY` - Required for admin API access
- `GRAFANA_PASSWORD` - Grafana admin password (default: admin)
- `WAF_PORT` - WAF listening port (default: 8080)
- `GRAFANA_PORT` - Grafana port (default: 3000)
- `PROMETHEUS_PORT` - Prometheus port (default: 9090)

### Backend Targets

Edit `configs/rules.yaml` to proxy to your backends:

```yaml
proxy:
  targets:
    - "http://backend1.example.com"
    - "http://backend2.example.com"
```

## Monitoring

### Logs

```bash
# WAF logs
docker-compose logs -f waf

# All services
docker-compose logs -f

# Specific time range
docker-compose logs --since 10m waf
```

### Health Checks

```bash
# WAF health
curl http://localhost:8080/metrics

# Backend status
docker-compose ps

# Container stats
docker stats
```

### Alerts

Prometheus includes 11 pre-configured alerts:

- WAF service down
- High error rate (>10%)
- Request latency (>500ms)
- High block rate (>50%)
- Backend unhealthy
- Circuit breaker open
- Rate limit exceeded
- High memory usage
- Component degraded
- No requests (potential issue)
- Request surge (>200% increase)

View active alerts at: http://localhost:9090/alerts

## Troubleshooting

### WAF won't start

```bash
# Check logs
docker-compose logs waf

# Verify config
docker-compose config

# Check API key is set
docker-compose exec waf env | grep WAF_API_KEY
```

### Can't access Grafana

```bash
# Restart Grafana
docker-compose restart grafana

# Check if port 3000 is in use
netstat -an | grep 3000

# View Grafana logs
docker-compose logs grafana
```

### Metrics not showing

```bash
# Check Prometheus targets
# Navigate to http://localhost:9090/targets
# All targets should show "UP"

# Verify WAF metrics endpoint
curl http://localhost:8080/metrics

# Restart Prometheus
docker-compose restart prometheus
```

### Backend unreachable

```bash
# Check backend container
docker-compose ps demo-backend

# Test connectivity from WAF container
docker-compose exec waf wget -O- http://demo-backend

# View backend logs
docker-compose logs demo-backend
```

## Stopping the Stack

```bash
# Stop services (keeps data)
docker-compose stop

# Stop and remove containers
docker-compose down

# Remove all data (volumes)
docker-compose down -v
```

## Production Deployment

For production use:

1. **Generate secure keys**:
   ```bash
   openssl rand -hex 32
   ```

2. **Enable TLS** in `configs/rules.yaml`:
   ```yaml
   server:
     cert_file: "certs/server.crt"
     key_file: "certs/server.key"
   ```

3. **Set resource limits** in `docker-compose.yml`

4. **Configure persistent volumes** for logs

5. **Set up external monitoring** (alertmanager, PagerDuty, etc.)

6. **Use production-grade backend** instead of demo

7. **Review and customize security rules** for your application

8. **Configure backup** for metrics and configurations

See `DEPLOYMENT.md` for comprehensive production guidance.

## Performance Tips

1. **Adjust rate limits** based on your traffic patterns
2. **Tune circuit breaker thresholds** for backend reliability
3. **Monitor memory usage** and adjust container limits
4. **Use multiple WAF instances** behind a load balancer
5. **Enable gzip compression** (already enabled by default)
6. **Optimize regex patterns** in security rules

## Next Steps

- Review the comprehensive `DEPLOYMENT.md` guide
- Customize security rules for your application
- Set up alerting to your notification channels
- Configure TLS certificates for production
- Integrate with your CI/CD pipeline
- Review and adjust resource limits

## Support

For issues, questions, or contributions:

- File bugs at: [GitHub Issues](https://github.com/yourusername/yxorp/issues)
- Review documentation: `DEPLOYMENT.md`, `README.md`
- Check logs: `docker-compose logs`

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────────────────────────┐
│      Yxorp WAF (Port 8080)      │
│  ┌──────────────────────────┐   │
│  │ Security Middleware      │   │
│  │ - 36 WAF Rules          │   │
│  │ - Rate Limiting         │   │
│  │ - Request Logger        │   │
│  └──────────┬───────────────┘   │
│             ▼                    │
│  ┌──────────────────────────┐   │
│  │ Reverse Proxy            │   │
│  │ - Circuit Breaker        │   │
│  │ - Load Balancing         │   │
│  │ - Health Checks          │   │
│  └──────────┬───────────────┘   │
└─────────────┼───────────────────┘
              │
              ▼
    ┌─────────────────┐
    │  Demo Backend   │
    │   (Nginx:80)    │
    └─────────────────┘
              │
              ├─→ Prometheus (Port 9090)
              │   └─→ Metrics Storage & Alerts
              │
              └─→ Grafana (Port 3000)
                  └─→ Dashboard Visualization
```

## License

See LICENSE file for details.
