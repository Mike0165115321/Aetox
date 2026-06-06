package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"aetox-cli/internal/app"
	"aetox-cli/internal/cognitive"
	"aetox-cli/internal/command"
	"aetox-cli/internal/config"
	"aetox-cli/internal/model"
	"aetox-cli/internal/skill"
)

const appVersion = "0.3.0-dev"

var (
	noBanner    bool
	showVersion bool
	showHelp    bool
	legacyYes   bool
)

func main() {
	var rootPath string
	var approvalTimeout int
	var modelProvider string
	var modelName string
	var modelAPIKey string
	var modelBaseURL string
	var modelTimeout int

	flag.StringVar(&rootPath, "root", "", "optional sandbox root directory (default: current directory)")
	flag.IntVar(&approvalTimeout, "approval-timeout", 60, "reserved for future approval controls")
	flag.StringVar(&modelProvider, "model-provider", "", "model provider (noop|openrouter|openai|deepseek|groq|mistral|together|perplexity|cohere|lmstudio|localai|ollama)")
	flag.StringVar(&modelName, "model-name", "", "model name (required for selected provider)")
	flag.StringVar(&modelAPIKey, "model-api-key", "", "model API key; fallback to provider env when empty")
	flag.StringVar(&modelBaseURL, "model-base-url", "", "override base URL for model provider")
	flag.IntVar(&modelTimeout, "model-timeout", 30, "model request timeout in seconds")
	flag.BoolVar(&noBanner, "no-banner", false, "disable startup banner in interactive mode")
	flag.BoolVar(&showVersion, "version", false, "print version")
	flag.BoolVar(&showHelp, "help", false, "print usage")
	flag.BoolVar(&legacyYes, "yes", false, "reserved compatibility flag")
	flag.Parse()

	providerExplicit := strings.TrimSpace(modelProvider) != ""

	if showVersion {
		fmt.Printf("aetox version %s\n", appVersion)
		return
	}
	if showHelp {
		printUsage()
		return
	}

	intent := command.Parse(flag.Args())
	cfg := config.Load(config.ConfigOptions{
		RootPath:        rootPath,
		AutoApprove:     legacyYes,
		MaxRetries:      2,
		MaxPlanRetries:  0,
		ApprovalTimeout: approvalTimeout,
		ModelProvider:   modelProvider,
		ModelName:       modelName,
		ModelAPIKey:     modelAPIKey,
		ModelBaseURL:    modelBaseURL,
		ModelTimeout:    modelTimeout,
	})

	modelProvider = cfg.ModelProvider
	modelName = cfg.ModelName
	modelAPIKey = cfg.ModelAPIKey
	modelBaseURL = cfg.ModelBaseURL

	if intent.Mode == command.ModeInteractive && isInteractive() && !providerExplicit {
		selectedProvider, selectedModel, selectedAPIKey, selectedBaseURL, ok := promptModelSelection(cfg)
		if ok {
			modelProvider = selectedProvider
			modelName = selectedModel
			modelAPIKey = selectedAPIKey
			modelBaseURL = selectedBaseURL
			cfg.ModelProvider = selectedProvider
			cfg.ModelName = selectedModel
			cfg.ModelAPIKey = selectedAPIKey
			cfg.ModelBaseURL = selectedBaseURL
		}
	}

	bootstrapResult := model.BootstrapProvider(model.BootstrapOptions{
		Provider: modelProvider,
		Model:    modelName,
		APIKey:   modelAPIKey,
		BaseURL:  modelBaseURL,
		Timeout:  time.Duration(cfg.ModelTimeoutSec) * time.Second,
	})
	if bootstrapResult.Warning != "" {
		fmt.Printf("warning: %s\n", bootstrapResult.Warning)
	}
	if bootstrapResult.Error != nil {
		fmt.Fprintf(os.Stderr, "Model fallback activated: %v\n", bootstrapResult.Error)
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider: bootstrapResult.Provider,
		Model:    modelName,
		SystemPrompt: "You are Aetox, a concise assistant in Thai and English " +
			"that helps users through a terminal conversation.",
	})

	console := app.NewStdIO()
	skillRegistry := skill.NewDefaultRegistry(skill.RegistryOptions{
		SandboxRoot: cfg.SandboxRoot,
	})
	skillDispatcher := skill.NewDispatcher(skillRegistry)
	aetoxApp, err := app.NewApp(app.Options{
		Agent:       agent,
		Console:     console,
		Dispatcher:  skillDispatcher,
		ShowBanner:  !noBanner,
		AutoApprove: cfg.AutoApprove,
		Title:       "Aetox CLI",
		Version:     appVersion,
		UserInfo:    resolveDisplayUser(),
		ModelStatus: resolveModelStatus(config.Config{
			ModelProvider: modelProvider,
			ModelName:     modelName,
		}, bootstrapResult),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime init failed: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	switch intent.Mode {
	case command.ModeHelp:
		printUsage()
	case command.ModeVersion:
		fmt.Printf("aetox version %s\n", appVersion)
	case command.ModeInteractive:
		if !isInteractive() {
			printUsage()
			os.Exit(2)
		}
		if err := aetoxApp.RunInteractive(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "interactive chat failed: %v\n", err)
			os.Exit(1)
		}
	case command.ModeOnce:
		response, err := aetoxApp.RunOnce(ctx, intent.Message)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Chat failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(response)
	default:
		printUsage()
		os.Exit(2)
	}
}

