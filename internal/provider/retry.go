package provider

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
)

const (
	defaultLLMMaxAttempts    = 4
	defaultLLMInitialDelay   = 750 * time.Millisecond
	defaultLLMMaxDelay       = 8 * time.Second
	defaultLLMRequestTimeout = 0
)

// RetryConfig controls retry behavior for transient model-provider failures.
// MaxAttempts includes the first request. A value of 1 disables retries.
// RequestTimeout is optional and applies to each request attempt when set.
type RetryConfig struct {
	MaxAttempts    int
	InitialDelay   time.Duration
	MaxDelay       time.Duration
	RequestTimeout time.Duration
}

func chatCompletionWithRetry(ctx context.Context, client openai.Client, params openai.ChatCompletionNewParams, retryConfig RetryConfig) (*openai.ChatCompletion, error) {
	retry := retryConfig.normalized()
	var lastErr error
	for attempt := 1; attempt <= retry.MaxAttempts; attempt++ {
		resp, err := client.Chat.Completions.New(ctx, params)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !shouldRetryProviderError(ctx, err) || attempt == retry.MaxAttempts {
			break
		}

		delay := retry.delay(attempt)
		log.Printf("[Provider] OpenAI/Zhipu API request failed, retrying attempt %d/%d in %s: %v", attempt+1, retry.MaxAttempts, delay, err)
		if err := sleepWithContext(ctx, delay); err != nil {
			return nil, fmt.Errorf("OpenAI/Zhipu API 请求取消: %w", err)
		}
	}

	if retry.MaxAttempts > 1 && shouldRetryProviderError(ctx, lastErr) {
		return nil, fmt.Errorf("OpenAI/Zhipu API 请求失败（已尝试 %d 次）: %w", retry.MaxAttempts, lastErr)
	}
	return nil, fmt.Errorf("OpenAI/Zhipu API 请求失败: %w", lastErr)
}

func shouldRetryProviderError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}

	if statusCode, ok := errorStatusCode(err); ok {
		return statusCode == http.StatusRequestTimeout ||
			statusCode == http.StatusConflict ||
			statusCode == http.StatusTooManyRequests ||
			statusCode >= http.StatusInternalServerError
	}

	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || os.IsTimeout(err) {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}

	message := strings.ToLower(err.Error())
	for _, token := range []string{
		"tls handshake timeout",
		"i/o timeout",
		"connection reset",
		"connection refused",
		"connection aborted",
		"server closed idle connection",
		"unexpected eof",
		"eof",
		"temporary failure",
	} {
		if strings.Contains(message, token) {
			return true
		}
	}
	return false
}

func errorStatusCode(err error) (int, bool) {
	for err != nil {
		if statusCode, ok := statusCodeFromValue(reflect.ValueOf(err)); ok {
			return statusCode, true
		}
		err = errors.Unwrap(err)
	}
	return 0, false
}

func statusCodeFromValue(value reflect.Value) (int, bool) {
	if !value.IsValid() {
		return 0, false
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return 0, false
		}
		value = value.Elem()
	}
	if value.Kind() != reflect.Struct {
		return 0, false
	}
	field := value.FieldByName("StatusCode")
	if !field.IsValid() || !field.CanInt() {
		return 0, false
	}
	return int(field.Int()), true
}

func sleepWithContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c RetryConfig) normalized() RetryConfig {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = defaultLLMMaxAttempts
	}
	if c.InitialDelay < 0 {
		c.InitialDelay = 0
	}
	if c.InitialDelay == 0 {
		c.InitialDelay = defaultLLMInitialDelay
	}
	if c.MaxDelay < 0 {
		c.MaxDelay = 0
	}
	if c.MaxDelay == 0 {
		c.MaxDelay = defaultLLMMaxDelay
	}
	if c.MaxDelay < c.InitialDelay {
		c.MaxDelay = c.InitialDelay
	}
	return c
}

func (c RetryConfig) delay(attempt int) time.Duration {
	c = c.normalized()
	delay := c.InitialDelay
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= c.MaxDelay {
			return c.MaxDelay
		}
	}
	if delay > c.MaxDelay {
		return c.MaxDelay
	}
	return delay
}

func retryConfigFromEnv() RetryConfig {
	return RetryConfig{
		MaxAttempts:    intFromEnv("FOXHARNESS_LLM_MAX_ATTEMPTS", defaultLLMMaxAttempts),
		InitialDelay:   durationFromEnv("FOXHARNESS_LLM_RETRY_INITIAL_DELAY", defaultLLMInitialDelay),
		MaxDelay:       durationFromEnv("FOXHARNESS_LLM_RETRY_MAX_DELAY", defaultLLMMaxDelay),
		RequestTimeout: durationFromEnv("FOXHARNESS_LLM_REQUEST_TIMEOUT", defaultLLMRequestTimeout),
	}
}

func intFromEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		log.Printf("[Provider] ignoring invalid %s=%q", name, raw)
		return fallback
	}
	return value
}

func durationFromEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < 0 {
		log.Printf("[Provider] ignoring invalid %s=%q", name, raw)
		return fallback
	}
	return value
}
