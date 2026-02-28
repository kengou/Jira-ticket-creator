// Package jira provides a client for the Jira REST API.
package jira

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client is a Jira REST API client with retry support.
type Client struct {
	baseURL    string
	token      string
	apiVersion string // "2" for Data Center, "3" for Cloud
	httpClient *http.Client

	// Configurable retry settings
	MaxRetries int
	RetryDelay time.Duration
}

// ClientOption configures the client.
type ClientOption func(*Client)

// WithMaxRetries sets the maximum number of retries.
func WithMaxRetries(n int) ClientOption {
	return func(c *Client) {
		c.MaxRetries = n
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = d
	}
}

// normalizeURL ensures the base URL has an https scheme and no trailing slash.
// If no scheme is present, https:// is prepended. Trailing slashes are stripped.
func normalizeURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return rawURL
	}

	// Prepend https:// if no scheme is present
	if !strings.Contains(rawURL, "://") {
		rawURL = "https://" + rawURL
	}

	// Strip trailing slashes
	rawURL = strings.TrimRight(rawURL, "/")

	return rawURL
}

// NewClient creates a new Jira client.
// Set isCloud to true for Jira Cloud (API v3), false for Data Center (API v2).
func NewClient(baseURL, token string, isCloud bool, opts ...ClientOption) *Client {
	apiVersion := "2"
	if isCloud {
		apiVersion = "3"
	}

	c := &Client{
		baseURL:    normalizeURL(baseURL),
		token:      token,
		apiVersion: apiVersion,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		MaxRetries: 3,
		RetryDelay: time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// CreateIssueRequest represents a create issue request.
type CreateIssueRequest struct {
	Fields map[string]any `json:"fields"`
}

// CreateIssueResponse represents a create issue response.
type CreateIssueResponse struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

// SearchResult represents a search result.
type SearchResult struct {
	Issues []SearchIssue `json:"issues"`
	Total  int           `json:"total"`
}

// SearchIssue represents an issue in search results.
type SearchIssue struct {
	ID     string         `json:"id"`
	Key    string         `json:"key"`
	Self   string         `json:"self"`
	Fields map[string]any `json:"fields"`
}

// IssueLinkRequest represents an issue link creation request.
type IssueLinkRequest struct {
	Type         IssueLinkType `json:"type"`
	InwardIssue  IssueRef      `json:"inwardIssue"`
	OutwardIssue IssueRef      `json:"outwardIssue"`
	Comment      *LinkComment  `json:"comment,omitempty"`
}

// IssueLinkType represents the type of link.
type IssueLinkType struct {
	Name string `json:"name"`
}

// IssueRef represents an issue reference.
type IssueRef struct {
	Key string `json:"key"`
}

// LinkComment represents a comment on a link.
type LinkComment struct {
	Body string `json:"body"`
}

// CreateIssue creates a new issue in Jira.
func (c *Client) CreateIssue(req *CreateIssueRequest) (*CreateIssueResponse, error) {
	endpoint := fmt.Sprintf("/rest/api/%s/issue", c.apiVersion)

	var resp CreateIssueResponse
	if err := c.doRequest(http.MethodPost, endpoint, req, &resp); err != nil {
		return nil, fmt.Errorf("create issue: %w", err)
	}

	return &resp, nil
}

// UpdateIssueRequest represents an update issue request.
type UpdateIssueRequest struct {
	Fields map[string]any `json:"fields"`
}

// UpdateIssue updates an existing issue by key.
func (c *Client) UpdateIssue(issueKey string, req *UpdateIssueRequest) error {
	endpoint := fmt.Sprintf("/rest/api/%s/issue/%s", c.apiVersion, issueKey)
	if err := c.doRequest(http.MethodPut, endpoint, req, nil); err != nil {
		return fmt.Errorf("update issue %s: %w", issueKey, err)
	}
	return nil
}

// SearchIssues searches for issues using JQL.
func (c *Client) SearchIssues(jql string, maxResults int) (*SearchResult, error) {
	endpoint := fmt.Sprintf("/rest/api/%s/search", c.apiVersion)

	params := url.Values{}
	params.Add("jql", jql)
	params.Add("maxResults", fmt.Sprintf("%d", maxResults))

	fullURL := endpoint + "?" + params.Encode()

	var result SearchResult
	if err := c.doRequest(http.MethodGet, fullURL, nil, &result); err != nil {
		return nil, fmt.Errorf("search issues: %w", err)
	}

	return &result, nil
}

// CreateIssueLink creates a link between two issues.
func (c *Client) CreateIssueLink(req *IssueLinkRequest) error {
	endpoint := fmt.Sprintf("/rest/api/%s/issueLink", c.apiVersion)
	if err := c.doRequest(http.MethodPost, endpoint, req, nil); err != nil {
		return fmt.Errorf("create issue link: %w", err)
	}
	return nil
}

// doRequest performs an HTTP request with retry logic for transient failures.
func (c *Client) doRequest(method, endpoint string, body any, result any) error {
	var bodyReader io.Reader
	var bodyBytes []byte

	if body != nil {
		var err error
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
	}

	fullURL := c.baseURL + endpoint
	var lastErr error

	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 1s, 4s, 9s
			delay := time.Duration(attempt*attempt) * c.RetryDelay
			time.Sleep(delay)
		}

		// Reset body reader for each attempt
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequest(method, fullURL, bodyReader)
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("execute request: %w", err)
			continue
		}

		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		// Success
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if result != nil && len(respBody) > 0 {
				if err := json.Unmarshal(respBody, result); err != nil {
					return fmt.Errorf("parse response: %w", err)
				}
			}
			return nil
		}

		// Rate limiting - retry with backoff guidance from server
		if resp.StatusCode == 429 {
			retryAfter := c.parseRetryAfter(resp.Header.Get("Retry-After"))
			delay := time.Duration(retryAfter) * time.Second
			lastErr = fmt.Errorf("rate limited (429); waiting %d seconds", retryAfter)
			time.Sleep(delay)
			continue
		}

		// Client errors - don't retry
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return &APIError{
				StatusCode: resp.StatusCode,
				Body:       string(respBody),
			}
		}

		// Server errors - retry
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("server error (%d): %s", resp.StatusCode, string(respBody))
			continue
		}
	}

	return fmt.Errorf("request failed after %d attempts: %w", c.MaxRetries+1, lastErr)
}

