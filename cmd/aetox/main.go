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

	"golang.org/x/term"
)

const appVersion = "0.3.0-dev"

var (
	noBanner    bool
	showVersion bool
	showHelp    bool
	legacyYes   bool
)

func main() {
	setUTF8Console()

	var rootPath string
	var approvalTimeout int
	var modelProvider string
	var modelName string
	var modelAPIKey string
	var modelBaseURL string
	var modelTimeout int

	flag.StringVar(&rootPath, "root", "", "optional sandbox root directory (default: current directory)")
	flag.IntVar(&approvalTimeout, "approval-timeout", 60, "reserved for future approval controls")
	flag.StringVar(&modelProvider, "model-provider", "", "model provider (openrouter|ollama)")
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
	modelNameExplicit := strings.TrimSpace(modelName) != ""
	baseURLExplicit := strings.TrimSpace(modelBaseURL) != ""
	explicitModelConfig := providerExplicit || modelNameExplicit || baseURLExplicit

	if showVersion {
		fmt.Printf("aetox version %s\n", appVersion)
		return
	}
	if showHelp {
		printUsage()
		return
	}

	intent := command.ParseArgs(flag.Args())
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

	storedPreference, hasStoredPreference, prefErr := config.LoadModelPreference()
	if prefErr != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot read model preference: %v\n", prefErr)
	}
	if !explicitModelConfig && !providerExplicit {
		if hasStoredPreference {
			if strings.TrimSpace(storedPreference.ModelProvider) != "" {
				modelProvider = strings.TrimSpace(storedPreference.ModelProvider)
				cfg.ModelProvider = modelProvider
			}
			if strings.TrimSpace(storedPreference.ModelName) != "" {
				modelName = strings.TrimSpace(storedPreference.ModelName)
				cfg.ModelName = modelName
			}
			if strings.TrimSpace(storedPreference.ModelBaseURL) != "" {
				modelBaseURL = strings.TrimSpace(storedPreference.ModelBaseURL)
				cfg.ModelBaseURL = modelBaseURL
			}
		}
	}

	currentConfig := cfg

	if intent.Mode == command.ModeInteractive && isInteractive() && !explicitModelConfig && !hasStoredPreference {
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
			currentConfig = cfg
			if saveErr := persistModelPreference(currentConfig); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot save model preference: %v\n", saveErr)
			}
		}
	}

	cfg.ModelProvider = strings.TrimSpace(modelProvider)
	cfg.ModelName = strings.TrimSpace(modelName)
	cfg.ModelAPIKey = strings.TrimSpace(modelAPIKey)
	cfg.ModelBaseURL = strings.TrimSpace(modelBaseURL)

	if strings.TrimSpace(cfg.ModelName) == "" &&
		!strings.EqualFold(strings.TrimSpace(cfg.ModelProvider), "noop") {
		cfg.ModelName = model.DefaultModel(cfg.ModelProvider)
		modelName = cfg.ModelName
	}

	currentConfig = cfg
	bootstrapResult, _ := bootstrapModelWithStatus(cfg)
	if bootstrapResult.Provider == nil {
		fmt.Fprintf(os.Stderr, "runtime init failed: %v\n", bootstrapResult.Error)
		os.Exit(1)
	}
	if bootstrapResult.Warning != "" {
		fmt.Fprintf(os.Stderr, "warning: %s\n", bootstrapResult.Warning)
		if bootstrapResult.Error != nil {
			fmt.Fprintf(os.Stderr, "detail: %v\n", bootstrapResult.Error)
		}
	}

	if err := persistModelPreference(currentConfig); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot save model preference: %v\n", err)
	}

	agent := cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        currentConfig.ModelName,
		SystemPrompt: buildSystemPrompt(cfg.SandboxRoot),
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
			ModelName:     currentConfig.ModelName,
		}, bootstrapResult),
		ModelSwitch: func(ctx context.Context) (*cognitive.Agent, string, bool, error) {
			return switchProvider(ctx, &currentConfig)
		},
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

