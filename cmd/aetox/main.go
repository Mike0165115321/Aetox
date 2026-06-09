package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"aetox-cli/internal/app"
	"aetox-cli/internal/cognitive"
	"aetox-cli/internal/command"
	"aetox-cli/internal/config"
	"aetox-cli/internal/model"
	"aetox-cli/internal/safety"
	"aetox-cli/internal/skill"
	"aetox-cli/internal/think"

	"golang.org/x/term"
)

const appVersion = "0.3.0-dev"
const defaultAgentMaxToolCalls = 4

var (
	noBanner        bool
	showVersion     bool
	showHelp        bool
	legacyYes       bool
	approvalMode    string
)

func parseModelWithThink(raw string) (string, string, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", "", false
	}

	openIdx := strings.LastIndex(value, "(")
	closeIdx := strings.LastIndex(value, ")")
	if openIdx < 0 || closeIdx < 0 || closeIdx != len(value)-1 || closeIdx <= openIdx+1 {
		return value, "", false
	}

	inner := strings.TrimSpace(value[openIdx+1 : closeIdx])
	modelName := strings.TrimSpace(value[:openIdx])
	if modelName == "" || inner == "" {
		return value, "", false
	}

	normalized, err := think.ParseLevel(inner)
	if err != nil {
		return value, "", false
	}

	return modelName, string(normalized), true
}

