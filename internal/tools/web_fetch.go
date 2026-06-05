package tools

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"aetox-cli/internal/contracts"
)

type WebFetchTool struct {
	client   *http.Client
	maxBytes int
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		client:   &http.Client{Timeout: 15 * time.Second},
		maxBytes: 20_000,
	}
}

func (w *WebFetchTool) Name() string {
	return "web"
}

func (w *WebFetchTool) Description() string {
	return "Fetches URL content with bounded output"
}

func (w *WebFetchTool) Actions() []string {
	return []string{"fetch"}
}

func (w *WebFetchTool) Risk(action string, _ map[string]any) contracts.RiskLevel {
	if action == "fetch" {
		return contracts.RiskHigh
	}
	return contracts.RiskHigh
}

func (w *WebFetchTool) Execute(ctx context.Context, action string, params map[string]any) (contracts.StepResult, error) {
	switch strings.TrimSpace(strings.ToLower(action)) {
	case "fetch":
		return w.execFetch(ctx, params)
	default:
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "unsupported action: " + action,
			Artifacts:  map[string]string{},
			Confidence: 0.0,
		}, fmt.Errorf("unsupported action: %s", action)
	}
}

func (w *WebFetchTool) execFetch(ctx context.Context, params map[string]any) (contracts.StepResult, error) {
	url := strings.TrimSpace(strParam(params, "url", ""))
	if url == "" {
		url = strings.TrimSpace(strParam(params, "target", ""))
	}
	if url == "" {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      "missing URL",
			Confidence: 0.0,
		}, fmt.Errorf("missing URL")
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      err.Error(),
			Confidence: 0.0,
		}, err
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      err.Error(),
			Confidence: 0.0,
		}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      fmt.Sprintf("unexpected status: %d", resp.StatusCode),
			Artifacts:  map[string]string{"url": url, "status": fmt.Sprint(resp.StatusCode)},
			Confidence: 0.0,
		}, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(w.maxBytes+1)))
	if err != nil {
		return contracts.StepResult{
			Status:     contracts.StatusFailure,
			Error:      err.Error(),
			Confidence: 0.0,
		}, err
	}
	output := string(body)
	truncated := false
	if len(body) > w.maxBytes {
		output = string(body[:w.maxBytes])
		truncated = true
	}
	output = strings.TrimSpace(output)
	if output == "" {
		output = "(empty response)"
	}

	if truncated {
		output += "\n[truncated]"
	}

	return contracts.StepResult{
		Status:     contracts.StatusSuccess,
		Output:     output,
		Artifacts:  map[string]string{"url": url, "status": fmt.Sprint(resp.StatusCode), "bytes": fmt.Sprint(len(body))},
		Confidence: 1.0,
		MemoryUpdates: map[string]any{
			"last_url": url,
			"bytes":    len(body),
		},
	}, nil
}