func resolveDisplayUser() string {
	if value := os.Getenv("AETOX_USER"); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value := os.Getenv("USER"); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	if value := os.Getenv("USERNAME"); strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return "local user"
}

func resolveModelStatus(cfg config.Config, bootstrapResult model.BootstrapResult) string {
	provider := strings.TrimSpace(cfg.ModelProvider)
	if provider == "" {
		provider = "noop"
	}
	modelName := strings.TrimSpace(cfg.ModelName)
	if modelName == "" {
		modelName = "default"
	}

	modelLabel := modelName
	switch provider {
	case "openrouter", "open-router", "or", "openrouterai":
		if modelLabel == "default" {
			modelLabel = "openrouter default"
		}
	case "openai", "gpt", "chatgpt", "openai-compatible", "compatible":
		if modelLabel == "default" {
			modelLabel = "gpt-4o-mini"
		}
	case "deepseek", "deepseek-api", "deepseek-ai":
		if modelLabel == "default" {
			modelLabel = "deepseek-chat"
		}
	}

	if bootstrapResult.Error != nil {
		return fmt.Sprintf("%s/%s (fallback: noop)", provider, modelLabel)
	}
	return provider + "/" + modelLabel
}

func promptModelSelection(cfg config.Config) (string, string, string, string, bool) {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println("No model provider configured. Select now, or press Enter to keep local AI mode.")
		providers := []string{
			"noop",
			"openrouter",
			"openai",
			"deepseek",
			"groq",
			"mistral",
			"together",
			"perplexity",
			"cohere",
			"lmstudio",
			"localai",
			"ollama",
		}
		for i, p := range providers {
			keyLabel := ""
			if shouldRequireAPIKey(p) {
				if key := strings.TrimSpace(config.ResolveModelAPIKey(p)); key != "" {
					keyLabel = " (env key found)"
				} else {
					keyLabel = " (needs key)"
				}
			}
			fmt.Printf("  %d) %s%s\n", i+1, p, keyLabel)
		}
		fmt.Printf("Select model provider [1-%d, Enter=noop]: ", len(providers))

		selection := strings.TrimSpace(readLine(reader))
		if selection == "" {
			return "noop", "", "", cfg.ModelBaseURL, true
		}
		idx, err := strconv.Atoi(selection)
		if err == nil && idx >= 1 && idx <= len(providers) {
			provider := providers[idx-1]
			model := strings.TrimSpace(cfg.ModelName)
			if provider == "noop" {
				return provider, model, "", cfg.ModelBaseURL, true
			}

			defaultModel := defaultModelForProvider(provider)
			prompt := fmt.Sprintf("Model name for %s [%s]: ", provider, defaultModel)
			fmt.Print(prompt)
			if model = strings.TrimSpace(readLine(reader)); model == "" {
				model = defaultModel
			}

			key := strings.TrimSpace(config.ResolveModelAPIKey(provider))
			if shouldRequireAPIKey(provider) {
				if key == "" {
					fmt.Printf("API key for %s (press Enter to keep local environment): ", provider)
					key = strings.TrimSpace(readLine(reader))
					if key == "" {
						fmt.Println("Missing API key. Try again.")
						continue
					}
				}
			}
			return provider, model, key, cfg.ModelBaseURL, true
		}

		fmt.Println("Invalid selection. Please try again.")
	}
}

func shouldRequireAPIKey(provider string) bool {
	switch config.NormalizeModelProvider(provider) {
	case "openrouter", "openai", "deepseek", "groq", "mistral", "together", "perplexity", "cohere":
		return true
	default:
		return false
	}
}

func defaultModelForProvider(provider string) string {
	switch config.NormalizeModelProvider(provider) {
	case "openrouter":
		return "deepseek/deepseek-r1"
	case "openai":
		return "gpt-4o-mini"
	case "deepseek":
		return "deepseek-chat"
	case "groq":
		return "llama-3.3-70b-versatile"
	case "mistral":
		return "mistral-small"
	case "together":
		return "google/gemma-2-9b-it"
	case "perplexity":
		return "llama-3.1-sonar-small-128k-online"
	case "cohere":
		return "command-r-plus"
	case "ollama":
		return "gemma3:4b"
	case "lmstudio":
		return "local-model"
	case "localai":
		return "local-model"
	default:
		return "default"
	}
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.TrimSuffix(line, "\r\n"))
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  aetox [flags] [goal...]")
	fmt.Println("  aetox chat \"goal\"       run one shot and exit")
	fmt.Println("  aetox                    interactive mode")
	fmt.Println("  aetox help               show this help")
	fmt.Println("Flags:")
	fmt.Println("  --model-provider: noop|openrouter|openai|deepseek|groq|mistral|together|perplexity|cohere|lmstudio|localai|ollama")
	fmt.Println("  --model-name <model>         required for openrouter")
	fmt.Println("  --model-api-key <key>        fallback: provider env (OPENAI_API_KEY, DEEPSEEK_API_KEY, GROQ_API_KEY, etc.)")
	fmt.Println("  --no-banner                 disable interactive banner")
	fmt.Println("  --yes                       auto-approve safe-mode safety prompts")
	fmt.Println("  --version                   print version")
}

func isInteractive() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
