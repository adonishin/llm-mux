package executor

import (
	"context"
	"net/http"
	"sync"
	"time"
)

var antigravityEndpoints = []string{
	"https://cloudcode-pa.googleapis.com",
	"https://oauth2.googleapis.com",
}

func PrewarmAntigravityConnections(ctx context.Context) {
	var wg sync.WaitGroup
	timeout := 5 * time.Second

	for _, endpoint := range antigravityEndpoints {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			prewarmHTTPWithRetry(ctx, url, timeout, 2)
		}(endpoint)
	}

	wg.Wait()
}

func prewarmHTTPWithRetry(ctx context.Context, baseURL string, timeout time.Duration, maxAttempts int) {
	backoff := 100 * time.Millisecond

	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
				backoff *= 2
			}
		}

		if prewarmHTTP(ctx, baseURL, timeout) {
			return
		}
	}
}

func prewarmHTTP(ctx context.Context, baseURL string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, nil)
	if err != nil {
		return false
	}

	client := &http.Client{
		Transport: SharedTransport,
		Timeout:   timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}
