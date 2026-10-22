package reverselan

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	MaxCommandBodyBytes = int64(1 << 20)
	MaxResultBodyBytes  = int64(8 << 20)
)

var (
	ErrPolicyRejected = errors.New("reverse LAN route rejected")
	allowedHeaders    = map[string]struct{}{
		"Accept":                  {},
		"Content-Type":            {},
		"X-Orange-Debug-Trace-Id": {},
	}
)

type RoutePolicy struct {
	Name              string
	Lane              string
	Method            string
	PathPattern       *regexp.Regexp
	RequestBodyLimit  int64
	ResponseBodyLimit int64
	Timeout           time.Duration
}

type PolicyRegistry struct {
	policies []RoutePolicy
}

func NewPolicyRegistry(policies ...RoutePolicy) *PolicyRegistry {
	return &PolicyRegistry{policies: append([]RoutePolicy(nil), policies...)}
}

func LearningPolicies() []RoutePolicy {
	const prefix = `^/api/v1/learning/sessions/[1-9][0-9]*/`
	return []RoutePolicy{
		learningPolicy("learning-current", http.MethodGet, prefix+`current$`, 0),
		learningPolicy("learning-answer", http.MethodPost, prefix+`answers$`, MaxCommandBodyBytes),
		learningPolicy("learning-mastery", http.MethodPost, prefix+`mastery$`, MaxCommandBodyBytes),
	}
}

func learningPolicy(name, method, pattern string, requestLimit int64) RoutePolicy {
	return RoutePolicy{
		Name:              name,
		Lane:              LaneInteractive,
		Method:            method,
		PathPattern:       regexp.MustCompile(pattern),
		RequestBodyLimit:  requestLimit,
		ResponseBodyLimit: MaxResultBodyBytes,
		Timeout:           20 * time.Second,
	}
}

func (registry *PolicyRegistry) Match(request Request) (RoutePolicy, error) {
	if request.APIVersion != APIVersion {
		return RoutePolicy{}, fmt.Errorf("%w: unsupported api version", ErrPolicyRejected)
	}
	if request.RequestID == "" {
		return RoutePolicy{}, fmt.Errorf("%w: empty request id", ErrPolicyRejected)
	}
	if request.Lane != LaneInteractive && request.Lane != LaneBackground {
		return RoutePolicy{}, fmt.Errorf("%w: invalid lane", ErrPolicyRejected)
	}
	if err := validateRelativeAPIPath(request.Path); err != nil {
		return RoutePolicy{}, err
	}
	for name := range request.Headers {
		canonical := http.CanonicalHeaderKey(name)
		if _, ok := allowedHeaders[canonical]; !ok {
			return RoutePolicy{}, fmt.Errorf("%w: header %s", ErrPolicyRejected, canonical)
		}
	}
	for _, policy := range registry.policies {
		if request.Lane != policy.Lane || request.Method != policy.Method ||
			!policy.PathPattern.MatchString(request.Path) {
			continue
		}
		if int64(len(request.Body)) > policy.RequestBodyLimit {
			return RoutePolicy{}, fmt.Errorf("%w: request body too large", ErrPolicyRejected)
		}
		return policy, nil
	}
	return RoutePolicy{}, fmt.Errorf("%w: no matching policy", ErrPolicyRejected)
}

func validateRelativeAPIPath(path string) error {
	if path == "" || !strings.HasPrefix(path, "/api/") || strings.ContainsAny(path, "\r\n") {
		return fmt.Errorf("%w: invalid path", ErrPolicyRejected)
	}
	parsed, err := url.ParseRequestURI(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" ||
		parsed.RawQuery != "" || parsed.Path != path {
		return fmt.Errorf("%w: invalid path", ErrPolicyRejected)
	}
	return nil
}
