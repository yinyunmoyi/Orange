package reverselan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
)

var ErrResponseTooLarge = errors.New("reverse LAN loopback response too large")

type Forwarder struct {
	baseURL  string
	policies *PolicyRegistry
	client   *http.Client
}

func NewForwarder(port int, policies *PolicyRegistry) *Forwarder {
	return &Forwarder{
		baseURL:  "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		policies: policies,
		client: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (forwarder *Forwarder) Forward(ctx context.Context, request Request) (Result, error) {
	policy, err := forwarder.policies.Match(request)
	if err != nil {
		return Result{}, err
	}
	requestContext, cancel := context.WithTimeout(ctx, policy.Timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(
		requestContext,
		request.Method,
		forwarder.baseURL+request.Path,
		bytes.NewReader([]byte(request.Body)),
	)
	if err != nil {
		return Result{}, fmt.Errorf("build loopback request: %w", err)
	}
	for name, value := range request.Headers {
		httpRequest.Header.Set(http.CanonicalHeaderKey(name), value)
	}
	httpRequest.Host = ""

	response, err := forwarder.client.Do(httpRequest)
	if err != nil {
		return Result{}, fmt.Errorf("execute loopback request: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, policy.ResponseBodyLimit+1))
	if err != nil {
		return Result{}, fmt.Errorf("read loopback response: %w", err)
	}
	if int64(len(body)) > policy.ResponseBodyLimit {
		return Result{}, ErrResponseTooLarge
	}
	headers := make(map[string]string)
	if contentType := response.Header.Get("Content-Type"); contentType != "" {
		headers["Content-Type"] = contentType
	}
	return Result{
		APIVersion: APIVersion,
		RequestID:  request.RequestID,
		Status:     response.StatusCode,
		Headers:    headers,
		Body:       string(body),
	}, nil
}
