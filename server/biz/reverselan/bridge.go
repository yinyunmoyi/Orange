package reverselan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"aaa_word/biz/discovery"
)

const (
	reverseLanPollWait       = 25 * time.Second
	reverseLanHTTPTimeout    = 30 * time.Second
	reverseLanResultRetryTTL = 60 * time.Second
)

var errBridgeUnauthorized = errors.New("reverse LAN bridge unauthorized")

type BridgeManager struct {
	registry  *Registry
	forwarder *Forwarder
	client    *http.Client

	mu      sync.Mutex
	bridges map[string]*managedBridge
	closed  bool
}

type managedBridge struct {
	clientID   string
	generation string
	cancel     context.CancelFunc
}

type bridge struct {
	candidate discovery.ReverseLanCandidate
	ticket    Ticket
	forwarder *Forwarder
	client    *http.Client
	baseURLs  []string
	nextURL   int
}

func NewBridgeManager(registry *Registry, forwarder *Forwarder) *BridgeManager {
	return &BridgeManager{
		registry:  registry,
		forwarder: forwarder,
		client: &http.Client{
			Timeout: reverseLanHTTPTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		bridges: make(map[string]*managedBridge),
	}
}

func (manager *BridgeManager) HandleCandidate(candidate discovery.ReverseLanCandidate) {
	ticket, ok := manager.registry.Lookup(candidate.ClientID, candidate.Generation)
	if !ok || ticket.ServerID != candidate.ServerID {
		return
	}
	baseURLs := candidateBaseURLs(candidate)
	if len(baseURLs) == 0 {
		return
	}
	key := ticketKey(candidate.ClientID, candidate.Generation)

	manager.mu.Lock()
	if manager.closed {
		manager.mu.Unlock()
		return
	}
	if _, exists := manager.bridges[key]; exists {
		manager.mu.Unlock()
		return
	}
	for existingKey, existing := range manager.bridges {
		if existing.clientID == candidate.ClientID && existing.generation != candidate.Generation {
			existing.cancel()
			delete(manager.bridges, existingKey)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	managed := &managedBridge{
		clientID:   candidate.ClientID,
		generation: candidate.Generation,
		cancel:     cancel,
	}
	manager.bridges[key] = managed
	manager.mu.Unlock()

	log.Printf("[reverse-lan] bridge starting client=%s generation=%s",
		candidate.ClientID, candidate.Generation)
	instance := &bridge{
		candidate: candidate,
		ticket:    ticket,
		forwarder: manager.forwarder,
		client:    manager.client,
		baseURLs:  baseURLs,
	}
	go func() {
		err := instance.run(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("[reverse-lan] bridge stopped client=%s generation=%s err=%v",
				candidate.ClientID, candidate.Generation, err)
		}
		manager.mu.Lock()
		if manager.bridges[key] == managed {
			delete(manager.bridges, key)
		}
		manager.mu.Unlock()
	}()
}

func (manager *BridgeManager) Shutdown() {
	manager.mu.Lock()
	manager.closed = true
	bridges := make([]*managedBridge, 0, len(manager.bridges))
	for _, item := range manager.bridges {
		bridges = append(bridges, item)
	}
	manager.bridges = make(map[string]*managedBridge)
	manager.mu.Unlock()
	for _, item := range bridges {
		item.cancel()
	}
}

func (item *bridge) run(ctx context.Context) error {
	if err := item.waitForCapabilities(ctx); err != nil {
		return err
	}
	log.Printf("[reverse-lan] bridge authenticated client=%s generation=%s",
		item.candidate.ClientID, item.candidate.Generation)
	backoff := 250 * time.Millisecond
	for {
		request, status, err := item.poll(ctx)
		if err != nil {
			if errors.Is(err, errBridgeUnauthorized) {
				return err
			}
			if err := sleepContext(ctx, backoff); err != nil {
				return err
			}
			backoff = nextBackoff(backoff)
			item.rotateEndpoint()
			continue
		}
		backoff = 250 * time.Millisecond
		if status == http.StatusNoContent {
			continue
		}
		result := item.execute(ctx, request)
		if err := item.postResultUntilDone(ctx, result); err != nil {
			return err
		}
	}
}

func (item *bridge) waitForCapabilities(ctx context.Context) error {
	backoff := 250 * time.Millisecond
	for {
		var capabilities Capabilities
		status, err := item.doJSON(ctx, http.MethodGet, "/v1/reverse/capabilities", nil, &capabilities, 64<<10)
		if err == nil && status == http.StatusOK &&
			capabilities.APIVersion == APIVersion &&
			containsString(capabilities.Capabilities, CapabilityHTTPJSON) &&
			containsString(capabilities.Lanes, LaneInteractive) {
			return nil
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden {
			return errBridgeUnauthorized
		}
		if err := sleepContext(ctx, backoff); err != nil {
			return err
		}
		backoff = nextBackoff(backoff)
		item.rotateEndpoint()
	}
}

func (item *bridge) poll(ctx context.Context) (Request, int, error) {
	path := "/v1/reverse/next?lane=" + url.QueryEscape(LaneInteractive) +
		"&waitMs=" + strconv.FormatInt(reverseLanPollWait.Milliseconds(), 10)
	var request Request
	status, err := item.doJSON(ctx, http.MethodGet, path, nil, &request, 2<<20)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return Request{}, status, errBridgeUnauthorized
	}
	if err != nil {
		return Request{}, status, err
	}
	if status == http.StatusNoContent {
		return Request{}, status, nil
	}
	if status != http.StatusOK {
		return Request{}, status, fmt.Errorf("reverse LAN poll status %d", status)
	}
	return request, status, nil
}

func (item *bridge) execute(ctx context.Context, request Request) Result {
	started := time.Now()
	result, err := item.forwarder.Forward(ctx, request)
	if err == nil {
		log.Printf("[reverse-lan] loopback completed request=%s status=%d elapsed=%s",
			request.RequestID, result.Status, time.Since(started))
		return result
	}
	log.Printf("[reverse-lan] loopback failed request=%s method=%s path=%s err=%v elapsed=%s",
		request.RequestID, request.Method, request.Path, err, time.Since(started))
	status := http.StatusBadRequest
	if !errors.Is(err, ErrPolicyRejected) {
		status = http.StatusBadGateway
	}
	return Result{
		APIVersion: APIVersion,
		RequestID:  request.RequestID,
		Status:     status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       `{"code":` + strconv.Itoa(status) + `,"msg":"reverse LAN forwarding failed"}`,
	}
}

func (item *bridge) postResultUntilDone(ctx context.Context, result Result) error {
	deadline := time.Now().Add(reverseLanResultRetryTTL)
	backoff := 250 * time.Millisecond
	path := "/v1/reverse/results/" + url.PathEscape(result.RequestID)
	for {
		status, err := item.doJSON(ctx, http.MethodPost, path, result, nil, 64<<10)
		if err == nil && (status == http.StatusOK || status == http.StatusNoContent) {
			return nil
		}
		if status == http.StatusUnauthorized || status == http.StatusForbidden ||
			status == http.StatusConflict {
			return errBridgeUnauthorized
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("reverse LAN result delivery expired")
		}
		if err := sleepContext(ctx, backoff); err != nil {
			return err
		}
		backoff = nextBackoff(backoff)
		item.rotateEndpoint()
	}
}

func (item *bridge) doJSON(
	ctx context.Context,
	method string,
	path string,
	body any,
	target any,
	responseLimit int64,
) (int, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, item.currentBaseURL()+path, reader)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+item.ticket.Secret)
	request.Header.Set(HeaderServerID, item.ticket.ServerID)
	request.Header.Set(HeaderGeneration, item.ticket.Generation)
	if body != nil {
		request.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	response, err := item.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if target == nil || response.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, responseLimit))
		return response.StatusCode, nil
	}
	limited := io.LimitReader(response.Body, responseLimit+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return response.StatusCode, err
	}
	if int64(len(data)) > responseLimit {
		return response.StatusCode, errors.New("reverse LAN response too large")
	}
	if err := json.Unmarshal(data, target); err != nil {
		return response.StatusCode, err
	}
	return response.StatusCode, nil
}

func (item *bridge) currentBaseURL() string {
	return item.baseURLs[item.nextURL%len(item.baseURLs)]
}

func (item *bridge) rotateEndpoint() {
	item.nextURL = (item.nextURL + 1) % len(item.baseURLs)
}

func candidateBaseURLs(candidate discovery.ReverseLanCandidate) []string {
	result := make([]string, 0, len(candidate.Addresses))
	for _, address := range candidate.Addresses {
		if !address.IsPrivate() && !address.IsLinkLocalUnicast() {
			continue
		}
		result = append(result, "http://"+net.JoinHostPort(address.String(), strconv.Itoa(candidate.Port)))
	}
	return result
}

func nextBackoff(current time.Duration) time.Duration {
	current *= 2
	if current > 5*time.Second {
		return 5 * time.Second
	}
	return current
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
