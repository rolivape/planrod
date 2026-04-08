package crossservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Client struct {
	Mode         string // "none" | "lite" | "strict"
	MemoryRodURL string
	Timeout      time.Duration
	HTTP         *http.Client
	Logger       *slog.Logger
}

type ValidationResult struct {
	Entity string `json:"entity"`
	Exists bool   `json:"exists"`
	Error  error  `json:"-"`
}

func NewClient(mode, memoryRodURL string, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{
		Mode:         mode,
		MemoryRodURL: memoryRodURL,
		Timeout:      timeout,
		HTTP:         &http.Client{Timeout: timeout},
		Logger:       logger,
	}
}

// ResolveMode returns the effective mode given the configured mode and an override.
func (c *Client) ResolveMode(override string) (string, error) {
	if override == "" {
		return c.Mode, nil
	}
	if c.Mode == "none" {
		return "", fmt.Errorf("cross_service_mode override rejected: service started in 'none' mode")
	}
	if override != "lite" && override != "strict" {
		return "", fmt.Errorf("invalid cross_service_mode override %q, must be 'lite' or 'strict'", override)
	}
	return override, nil
}

// ValidateEntities checks that entity names exist in MemoryRod (strict mode only).
func (c *Client) ValidateEntities(ctx context.Context, names []string, override string) ([]ValidationResult, error) {
	mode, err := c.ResolveMode(override)
	if err != nil {
		return nil, err
	}

	results := make([]ValidationResult, len(names))
	if mode != "strict" {
		for i, n := range names {
			results[i] = ValidationResult{Entity: n, Exists: true}
		}
		return results, nil
	}

	for i, name := range names {
		results[i] = ValidationResult{Entity: name}
		exists, err := c.checkEntity(ctx, name)
		if err != nil {
			results[i].Error = err
			c.Logger.Warn("cross-service validation failed",
				"entity", name, "error", err,
				"service_mode", c.Mode, "override_mode", override)
			return results, fmt.Errorf("cross_service_validation_failed: entity %q: %w", name, err)
		}
		if !exists {
			c.Logger.Warn("cross-service entity not found",
				"entity", name,
				"service_mode", c.Mode, "override_mode", override)
			return results, fmt.Errorf("cross_service_validation_failed: entity %q not found in MemoryRod", name)
		}
		results[i].Exists = true
	}

	c.Logger.Info("cross-service validation passed",
		"entities", names, "mode", mode,
		"service_mode", c.Mode, "override_mode", override)
	return results, nil
}

// ValidateSpecs checks that spec titles exist in MemoryRod (strict mode only).
func (c *Client) ValidateSpecs(ctx context.Context, titles []string, override string) ([]ValidationResult, error) {
	mode, err := c.ResolveMode(override)
	if err != nil {
		return nil, err
	}

	results := make([]ValidationResult, len(titles))
	if mode != "strict" {
		for i, t := range titles {
			results[i] = ValidationResult{Entity: t, Exists: true}
		}
		return results, nil
	}

	for i, title := range titles {
		results[i] = ValidationResult{Entity: title}
		exists, err := c.checkSpec(ctx, title)
		if err != nil {
			results[i].Error = err
			return results, fmt.Errorf("cross_service_validation_failed: spec %q: %w", title, err)
		}
		if !exists {
			return results, fmt.Errorf("cross_service_validation_failed: spec %q not found in MemoryRod", title)
		}
		results[i].Exists = true
	}
	return results, nil
}

// IsReachable checks if MemoryRod is reachable.
func (c *Client) IsReachable(ctx context.Context) bool {
	if c.Mode == "none" {
		return false
	}
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/list",
		"id":      1,
	}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.MemoryRodURL, bytes.NewReader(body))
	if err != nil {
		return false
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func (c *Client) checkEntity(ctx context.Context, name string) (bool, error) {
	return c.mcpCall(ctx, "memory_get_entity", map[string]interface{}{"name": name})
}

func (c *Client) checkSpec(ctx context.Context, title string) (bool, error) {
	return c.mcpCall(ctx, "memory_get_spec", map[string]interface{}{"title": title})
}

func (c *Client) mcpCall(ctx context.Context, tool string, args map[string]interface{}) (bool, error) {
	req := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      tool,
			"arguments": args,
		},
		"id": 1,
	}

	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.MemoryRodURL, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return false, fmt.Errorf("memoryrod_unavailable: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var rpcResp struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return false, fmt.Errorf("invalid response from MemoryRod: %w", err)
	}

	if rpcResp.Result.IsError {
		return false, nil
	}
	return true, nil
}
