package config

import (
	"os"
	"strings"
)

type Config struct {
	SandboxRoot        string
	AutoApprove        bool
	MaxRetries         int
	MaxPlanRetries     int
	ApprovalTimeoutSec int
	MaxOutputFiles     int
	ModelProvider      string
	ModelName          string
	ModelAPIKey        string
	ModelBaseURL       string
	ModelTimeoutSec    int
}

type ConfigOptions struct {
	RootPath        string
	AutoApprove     bool
	MaxRetries      int
	MaxPlanRetries  int
	ApprovalTimeout int
	ModelProvider   string
	ModelName       string
	ModelAPIKey     string
	ModelBaseURL    string
	ModelTimeout    int
}

func Load(opt ConfigOptions) Config {
	root := opt.RootPath
	if root == "" {
		root, _ = os.Getwd()
	}

	maxRetries := opt.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	maxPlanRetries := opt.MaxPlanRetries
	if maxPlanRetries < 0 {
		maxPlanRetries = 0
	}

	timeout := opt.ApprovalTimeout
	if timeout <= 0 {
		timeout = 60
	}

	provider := strings.ToLower(strings.TrimSpace(opt.ModelProvider))
	if provider == "" {
		provider = "noop"
	}

	modelName := strings.TrimSpace(opt.ModelName)
	modelAPIKey := strings.TrimSpace(opt.ModelAPIKey)
	if modelAPIKey == "" {
		modelAPIKey = strings.TrimSpace(resolveModelAPIKey(provider))
	}
	baseURL := strings.TrimSpace(opt.ModelBaseURL)
	modelTimeout := opt.ModelTimeout
	if modelTimeout <= 0 {
		modelTimeout = 30
	}

	return Config{
		SandboxRoot:        root,
		AutoApprove:        opt.AutoApprove,
		MaxRetries:         maxRetries,
		MaxPlanRetries:     maxPlanRetries,
		ApprovalTimeoutSec: timeout,
		MaxOutputFiles:     2000,
		ModelProvider:      provider,
		ModelName:          modelName,
		ModelAPIKey:        modelAPIKey,
		ModelBaseURL:       baseURL,
		ModelTimeoutSec:    modelTimeout,
	}
}

func ResolveModelAPIKey(provider string) string {
	return strings.TrimSpace(resolveModelAPIKey(provider))
}

func NormalizeModelProvider(provider string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	switch provider {
	case "open-router", "openrouterai", "or":
		return "openrouter"
	case "chatgpt", "gpt", "openai-compatible", "compatible":
		return "openai"
	case "deepseek-api", "deepseek-ai":
		return "deepseek"
	case "groqcloud":
		return "groq"
	case "mistralai":
		return "mistral"
	case "togetherai", "together-ai":
		return "together"
	case "perplexityai", "pplx":
		return "perplexity"
	case "ollamaai":
		return "ollama"
	case "lmstudio", "localai", "local-ai":
		return "lmstudio"
	default:
		return provider
	}
}

func resolveModelAPIKey(provider string) string {
	switch provider {
	case "openrouter", "open-router", "openrouterai", "or":
		return strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	case "openai", "chatgpt", "gpt", "openai-compatible", "compatible":
		if key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("OPENAI_API_TOKEN"))
	case "deepseek", "deepseek-api", "deepseek-ai":
		return strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))
	case "groq", "groqcloud":
		return strings.TrimSpace(os.Getenv("GROQ_API_KEY"))
	case "mistral", "mistralai":
		return strings.TrimSpace(os.Getenv("MISTRAL_API_KEY"))
	case "together", "togetherai", "together-ai":
		return strings.TrimSpace(os.Getenv("TOGETHER_API_KEY"))
	case "perplexity", "perplexityai", "pplx":
		return strings.TrimSpace(os.Getenv("PERPLEXITY_API_KEY"))
	case "cohere", "command-r":
		return strings.TrimSpace(os.Getenv("COHERE_API_KEY"))
	case "lmstudio", "localai", "local-ai":
		if key := strings.TrimSpace(os.Getenv("LLM_API_KEY")); key != "" {
			return key
		}
		return strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	case "ollama", "ollamaai":
		return ""
	default:
		return ""
	}
}