func switchProvider(ctx context.Context, cfg *config.Config) (*cognitive.Agent, string, bool, error) {
	if ctx == nil {
		return nil, "", false, nil
	}

	select {
	case <-ctx.Done():
		return nil, "", false, ctx.Err()
	default:
	}

	selectedProvider, selectedModel, selectedAPIKey, selectedBaseURL, ok := promptModelSelection(*cfg)
	if !ok {
		return nil, "", false, nil
	}

	cfg.ModelProvider = strings.TrimSpace(selectedProvider)
	cfg.ModelName = strings.TrimSpace(selectedModel)
	cfg.ModelAPIKey = strings.TrimSpace(selectedAPIKey)
	cfg.ModelBaseURL = strings.TrimSpace(selectedBaseURL)

	if cfg.ModelName == "" && !strings.EqualFold(cfg.ModelProvider, "noop") {
		cfg.ModelName = model.DefaultModel(cfg.ModelProvider)
	}

	fmt.Printf("Initializing model provider: %s/%s...\n", cfg.ModelProvider, cfg.ModelName)
	bootstrapResult, modelStatus := bootstrapModelWithStatus(*cfg)
	if bootstrapResult.Provider == nil {
		return nil, "", false, bootstrapResult.Error
	}
	if bootstrapResult.Warning != "" {
		fmt.Printf("warning: %s\n", bootstrapResult.Warning)
		if bootstrapResult.Error != nil {
			fmt.Printf("detail: %v\n", bootstrapResult.Error)
		}
	}

	if err := persistModelPreference(*cfg); err != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot save model preference: %v\n", err)
	}

	return cognitive.NewAgent(cognitive.AgentConfig{
		Provider:     bootstrapResult.Provider,
		Model:        cfg.ModelName,
		SystemPrompt: buildSystemPrompt(cfg.SandboxRoot),
	}), modelStatus, true, nil
}

func buildSystemPrompt(root string) string {
	sandboxRoot := strings.TrimSpace(root)
	if sandboxRoot == "" {
		sandboxRoot = "(unknown)"
	}
	return "You are Aetox, a concise assistant in Thai and English " +
		"that helps users through a terminal conversation.\n" +
		"Current working sandbox root is: " + sandboxRoot + ".\n" +
		"Always answer location-related questions using this path unless explicitly changed by user commands."
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
	return model.ResolveStatus(cfg.ModelProvider, cfg.ModelName, bootstrapResult.Error)
}

func bootstrapModelWithStatus(cfg config.Config) (model.BootstrapResult, string) {
	timeout := time.Duration(cfg.ModelTimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	result := model.BootstrapProvider(model.BootstrapOptions{
		Provider: cfg.ModelProvider,
		Model:    cfg.ModelName,
		APIKey:   cfg.ModelAPIKey,
		BaseURL:  cfg.ModelBaseURL,
		Timeout:  timeout,
	})
	return result, resolveModelStatus(cfg, result)
}

func persistModelPreference(cfg config.Config) error {
	provider := strings.TrimSpace(cfg.ModelProvider)
	if provider == "" {
		return nil
	}
	pref := config.ModelPreference{
		ModelProvider: provider,
		ModelName:     strings.TrimSpace(cfg.ModelName),
		ModelBaseURL:  strings.TrimSpace(cfg.ModelBaseURL),
	}
	return config.SaveModelPreference(pref)
}

func promptModelSelection(cfg config.Config) (string, string, string, string, bool) {
	reader := bufio.NewReader(os.Stdin)

	providers := model.SupportedProviders()
	providerOptions := make([]string, 0, len(providers))
	for _, p := range providers {
		label := p
		if model.RequiresAPIKey(p) {
			label = model.FormatProviderMenuLabel(p, model.ResolveModelAPIKey(p) != "")
		}
		providerOptions = append(providerOptions, label)
	}

	for {
		idx, ok := pickFromMenu(reader, "No model provider configured. Select one.", providerOptions, 0, "Use ↑/↓ then Enter.")
		if !ok {
			return providers[0], model.DefaultModel(providers[0]), "", cfg.ModelBaseURL, false
		}
		provider := providers[idx]

		selectedModel := pickModelForProvider(reader, provider, cfg.ModelName, cfg.ModelBaseURL)
		fmt.Printf("Selected: %s / %s\n\n", provider, selectedModel)

		key := strings.TrimSpace(model.ResolveModelAPIKey(provider))
		if model.RequiresAPIKey(provider) {
			if key == "" {
				fmt.Printf("API key for %s (press Enter to keep local environment): ", provider)
				key = strings.TrimSpace(readLine(reader))
				if key == "" {
					fmt.Println("Missing API key. Try again.")
					continue
				}
			}
		}
		return provider, selectedModel, key, cfg.ModelBaseURL, true
	}
}

func pickModelForProvider(reader *bufio.Reader, provider, existing, baseURL string) string {
	modelChoices, err := model.ModelChoicesWithEndpoint(provider, baseURL)
	if err != nil || len(modelChoices) == 0 {
		modelChoices = model.ModelChoices(provider)
	}
	defaultModel := model.DefaultModel(provider)
	if existing != "" {
		defaultModel = existing
	}

	if len(modelChoices) == 0 {
		fmt.Printf("Model name for %s [%s] (or type custom): ", provider, defaultModel)
		if model := strings.TrimSpace(readLine(reader)); model != "" {
			return model
		}
		return defaultModel
	}

	options := append([]string{}, modelChoices...)
	options = append(options, "custom model ...")
	defaultIndex := 0
	for i, m := range modelChoices {
		if m == defaultModel {
			defaultIndex = i
			break
		}
	}

	idx, ok := pickFromMenu(reader, fmt.Sprintf("Choose model for %s", provider), options, defaultIndex, "Use ↑/↓ then Enter.")
	if !ok {
		return defaultModel
	}

	if idx == len(modelChoices) {
		fmt.Printf("Model name for %s [%s]: ", provider, defaultModel)
		if model := strings.TrimSpace(readLine(reader)); model != "" {
			return model
		}
		return defaultModel
	}

	return modelChoices[idx]
}

func pickFromMenu(reader *bufio.Reader, title string, options []string, defaultIndex int, hint string) (int, bool) {
	if len(options) == 0 {
		return 0, true
	}
	selected := defaultIndex
	if selected < 0 || selected >= len(options) {
		selected = 0
	}
	renderedLines := len(options) + 3
	interactiveMode := isInteractive()
	render := func() {
		fmt.Println()
		fmt.Println(title)
		for i, option := range options {
			prefix := "  "
			if i == selected {
				prefix = " >"
			}
			fmt.Printf("%s %s\n", prefix, option)
		}
		fmt.Println(hint)
	}
	redrawMenu := func() {
		if !interactiveMode {
			return
		}
		for i := 0; i < renderedLines; i++ {
			fmt.Print("\033[2K\r\033[F")
		}
	}
	clearMenu := func() {
		if !interactiveMode {
			return
		}
		for i := 0; i < renderedLines+1; i++ {
			fmt.Print("\033[2K\r\033[F")
		}
	}

	if !isInteractive() {
		fmt.Println(title)
		for i, option := range options {
			fmt.Printf("  %d) %s\n", i+1, option)
		}
		for {
			fmt.Printf("Select [1-%d]: ", len(options))
			input := strings.TrimSpace(readLine(reader))
			if input == "" {
				return selected, true
			}
			if input == "0" {
				return selected, true
			}
			for i := range options {
				if input == fmt.Sprint(i+1) {
					return i, true
				}
			}
			fmt.Println("Invalid selection.")
		}
	}

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		// fallback: keep old behavior.
		return selectMenuUsingNumbers(reader, title, options, selected)
	}
	defer func() {
		_ = term.Restore(int(os.Stdin.Fd()), oldState)
	}()

	render()
	for {
		input, err := readSingleKey(reader)
		if err != nil {
			return selected, false
		}
		switch input {
		case keyMenuUp:
			selected--
			if selected < 0 {
				selected = len(options) - 1
			}
		case keyMenuDown:
			selected++
			if selected >= len(options) {
				selected = 0
			}
		case keyMenuEnter:
			clearMenu()
			return selected, true
		case keyMenuCancel:
			clearMenu()
			return selected, false
		}
		redrawMenu()
		render()
	}
}

