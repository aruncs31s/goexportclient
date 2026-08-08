// Package client provides a Go SDK / driver for interacting with the GOExport SaaS API.
// Mimics standard database and storage drivers like AWS S3 SDK, Redis, and GORM.
package goexporclient

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is the GOExport SaaS driver instance.
type Client struct {
	baseURL      string
	token        string
	tenantID     string
	userID       string
	httpClient   *http.Client
	s3Endpoint   string
	s3Bucket     string
	s3AccessKey  string
	s3SecretKey  string
	s3Region     string
	useDefaultS3 bool
}

// Option configures a Client instance at initialization.
type Option func(*Client)

// WithTenant sets the default X-Tenant-ID header for all requests made by this client.
func WithTenant(tenantID string) Option {
	return func(c *Client) { c.tenantID = tenantID }
}

// WithUser sets the default X-User-ID header for all requests made by this client.
func WithUser(userID string) Option {
	return func(c *Client) { c.userID = userID }
}

// WithHTTPClient sets a custom http.Client for network calls.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithClientS3Storage sets the default S3 configuration on the client.
func WithClientS3Storage(endpoint, bucket, accessKey, secretKey, region string) Option {
	return func(c *Client) {
		c.s3Endpoint = endpoint
		c.s3Bucket = bucket
		c.s3AccessKey = accessKey
		c.s3SecretKey = secretKey
		c.s3Region = region
	}
}

// WithClientDefaultS3 configures the client to default to GoExport's default S3 bucket.
func WithClientDefaultS3() Option {
	return func(c *Client) {
		c.useDefaultS3 = true
	}
}

