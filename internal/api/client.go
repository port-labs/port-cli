package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/port-labs/port-cli/internal/auth"
	"github.com/port-labs/port-cli/internal/useragent"
)

const (
	maxRetries       = 5
	baseRetryDelay   = 100 * time.Millisecond
	maxRetryDelay    = 5 * time.Second
	maxRateLimitWait = 120 * time.Second // cap for Retry-After
)

// APIError represents a non-2xx response from the Port API.
type APIError struct {
	Method     string
	URL        string
	Status     string
	StatusCode int
	Body       string
	Code       string
	Message    string
	Details    map[string]interface{}
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Body != "" {
		return fmt.Sprintf("API request to %s %s failed: %s. Body: %s", e.URL, e.Method, e.Status, e.Body)
	}
	return fmt.Sprintf("API request to %s %s failed: %s", e.URL, e.Method, e.Status)
}

var retryableStatuses = map[int]bool{
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

// Client handles authenticated requests to Port's API.
type Client struct {
	httpClient *http.Client
	tokenMgr   *TokenManager
	apiURL     string
	timeout    time.Duration
}

// TokenResponse represents the Port API token response.
type TokenResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
	TokenType   string `json:"tokenType"`
}

type ClientOpts struct {
	Token        *auth.Token
	ClientID     string
	ClientSecret string
	APIURL       string
	Timeout      time.Duration
}

// NewClient creates a new Port API client.
func NewClient(opts ClientOpts) *Client {
	apiURL := opts.APIURL
	clientID := opts.ClientID
	clientSecret := opts.ClientSecret
	token := opts.Token
	timeout := opts.Timeout

	if apiURL == "" {
		apiURL = "https://api.getport.io/v1"
	}

	if timeout == 0 {
		timeout = 300 * time.Second
	}

	// Remove trailing slash
	if len(apiURL) > 0 && apiURL[len(apiURL)-1] == '/' {
		apiURL = apiURL[:len(apiURL)-1]
	}

	tm := NewTokenManager(clientID, clientSecret, apiURL)
	if token != nil {
		tm.SetToken(token.Token, token.Claims.Expiry)
	}
	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		tokenMgr: tm,
		apiURL:   apiURL,
		timeout:  timeout,
	}
}

// getToken gets or refreshes the authentication token.
func (c *Client) getToken(ctx context.Context) (string, error) {
	token, err := c.tokenMgr.GetToken()
	if err == nil && token != "" {
		return token, nil
	}

	// Refresh token
	return c.refreshToken(ctx)
}

// refreshToken requests a new token from the API.
func (c *Client) refreshToken(ctx context.Context) (string, error) {
	authURL := fmt.Sprintf("%s/auth/access_token", c.apiURL)
	payload := map[string]string{
		"clientId":     c.tokenMgr.ClientID,
		"clientSecret": c.tokenMgr.ClientSecret,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create auth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", useragent.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("authentication failed: %s", string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	// Cache the token
	expiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	c.tokenMgr.SetToken(tokenResp.AccessToken, expiry)

	return tokenResp.AccessToken, nil
}

// request makes an authenticated request to the Port API.
func (c *Client) request(ctx context.Context, method, path string, data any, params map[string]string) (*http.Response, error) {
	token, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s%s", c.apiURL, path)

	var jsonData []byte
	if data != nil {
		jsonData, err = json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	newRequest := func() (*http.Request, error) {
		var reqBody io.Reader
		if jsonData != nil {
			reqBody = bytes.NewReader(jsonData)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", useragent.String())
		if params != nil {
			q := req.URL.Query()
			for k, v := range params {
				q.Set(k, v)
			}
			req.URL.RawQuery = q.Encode()
		}
		return req, nil
	}

	var resp *http.Response

	// Retry logic with exponential backoff
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			delay := baseRetryDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxRetryDelay {
				delay = maxRetryDelay
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := newRequest()
		if err != nil {
			return nil, err
		}

		resp, err = c.httpClient.Do(req)
		if err != nil {
			if attempt == maxRetries {
				return nil, fmt.Errorf("failed to execute request after %d attempts: %w", maxRetries+1, err)
			}
			// Retry on network errors
			continue
		}

		// Check if status code is retryable.
		if retryableStatuses[resp.StatusCode] && attempt < maxRetries {
			delay := retryAfterDelay(resp, attempt)
			resp.Body.Close()
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		// Non-retryable status codes
		if resp.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			bodyStr := string(body)
			apiErr := &APIError{
				Method:     method,
				URL:        url,
				Status:     resp.Status,
				StatusCode: resp.StatusCode,
				Body:       bodyStr,
			}
			if bodyStr != "" {
				var parsed struct {
					Error   string                 `json:"error"`
					Message string                 `json:"message"`
					Details map[string]interface{} `json:"details"`
				}
				if err := json.Unmarshal(body, &parsed); err == nil {
					apiErr.Code = parsed.Error
					apiErr.Message = parsed.Message
					apiErr.Details = parsed.Details
				}
			}
			return nil, apiErr
		}

		// Success
		return resp, nil
	}

	return resp, err
}

// retryAfterDelay returns how long to wait after a 429 response.
// Reads Retry-After header first; falls back to exponential backoff.
func retryAfterDelay(resp *http.Response, attempt int) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		if secs, err := strconv.Atoi(ra); err == nil {
			d := time.Duration(secs) * time.Second
			if d > maxRateLimitWait {
				d = maxRateLimitWait
			}
			return d
		}
	}
	delay := baseRetryDelay * time.Duration(1<<uint(attempt))
	if delay > maxRateLimitWait {
		delay = maxRateLimitWait
	}
	return delay
}

// Close closes the HTTP client (no-op for standard client, but implements closer pattern).
func (c *Client) Close() error {
	return nil
}
