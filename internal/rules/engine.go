package rules

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/yxorp/internal/config"
)

type Rule struct {
	Name     string
	Pattern  *regexp.Regexp
	Location string
}

type Engine struct {
	Rules       []Rule
	hasBodyRule bool
}

func NewEngine(cfgRules []config.SecurityRule) (*Engine, error) {
	var rules []Rule
	hasBodyRule := false
	for _, r := range cfgRules {
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid regex for rule %s: %w", r.Name, err)
		}
		if r.Location == "body" {
			hasBodyRule = true
		}
		rules = append(rules, Rule{
			Name:     r.Name,
			Pattern:  re,
			Location: r.Location,
		})
	}
	return &Engine{Rules: rules, hasBodyRule: hasBodyRule}, nil
}

func (e *Engine) HasBodyRules() bool {
	return e.hasBodyRule
}

func (e *Engine) Check(r *http.Request, body []byte) (bool, string) {
	for _, rule := range e.Rules {
		matched := false
		switch rule.Location {
		case "body":
			if len(body) > 0 {
				matched = rule.Pattern.Match(body)
			}
		case "query_params":
			for _, values := range r.URL.Query() {
				for _, v := range values {
					if rule.Pattern.MatchString(v) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		case "uri":
			matched = rule.Pattern.MatchString(r.URL.Path)
		case "headers":
			for _, values := range r.Header {
				for _, v := range values {
					if rule.Pattern.MatchString(v) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
			// Body inspection would go here (requires reading and restoring body)
		}

		if matched {
			return true, rule.Name
		}
	}
	return false, ""
}
