package model

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OllamaConfig struct {
	Model   string
	BaseURL string
	Timeout time.Duration
}

type OllamaProvider struct {
	model      string
	baseURL    string
	httpClient *http.Client
}

func NewOllamaProvider(cfg OllamaConfig) (*OllamaProvider, error) {
	model := strings.TrimSpace(cfg.Model)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if model == "" {
		return nil, ErrMissingModel
	}
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &OllamaProvider{
		model:      model,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{
		Model:    model,
		Messages: req.Messages,
		Stream:   false,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return Response{}, err
	}

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("ollama request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	var parsed struct {
		Model            string  `json:"model"`
		Message          Message `json:"message"`
		Response         string  `json:"response"`
		Done             bool    `json:"done"`
		Error            string  `json:"error"`
		PromptTokens     int     `json:"prompt_eval_count"`
		CompletionTokens int     `json:"eval_count"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("ollama response parse failed: %w", err)
	}
	if parsed.Error != "" {
		return Response{}, fmt.Errorf("ollama error: %s", parsed.Error)
	}

	reply := strings.TrimSpace(parsed.Message.Content)
	if reply == "" {
		reply = strings.TrimSpace(parsed.Response)
	}
	if !parsed.Done && reply == "" {
		return Response{}, fmt.Errorf("ollama streaming mode is unsupported in this adapter")
	}
	if reply == "" {
		return Response{}, fmt.Errorf("ollama response has empty text")
	}

	return Response{
		Provider: p.Name(),
		Model:    modelOr(parsed.Model, model),
		Text:     reply,
		Usage:    normalizeUsage(Usage{PromptTokens: parsed.PromptTokens, CompletionTokens: parsed.CompletionTokens}),
	}, nil
}

func (p *OllamaProvider) StreamComplete(ctx context.Context, req Request, onChunk StreamChunkHandler) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
		Stream   bool      `json:"stream"`
	}{
		Model:    model,
		Messages: req.Messages,
		Stream:   true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/api/chat"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		responseBody, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return Response{}, fmt.Errorf("ollama request failed with status %d", httpResp.StatusCode)
		}
		return Response{}, fmt.Errorf("ollama request failed with status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(responseBody)))
	}

	scanner := bufio.NewScanner(httpResp.Body)
	var builder strings.Builder
	var lastUsage *Usage
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var parsed struct {
			Model            string  `json:"model"`
			Message          Message `json:"message"`
			Response         string  `json:"response"`
			Done             bool    `json:"done"`
			Error            string  `json:"error"`
			PromptTokens     int     `json:"prompt_eval_count"`
			CompletionTokens int     `json:"eval_count"`
		}
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			return Response{}, fmt.Errorf("ollama stream parse failed: %w", err)
		}
		if parsed.Error != "" {
			return Response{}, fmt.Errorf("ollama error: %s", parsed.Error)
		}
		chunk := strings.TrimSpace(parsed.Message.Content)
		if chunk == "" {
			chunk = strings.TrimSpace(parsed.Response)
		}
		if chunk != "" {
			builder.WriteString(chunk)
			if onChunk != nil {
				if err := onChunk(chunk); err != nil {
					return Response{}, err
				}
			}
		}
		lastPromptTokens := maxInt(0, parsed.PromptTokens)
		lastCompletionTokens := maxInt(0, parsed.CompletionTokens)
		if parsed.PromptTokens > 0 || parsed.CompletionTokens > 0 {
			lastUsage = &Usage{
				PromptTokens:     lastPromptTokens,
				CompletionTokens: lastCompletionTokens,
				TotalTokens:      lastPromptTokens + lastCompletionTokens,
			}
		}
		if parsed.Done {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return Response{}, err
	}

	reply := strings.TrimSpace(builder.String())
	if reply == "" {
		return Response{}, fmt.Errorf("ollama stream response has empty text")
	}

	return Response{
		Provider: p.Name(),
		Model:    model,
		Text:     reply,
		Usage:    lastUsage,
	}, nil
}
