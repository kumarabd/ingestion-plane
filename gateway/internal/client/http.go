package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
	headers    map[string]string
}

type ClientConfig struct {
	BaseURL    string
	Timeout    time.Duration
	Headers    map[string]string
}

func NewClient(cfg *ClientConfig) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	httpClient := &http.Client{
		Timeout: cfg.Timeout,
	}

	return &Client{
		httpClient: httpClient,
		baseURL:    cfg.BaseURL,
		headers:    cfg.Headers,
	}
}

func (c *Client) Get(path string, headers map[string]string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Add default headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Add request-specific headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.httpClient.Do(req)
}

func (c *Client) Post(path string, body interface{}, headers map[string]string) (*http.Response, error) {
	url := c.baseURL + path
	
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest("POST", url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Add default headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Add request-specific headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Set content type for JSON
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

func (c *Client) Put(path string, body interface{}, headers map[string]string) (*http.Response, error) {
	url := c.baseURL + path
	
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest("PUT", url, bodyReader)
	if err != nil {
		return nil, err
	}

	// Add default headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Add request-specific headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	// Set content type for JSON
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.httpClient.Do(req)
}

func (c *Client) Delete(path string, headers map[string]string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return nil, err
	}

	// Add default headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	// Add request-specific headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.httpClient.Do(req)
}
