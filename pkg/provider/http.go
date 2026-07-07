package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// newHTTPClient creates an HTTP client with the given timeout
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
	}
}

// setHeaders sets common headers on an HTTP request
func setHeaders(req *http.Request, apiKey string, extraHeaders map[string]string) {
	req.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	for k, v := range extraHeaders {
		req.Header.Set(k, v)
	}
}

// maxRetries is the maximum number of retry attempts for transient HTTP failures
const maxRetries = 3

// doWithRetry executes an HTTP request with exponential backoff retry for transient failures.
// Retries on 429 (rate limit) and 5xx (server error) status codes.
// After exhausting retries on status codes, the last response is returned so callers
// can map the status code appropriately (e.g. via mapHTTPError).
func doWithRetry(ctx context.Context, client *http.Client, req *http.Request, maxRetries int, bodyFn func() (io.ReadCloser, error)) (*http.Response, error) {
	var lastErr error
	var lastResp *http.Response
	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Create a fresh body reader for each attempt (body may be consumed)
		if bodyFn != nil {
			body, err := bodyFn()
			if err != nil {
				return nil, fmt.Errorf("create request body: %w", err)
			}
			req.Body = body
		}

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			// Network errors are retryable
			if attempt < maxRetries {
				waitTime := time.Duration(1<<uint(attempt)) * time.Second // exponential: 1s, 2s, 4s, ...
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(waitTime):
					continue
				}
			}
			continue
		}

		// Check for retryable status codes
		if resp.StatusCode == 429 || resp.StatusCode >= 500 {
			lastResp = resp
			if attempt < maxRetries {
				resp.Body.Close()
				// Check Retry-After header
				retryAfter := resp.Header.Get("Retry-After")
				waitTime := time.Duration(1<<uint(attempt)) * time.Second
				if retryAfter != "" {
					if seconds, err := strconv.Atoi(retryAfter); err == nil {
						waitTime = time.Duration(seconds) * time.Second
					}
				}

				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(waitTime):
					continue
				}
			}
			// Retries exhausted: return last response so caller can handle the status code
			return lastResp, nil
		}

		return resp, nil
	}
	// Only reachable if all attempts returned transport-level errors
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// readBody reads the full response body
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// jsonRequest performs a JSON POST request with retry logic and returns the raw response body
func jsonRequest(ctx context.Context, client *http.Client, url string, headers map[string]string, body any) ([]byte, int, error) {
	// 1. Marshal body to JSON
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	// 2. Create POST request (body will be replaced on each retry)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, 0, err
	}

	// 3. Set headers
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 4. Execute request with retry
	bodyFn := func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(jsonBody)), nil
	}
	resp, err := doWithRetry(ctx, client, req, maxRetries, bodyFn)
	if err != nil {
		return nil, 0, err
	}

	// 5. Read response body
	respBody, err := readBody(resp)
	if err != nil {
		return nil, 0, err
	}

	// 6. Return (body, statusCode, error)
	return respBody, resp.StatusCode, nil
}
