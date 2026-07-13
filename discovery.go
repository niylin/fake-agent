package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type discoveryResult struct {
	UUID  string
	Token string
}

type discoveryAPIResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Data    struct {
		UUID  string `json:"uuid"`
		Token string `json:"token"`
	} `json:"data"`
	UUID  string `json:"uuid"`
	Token string `json:"token"`
}

var newDiscoveryHTTPClient = func(ignoreUnsafeCert bool) *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: ignoreUnsafeCert,
				MinVersion:         tls.VersionTLS12,
			},
		},
	}
}

func RegisterByDiscoveryKey(ctx context.Context, cfg AgentConfig) (discoveryResult, error) {
	if strings.TrimSpace(cfg.DiscoveryKey) == "" {
		return discoveryResult{}, errors.New("auto discovery key is required")
	}
	u, err := buildRegisterURL(cfg.Endpoint, cfg.Name)
	if err != nil {
		return discoveryResult{}, err
	}

	client := newDiscoveryHTTPClient(cfg.IgnoreUnsafeCert)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return discoveryResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(cfg.DiscoveryKey))
	req.Header.Set("User-Agent", "fake-komari-agent/"+appVersion)

	resp, err := client.Do(req)
	if err != nil {
		return discoveryResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return discoveryResult{}, err
	}
	var parsed discoveryAPIResponse
	if len(body) > 0 {
		_ = json.Unmarshal(body, &parsed)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := parsed.Message
		if msg == "" {
			msg = strings.TrimSpace(string(body))
		}
		if msg == "" {
			msg = resp.Status
		}
		return discoveryResult{}, fmt.Errorf("auto discovery failed: %s", msg)
	}

	result := discoveryResult{
		UUID:  parsed.Data.UUID,
		Token: parsed.Data.Token,
	}
	if result.UUID == "" {
		result.UUID = parsed.UUID
	}
	if result.Token == "" {
		result.Token = parsed.Token
	}
	if result.Token == "" {
		return discoveryResult{}, fmt.Errorf("auto discovery response did not include token: %s", strings.TrimSpace(string(body)))
	}
	return result, nil
}

func buildRegisterURL(endpoint, name string) (*url.URL, error) {
	u, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
	default:
		return nil, fmt.Errorf("unsupported endpoint scheme %q", u.Scheme)
	}
	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/api/clients/register") {
		path += "/api/clients/register"
	}
	if path == "" {
		path = "/api/clients/register"
	}
	u.Path = path
	q := u.Query()
	if strings.TrimSpace(name) != "" {
		q.Set("name", strings.TrimSpace(name))
	}
	u.RawQuery = q.Encode()
	return u, nil
}

func parseEndpoint(endpoint string) (*url.URL, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, errors.New("endpoint is empty")
	}
	if !strings.Contains(endpoint, "://") {
		endpoint = "https://" + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, errors.New("endpoint host is empty")
	}
	return u, nil
}
