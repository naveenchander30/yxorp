package validation

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/yxorp/internal/config"
)

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationErrors is a collection of validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return "validation errors: " + strings.Join(messages, "; ")
}

// Validator provides comprehensive configuration validation
type Validator struct {
	errors ValidationErrors
}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{
		errors: make(ValidationErrors, 0),
	}
}

// AddError adds a validation error
func (v *Validator) AddError(field, message string) {
	v.errors = append(v.errors, ValidationError{
		Field:   field,
		Message: message,
	})
}

// HasErrors returns true if there are validation errors
func (v *Validator) HasErrors() bool {
	return len(v.errors) > 0
}

// Errors returns all validation errors
func (v *Validator) Errors() error {
	if len(v.errors) == 0 {
		return nil
	}
	return v.errors
}

// ValidateConfig performs comprehensive validation of the configuration
func ValidateConfig(cfg *config.Config) error {
	v := NewValidator()

	v.validateServer(&cfg.Server)
	v.validateProxy(&cfg.Proxy)
	v.validateSecurity(&cfg.Security)

	return v.Errors()
}

func (v *Validator) validateServer(cfg *config.ServerConfig) {
	// Validate port
	if cfg.Port == "" {
		v.AddError("server.port", "port is required")
	} else {
		port, err := strconv.Atoi(cfg.Port)
		if err != nil {
			v.AddError("server.port", "port must be a number")
		} else if port < 1 || port > 65535 {
			v.AddError("server.port", "port must be between 1 and 65535")
		}
	}

	// Validate timeouts
	if cfg.ReadTimeout < 0 {
		v.AddError("server.read_timeout", "read timeout cannot be negative")
	}
	if cfg.WriteTimeout < 0 {
		v.AddError("server.write_timeout", "write timeout cannot be negative")
	}

	// Validate TLS configuration
	if (cfg.CertFile != "" && cfg.KeyFile == "") || (cfg.CertFile == "" && cfg.KeyFile != "") {
		v.AddError("server.tls", "both cert_file and key_file must be provided for TLS")
	}
}

func (v *Validator) validateProxy(cfg *config.ProxyConfig) {
	// Validate targets
	if len(cfg.Targets) == 0 {
		v.AddError("proxy.targets", "at least one proxy target is required")
	}

	for i, target := range cfg.Targets {
		// Add scheme if missing for validation
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			target = "https://" + target
		}

		_, err := url.Parse(target)
		if err != nil {
			v.AddError(fmt.Sprintf("proxy.targets[%d]", i), fmt.Sprintf("invalid URL: %v", err))
		}
	}

	// Validate max request size
	if cfg.MaxRequestSize < 0 {
		v.AddError("proxy.max_request_size", "max request size cannot be negative")
	}
	if cfg.MaxRequestSize > 0 && cfg.MaxRequestSize < 1024 {
		v.AddError("proxy.max_request_size", "max request size should be at least 1024 bytes (1KB)")
	}
}

func (v *Validator) validateSecurity(cfg *config.SecurityConfig) {
	// Validate rate limiting
	v.validateRateLimit(&cfg.RateLimit)

	// Validate security rules
	v.validateRules(cfg.Rules)

	// Validate body size limits
	if cfg.MaxBodySize < 0 {
		v.AddError("security.max_body_size", "max body size cannot be negative")
	}
	if cfg.MaxBodySize > 0 && cfg.MaxBodySize < 1024 {
		v.AddError("security.max_body_size", "max body size should be at least 1024 bytes (1KB)")
	}

	if cfg.MaxDecompressedSize < 0 {
		v.AddError("security.max_decompressed_size", "max decompressed size cannot be negative")
	}
	if cfg.MaxDecompressedSize > 0 && cfg.MaxDecompressedSize < 1024 {
		v.AddError("security.max_decompressed_size", "max decompressed size should be at least 1024 bytes (1KB)")
	}

	// Validate block user agents
	for i, agent := range cfg.BlockUserAgents {
		if strings.TrimSpace(agent) == "" {
			v.AddError(fmt.Sprintf("security.block_user_agents[%d]", i), "user agent cannot be empty")
		}
	}
}

func (v *Validator) validateRateLimit(cfg *config.RateLimitConfig) {
	if cfg.Enabled {
		if cfg.RequestsPerMinute <= 0 {
			v.AddError("security.rate_limit.requests_per_minute", "requests per minute must be greater than 0 when rate limiting is enabled")
		}
		if cfg.RequestsPerMinute > 100000 {
			v.AddError("security.rate_limit.requests_per_minute", "requests per minute seems unreasonably high (>100000)")
		}
	}
}

func (v *Validator) validateRules(rules []config.SecurityRule) {
	if len(rules) == 0 {
		// No rules is valid, just a warning
		return
	}

	validLocations := map[string]bool{
		"body":         true,
		"query_params": true,
		"uri":          true,
		"headers":      true,
	}

	ruleNames := make(map[string]bool)

	for i, rule := range rules {
		// Validate rule name
		if rule.Name == "" {
			v.AddError(fmt.Sprintf("security.rules[%d].name", i), "rule name is required")
		} else {
			if ruleNames[rule.Name] {
				v.AddError(fmt.Sprintf("security.rules[%d].name", i), fmt.Sprintf("duplicate rule name: %s", rule.Name))
			}
			ruleNames[rule.Name] = true
		}

		// Validate pattern
		if rule.Pattern == "" {
			v.AddError(fmt.Sprintf("security.rules[%d].pattern", i), "pattern is required")
		} else {
			_, err := regexp.Compile(rule.Pattern)
			if err != nil {
				v.AddError(fmt.Sprintf("security.rules[%d].pattern", i), fmt.Sprintf("invalid regex: %v", err))
			}
		}

		// Validate location
		if rule.Location == "" {
			v.AddError(fmt.Sprintf("security.rules[%d].location", i), "location is required")
		} else if !validLocations[rule.Location] {
			v.AddError(fmt.Sprintf("security.rules[%d].location", i), fmt.Sprintf("invalid location '%s', must be one of: body, query_params, uri, headers", rule.Location))
		}
	}
}

// ValidateAndFix attempts to fix common configuration issues
func ValidateAndFix(cfg *config.Config) (*config.Config, []string) {
	warnings := make([]string, 0)

	// Fix missing schemes in proxy targets
	for i, target := range cfg.Proxy.Targets {
		if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
			cfg.Proxy.Targets[i] = "https://" + target
			warnings = append(warnings, fmt.Sprintf("Added https:// scheme to proxy target: %s", target))
		}
	}

	// Set reasonable defaults for missing values
	if cfg.Server.ReadTimeout == 0 {
		cfg.Server.ReadTimeout = 30
		warnings = append(warnings, "Set default read_timeout to 30s")
	}
	if cfg.Server.WriteTimeout == 0 {
		cfg.Server.WriteTimeout = 30
		warnings = append(warnings, "Set default write_timeout to 30s")
	}

	// Validate the fixed config
	if err := ValidateConfig(cfg); err != nil {
		return cfg, warnings
	}

	return cfg, warnings
}

// QuickValidate performs basic validation (for backwards compatibility)
func QuickValidate(cfg *config.Config) error {
	if cfg.Server.Port == "" {
		return errors.New("server port is required")
	}
	if len(cfg.Proxy.Targets) == 0 {
		return errors.New("at least one proxy target is required")
	}
	return nil
}
