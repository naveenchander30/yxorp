package rules

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yxorp/internal/config"
)

func TestNewEngine(t *testing.T) {
	tests := []struct {
		name        string
		rules       []config.SecurityRule
		expectError bool
	}{
		{
			name: "valid rules",
			rules: []config.SecurityRule{
				{Name: "sql_injection", Pattern: `(?i)(union|select|insert|drop)`, Location: "query_params"},
				{Name: "xss", Pattern: `<script`, Location: "body"},
			},
			expectError: false,
		},
		{
			name: "invalid regex",
			rules: []config.SecurityRule{
				{Name: "bad_regex", Pattern: `[invalid(`, Location: "query_params"},
			},
			expectError: true,
		},
		{
			name:        "empty rules",
			rules:       []config.SecurityRule{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(tt.rules)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.expectError && engine == nil {
				t.Errorf("expected engine but got nil")
			}
		})
	}
}

func TestEngineCheck_QueryParams(t *testing.T) {
	rules := []config.SecurityRule{
		{Name: "sql_injection", Pattern: `(?i)(union|select|insert)`, Location: "query_params"},
	}

	engine, err := NewEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		name          string
		url           string
		expectBlocked bool
		expectedRule  string
	}{
		{
			name:          "malicious query",
			url:           "http://example.com/test?q=SELECT+*+FROM+users",
			expectBlocked: true,
			expectedRule:  "sql_injection",
		},
		{
			name:          "clean query",
			url:           "http://example.com/test?q=hello",
			expectBlocked: false,
			expectedRule:  "",
		},
		{
			name:          "union attack",
			url:           "http://example.com/test?id=1+UNION+SELECT",
			expectBlocked: true,
			expectedRule:  "sql_injection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.url, nil)
			blocked, rule := engine.Check(req, nil)

			if blocked != tt.expectBlocked {
				t.Errorf("expected blocked=%v, got %v", tt.expectBlocked, blocked)
			}
			if rule != tt.expectedRule {
				t.Errorf("expected rule=%s, got %s", tt.expectedRule, rule)
			}
		})
	}
}

func TestEngineCheck_Body(t *testing.T) {
	rules := []config.SecurityRule{
		{Name: "xss", Pattern: `<script`, Location: "body"},
	}

	engine, err := NewEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		name          string
		body          string
		expectBlocked bool
		expectedRule  string
	}{
		{
			name:          "xss attack in body",
			body:          `{"comment": "<script>alert('xss')</script>"}`,
			expectBlocked: true,
			expectedRule:  "xss",
		},
		{
			name:          "clean body",
			body:          `{"comment": "hello world"}`,
			expectBlocked: false,
			expectedRule:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "http://example.com/test", strings.NewReader(tt.body))
			blocked, rule := engine.Check(req, []byte(tt.body))

			if blocked != tt.expectBlocked {
				t.Errorf("expected blocked=%v, got %v", tt.expectBlocked, blocked)
			}
			if rule != tt.expectedRule {
				t.Errorf("expected rule=%s, got %s", tt.expectedRule, rule)
			}
		})
	}
}

func TestEngineCheck_URI(t *testing.T) {
	rules := []config.SecurityRule{
		{Name: "path_traversal", Pattern: `\.\.\/`, Location: "uri"},
	}

	engine, err := NewEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		name          string
		path          string
		expectBlocked bool
		expectedRule  string
	}{
		{
			name:          "path traversal attack",
			path:          "/../../etc/passwd",
			expectBlocked: true,
			expectedRule:  "path_traversal",
		},
		{
			name:          "normal path",
			path:          "/api/users/123",
			expectBlocked: false,
			expectedRule:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com"+tt.path, nil)
			blocked, rule := engine.Check(req, nil)

			if blocked != tt.expectBlocked {
				t.Errorf("expected blocked=%v, got %v", tt.expectBlocked, blocked)
			}
			if rule != tt.expectedRule {
				t.Errorf("expected rule=%s, got %s", tt.expectedRule, rule)
			}
		})
	}
}

func TestEngineCheck_Headers(t *testing.T) {
	rules := []config.SecurityRule{
		{Name: "suspicious_header", Pattern: `malicious`, Location: "headers"},
	}

	engine, err := NewEngine(rules)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tests := []struct {
		name          string
		headers       map[string]string
		expectBlocked bool
		expectedRule  string
	}{
		{
			name:          "malicious header",
			headers:       map[string]string{"X-Custom": "malicious-payload"},
			expectBlocked: true,
			expectedRule:  "suspicious_header",
		},
		{
			name:          "clean headers",
			headers:       map[string]string{"User-Agent": "Mozilla/5.0"},
			expectBlocked: false,
			expectedRule:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://example.com/test", nil)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			blocked, rule := engine.Check(req, nil)

			if blocked != tt.expectBlocked {
				t.Errorf("expected blocked=%v, got %v", tt.expectBlocked, blocked)
			}
			if rule != tt.expectedRule {
				t.Errorf("expected rule=%s, got %s", tt.expectedRule, rule)
			}
		})
	}
}

func TestEngineHasBodyRules(t *testing.T) {
	tests := []struct {
		name     string
		rules    []config.SecurityRule
		expected bool
	}{
		{
			name: "has body rules",
			rules: []config.SecurityRule{
				{Name: "xss", Pattern: `<script`, Location: "body"},
				{Name: "sql", Pattern: `union`, Location: "query_params"},
			},
			expected: true,
		},
		{
			name: "no body rules",
			rules: []config.SecurityRule{
				{Name: "sql", Pattern: `union`, Location: "query_params"},
			},
			expected: false,
		},
		{
			name:     "empty rules",
			rules:    []config.SecurityRule{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, err := NewEngine(tt.rules)
			if err != nil {
				t.Fatalf("failed to create engine: %v", err)
			}

			if got := engine.HasBodyRules(); got != tt.expected {
				t.Errorf("expected HasBodyRules()=%v, got %v", tt.expected, got)
			}
		})
	}
}
