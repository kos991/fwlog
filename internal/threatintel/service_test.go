package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestServiceCoalescesConcurrentAnalysisAndSavesOnce(t *testing.T) {
	adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "malicious", RawResponse: json.RawMessage(`{}`)}, release: make(chan struct{})}
	store := &fakeResultStore{}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, 15*time.Second)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
		}()
	}
	close(start)
	time.Sleep(20 * time.Millisecond)
	close(adapter.release)
	wg.Wait()

	if got := adapter.calls.Load(); got != 1 {
		t.Fatalf("adapter calls = %d, want 1", got)
	}
	if got := store.saveCalls.Load(); got != 1 {
		t.Fatalf("store saves = %d, want 1", got)
	}
}

func TestServiceAnalyzeReadsLocalResultWithoutCallingAdapter(t *testing.T) {
	previous := Result{Provider: ProviderThreatBook, IP: "8.8.8.8", Verdict: "benign"}
	adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "malicious"}}
	store := &fakeResultStore{latest: previous, found: true}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	outcome, err := service.Result(context.Background(), ProviderThreatBook, "8.8.8.8")
	if err != nil {
		t.Fatalf("Result returned error: %v", err)
	}
	if outcome == nil || outcome.Verdict != "benign" {
		t.Fatalf("Result = %#v, want local result", outcome)
	}
	if got := adapter.calls.Load(); got != 0 {
		t.Fatalf("adapter calls = %d, want 0", got)
	}
	if got := store.saveCalls.Load(); got != 0 {
		t.Fatalf("store saves = %d, want 0", got)
	}
}

func TestServiceAnalyzeRejectsDisabledAndUnconfiguredProvidersBeforeAdapter(t *testing.T) {
	tests := []struct {
		name     string
		config   ProviderConfig
		wantCode ErrorCode
	}{
		{name: "disabled", config: ProviderConfig{Provider: ProviderThreatBook, Enabled: false, Credential: "secret"}, wantCode: ErrorProviderDisabled},
		{name: "unconfigured", config: ProviderConfig{Provider: ProviderThreatBook, Enabled: true}, wantCode: ErrorProviderNotConfigured},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "malicious"}}
			store := &fakeResultStore{}
			service := NewService(&fakeConfigStore{config: tt.config}, store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

			_, err := service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
			if got := ErrorCodeOf(err); got != tt.wantCode {
				t.Fatalf("ErrorCodeOf = %q, want %q (err=%v)", got, tt.wantCode, err)
			}
			if got := adapter.calls.Load(); got != 0 {
				t.Fatalf("adapter calls = %d, want 0", got)
			}
		})
	}
}

func TestServiceAnalyzeFailureReturnsPreviousResultAndDoesNotSave(t *testing.T) {
	previous := Result{Provider: ProviderThreatBook, IP: "8.8.8.8", Verdict: "benign"}
	adapterErr := newServiceError(ErrorProviderUnavailable, "provider unavailable", errors.New("transport failed"))
	adapter := &fakeAdapter{provider: ProviderThreatBook, err: adapterErr}
	store := &fakeResultStore{latest: previous, found: true}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	outcome, err := service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
	if ErrorCodeOf(err) != ErrorProviderUnavailable {
		t.Fatalf("ErrorCodeOf = %q, want %q", ErrorCodeOf(err), ErrorProviderUnavailable)
	}
	if outcome.Result != nil || outcome.PreviousResult == nil || outcome.PreviousResult.Verdict != "benign" {
		t.Fatalf("outcome = %#v, want previous result only", outcome)
	}
	if got := store.saveCalls.Load(); got != 0 {
		t.Fatalf("store saves = %d, want 0", got)
	}
}

func TestServiceAnalyzePlainAdapterErrorReturnsProviderUnavailable(t *testing.T) {
	adapter := &fakeAdapter{provider: ProviderThreatBook, err: errors.New("transport failed")}
	service := NewService(configuredStore(ProviderThreatBook), &fakeResultStore{}, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	_, err := service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
	if got := ErrorCodeOf(err); got != ErrorProviderUnavailable {
		t.Fatalf("ErrorCodeOf = %q, want %q", got, ErrorProviderUnavailable)
	}
}

func TestServiceAnalyzeSavesUnknownResult(t *testing.T) {
	adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "unknown"}}
	store := &fakeResultStore{}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	outcome, err := service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if outcome.Result == nil || outcome.Result.Verdict != "unknown" {
		t.Fatalf("outcome = %#v, want unknown result", outcome)
	}
	if got := store.saveCalls.Load(); got != 1 {
		t.Fatalf("store saves = %d, want 1", got)
	}
}

func TestServiceTestProviderUsesFixedIPAndDoesNotSaveResult(t *testing.T) {
	adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "benign"}}
	config := configuredStore(ProviderThreatBook)
	store := &fakeResultStore{}
	service := NewService(config, store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	status, err := service.TestProvider(context.Background(), ProviderThreatBook)
	if err != nil {
		t.Fatalf("TestProvider returned error: %v", err)
	}
	if status.Status != "success" {
		t.Fatalf("status = %#v, want success", status)
	}
	if adapter.lastIP != "1.1.1.1" {
		t.Fatalf("adapter IP = %q, want 1.1.1.1", adapter.lastIP)
	}
	if got := store.saveCalls.Load(); got != 0 {
		t.Fatalf("store saves = %d, want 0", got)
	}
	if config.lastTest.Status != "success" {
		t.Fatalf("recorded test = %#v, want success", config.lastTest)
	}
}

