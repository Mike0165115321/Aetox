package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	Provider      string
	Model         string
	APIKey        string
	BaseURL       string
	Timeout       time.Duration
	RequireAPIKey *bool
}

type OpenAICompatibleProvider struct {
	provider   string
	model      string
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

func NewOpenAICompatibleProvider(cfg OpenAICompatibleConfig) (*OpenAICompatibleProvider, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = "openai-compatible"
	}
	model := strings.TrimSpace(cfg.Model)
	apiKey := strings.TrimSpace(cfg.APIKey)
	baseURL := strings.TrimSpace(cfg.BaseURL)
	requireAPIKey := true
	if cfg.RequireAPIKey != nil {
		requireAPIKey = *cfg.RequireAPIKey
	}

	if model == "" {
		return nil, ErrMissingModel
	}
	if requireAPIKey && apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	if baseURL == "" {
		baseURL = defaultOpenAICompatibleBaseURL(provider)
	}
	baseURL = strings.TrimSuffix(baseURL, "/")

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}

	return &OpenAICompatibleProvider{
		provider:   provider,
		model:      model,
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func defaultOpenAICompatibleBaseURL(provider string) string {
	switch provider {
	case "groq":
		return "https://api.groq.com/openai/v1"
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "mistral", "mistralai":
		return "https://api.mistral.ai/v1"
	case "together", "togetherai", "together-ai":
		return "https://api.together.xyz/v1"
	case "perplexity", "perplexityai", "pplx":
		return "https://api.perplexity.ai"
	case "cohere", "command-r":
		return "https://api.cohere.com/v1"
	case "lmstudio", "localai", "local-ai":
		return "http://localhost:1234/v1"
	case "openai", "gpt", "chatgpt", "openai-compatible":
		fallthrough
	default:
		return "https://api.openai.com/v1"
	}
}

func (p *OpenAICompatibleProvider) Name() string {
	return p.provider
}

func (p *OpenAICompatibleProvider) Complete(ctx context.Context, req Request) (Response, error) {
	if len(req.Messages) == 0 {
		return Response{}, ErrNoMessages
	}

	model := req.Model
	if model == "" {
		model = p.model
	}

	payload := struct {
		Model       string    `json:"model"`
		Messages    []Message `json:"messages"`
		Temperature float64   `json:"temperature,omitempty"`
		MaxTokens   int       `json:"max_tokens,omitempty"`
	}{
		Model:       model,
		Messages:    req.Messages,
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}

	requestURL := p.baseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

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
		return Response{}, fmt.Errorf(
			"%s request failed with status %d: %s",
			p.provider,
			httpResp.StatusCode,
			strings.TrimSpace(string(responseBody)),
		)
	}

	var parsed struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return Response{}, fmt.Errorf("%s response parse failed: %w", p.provider, err)
	}
	if len(parsed.Choices) == 0 {
		return Response{}, fmt.Errorf("%s response has no choices", p.provider)
	}

	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if text == "" {
		return Response{}, fmt.Errorf("%s response has empty text", p.provider)
	}

	return Response{
		Provider: p.Name(),
		Model:    modelOr(parsed.Model, model),
		Text:     text,
	}, nil
}