// APIError represents a Jira API error.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("API error (%d): %s", e.StatusCode, e.Body)
}

// BuildIssueFields builds the fields map for issue creation.
func BuildIssueFields(projectKey string, fields map[string]any) map[string]any {
	result := make(map[string]any)

	result["project"] = map[string]any{
		"key": projectKey,
	}

	for k, v := range fields {
		result[k] = v
	}

	return result
}

// FormatIssueType formats an issue type for the Jira API.
func FormatIssueType(name string) map[string]any {
	return map[string]any{"name": name}
}

// FormatPriority formats a priority for the Jira API.
func FormatPriority(name string) map[string]any {
	return map[string]any{"name": name}
}

// FormatUser formats a user for the Jira API.
func FormatUser(username string) map[string]any {
	return map[string]any{"name": username}
}

// FormatComponent formats a component for the Jira API.
func FormatComponent(name string) map[string]any {
	return map[string]any{"name": name}
}

// FormatVersion formats a version for the Jira API.
func FormatVersion(name string) map[string]any {
	return map[string]any{"name": name}
}

// Epic represents a Jira Epic with its key fields.
type Epic struct {
	Key         string `json:"key"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	Description string `json:"description,omitempty"`
}

// Field represents a Jira field definition returned by the /rest/api/2/field endpoint.
type Field struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Custom bool         `json:"custom"`
	Schema *FieldSchema `json:"schema,omitempty"`
}

// FieldSchema describes the schema of a Jira field.
type FieldSchema struct {
	Type     string `json:"type"`
	Custom   string `json:"custom,omitempty"`
	CustomID int    `json:"customId,omitempty"`
}

// FetchFields retrieves all field definitions from the Jira instance.
// This is useful for discovering custom field IDs (e.g. finding the correct
// epic link field ID for your Jira Data Center configuration).
func (c *Client) FetchFields() ([]Field, error) {
	endpoint := fmt.Sprintf("/rest/api/%s/field", c.apiVersion)

	var fields []Field
	if err := c.doRequest(http.MethodGet, endpoint, nil, &fields); err != nil {
		return nil, fmt.Errorf("fetch fields: %w", err)
	}

	return fields, nil
}

// IssueLinkTypeInfo represents a link type returned by the Jira REST API.
// Each link type has an official name (e.g. "Blocks") and two directional
// descriptions: inward (e.g. "is blocked by") and outward (e.g. "blocks").
type IssueLinkTypeInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Inward  string `json:"inward"`
	Outward string `json:"outward"`
}

// issueLinkTypesResponse wraps the response from GET /rest/api/2/issueLinkType.
type issueLinkTypesResponse struct {
	IssueLinkTypes []IssueLinkTypeInfo `json:"issueLinkTypes"`
}

// FetchIssueLinkTypes retrieves all issue link types available on the instance.
// This is useful for discovering the correct link type names and their
// inward/outward descriptions before creating issue links.
func (c *Client) FetchIssueLinkTypes() ([]IssueLinkTypeInfo, error) {
	endpoint := fmt.Sprintf("/rest/api/%s/issueLinkType", c.apiVersion)

	var resp issueLinkTypesResponse
	if err := c.doRequest(http.MethodGet, endpoint, nil, &resp); err != nil {
		return nil, fmt.Errorf("fetch issue link types: %w", err)
	}

	return resp.IssueLinkTypes, nil
}

// FetchEpics retrieves all existing Epics for the given project key.
// If statusFilter is non-empty, only epics matching that status are returned.
// Prefix the status with "NOT:" to negate it (e.g. "NOT:Done" means status != Done).
// It paginates through results automatically and returns all epics found.
func (c *Client) FetchEpics(projectKey, statusFilter string) ([]Epic, error) {
	jql := fmt.Sprintf(`project = %q AND issuetype = Epic`, projectKey)
	if statusFilter != "" {
		if after, ok := strings.CutPrefix(statusFilter, "NOT:"); ok {
			jql += fmt.Sprintf(` AND status != %q`, after)
		} else {
			jql += fmt.Sprintf(` AND status = %q`, statusFilter)
		}
	}
	jql += ` ORDER BY created DESC`
	const pageSize = 50

	var epics []Epic
	startAt := 0

	for {
		endpoint := fmt.Sprintf("/rest/api/%s/search", c.apiVersion)

		params := url.Values{}
		params.Add("jql", jql)
		params.Add("maxResults", fmt.Sprintf("%d", pageSize))
		params.Add("startAt", fmt.Sprintf("%d", startAt))
		params.Add("fields", "summary,status,description")

		fullURL := endpoint + "?" + params.Encode()

		var result SearchResult
		if err := c.doRequest(http.MethodGet, fullURL, nil, &result); err != nil {
			return nil, fmt.Errorf("fetch epics: %w", err)
		}

		for _, issue := range result.Issues {
			epic := Epic{Key: issue.Key}

			if s, ok := issue.Fields["summary"].(string); ok {
				epic.Summary = s
			}
			if status, ok := issue.Fields["status"].(map[string]any); ok {
				if name, ok := status["name"].(string); ok {
					epic.Status = name
				}
			}
			if desc, ok := issue.Fields["description"].(string); ok {
				epic.Description = desc
			}

			epics = append(epics, epic)
		}

		startAt += len(result.Issues)
		if startAt >= result.Total || len(result.Issues) == 0 {
			break
		}
	}

	return epics, nil
}

// parseRetryAfter parses the Retry-After header (RFC 7231).
// Returns seconds to wait (1–3600), or default 1 if unparseable.
func (c *Client) parseRetryAfter(header string) int {
	if header == "" {
		return 1 // default retry delay
	}

	// Try parsing as decimal seconds
	if n, err := strconv.Atoi(header); err == nil && n > 0 && n <= 3600 {
		return n
	}

	// Try parsing as HTTP-date (e.g., "Wed, 21 Oct 2026 07:28:00 GMT")
	if t, err := time.Parse(time.RFC1123, header); err == nil {
		delta := int(time.Until(t).Seconds())
		if delta > 0 && delta <= 3600 {
			return delta
		}
		if delta <= 0 {
			return 1
		}
		return 3600 // cap at 1 hour
	}

	// Fallback
	return 1
}