func TestServiceTestProviderAllowsDisabledButRequiresCredential(t *testing.T) {
	config := configuredStore(ProviderThreatBook)
	config.config.Enabled = false
	adapter := &fakeAdapter{provider: ProviderThreatBook}
	service := NewService(config, &fakeResultStore{}, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	if _, err := service.TestProvider(context.Background(), ProviderThreatBook); err != nil {
		t.Fatalf("disabled configured provider test failed: %v", err)
	}

	config.config.Credential = ""
	_, err := service.TestProvider(context.Background(), ProviderThreatBook)
	if got := ErrorCodeOf(err); got != ErrorProviderNotConfigured {
		t.Fatalf("ErrorCodeOf = %q, want %q", got, ErrorProviderNotConfigured)
	}
}

func TestServiceAnalyzeTimeoutReturnsTimeoutAndPreservesPreviousResult(t *testing.T) {
	previous := Result{Provider: ProviderThreatBook, IP: "8.8.8.8", Verdict: "benign"}
	adapter := &fakeAdapter{provider: ProviderThreatBook, waitForContext: true}
	store := &fakeResultStore{latest: previous, found: true}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, 10*time.Millisecond)

	outcome, err := service.Analyze(context.Background(), ProviderThreatBook, "8.8.8.8")
	if got := ErrorCodeOf(err); got != ErrorTimeout {
		t.Fatalf("ErrorCodeOf = %q, want %q (err=%v)", got, ErrorTimeout, err)
	}
	if outcome.PreviousResult == nil || outcome.PreviousResult.Verdict != "benign" {
		t.Fatalf("outcome = %#v, want previous result", outcome)
	}
	if got := store.saveCalls.Load(); got != 0 {
		t.Fatalf("store saves = %d, want 0", got)
	}
}

func TestServiceAnalyzeRejectsNSFocusIPv6(t *testing.T) {
	service := NewService(configuredStore(ProviderNSFocus), &fakeResultStore{}, map[Provider]Adapter{ProviderNSFocus: NewNSFocusAdapter(nil, "", nil)}, time.Second)

	_, err := service.Analyze(context.Background(), ProviderNSFocus, "2001:4860:4860::8888")
	if got := ErrorCodeOf(err); got != ErrorUnsupportedIP {
		t.Fatalf("ErrorCodeOf = %q, want %q", got, ErrorUnsupportedIP)
	}
}

func TestServiceAnalyzeContinuesAfterClientCancellation(t *testing.T) {
	adapter := &fakeAdapter{provider: ProviderThreatBook, result: Result{Verdict: "malicious"}, started: make(chan struct{}), release: make(chan struct{})}
	store := &fakeResultStore{}
	service := NewService(configuredStore(ProviderThreatBook), store, map[Provider]Adapter{ProviderThreatBook: adapter}, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = service.Analyze(ctx, ProviderThreatBook, "8.8.8.8")
		close(done)
	}()
	select {
	case <-adapter.started:
	case <-time.After(time.Second):
		t.Fatal("adapter did not start")
	}
	cancel()
	close(adapter.release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shared analysis did not complete after client cancellation")
	}
	if got := store.saveCalls.Load(); got != 1 {
		t.Fatalf("store saves = %d, want 1", got)
	}
}

func configuredStore(provider Provider) *fakeConfigStore {
	return &fakeConfigStore{config: ProviderConfig{Provider: provider, Enabled: true, Credential: "test-key"}}
}

type fakeAdapter struct {
	provider       Provider
	result         Result
	err            error
	waitForContext bool
	started        chan struct{}
	release        chan struct{}
	calls          atomic.Int32
	mu             sync.Mutex
	lastIP         string
}

func (a *fakeAdapter) Provider() Provider { return a.provider }

func (a *fakeAdapter) Analyze(ctx context.Context, _ string, ip string) (Result, error) {
	a.calls.Add(1)
	a.mu.Lock()
	a.lastIP = ip
	a.mu.Unlock()
	if a.started != nil {
		select {
		case <-a.started:
		default:
			close(a.started)
		}
	}
	if a.waitForContext {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}
	if a.release != nil {
		<-a.release
	}
	if a.err != nil {
		return Result{}, a.err
	}
	a.result.Provider = a.provider
	a.result.IP = ip
	return a.result, nil
}

type fakeConfigStore struct {
	config    ProviderConfig
	statuses  []ProviderStatus
	lastTest  ProviderTestStatus
	configErr error
}

func (s *fakeConfigStore) Statuses(context.Context) ([]ProviderStatus, error) {
	return append([]ProviderStatus(nil), s.statuses...), nil
}

func (s *fakeConfigStore) Config(context.Context, Provider) (ProviderConfig, error) {
	if s.configErr != nil {
		return ProviderConfig{}, s.configErr
	}
	return s.config, nil
}

func (s *fakeConfigStore) Update(context.Context, Provider, ProviderConfigUpdate) (ProviderStatus, error) {
	return ProviderStatus{}, nil
}

func (s *fakeConfigStore) RecordTest(_ context.Context, _ Provider, status ProviderTestStatus) error {
	s.lastTest = status
	return nil
}

type fakeResultStore struct {
	latest    Result
	found     bool
	latestErr error
	saveErr   error
	saveCalls atomic.Int32
}

func (s *fakeResultStore) LatestResult(context.Context, Provider, string) (Result, bool, error) {
	return s.latest, s.found, s.latestErr
}

func (s *fakeResultStore) SaveResult(_ context.Context, result Result) error {
	s.saveCalls.Add(1)
	if s.saveErr != nil {
		return s.saveErr
	}
	s.latest = result
	s.found = true
	return nil
}