// New initializes a new GOExport SDK Client driver.
func New(baseURL, token string, opts ...Option) *Client {
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ExportRequest represents parameters for queueing a new PDF export.
type ExportRequest struct {
	URL          string `json:"url"`
	HTML         string `json:"html,omitempty"`
	Section      string `json:"section,omitempty"`
	CallbackURL  string `json:"callback_url,omitempty"`
	Sync         bool   `json:"sync,omitempty"`
	UseDefaultS3 bool   `json:"use_default_s3,omitempty"`

	// Optional Custom S3 configuration for this job
	S3Endpoint  string `json:"s3_endpoint,omitempty"`
	S3Bucket    string `json:"s3_bucket,omitempty"`
	S3AccessKey string `json:"s3_access_key,omitempty"`
	S3SecretKey string `json:"s3_secret_key,omitempty"`
	S3Region    string `json:"s3_region,omitempty"`
}

// ExportResponse is returned upon successfully queuing an export.
type ExportResponse struct {
	ID      string `json:"id"`
	Section string `json:"section"`
	State   string `json:"state"`
}

// ExportStatus details the current state and results of an export job.
type ExportStatus struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Section     string    `json:"section"`
	TenantID    string    `json:"tenant_id,omitempty"`
	UserID      string    `json:"user_id,omitempty"`
	State       string    `json:"state"`
	ObjectKey   string    `json:"object_key,omitempty"`
	Error       string    `json:"error,omitempty"`
	CallbackURL string    `json:"callback_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// ExportListResponse holds paginated export statuses.
type ExportListResponse struct {
	Exports []ExportStatus `json:"exports"`
	Count   int            `json:"count"`
	Total   int            `json:"total"`
	Limit   int            `json:"limit"`
	Offset  int            `json:"offset"`
}

// CallConfig holds options for overriding settings per request.
type callConfig struct {
	tenantID     string
	userID       string
	s3Endpoint   string
	s3Bucket     string
	s3AccessKey  string
	s3SecretKey  string
	s3Region     string
	useDefaultS3 bool
	sync         bool
}

// CallOption overrides configuration for a single SDK method invocation.
type CallOption func(*callConfig)

// WithCallTenant sets or overrides the X-Tenant-ID header for a single API call.
func WithCallTenant(tenantID string) CallOption {
	return func(cc *callConfig) { cc.tenantID = tenantID }
}

// WithCallUser sets or overrides the X-User-ID header for a single API call.
func WithCallUser(userID string) CallOption {
	return func(cc *callConfig) { cc.userID = userID }
}

// WithS3Storage specifies a custom S3 storage destination for this export request.
func WithS3Storage(endpoint, bucket, accessKey, secretKey, region string) CallOption {
	return func(cc *callConfig) {
		cc.s3Endpoint = endpoint
		cc.s3Bucket = bucket
		cc.s3AccessKey = accessKey
		cc.s3SecretKey = secretKey
		cc.s3Region = region
		cc.useDefaultS3 = false
	}
}

// WithDefaultS3 specifies using GoExport's default S3 bucket for this export request.
func WithDefaultS3() CallOption {
	return func(cc *callConfig) {
		cc.useDefaultS3 = true
		cc.s3Bucket = ""
	}
}

// WithSync makes the request synchronous, returning the PDF bytes directly.
func WithSync() CallOption {
	return func(cc *callConfig) {
		cc.sync = true
	}
}

func (c *Client) buildCallConfig(opts []CallOption) callConfig {
	cc := callConfig{
		tenantID:     c.tenantID,
		userID:       c.userID,
		s3Endpoint:   c.s3Endpoint,
		s3Bucket:     c.s3Bucket,
		s3AccessKey:  c.s3AccessKey,
		s3SecretKey:  c.s3SecretKey,
		s3Region:     c.s3Region,
		useDefaultS3: c.useDefaultS3,
	}
	for _, opt := range opts {
		opt(&cc)
	}
	return cc
}

func (c *Client) applyHeaders(req *http.Request, cc callConfig) {
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if cc.tenantID != "" {
		req.Header.Set("X-Tenant-ID", cc.tenantID)
	}
	if cc.userID != "" {
		req.Header.Set("X-User-ID", cc.userID)
	}
}

// CreateExport submits a new PDF export job to GOExport.
func (c *Client) CreateExport(ctx context.Context, req ExportRequest, opts ...CallOption) (*ExportResponse, error) {
	cc := c.buildCallConfig(opts)

	req.Sync = cc.sync
	req.UseDefaultS3 = cc.useDefaultS3

	if !cc.useDefaultS3 && cc.s3Bucket != "" {
		req.S3Bucket = cc.s3Bucket
		req.S3Endpoint = cc.s3Endpoint
		req.S3Region = cc.s3Region

		// Encrypt S3 credentials using built-in default key (empty key defaults to built-in)
		encAccessKey, err := encrypt(cc.s3AccessKey, "")
		if err == nil {
			req.S3AccessKey = encAccessKey
		} else {
			req.S3AccessKey = cc.s3AccessKey
		}

		encSecretKey, err := encrypt(cc.s3SecretKey, "")
		if err == nil {
			req.S3SecretKey = encSecretKey
		} else {
			req.S3SecretKey = cc.s3SecretKey
		}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/exports", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.applyHeaders(httpReq, cc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK && cc.sync {
		return nil, fmt.Errorf("synchronous PDF response returned. Use ExportHTML or ExportURL with WithSync() call option instead of calling CreateExport directly")
	}

	if resp.StatusCode != http.StatusAccepted {
		return nil, parseAPIError(resp)
	}

	var out ExportResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &out, nil
}

// GetStatus checks the current status of an export job by ID.
func (c *Client) GetStatus(ctx context.Context, id string, opts ...CallOption) (*ExportStatus, error) {
	cc := c.buildCallConfig(opts)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/exports/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.applyHeaders(httpReq, cc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var status ExportStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &status, nil
}

// DownloadPDF retrieves the raw PDF bytes for a completed export job.
func (c *Client) DownloadPDF(ctx context.Context, id string, opts ...CallOption) ([]byte, error) {
	cc := c.buildCallConfig(opts)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/exports/"+url.PathEscape(id)+"/pdf", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.applyHeaders(httpReq, cc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
		return nil, parseAPIError(resp)
	}

	return io.ReadAll(resp.Body)
}

// ListMyExports retrieves exports scoped to the calling user and tenant.
func (c *Client) ListMyExports(ctx context.Context, section string, limit, offset int, opts ...CallOption) (*ExportListResponse, error) {
	cc := c.buildCallConfig(opts)
	q := url.Values{}
	if section != "" {
		q.Set("section", section)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	if offset >= 0 {
		q.Set("offset", strconv.Itoa(offset))
	}

	endpoint := c.baseURL + "/exports/my?" + q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.applyHeaders(httpReq, cc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	var list ExportListResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &list, nil
}

// ExportURL is a high-level driver helper method.
// It submits the export request, polls for status until completion or failure,
// and downloads the PDF bytes directly in a single call.
func (c *Client) ExportURL(ctx context.Context, targetURL, section string, pollInterval time.Duration, opts ...CallOption) ([]byte, error) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	req := ExportRequest{URL: targetURL, Section: section}
	return c.exportAndWait(ctx, req, pollInterval, opts...)
}

// ExportHTML is a high-level driver helper method.
// It submits raw HTML content, polls for status until completion or failure,
// and downloads the PDF bytes directly in a single call.
func (c *Client) ExportHTML(ctx context.Context, html, section string, pollInterval time.Duration, opts ...CallOption) ([]byte, error) {
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	req := ExportRequest{HTML: html, Section: section}
	return c.exportAndWait(ctx, req, pollInterval, opts...)
}

func (c *Client) exportAndWait(ctx context.Context, req ExportRequest, pollInterval time.Duration, opts ...CallOption) ([]byte, error) {
	cc := c.buildCallConfig(opts)
	if cc.sync {
		return c.exportSync(ctx, req, opts...)
	}

	created, err := c.CreateExport(ctx, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("export submit failed: %w", err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			st, err := c.GetStatus(ctx, created.ID, opts...)
			if err != nil {
				return nil, fmt.Errorf("poll status failed: %w", err)
			}
			switch st.State {
			case "completed":
				return c.DownloadPDF(ctx, created.ID, opts...)
			case "failed":
				return nil, fmt.Errorf("export job %s failed: %s", created.ID, st.Error)
			}
		}
	}
}

func (c *Client) exportSync(ctx context.Context, req ExportRequest, opts ...CallOption) ([]byte, error) {
	cc := c.buildCallConfig(opts)
	req.Sync = true
	req.UseDefaultS3 = cc.useDefaultS3

	if !cc.useDefaultS3 && cc.s3Bucket != "" {
		req.S3Bucket = cc.s3Bucket
		req.S3Endpoint = cc.s3Endpoint
		req.S3Region = cc.s3Region

		encAccessKey, err := encrypt(cc.s3AccessKey, "")
		if err == nil {
			req.S3AccessKey = encAccessKey
		} else {
			req.S3AccessKey = cc.s3AccessKey
		}

		encSecretKey, err := encrypt(cc.s3SecretKey, "")
		if err == nil {
			req.S3SecretKey = encSecretKey
		} else {
			req.S3SecretKey = cc.s3SecretKey
		}
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/exports", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	c.applyHeaders(httpReq, cc)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, parseAPIError(resp)
	}

	return io.ReadAll(resp.Body)
}


// ExportHTMLToKey submits the raw HTML, polls for status until completion, and returns the S3 ObjectKey instead of downloading the file.
func (c *Client) ExportHTMLToKey(ctx context.Context, html, section string, pollInterval time.Duration, opts ...CallOption) (string, error) {
	cc := c.buildCallConfig(opts)
	if cc.sync {
		return "", fmt.Errorf("synchronous mode cannot be used with ExportHTMLToKey (which expects an S3 object key). Use ExportHTML or ExportURL instead")
	}

	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}

	req := ExportRequest{HTML: html, Section: section}
	created, err := c.CreateExport(ctx, req, opts...)
	if err != nil {
		return "", fmt.Errorf("export submit failed: %w", err)
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
			st, err := c.GetStatus(ctx, created.ID, opts...)
			if err != nil {
				return "", fmt.Errorf("poll status failed: %w", err)
			}
			switch st.State {
			case "completed":
				return st.ObjectKey, nil
			case "failed":
				return "", fmt.Errorf("export job %s failed: %s", created.ID, st.Error)
			}
		}
	}
}


func parseAPIError(resp *http.Response) error {
	var errBody struct {
		Error string `json:"error"`
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &errBody); err == nil && errBody.Error != "" {
		return fmt.Errorf("API error (%d): %s", resp.StatusCode, errBody.Error)
	}
	return fmt.Errorf("API error (%d): %s", resp.StatusCode, string(bodyBytes))
}

func getAESKey(keyStr string) []byte {
	if keyStr == "" {
		keyStr = "goexport-default-secure-shared-key-2026"
	}
	hasher := sha256.New()
	hasher.Write([]byte(keyStr))
	return hasher.Sum(nil)
}

func encrypt(plaintext, keyStr string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	key := getAESKey(keyStr)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

