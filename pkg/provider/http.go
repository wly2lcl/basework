package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

// readBody reads the full response body
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// jsonRequest performs a JSON POST request and returns the raw response body
func jsonRequest(ctx context.Context, client *http.Client, url string, headers map[string]string, body any) ([]byte, int, error) {
	// 1. Marshal body to JSON
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, err
	}

	// 2. Create POST request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, err
	}

	// 3. Set headers
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// 4. Execute request
	resp, err := client.Do(req)
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