const (
	keyMenuUp = iota + 1
	keyMenuDown
	keyMenuEnter
	keyMenuCancel
)

func readSingleKey(reader *bufio.Reader) (int, error) {
	b, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 0x00:
		next, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch next {
		case 'H':
			return keyMenuUp, nil
		case 'P':
			return keyMenuDown, nil
		default:
			return 0, nil
		}
	case 0x1b:
		next, err := reader.ReadByte()
		if err != nil {
			return 0, err
		}
		if next != '[' {
			return 0, nil
		}
		next, err = reader.ReadByte()
		if err != nil {
			return 0, err
		}
		switch next {
		case 'A':
			return keyMenuUp, nil
		case 'B':
			return keyMenuDown, nil
		default:
			return 0, nil
		}
	case 0x0d, 0x0a:
		return keyMenuEnter, nil
	case 0x03:
		return keyMenuCancel, nil
	default:
		return int(b), nil
	}
}

func selectMenuUsingNumbers(reader *bufio.Reader, title string, options []string, selected int) (int, bool) {
	for {
		fmt.Println(title)
		for i, option := range options {
			prefix := "  "
			if i == selected {
				prefix = " >"
			}
			fmt.Printf("%s %s\n", prefix, option)
		}
		fmt.Printf("Select [1-%d, Enter=default]: ", len(options))
		input := strings.TrimSpace(readLine(reader))
		if input == "" {
			return selected, true
		}
		if n, err := parseIndexSelection(input); err == nil {
			if n < 0 || n >= len(options) {
				fmt.Println("Invalid selection.")
				continue
			}
			return n, true
		}
		fmt.Println("Invalid selection.")
	}
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(strings.TrimSuffix(line, "\r\n"))
}

func parseIndexSelection(input string) (int, error) {
	value, err := strconv.Atoi(input)
	if err != nil {
		return 0, err
	}
	return value - 1, nil
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  aetox [flags] [goal...]")
	fmt.Println("  aetox chat \"goal\"       run one shot and exit")
	fmt.Println("  aetox                    interactive mode")
	fmt.Println("  aetox help               show this help")
	fmt.Println("Flags:")
	fmt.Println("  --model-provider: openrouter|ollama")
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
