package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"
)

const testProviderIP = "1.1.1.1"

type Service struct {
	config         ConfigStore
	results        ResultStore
	adapters       map[Provider]Adapter
	timeout        time.Duration
	flights        singleflight.Group
	beforeAnalysis func()
}

func NewService(config ConfigStore, results ResultStore, adapters map[Provider]Adapter, timeout time.Duration) *Service {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Service{
		config:   config,
		results:  results,
		adapters: adapters,
		timeout:  timeout,
	}
}

func (s *Service) Providers(ctx context.Context) ([]ProviderStatus, error) {
	if s == nil || s.config == nil {
		return nil, newServiceError(ErrorInternal, "威胁情报配置存储不可用", errors.New("threat intelligence config store is not initialized"))
	}
	return s.config.Statuses(ctx)
}

func (s *Service) UpdateProvider(ctx context.Context, provider Provider, update ProviderConfigUpdate) (ProviderStatus, error) {
	if s == nil || s.config == nil {
		return ProviderStatus{}, newServiceError(ErrorInternal, "威胁情报配置存储不可用", errors.New("threat intelligence config store is not initialized"))
	}
	return s.config.Update(ctx, provider, update)
}

func (s *Service) Result(ctx context.Context, provider Provider, rawIP string) (*Result, error) {
	ip, err := NormalizePublicIP(rawIP)
	if err != nil {
		return nil, err
	}
	if s == nil || s.results == nil {
		return nil, newServiceError(ErrorInternal, "威胁情报结果存储不可用", errors.New("threat intelligence result store is not initialized"))
	}
	result, found, err := s.results.LatestResult(ctx, provider, ip)
	if err != nil {
		return nil, newServiceError(ErrorInternal, "读取威胁情报结果失败", err)
	}
	if !found {
		return nil, nil
	}
	clone := cloneResult(result)
	return &clone, nil
}

func (s *Service) Analyze(ctx context.Context, provider Provider, rawIP string) (AnalyzeOutcome, error) {
	if s == nil || s.config == nil || s.results == nil {
		return AnalyzeOutcome{}, newServiceError(ErrorInternal, "威胁情报服务不可用", errors.New("threat intelligence service is not initialized"))
	}
	if s.beforeAnalysis != nil {
		s.beforeAnalysis()
	}
	ip, err := NormalizePublicIP(rawIP)
	if err != nil {
		return AnalyzeOutcome{}, err
	}

	value, err, _ := s.flights.Do(string(provider)+"\x00"+ip, func() (any, error) {
		callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
		defer cancel()
		return s.analyzeOnce(callCtx, provider, ip)
	})
	outcome, _ := value.(AnalyzeOutcome)
	return outcome, err
}

func (s *Service) TestProvider(ctx context.Context, provider Provider) (ProviderTestStatus, error) {
	if s == nil || s.config == nil {
		return ProviderTestStatus{}, newServiceError(ErrorInternal, "威胁情报配置存储不可用", errors.New("threat intelligence config store is not initialized"))
	}
	config, err := s.config.Config(ctx, provider)
	if err != nil {
		return ProviderTestStatus{}, credentialConfigError(err)
	}
	if strings.TrimSpace(config.Credential) == "" {
		return ProviderTestStatus{}, newServiceError(ErrorProviderNotConfigured, "威胁情报平台尚未配置凭据", nil)
	}
	adapter, ok := s.adapters[provider]
	if !ok || adapter == nil {
		return ProviderTestStatus{}, newServiceError(ErrorProviderUnavailable, "威胁情报平台适配器不可用", errors.New("threat intelligence adapter is not configured"))
	}

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
	defer cancel()
	status := ProviderTestStatus{TestedAt: time.Now().UTC()}
	var opErr error
	if _, err := adapter.Analyze(callCtx, config.Credential, testProviderIP); err != nil {
		opErr = normalizeOperationError(err, callCtx)
		status.Status = "failed"
		status.Message = opErr.Error()
	} else {
		status.Status = "success"
		status.Message = "连接测试成功"
	}
	recordCtx, recordCancel := context.WithTimeout(context.WithoutCancel(ctx), s.timeout)
	defer recordCancel()
	if recordErr := s.config.RecordTest(recordCtx, provider, status); recordErr != nil {
		return status, newServiceError(ErrorInternal, "保存连接测试状态失败", recordErr)
	}
	if status.Status == "failed" {
		return status, opErr
	}
	return status, nil
}

func (s *Service) analyzeOnce(ctx context.Context, provider Provider, ip string) (AnalyzeOutcome, error) {
	if s == nil || s.config == nil || s.results == nil {
		return AnalyzeOutcome{}, newServiceError(ErrorInternal, "威胁情报服务不可用", errors.New("threat intelligence service is not initialized"))
	}

	previous, found, err := s.results.LatestResult(ctx, provider, ip)
	if err != nil {
		return AnalyzeOutcome{}, newServiceError(ErrorInternal, "读取威胁情报结果失败", err)
	}
	outcome := AnalyzeOutcome{}
	if found {
		clone := cloneResult(previous)
		outcome.PreviousResult = &clone
	}

	config, err := s.config.Config(ctx, provider)
	if err != nil {
		return outcome, credentialConfigError(err)
	}
	if !config.Enabled {
		return outcome, newServiceError(ErrorProviderDisabled, "威胁情报平台已停用", nil)
	}
	if strings.TrimSpace(config.Credential) == "" {
		return outcome, newServiceError(ErrorProviderNotConfigured, "威胁情报平台尚未配置凭据", nil)
	}

	adapter, ok := s.adapters[provider]
	if !ok || adapter == nil {
		return outcome, newServiceError(ErrorProviderUnavailable, "威胁情报平台适配器不可用", errors.New("threat intelligence adapter is not configured"))
	}
	result, err := adapter.Analyze(ctx, config.Credential, ip)
	if err != nil {
		return outcome, normalizeOperationError(err, ctx)
	}
	result.Provider = provider
	result.IP = ip
	if err := s.results.SaveResult(ctx, result); err != nil {
		return outcome, normalizeSaveError(err, ctx)
	}
	clone := cloneResult(result)
	outcome.Result = &clone
	return outcome, nil
}

func normalizeSaveError(err error, ctx context.Context) error {
	if err == nil {
		return nil
	}
	if ErrorCodeOf(err) == ErrorTimeout || errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return newServiceError(ErrorTimeout, "威胁情报分析超时", err)
	}
	return newServiceError(ErrorInternal, "保存威胁情报结果失败", err)
}

func credentialConfigError(err error) error {
	if err == nil {
		return nil
	}
	if ErrorCodeOf(err) != ErrorInternal {
		return err
	}
	return newServiceError(ErrorCredentialUnavailable, "威胁情报凭据不可用", err)
}

func normalizeOperationError(err error, ctx context.Context) error {
	if err == nil {
		return nil
	}
	if ErrorCodeOf(err) != ErrorInternal {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return newServiceError(ErrorTimeout, "威胁情报分析超时", err)
	}
	return newServiceError(ErrorProviderUnavailable, "威胁情报平台暂时不可用", err)
}

func cloneResult(result Result) Result {
	result.Tags = append([]string(nil), result.Tags...)
	result.RawResponse = append(json.RawMessage(nil), result.RawResponse...)
	return result
}