func main() {
	setUTF8Console()
	providerUsageHint := "model provider (" + strings.Join(model.SupportedProviders(), "|") + ")"

	var rootPath string
	var approvalTimeout int
	var modelProvider string
	var modelName string
	var modelAPIKey string
	var modelBaseURL string
	var modelTimeout int
	var modelContextTokens int
	var thinkLevel string

	flag.StringVar(&rootPath, "root", "", "optional sandbox root directory (default: current directory)")
	flag.IntVar(&approvalTimeout, "approval-timeout", 60, "reserved for future approval controls")
	flag.StringVar(&modelProvider, "model-provider", "", providerUsageHint)
	flag.StringVar(&modelName, "model-name", "", "model name or model(think-level)")
	flag.StringVar(&modelAPIKey, "model-api-key", "", "model API key; fallback to provider env when empty")
	flag.StringVar(&modelBaseURL, "model-base-url", "", "override base URL for model provider")
	flag.IntVar(&modelTimeout, "model-timeout", 30, "model request timeout in seconds")
	flag.IntVar(&modelContextTokens, "model-context-tokens", 0, "model context window token cap (0=auto/unknown)")
	flag.StringVar(&thinkLevel, "think", "", "thinking level (model/provider specific; deepseek: off-think|high|max)")
	flag.BoolVar(&noBanner, "no-banner", false, "disable startup banner in interactive mode")
	flag.BoolVar(&showVersion, "version", false, "print version")
	flag.BoolVar(&showHelp, "help", false, "print usage")
	flag.BoolVar(&legacyYes, "yes", false, "reserved compatibility flag")
	flag.StringVar(&approvalMode, "approval", "", "approval mode: ask, unsafe-only, or full-access (default: ask)")
	argsWithoutGlobal, argsForIntent, preParseErr := preparseGlobalFlags(os.Args[1:])
	if preParseErr != nil {
		fmt.Fprintf(os.Stderr, "invalid flags: %v\n", preParseErr)
		os.Exit(2)
	}

	preParser := flag.NewFlagSet("aetox", flag.ContinueOnError)
	preParser.SetOutput(io.Discard)
	preParser.StringVar(&rootPath, "root", "", "optional sandbox root directory (default: current directory)")
	preParser.IntVar(&approvalTimeout, "approval-timeout", 60, "reserved for future approval controls")
	preParser.StringVar(&modelProvider, "model-provider", "", providerUsageHint)
	preParser.StringVar(&modelName, "model-name", "", "model name or model(think-level)")
	preParser.StringVar(&modelAPIKey, "model-api-key", "", "model API key; fallback to provider env when empty")
	preParser.StringVar(&modelBaseURL, "model-base-url", "", "override base URL for model provider")
	preParser.IntVar(&modelTimeout, "model-timeout", 30, "model request timeout in seconds")
	preParser.IntVar(&modelContextTokens, "model-context-tokens", 0, "model context window token cap (0=auto/unknown)")
	preParser.StringVar(&thinkLevel, "think", "", "thinking level (model/provider specific; deepseek: off-think|high|max)")
	preParser.BoolVar(&noBanner, "no-banner", false, "disable startup banner in interactive mode")
	preParser.BoolVar(&showVersion, "version", false, "print version")
	preParser.BoolVar(&showHelp, "help", false, "print usage")
	preParser.BoolVar(&legacyYes, "yes", false, "reserved compatibility flag")
	preParser.StringVar(&approvalMode, "approval", "", "approval mode: ask, unsafe-only, or full-access (default: ask)")
	_ = preParser.Bool("h", false, "help alias")
	_ = preParser.Bool("v", false, "version alias")
	_ = preParser.Parse(argsWithoutGlobal)

	providerExplicit := strings.TrimSpace(modelProvider) != ""
	modelNameExplicit := strings.TrimSpace(modelName) != ""
	baseURLExplicit := strings.TrimSpace(modelBaseURL) != ""
	thinkLevelExplicit := strings.TrimSpace(thinkLevel) != ""
	explicitModelConfig := providerExplicit || modelNameExplicit || baseURLExplicit
	if thinkLevelExplicit {
		parsedThinkLevel, err := think.ParseLevel(thinkLevel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid flags: %v\n", err)
			os.Exit(2)
		}
		thinkLevel = string(parsedThinkLevel)
	}
	modelNameFromFlag, parsedThinkLevel, modelNameHasThink := parseModelWithThink(modelName)
	if modelNameHasThink && !thinkLevelExplicit {
		modelName = modelNameFromFlag
		thinkLevel = parsedThinkLevel
	}

	if showVersion {
		fmt.Printf("aetox version %s\n", appVersion)
		return
	}
	if showHelp {
		printUsage()
		return
	}

	intent := command.ParseArgs(argsForIntent)
	cfg := config.Load(config.ConfigOptions{
		RootPath:           rootPath,
		AutoApprove:        legacyYes,
		ApprovalMode:       resolveInitialApprovalMode(approvalMode, legacyYes),
		MaxRetries:         2,
		MaxPlanRetries:     0,
		ApprovalTimeout:    approvalTimeout,
		ModelProvider:      modelProvider,
		ModelName:          modelName,
		ModelAPIKey:        modelAPIKey,
		ModelBaseURL:       modelBaseURL,
		ModelTimeout:       modelTimeout,
		ModelContextTokens: modelContextTokens,
		ThinkLevel:         thinkLevel,
	})

	modelProvider = cfg.ModelProvider
	modelName = cfg.ModelName
	modelAPIKey = cfg.ModelAPIKey
	modelBaseURL = cfg.ModelBaseURL
	modelContextTokens = cfg.ModelContextTokens
	thinkLevel = cfg.ThinkLevel

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
			if key := storedPreference.APIKeyForProvider(modelProvider); key != "" {
				modelAPIKey = key
			}
		}
	}
	if !thinkLevelExplicit && !modelNameHasThink && hasStoredPreference && strings.TrimSpace(storedPreference.ThinkLevel) != "" {
		thinkLevel = string(think.NormalizeLevel(storedPreference.ThinkLevel))
		cfg.ThinkLevel = thinkLevel
	}

	approvalExplicit := strings.TrimSpace(approvalMode) != ""
	if !approvalExplicit && !legacyYes && hasStoredPreference && strings.TrimSpace(storedPreference.ApprovalMode) != "" {
		cfg.ApprovalMode = string(safety.NormalizeApprovalMode(storedPreference.ApprovalMode))
	}

	currentConfig := cfg

	if intent.Mode == command.ModeInteractive && isInteractive() && !explicitModelConfig && !hasStoredPreference {
		selectedProvider, selectedModel, selectedAPIKey, selectedBaseURL, selectedThinkLevel, ok := promptModelSelection(cfg, !thinkLevelExplicit)
		if ok {
			modelProvider = selectedProvider
			modelName = selectedModel
			modelAPIKey = selectedAPIKey
			modelBaseURL = selectedBaseURL
			if !thinkLevelExplicit {
				thinkLevel = selectedThinkLevel
			}
			cfg.ModelProvider = selectedProvider
			cfg.ModelName = selectedModel
			cfg.ModelAPIKey = selectedAPIKey
			cfg.ModelBaseURL = selectedBaseURL
			if !thinkLevelExplicit {
				cfg.ThinkLevel = selectedThinkLevel
			}
			currentConfig = cfg
			if saveErr := persistModelPreference(currentConfig); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot save model preference: %v\n", saveErr)
			}
		}
	}

	cfg.ModelProvider = strings.TrimSpace(modelProvider)
	cfg.ModelName = strings.TrimSpace(modelName)
	cfg.ModelAPIKey = strings.TrimSpace(modelAPIKey)
	if cfg.ModelAPIKey == "" {
		cfg.ModelAPIKey = model.ResolveModelAPIKey(cfg.ModelProvider)
	}
	cfg.ModelBaseURL = strings.TrimSpace(modelBaseURL)
	cfg.ModelContextTokens = modelContextTokens

	if strings.TrimSpace(cfg.ModelName) == "" &&
		!strings.EqualFold(strings.TrimSpace(cfg.ModelProvider), "noop") {
		cfg.ModelName = model.DefaultModel(cfg.ModelProvider)
		modelName = cfg.ModelName
	}
	cfg.ThinkLevel = model.NormalizeThinkingLevel(cfg.ModelProvider, cfg.ModelName, thinkLevel)

	currentConfig = cfg
	bootstrapResult, _ := bootstrapModelWithStatus(cfg)

	effectiveApprovalMode := safety.ApprovalMode(cfg.ApprovalMode)
	if intent.Mode == command.ModeOnce {
		effectiveApprovalMode = safety.ApprovalFullAccess
	}
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
		MaxToolCalls: defaultAgentMaxToolCalls,
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
		ApprovalMode: effectiveApprovalMode,
		OnApprovalChange: func(mode safety.ApprovalMode) {
			currentConfig.ApprovalMode = string(mode)
			if saveErr := persistModelPreference(currentConfig); saveErr != nil {
				fmt.Fprintf(os.Stderr, "warning: cannot save approval mode: %v\n", saveErr)
			}
		},
		Title:       "Aetox CLI",
		Version:     appVersion,
		UserInfo:    resolveDisplayUser(),
		ModelStatus: resolveModelStatus(config.Config{
			ModelProvider: modelProvider,
			ModelName:     currentConfig.ModelName,
			ThinkLevel:    currentConfig.ThinkLevel,
		}, bootstrapResult),
		ModelContextTokens: currentConfig.ModelContextTokens,
		ThinkLevel:         think.Level(currentConfig.ThinkLevel),
		ModelSwitch: func(ctx context.Context) (app.ModelSwitchResult, error) {
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

func resolveInitialApprovalMode(flagValue string, legacyYes bool) string {
	if strings.TrimSpace(flagValue) != "" {
		return string(safety.NormalizeApprovalMode(flagValue))
	}
	if legacyYes {
		return string(safety.ApprovalFullAccess)
	}
	return string(safety.ApprovalAsk)
}

func switchProvider(ctx context.Context, cfg *config.Config) (app.ModelSwitchResult, error) {
	if ctx == nil {
		return app.ModelSwitchResult{}, nil
	}

	select {
	case <-ctx.Done():
		return app.ModelSwitchResult{}, ctx.Err()
	default:
	}

	selectedProvider, selectedModel, selectedAPIKey, selectedBaseURL, selectedThinkLevel, ok := promptModelSelection(*cfg, true)
	if !ok {
		return app.ModelSwitchResult{}, nil
	}

	cfg.ModelProvider = strings.TrimSpace(selectedProvider)
	cfg.ModelName = strings.TrimSpace(selectedModel)
	cfg.ModelAPIKey = strings.TrimSpace(selectedAPIKey)
	cfg.ModelBaseURL = strings.TrimSpace(selectedBaseURL)
	cfg.ThinkLevel = selectedThinkLevel

	if cfg.ModelName == "" && !strings.EqualFold(cfg.ModelProvider, "noop") {
		cfg.ModelName = model.DefaultModel(cfg.ModelProvider)
	}
	cfg.ThinkLevel = model.NormalizeThinkingLevel(cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel)

	fmt.Printf("เปลี่ยนโมเดลเป็น: %s...\n", formatModelModeLabel(cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel))
	bootstrapResult, modelStatus := bootstrapModelWithStatus(*cfg)
	if bootstrapResult.Provider == nil {
		return app.ModelSwitchResult{}, bootstrapResult.Error
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

	return app.ModelSwitchResult{
		Agent: cognitive.NewAgent(cognitive.AgentConfig{
			Provider:     bootstrapResult.Provider,
			Model:        cfg.ModelName,
			SystemPrompt: buildSystemPrompt(cfg.SandboxRoot),
		}),
		ModelStatus:        modelStatus,
		ModelContextTokens: cfg.ModelContextTokens,
		ThinkLevel:         think.Level(cfg.ThinkLevel),
		Changed:            true,
	}, nil
}

func buildSystemPrompt(root string) string {
	sandboxRoot := strings.TrimSpace(root)
	if sandboxRoot == "" {
		sandboxRoot = "(unknown)"
	}
	return "You are Aetox, a concise assistant in Thai and English " +
		"that helps users through a terminal conversation.\n" +
		"Current working sandbox root is: " + sandboxRoot + ".\n" +
		"Do NOT proactively mention or leak this path to the user in general greetings or unrelated conversation " +
		"unless they explicitly ask about files, directories, paths, or workspace locations."
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

func formatModelModeLabel(providerName, modelName, thinkLevel string) string {
	status := model.ResolveStatus(providerName, modelName, nil)
	return fmt.Sprintf("%s(%s)", status, defaultThinkLevel(providerName, modelName, thinkLevel))
}

func resolveModelStatus(cfg config.Config, bootstrapResult model.BootstrapResult) string {
	_ = bootstrapResult
	return formatModelModeLabel(cfg.ModelProvider, cfg.ModelName, cfg.ThinkLevel)
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
	canonicalProvider := model.NormalizeProvider(provider)
	storedPreference, hasStoredPreference, _ := config.LoadModelPreference()
	if hasStoredPreference {
		storedPreference = normalizeProviderPreference(storedPreference)
	} else {
		storedPreference = config.ModelPreference{}
	}
	if strings.TrimSpace(cfg.ModelAPIKey) != "" {
		storedPreference.SetAPIKeyForProvider(canonicalProvider, cfg.ModelAPIKey)
	}
	pref := config.ModelPreference{
		ModelProvider: canonicalProvider,
		ModelName:     strings.TrimSpace(cfg.ModelName),
		ModelBaseURL:  strings.TrimSpace(cfg.ModelBaseURL),
		ThinkLevel:    model.NormalizeThinkingLevel(canonicalProvider, strings.TrimSpace(cfg.ModelName), cfg.ThinkLevel),
		ApprovalMode:  string(safety.NormalizeApprovalMode(cfg.ApprovalMode)),
		ModelAPIKeys:  storedPreference.ModelAPIKeys,
	}
	if len(pref.ModelAPIKeys) == 0 {
		pref.ModelAPIKeys = nil
	}
	return config.SaveModelPreference(pref)
}

func normalizeProviderPreference(pref config.ModelPreference) config.ModelPreference {
	normalized := config.ModelPreference{
		ModelProvider: strings.TrimSpace(pref.ModelProvider),
		ModelName:     strings.TrimSpace(pref.ModelName),
		ModelBaseURL:  strings.TrimSpace(pref.ModelBaseURL),
		ThinkLevel:    string(think.NormalizeLevel(pref.ThinkLevel)),
	}
	for _, key := range model.SupportedProviders() {
		modelName := key
		if value := pref.APIKeyForProvider(key); value != "" {
			normalized.SetAPIKeyForProvider(modelName, value)
		}
	}
	return normalized
}

func promptModelSelection(cfg config.Config, askThinkLevel bool) (string, string, string, string, string, bool) {
	reader := bufio.NewReader(os.Stdin)
	storedPreference, hasStoredPreference, prefErr := config.LoadModelPreference()
	if prefErr != nil {
		fmt.Fprintf(os.Stderr, "warning: cannot read model preference: %v\n", prefErr)
	}

	providers := model.SupportedProviders()
	providerOptions := make([]string, 0, len(providers))
	for _, p := range providers {
		label := p
		if model.RequiresAPIKey(p) {
			keyFound := model.ResolveModelAPIKey(p) != "" || (hasStoredPreference && storedPreference.APIKeyForProvider(p) != "")
			label = model.FormatProviderMenuLabel(p, keyFound)
		}
		providerOptions = append(providerOptions, label)
	}

	for {
		idx, ok := pickFromMenu(reader, "No model provider configured. Select one.", providerOptions, 0, "Use ↑/↓ then Enter.")
		if !ok {
			defaultProvider := providers[0]
			defaultModel := model.DefaultModel(defaultProvider)
			return defaultProvider, defaultModel, "", cfg.ModelBaseURL, defaultThinkLevel(defaultProvider, defaultModel, cfg.ThinkLevel), false
		}
		provider := providers[idx]
		providerBaseURL := strings.TrimSpace(cfg.ModelBaseURL)
		if providerBaseURL == "" {
			providerBaseURL = model.DefaultBaseURL(provider)
		}

		key := strings.TrimSpace(storedPreference.APIKeyForProvider(provider))
		if key == "" && strings.EqualFold(cfg.ModelProvider, provider) {
			key = strings.TrimSpace(cfg.ModelAPIKey)
		}
		if key == "" {
			key = strings.TrimSpace(model.ResolveModelAPIKey(provider))
		}

		if model.RequiresAPIKey(provider) {
			if key == "" {
				if hasStoredPreference {
					fmt.Printf("No cached API key for %s.\n", provider)
				}
				for {
					fmt.Printf("API key for %s: ", provider)
					key = strings.TrimSpace(readLine(reader))
					if key != "" {
						break
					}
					fmt.Println("Missing API key. Try again.")
				}
			} else {
				fmt.Printf("Use existing API key for %s.\n", provider)
			}
		}

		selectedModel := pickModelForProvider(reader, provider, cfg.ModelName, providerBaseURL, key)
		selectedModel, selectedThinkLevel, parsedModelThink := parseModelWithThink(selectedModel)
		if !parsedModelThink {
			selectedThinkLevel = defaultThinkLevel(provider, selectedModel, cfg.ThinkLevel)
			if askThinkLevel {
				selectedThinkLevel = promptThinkLevelSelection(reader, provider, selectedModel, cfg.ThinkLevel)
			}
		}

		fmt.Printf("Selected: %s\n\n", formatModelModeLabel(provider, selectedModel, selectedThinkLevel))

		return provider, selectedModel, key, providerBaseURL, selectedThinkLevel, true
	}
}

func defaultThinkLevel(provider, modelName, existing string) string {
	return model.NormalizeThinkingLevel(provider, modelName, existing)
}

func promptThinkLevelSelection(reader *bufio.Reader, provider, modelName, existing string) string {
	defaultLevel := defaultThinkLevel(provider, modelName, existing)
	if reader == nil {
		return defaultLevel
	}

	options := model.SupportedThinkingLevels(provider, modelName)
	if len(options) == 0 {
		return defaultLevel
	}
	defaultIndex := 0
	for i, option := range options {
		if option == defaultLevel {
			defaultIndex = i
			break
		}
	}

	idx, ok := pickFromMenu(reader, "Choose thinking level", options, defaultIndex, "Use ↑/↓ then Enter.")
	if !ok {
		return defaultLevel
	}
	return options[idx]
}

func pickModelForProvider(reader *bufio.Reader, provider, existing, baseURL, apiKey string) string {
	modelChoices, err := model.ModelChoicesWithEndpointAndAPIKey(provider, baseURL, apiKey)
	if err != nil || len(modelChoices) == 0 {
		if err != nil && strings.TrimSpace(apiKey) != "" {
			fmt.Printf("โหลดรายชื่อโมเดลจาก API ของ %s ไม่ได้ (%v)\n", provider, err)
			fmt.Println("กำลังใช้รายชื่อสำรองจากค่าสำรอง")
		}
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
	// If current model is not in advertised list, keep it as a selectable default.
	if defaultModel != "" {
		foundDefault := false
		for _, m := range options {
			if m == defaultModel {
				foundDefault = true
				break
			}
		}
		if !foundDefault {
			options = append([]string{defaultModel}, options...)
		}
	}
	options = append(options, "custom model ...")
	defaultIndex := 0
	for i, m := range options {
		if i >= len(options)-1 {
			break
		}
		if m == defaultModel {
			defaultIndex = i
			break
		}
	}

	idx, ok := pickFromMenu(reader, fmt.Sprintf("Choose model for %s", provider), options, defaultIndex, "Use ↑/↓ then Enter.")
	if !ok {
		return defaultModel
	}

	if idx == len(options)-1 {
		fmt.Printf("Model name for %s [%s]: ", provider, defaultModel)
		if model := strings.TrimSpace(readLine(reader)); model != "" {
			return model
		}
		return defaultModel
	}

	return options[idx]
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
	fmt.Printf("  --model-provider: %s\n", strings.Join(model.SupportedProviders(), "|"))
	fmt.Println("  --model-name <model[(think-level)]> optional; provider defaults are auto-selected when omitted")
	fmt.Println("  --model-api-key <key>        fallback: provider env (OPENAI_API_KEY, DEEPSEEK_API_KEY, GROQ_API_KEY, etc.)")
	fmt.Println("  --model-context-tokens <n>   override context window display (0=auto/unknown)")
	fmt.Println("  --think <level>              model/provider specific thinking level (DeepSeek: off-think|high|max)")
	fmt.Println("  --no-banner                 disable interactive banner")
	fmt.Println("  --approval <mode>           approval mode: ask, unsafe-only, full-access (default: ask)")
	fmt.Println("  --yes                       auto-approve safety prompts (legacy, prefer --approval full-access)")
	fmt.Println("  --version                   print version")
}

func isInteractive() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func preparseGlobalFlags(rawArgs []string) ([]string, []string, error) {
	global := make([]string, 0, len(rawArgs))
	remaining := make([]string, 0, len(rawArgs))

	isValueFlag := func(arg string) bool {
		switch arg {
		case "--root", "--approval-timeout", "--model-provider", "--model-name", "--model-api-key", "--model-base-url", "--model-timeout", "--model-context-tokens", "--think", "--approval":
			return true
		}
		return false
	}

	isBoolFlag := func(arg string) bool {
		switch arg {
		case "--yes", "--no-banner", "--version", "--help", "-v", "-h":
			return true
		}
		return false
	}

	for idx := 0; idx < len(rawArgs); idx++ {
		raw := strings.TrimSpace(rawArgs[idx])
		if raw == "--" {
			remaining = append(remaining, raw)
			if idx+1 < len(rawArgs) {
				remaining = append(remaining, rawArgs[idx+1:]...)
			}
			break
		}

		if !strings.HasPrefix(raw, "--") && !(raw == "-h" || raw == "-v") {
			remaining = append(remaining, raw)
			continue
		}

		if strings.Contains(raw, "=") {
			nameValue := strings.SplitN(raw, "=", 2)
			name := strings.ToLower(strings.TrimSpace(nameValue[0]))
			value := ""
			if len(nameValue) > 1 {
				value = nameValue[1]
			}
			if isValueFlag(name) {
				global = append(global, name, value)
				continue
			}
			if isBoolFlag(name) {
				global = append(global, name)
				continue
			}
			remaining = append(remaining, raw)
			continue
		}

		if isBoolFlag(raw) {
			global = append(global, raw)
			continue
		}

		if isValueFlag(raw) {
			if idx+1 >= len(rawArgs) {
				return nil, nil, fmt.Errorf("flag %s requires a value", raw)
			}
			global = append(global, raw, rawArgs[idx+1])
			idx++
			continue
		}

		remaining = append(remaining, raw)
	}

	return global, remaining, nil
}
