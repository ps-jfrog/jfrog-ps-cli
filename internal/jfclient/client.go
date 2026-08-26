// Package jfclient provides a minimal, dependency-free HTTP client for calling
// JFrog Platform REST APIs (Artifactory, Access, Xray) with either bearer-token
// or basic authentication.
package jfclient

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a single JFrog Platform Deployment (JPD).
type Client struct {
	// BaseURL is the JPD root, e.g. "https://mycompany.jfrog.io" (no trailing
	// slash, no "/artifactory" suffix -- callers pass the full API path,
	// e.g. "/artifactory/api/repositories" or "/access/api/v2/users").
	BaseURL string

	// Token, if set, is sent as "Authorization: Bearer <Token>".
	Token string

	// User/Password are used for basic auth when Token is empty.
	User     string
	Password string

	HTTPClient *http.Client
}

// New creates a Client with sane defaults (30s timeout). If insecure is true,
// TLS certificate verification is disabled (useful for self-signed test JPDs).
func New(baseURL, token, user, password string, insecure bool) *Client {
	tr := &http.Transport{}
	if insecure {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Token:    token,
		User:     user,
		Password: password,
		HTTPClient: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}
}

// Response wraps the outcome of an API call.
type Response struct {
	StatusCode int
	Body       []byte
}

// OK reports whether the call returned a 2xx status.
func (r *Response) OK() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// Unmarshal decodes the response body as JSON into v.
func (r *Response) Unmarshal(v interface{}) error {
	if len(r.Body) == 0 {
		return nil
	}
	return json.Unmarshal(r.Body, v)
}

// Error returns a formatted error if the response was not 2xx, else nil.
func (r *Response) Error(context string) error {
	if r.OK() {
		return nil
	}
	body := strings.TrimSpace(string(r.Body))
	if len(body) > 500 {
		body = body[:500] + "...(truncated)"
	}
	return fmt.Errorf("%s: HTTP %d: %s", context, r.StatusCode, body)
}

// Do issues an HTTP request against path (relative to BaseURL) with the given
// method. body, if non-nil, is marshaled as JSON unless it is a []byte or
// string, which are sent as-is (for AQL / plain-text payloads).
func (c *Client) Do(method, path string, body interface{}) (*Response, error) {
	var reader io.Reader
	contentType := "application/json"
	switch b := body.(type) {
	case nil:
		reader = nil
	case []byte:
		reader = bytes.NewReader(b)
		contentType = "text/plain"
	case string:
		reader = strings.NewReader(b)
		contentType = "text/plain"
	default:
		data, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(data)
	}

	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, url, err)
	}
	if reader != nil {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	c.setAuth(req)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, url, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response from %s %s: %w", method, url, err)
	}
	return &Response{StatusCode: resp.StatusCode, Body: data}, nil
}

func (c *Client) setAuth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		return
	}
	if c.User != "" {
		req.SetBasicAuth(c.User, c.Password)
	}
}

// Get is a convenience wrapper around Do("GET", path, nil).
func (c *Client) Get(path string) (*Response, error) { return c.Do(http.MethodGet, path, nil) }

// Post is a convenience wrapper around Do("POST", path, body).
func (c *Client) Post(path string, body interface{}) (*Response, error) {
	return c.Do(http.MethodPost, path, body)
}

// Put is a convenience wrapper around Do("PUT", path, body).
func (c *Client) Put(path string, body interface{}) (*Response, error) {
	return c.Do(http.MethodPut, path, body)
}

// Patch is a convenience wrapper around Do("PATCH", path, body).
func (c *Client) Patch(path string, body interface{}) (*Response, error) {
	return c.Do(http.MethodPatch, path, body)
}

// Delete is a convenience wrapper around Do("DELETE", path, nil).
func (c *Client) Delete(path string) (*Response, error) { return c.Do(http.MethodDelete, path, nil) }

// AQL executes an Artifact Query Language search against
// /artifactory/api/search/aql and returns the raw response.
func (c *Client) AQL(query string) (*Response, error) {
	return c.Do(http.MethodPost, "/artifactory/api/search/aql", query)
}

// Ping checks connectivity to Artifactory (GET /artifactory/api/system/ping).
func (c *Client) Ping() error {
	resp, err := c.Get("/artifactory/api/system/ping")
	if err != nil {
		return err
	}
	return resp.Error("ping")
}
