package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"syscall"
	"time"

	"gateway-svc/internal/metrics"
)

var ErrCircuitOpen = errors.New("circuit open")

type resilientTransport struct {
	base       http.RoundTripper
	timeout    time.Duration
	maxRetries int
	breakers   *BreakerRegistry
	metrics    *metrics.Metrics
	upstream   string
}

func NewResilientTransport(
	upstream string,
	timeout time.Duration,
	maxRetries int,
	breakers *BreakerRegistry,
	m *metrics.Metrics,
) http.RoundTripper {
	dialer := &net.Dialer{
		Timeout:   3 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	base := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   20,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   3 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: timeout,
	}
	return &resilientTransport{
		base:       base,
		timeout:    timeout,
		maxRetries: maxRetries,
		breakers:   breakers,
		metrics:    m,
		upstream:   upstream,
	}
}

func (t *resilientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()

	done, err := t.breakers.Allow(t.upstream)
	if err != nil {
		if t.metrics != nil {
			t.metrics.RequestDuration.WithLabelValues(t.upstream).Observe(time.Since(start).Seconds())
			t.metrics.RequestsTotal.WithLabelValues(t.upstream, req.Method, metrics.StatusCode(http.StatusServiceUnavailable)).Inc()
		}
		return nil, err
	}

	attempts := 1
	if isIdempotent(req.Method) {
		attempts = t.maxRetries + 1
	}

	var (
		resp    *http.Response
		lastErr error
	)

	for i := 0; i < attempts; i++ {
		if i > 0 {
			if t.metrics != nil {
				t.metrics.RetriesTotal.WithLabelValues(t.upstream).Inc()
			}
			backoff := time.Duration(20+rand.Intn(40)) * time.Millisecond
			select {
			case <-req.Context().Done():
				done(false)
				return nil, req.Context().Err()
			case <-time.After(backoff):
			}
		}

		ctx, cancel := context.WithTimeout(req.Context(), t.timeout)
		r := req.Clone(ctx)
		if req.GetBody != nil {
			body, bodyErr := req.GetBody()
			if bodyErr != nil {
				cancel()
				done(false)
				return nil, bodyErr
			}
			r.Body = body
		}

		attemptResp, attemptErr := t.base.RoundTrip(r)
		if attemptErr != nil {
			cancel()
			lastErr = attemptErr
			if isRetryableNetErr(attemptErr) && i < attempts-1 {
				continue
			}
			done(false)
			if t.metrics != nil {
				t.metrics.RequestDuration.WithLabelValues(t.upstream).Observe(time.Since(start).Seconds())
				t.metrics.RequestsTotal.WithLabelValues(t.upstream, req.Method, "0").Inc()
			}
			return nil, attemptErr
		}

		if isRetryableStatus(attemptResp.StatusCode) && i < attempts-1 {
			_, _ = io.Copy(io.Discard, attemptResp.Body)
			_ = attemptResp.Body.Close()
			cancel()
			continue
		}

		attemptResp.Body = &cancelOnClose{ReadCloser: attemptResp.Body, cancel: cancel}
		resp = attemptResp
		break
	}

	if resp == nil {
		done(false)
		if lastErr == nil {
			lastErr = fmt.Errorf("upstream %s failed after retries", t.upstream)
		}
		if t.metrics != nil {
			t.metrics.RequestDuration.WithLabelValues(t.upstream).Observe(time.Since(start).Seconds())
			t.metrics.RequestsTotal.WithLabelValues(t.upstream, req.Method, "0").Inc()
		}
		return nil, lastErr
	}

	success := !isRetryableStatus(resp.StatusCode) && resp.StatusCode < 500
	done(success)

	if t.metrics != nil {
		t.metrics.RequestDuration.WithLabelValues(t.upstream).Observe(time.Since(start).Seconds())
		t.metrics.RequestsTotal.WithLabelValues(t.upstream, req.Method, metrics.StatusCode(resp.StatusCode)).Inc()
	}
	return resp, nil
}

type cancelOnClose struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c *cancelOnClose) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func isIdempotent(method string) bool {
	return method == http.MethodGet || method == http.MethodHead
}

func isRetryableStatus(code int) bool {
	return code == http.StatusBadGateway ||
		code == http.StatusServiceUnavailable ||
		code == http.StatusGatewayTimeout
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}
