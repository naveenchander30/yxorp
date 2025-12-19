# Yxorp

Yxorp is a high-performance Web Application Firewall (WAF) and Reverse Proxy written in Go. It serves as a security gateway for web applications, providing protection against application-layer attacks, load balancing capabilities, and real-time traffic observability.

## Overview

The project implements a modular proxy architecture designed for reliability and performance. It intercepts incoming HTTP traffic, applies a chain of security middleware, and forwards legitimate requests to backend servers.

### Key Capabilities

*   **Rule-Based Threat Detection**: Implements a regex-based engine to detect and block OWASP Top 10 threats, including SQL Injection, XSS, and Remote Code Execution.
*   **Load Balancing**: Supports round-robin traffic distribution across multiple backend targets with active health monitoring.
*   **Rate Limiting**: Utilizes a token bucket algorithm to throttle requests per IP address, mitigating DDoS and brute-force attacks.
*   **Circuit Breaking**: Automatically isolates failing backends to prevent cascading system failures.
*   **Observability**: Provides a built-in dashboard for real-time monitoring, along with JSON-formatted logs and expvar metrics.
*   **Hot Reloading**: Supports dynamic configuration updates without service interruption.

## Getting Started

### Prerequisites

*   Go 1.21 or higher

### Installation

1.  Clone the repository:
    ```bash
    git clone https://github.com/yourusername/yxorp.git
    ```

2.  Start the server:
    ```bash
    go run cmd/waf/main.go
    ```

The proxy listens on port **8080**, and the monitoring dashboard is available on port **8081**.

## Configuration

Configuration is managed via `configs/rules.yaml`. The system watches this file for changes and reloads rules automatically.

```yaml
server:
  port: "8080"
  read_timeout: 5s
  write_timeout: 10s

proxy:
  targets:
    - "https://primary-backend.com"
    - "https://secondary-backend.com"

security:
  rate_limit:
    enabled: true
    requests_per_minute: 1000

  rules:
    - name: "SQL Injection"
      pattern: "(UNION SELECT|DROP TABLE)"
      location: "query_params"
```

## Observability

Yxorp exposes real-time metrics and logs for monitoring system health and security events.

*   **Dashboard**: `http://localhost:8081/dashboard`
*   **Metrics API**: `http://localhost:8081/api/stats`
*   **Logs API**: `http://localhost:8081/api/logs`

## Development

### Project Structure

*   `cmd/waf`: Application entry point.
*   `internal/proxy`: Reverse proxy and load balancer logic.
*   `internal/rules`: Security rule engine.
*   `internal/middleware`: HTTP middleware chain (logging, rate limit, security).
*   `internal/stats`: Metrics collection and aggregation.

### Testing

Run the test suite:

```bash
go test ./...
```

## License

MIT License
