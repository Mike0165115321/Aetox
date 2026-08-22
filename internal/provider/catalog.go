// Package provider holds metadata, aliases, capabilities, and
// validation helpers for every supported model provider.
//
// This package MUST NOT make HTTP requests. It is a registry (ทะเบียนบ้าน),
// not a client (คนออกไปทำงานจริง).
//
// Hardcoded model names and capability flags in this package are
// provider-level fallbacks only. They do not represent guaranteed
// capabilities for every model behind a provider. Final model-selection
// and capability checks should eventually move to a model-level catalog
// or runtime discovery layer (internal/model is responsible for calling
// provider APIs to fetch live model lists).
package provider

import (
	"os"
	"sort"
	"strings"
)

// Runtime classifies how a provider is invoked.
type Runtime string

const (
	RuntimeNoop             Runtime = "noop"
	RuntimeOpenAICompatible Runtime = "openai-compatible"
	RuntimeOllama           Runtime = "ollama"
	RuntimeAnthropic        Runtime = "anthropic"
	// RuntimeResponses is the OpenAI Responses API — typed input items and a
	// dozen named SSE event types, sharing almost nothing with
	// RuntimeOpenAICompatible beyond the company that publishes both. It is
	// what a ChatGPT subscription speaks and the only thing that endpoint
	// serves.
	RuntimeResponses Runtime = "responses"
)

// ModelDefaults holds the static fallback model names for a provider.
// These are used ONLY when no model name has been configured and no
// live model list could be fetched. They are not authoritative.
type ModelDefaults struct {
	// FallbackModel is the model to use when nothing else is configured.
	// This is a best-effort default; it may become unavailable.
	FallbackModel string

	// RecommendedModels is an optional short list of well-known models
	// that work well with this provider. It is a discovery hint, not
	// a complete catalog.
	RecommendedModels []string
}

// Capabilities records provider-level or runtime-level feature support.
//
// These flags represent what the provider API surface can express, NOT
// what every specific model behind that provider guarantees. Model-level
// capability checks belong in a future model-level catalog or runtime
// discovery layer.
type Capabilities struct {
	// ToolCalling reports whether this provider supports the
	// tools / tool_calls API protocol.
	ToolCalling bool

	// Reasoning reports whether this provider supports a dedicated
	// reasoning / thinking effort knob (e.g. OpenRouter).
	Reasoning bool
}

// BalanceKind says what a provider can tell us about money, so the UI never
// has to guess why a number is missing. It describes the *account*, not one
// request — reading the actual figure is internal/model's job.
type BalanceKind string

const (
	// BalanceMoney: the provider serves a balance endpoint our own API key
	// can read.
	BalanceMoney BalanceKind = "money"
	// BalanceFree: runs on this machine, so there is nothing to spend.
	BalanceFree BalanceKind = "free"
	// BalanceSubscription: paid for by a plan rather than a wallet. There is
	// no figure to show — only a quota, if QuotaSource says where.
	BalanceSubscription BalanceKind = "subscription"
	// BalanceWebOnly: a real prepaid account whose figure is only readable
	// from the provider's own dashboard, not with a chat API key.
	BalanceWebOnly BalanceKind = "web-only"
)

// QuotaSource names the dialect a provider states its remaining rate-limit
// window in. Unlike a balance, a quota is not fetched on demand: it rides
// along on the responses of real turns, so this only says how to read it.
type QuotaSource string

const (
	// QuotaNone: pay-as-you-go with no window worth showing.
	QuotaNone QuotaSource = ""
	// QuotaCodex: the x-codex-* header family on the ChatGPT backend, which
	// carries both a 5-hour and a weekly window. Undocumented — read it
	// opportunistically and show nothing when it stops appearing.
	QuotaCodex QuotaSource = "codex"
	// QuotaAnthropic: anthropic-ratelimit-* headers.
	QuotaAnthropic QuotaSource = "anthropic"
	// QuotaOpenAIStd: the x-ratelimit-remaining-{requests,tokens} family most
	// OpenAI-compatible hosts send. Set wherever the runtime could carry it —
	// a host that stays silent simply reports no quota, which the UI shows as
	// "this provider does not report one" rather than a wrong number.
	QuotaOpenAIStd QuotaSource = "openai-std"
	// QuotaOpenRouter: returned as JSON by GET /key alongside the credits,
	// so this is the one quota that can be fetched on demand.
	QuotaOpenRouter QuotaSource = "openrouter"
)

// Spec is the canonical metadata for one supported provider.
type Spec struct {
	Canonical      string
	Aliases        []string
	RequiresAPIKey bool
	Runtime        Runtime
	BaseURL        string
	// AltRuntime and AltBaseURL describe a second wire format the same
	// provider account can speak (e.g. DeepSeek exposes both an
	// OpenAI-compatible endpoint and an Anthropic Messages-format endpoint
	// for the same models). Empty when the provider has only one format.
	// The user picks between them (Settings.ProviderWireFormats /
	// SetProviderWireFormat) instead of Aetox guessing.
	AltRuntime    Runtime
	AltBaseURL    string
	EnvKeys       []string
	ModelDefaults ModelDefaults
	Capabilities  Capabilities
	BalanceKind   BalanceKind
	QuotaSource   QuotaSource
	// APIKeyURL is the page on the provider's own site where a user creates the
	// key this row asks them to paste. Empty when there is nothing to link to —
	// a local runtime with no account, or a sign-in provider where a pasted key
	// is not a credential that exists (see AcceptsAPIKey).
	//
	// It lives here rather than in the UI because it is a fact about the
	// provider, and the UI already reads every other such fact from this
	// registry. A second table in TypeScript would be a second place answering
	// the same question.
	APIKeyURL string
	// AcceptsAPIKey is false for a provider where a pasted key is not a
	// credential that exists. It is not the opposite of RequiresAPIKey:
	// Codex requires credentials and has no API key to give, because it is
	// reached at chatgpt.com on a subscription while the key a user would
	// paste belongs to api.openai.com — a different host, a different bill,
	// and a guaranteed 401. Offering the field there is offering a dead end.
	AcceptsAPIKey bool
}

// ---------------------------------------------------------------------------
// Catalog
// ---------------------------------------------------------------------------

type entry struct {
	canonical      string
	aliases        []string
	requiresAPIKey bool
	runtime        Runtime
	baseURL        string
	altRuntime     Runtime
	altBaseURL     string
	envKeys        []string
	modelDefaults  ModelDefaults
	capabilities   Capabilities
	balanceKind    BalanceKind
	quotaSource    QuotaSource
	apiKeyURL      string
	// signInOnly is the negative form so that the zero value — every other
	// provider — means "a pasted key works here", which is the common case.
	signInOnly bool
}

// The provider catalog is the single source of truth for provider
// metadata. Model names inside ModelDefaults are static fallbacks
// only — they are not guaranteed to be live, available, or appropriate
// for every model behind the provider.
var catalog = map[string]*entry{
	"aetox": {
		canonical:   "aetox",
		balanceKind: BalanceFree,
		quotaSource: QuotaNone,
		// "noop" stays a recognized alias so preference files saved before
		// this rename (config.ModelPreference.ModelProvider/EnabledProviders
		// literally containing "noop") keep normalizing correctly — the same
		// mechanism every other provider's aliases already use, no migration
		// step needed.
		aliases:        []string{"aetox", "noop", "none", "stub"},
		requiresAPIKey: false,
		runtime:        RuntimeNoop,
		baseURL:        "",
		envKeys:        nil,
		modelDefaults: ModelDefaults{
			FallbackModel: "aetox-grid",
			// Each test model exercises one path without an API key: plain
			// echo, the whole chat renderer (markdown and images — one
			// renderer, so one bench), the live thinking panel, the
			// tool-driven UI, and delegation end to end (start a sub-agent, it
			// asks, answer it, collect). Behavior switches on the model name in
			// noop.go, which still answers to the two names the render model
			// had when it was two.
			//
			// `aetox-tools:test` also carries the opt-in briefs: a message
			// containing `memory` proposes something to remember (§82), and one
			// containing `subagent` delegates — keywords rather than models,
			// because a picker nobody reads to the bottom is not a bench.
			RecommendedModels: []string{
				"aetox-grid",
				"aetox-render:test",
				"aetox-think:test",
				"aetox-tools:test",
				"aetox-subagent:test",
				// The newest bench, and the one that needs a bench most: a
				// declared run is six delegates under three phases, and the
				// thing being tested is the CARD — grouping, a wave of three
				// starting at the same instant, and a phase drawn at 0 before
				// anyone has worked in it. None of that is visible from a unit
				// test, and reproducing it on a real provider costs an API key
				// and a prompt that happens to make the model fan out.
				"aetox-run:test",
			},
		},
		capabilities: Capabilities{},
	},
	"openrouter": {
		canonical:      "openrouter",
		balanceKind:    BalanceMoney,
		quotaSource:    QuotaOpenRouter,
		aliases:        []string{"openrouter", "open-router", "openrouterai", "or"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://openrouter.ai/api/v1",
		envKeys:        []string{"OPENROUTER_API_KEY"},
		apiKeyURL:      "https://openrouter.ai/keys",
		modelDefaults: ModelDefaults{
			FallbackModel: "deepseek/deepseek-r1",
		},
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"openai": {
		canonical:      "openai",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"openai", "chatgpt", "gpt", "openai-compatible", "compatible"},
		requiresAPIKey: true,
		// /chat/completions stays the default — OpenAI's own model pages list
		// it alongside /responses for the ordinary line (gpt-5.6 sol, luna,
		// terra, 5.5), and it is the format this row has always spoken, which
		// matters because the same row is what a third-party OpenAI-compatible
		// endpoint gets pointed at.
		//
		// /responses is the alt because a whole family is served there and
		// nowhere else: the codex models (gpt-5-codex, 5.1/5.3-codex), the Pro
		// tiers (5.4-pro, 5.5-pro) and 5.6-cyber. On /chat/completions those
		// answer 400 rather than a reply, and before this row had a second
		// format there was no way to reach them with an API key at all — only
		// through a ChatGPT sign-in, which is a different account and a
		// different bill (see "codex" below).
		//
		// Same host either way, so the URL does not change with the format.
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.openai.com/v1",
		altRuntime:     RuntimeResponses,
		altBaseURL:     "https://api.openai.com/v1",
		envKeys:        []string{"OPENAI_API_KEY", "OPENAI_TOKEN"},
		apiKeyURL:      "https://platform.openai.com/api-keys",
		modelDefaults:  ModelDefaults{FallbackModel: "gpt-4o-mini"},
		capabilities:   Capabilities{ToolCalling: true, Reasoning: true},
	},
	// codex is a ChatGPT *subscription*, reached at chatgpt.com rather than
	// api.openai.com and paid for by the user's plan rather than per token —
	// the same quota the Codex CLI spends.
	//
	// Kept separate from "openai" instead of being a wire-format alternative on
	// it: different host, different credentials, different billing, different
	// model list. The "chatgpt" alias deliberately stays on "openai" so nobody's
	// saved preference silently changes meaning.
	"codex": {
		canonical:      "codex",
		balanceKind:    BalanceSubscription,
		quotaSource:    QuotaCodex,
		signInOnly:     true,
		aliases:        []string{"codex", "chatgpt-codex", "chatgpt-subscription", "openai-codex"},
		requiresAPIKey: true,
		runtime:        RuntimeResponses,
		baseURL:        "https://chatgpt.com/backend-api/codex",
		envKeys:        nil,
		// DiscoverResponsesModels answers this per account and per plan; the
		// name below is only what to try before anyone has signed in.
		modelDefaults: ModelDefaults{FallbackModel: "gpt-5.5"},
		capabilities:  Capabilities{ToolCalling: true, Reasoning: true},
	},
	// API key only since v0.8.1 (§65): the qwen-code device flow that used to
	// stand behind this entry never completed a sign-in and is gone. DashScope's
	// OpenAI-compatible endpoint is the only path left, so it is a fixed base
	// URL again rather than one the login hands back.
	"qwen": {
		canonical:      "qwen",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"qwen", "qwen-code", "dashscope", "tongyi"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://dashscope.aliyuncs.com/compatible-mode/v1",
		envKeys:        []string{"DASHSCOPE_API_KEY", "QWEN_API_KEY"},
		apiKeyURL:      "https://bailian.console.alibabacloud.com/?tab=model#/api-key",
		modelDefaults:  ModelDefaults{FallbackModel: "qwen3-coder-plus"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"deepseek": {
		canonical:      "deepseek",
		balanceKind:    BalanceMoney,
		quotaSource:    QuotaNone,
		aliases:        []string{"deepseek", "deepseek-api", "deepseek-ai"},
		requiresAPIKey: true,
		// Default to DeepSeek's Anthropic-format endpoint (/anthropic): it
		// returns structured tool_use + native thinking blocks, which the
		// Anthropic runtime already streams cleanly — sidestepping the DSML
		// tool-call leak the OpenAI-compatible path had to parse out of plain
		// text. The Anthropic SDK appends /v1/messages, so the base URL
		// includes /anthropic/v1. The plain OpenAI-compatible endpoint is kept
		// as the alt format (user-selectable in Settings) since it's the
		// longer-proven path and some routing setups may still prefer it.
		runtime:       RuntimeAnthropic,
		baseURL:       "https://api.deepseek.com/anthropic/v1",
		altRuntime:    RuntimeOpenAICompatible,
		altBaseURL:    "https://api.deepseek.com",
		envKeys:       []string{"DEEPSEEK_API_KEY"},
		apiKeyURL:     "https://platform.deepseek.com/api_keys",
		modelDefaults: ModelDefaults{FallbackModel: "deepseek-v4-flash"},
		capabilities:  Capabilities{ToolCalling: true, Reasoning: true},
	},
	"minimax": {
		canonical:      "minimax",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"minimax", "minimaxi", "minimax-ai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.minimax.io/v1",
		envKeys:        []string{"MINIMAX_API_KEY"},
		apiKeyURL:      "https://platform.minimax.io/user-center/basic-information/interface-key",
		// A last resort for a cold start with no network, not a model list:
		// this provider serves GET /v1/models, so the picker is filled from the
		// account's own catalog (ModelChoicesWithEndpointAndAPIKey) and a name
		// written here would only ever be the offline fallback.
		modelDefaults: ModelDefaults{FallbackModel: "MiniMax-M3"},
		// Thinking is a switch here, not a ladder — there is no effort field at
		// all. See resolveMiniMaxThinkingCapabilities.
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"kimi": {
		canonical:   "kimi",
		balanceKind: BalanceMoney,
		quotaSource: QuotaOpenAIStd,
		// "moonshot" is the company and the API host; "kimi" is the name on the
		// models and on the docs site the platform now redirects to. Both are
		// what a user will type.
		aliases:        []string{"kimi", "moonshot", "moonshotai", "kimi-ai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.moonshot.ai/v1",
		envKeys:        []string{"MOONSHOT_API_KEY", "KIMI_API_KEY"},
		apiKeyURL:      "https://platform.moonshot.ai/console/api-keys",
		// K3 never reached the endpoint this row points at. On 2026-08-20 a
		// real call to /v1/models returned exactly two ids, kimi-k2.6 and
		// kimi-k2.7-code, and models.dev still listed kimi-k3 as current — so
		// the third-party catalog agreed with the stale name and only the
		// provider itself knew. k2.6 is the general model of the two; the
		// -code sibling is not chosen here because nothing has established
		// whether it is a chat model or a completion one.
		//
		// Three tables still branch on a "kimi-k3" prefix (context_window.go,
		// thinking_capabilities.go, vision_capabilities.go). All three fall
		// through to the conservative K2 answer, which is correct for what is
		// served, so they are dead rather than wrong. Updating them needs
		// Moonshot docs for k2.6/k2.7 that nobody has read yet.
		modelDefaults:  ModelDefaults{FallbackModel: "kimi-k2.6"},
		// Reasoning is a real dial here (reasoning_effort: low/high/max) but it
		// has no off position — K3 always thinks. See
		// resolveKimiThinkingCapabilities for what that costs the picker.
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"zai": {
		canonical:      "zai",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"zai", "z.ai", "zhipu", "zhipuai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.z.ai/api/paas/v4",
		envKeys:        []string{"ZAI_API_KEY"},
		apiKeyURL:      "https://z.ai/manage-apikey/apikey-list",
		modelDefaults:  ModelDefaults{FallbackModel: "glm-4.6"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"gemini": {
		canonical:   "gemini",
		balanceKind: BalanceWebOnly,
		// QuotaNone, measured rather than assumed: a real key against the live
		// OpenAI-compat endpoint returns 200 with thirteen headers and not one
		// of them is a rate-limit field (verified 2026-08-14). It was declared
		// QuotaOpenAIStd on the theory that any OpenAI-compatible host might
		// send that family, which made the card promise "the limit appears once
		// you chat" — a promise this endpoint can never keep.
		quotaSource:    QuotaNone,
		aliases:        []string{"gemini", "google", "google-ai", "googleai", "google-gemini"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://generativelanguage.googleapis.com/v1beta/openai",
		envKeys:        []string{"GEMINI_API_KEY", "GOOGLE_API_KEY"},
		apiKeyURL:      "https://aistudio.google.com/apikey",
		modelDefaults: ModelDefaults{
			// gemini-2.5-flash, not -flash-lite: the lite model is still listed
			// by the models endpoint but the chat endpoint answers 404 "no
			// longer available to new users", so it was a first-run failure for
			// anyone picking Gemini today. Verified against the live API.
			// Last resort only — DiscoverGeminiModels answers the picker.
			FallbackModel: "gemini-2.5-flash",
		},
		capabilities: Capabilities{ToolCalling: true, Reasoning: true},
	},
	"groq": {
		canonical:      "groq",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"groq", "groqcloud"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.groq.com/openai/v1",
		envKeys:        []string{"GROQ_API_KEY"},
		apiKeyURL:      "https://console.groq.com/keys",
		// Was llama-3.3-70b-versatile until 2026-08-20, when a live call found
		// Groq no longer serves it, or any Llama chat model: of the 13 ids on
		// the endpoint that day, the rest were Whisper, TTS, prompt-guard
		// classifiers and one Arabic 7B that answers tool calls with a 400.
		// gpt-oss-120b is the one Groq serves that does what Aetox needs on
		// the first turn: tools, and a reasoning dial thinking_capabilities.go
		// already recognises by its openai/gpt-oss- prefix.
		modelDefaults:  ModelDefaults{FallbackModel: "openai/gpt-oss-120b"},
		capabilities:   Capabilities{ToolCalling: true, Reasoning: true},
	},
	"mistral": {
		canonical:      "mistral",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"mistral", "mistralai"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.mistral.ai/v1",
		envKeys:        []string{"MISTRAL_API_KEY"},
		apiKeyURL:      "https://console.mistral.ai/api-keys",
		// "mistral-small" with no suffix is not an id Mistral serves; the
		// self-updating alias is. Same intent, and it cannot rot the way a
		// pinned date can.
		modelDefaults:  ModelDefaults{FallbackModel: "mistral-small-latest"},
		capabilities:   Capabilities{ToolCalling: true},
	},
	"lmstudio": {
		canonical:      "lmstudio",
		balanceKind:    BalanceFree,
		quotaSource:    QuotaNone,
		aliases:        []string{"lmstudio", "localai", "local-ai"},
		requiresAPIKey: false,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "http://localhost:1234/v1",
		envKeys:        []string{"LLM_API_KEY", "OPENAI_API_KEY"},
		// No FallbackModel on purpose — see "ollama" below.
		modelDefaults: ModelDefaults{},
		capabilities:  Capabilities{ToolCalling: true},
	},
	"ollama": {
		canonical:      "ollama",
		balanceKind:    BalanceFree,
		quotaSource:    QuotaNone,
		aliases:        []string{"ollama", "ollamaai"},
		requiresAPIKey: false,
		runtime:        RuntimeOllama,
		baseURL:        "http://localhost:11434",
		envKeys:        nil,
		// No FallbackModel on purpose. A local runtime serves whatever the
		// user pulled, so any name written here is a guess that fails as
		// "model 'x' not found" against a server that is working fine. The
		// empty fallback is the signal to ask the server instead — see
		// model.ResolveDefaultModel.
		modelDefaults: ModelDefaults{},
		capabilities:  Capabilities{ToolCalling: true},
	},
	"anthropic": {
		canonical:      "anthropic",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaAnthropic,
		aliases:        []string{"anthropic", "claude"},
		requiresAPIKey: true,
		runtime:        RuntimeAnthropic,
		baseURL:        "https://api.anthropic.com/v1",
		envKeys:        []string{"ANTHROPIC_API_KEY"},
		apiKeyURL:      "https://console.anthropic.com/settings/keys",
		// No RecommendedModels: GET /v1/models answers this, and any list
		// written here goes stale within months (model.DiscoverAnthropicModels).
		modelDefaults: ModelDefaults{FallbackModel: "claude-haiku-4-5"},
		capabilities:  Capabilities{ToolCalling: true, Reasoning: true},
	},
	// Grok already reaches Aetox through the openrouter row, so this one does
	// not buy access. It buys the routing margin back, first-party prompt
	// caching, and an xhigh effort level a re-router does not always forward.
	"xai": {
		canonical:      "xai",
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"xai", "x-ai", "grok"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.x.ai/v1",
		envKeys:        []string{"XAI_API_KEY"},
		apiKeyURL:      "https://console.x.ai/team/default/api-keys",
		// No RecommendedModels: GET /v1/models answers this
		// (model.DiscoverOpenAICompatibleModels), and xAI is the provider that
		// proves why a list here rots — grok-4 retired on 2026-08-15 and the
		// 15 May retirement before it silently redirected older ids onto
		// grok-4.3 and billed them at grok-4.3 rates. A name written here
		// cannot learn that; the endpoint already knows it.
		modelDefaults: ModelDefaults{FallbackModel: "grok-4.6"},
		capabilities:  Capabilities{ToolCalling: true, Reasoning: true},
	},
	// Thailand's sovereign-AI endpoint. One key reaches all four Thai labs
	// (AIEAT, SCB 10X, NECTEC, KBTG) plus two Qwen models, served on NT
	// hardware under the Ministry of Digital Economy's fund.
	//
	// It fronts Kong, whose own key auth is an `apikey:` header, so this row
	// is one struct literal only because the gateway also accepts the standard
	// bearer. Measured against the live endpoint on 2026-08-20: `Authorization:
	// Bearer` and `apikey:` both answered "Unauthorized" (key seen, key wrong)
	// while `x-api-key:` answered "No API key found in request".
	//
	// Every Thai model here is 8B and labelled "Research Preview" upstream, so
	// this is the row that lets Aetox speak Thai with a Thai model, not the row
	// that survives a long tool loop. qwen3.6-35b-a3b is the only model on the
	// endpoint sized for that, and it is Alibaba's, not Thai.
	"thaillm": {
		canonical: "thaillm",
		// Free for the launch period, and no endpoint a chat key can read: the
		// figure lives behind the playground sign-in, like every other
		// web-only wallet. Read web-only here as "not from this key", which
		// stays true whether or not the free period ends.
		balanceKind:    BalanceWebOnly,
		quotaSource:    QuotaOpenAIStd,
		aliases:        []string{"thaillm", "thai-llm"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api.thaillm.or.th/v1",
		envKeys:        []string{"THAILLM_API_KEY"},
		apiKeyURL:      "https://playground.thaillm.or.th/login/",
		// No RecommendedModels: GET /v1/models answers this
		// (model.DiscoverOpenAICompatibleModels), and this endpoint answers it
		// without a key at all, so the live list is always the better one.
		//
		// Pathumma is the fallback because it is the only model here whose card
		// documents tool-calling training (dedicated tool-call datasets, the
		// Hermes parser). The other three state no tool contract at all, and
		// Aetox calls a tool on the first turn. The id is version-pinned
		// because that is the only spelling the service serves: there is no
		// unversioned alias, and a shortened one 404s.
		modelDefaults: ModelDefaults{FallbackModel: "Pathumma-ThaiLLM-qwen3-8b-think-3.0.0"},
		capabilities:  Capabilities{ToolCalling: true},
	},
	// Alibaba's model hub, and the row for someone who has no card yet: of the
	// models here that any source describes at all, every tool-capable one is
	// priced at zero.
	//
	// It is not a second `qwen`. DashScope is Alibaba's paid API with its own
	// console, its own key and per-token billing; this is the hub's free
	// inference tier on a different host with a different token. Same company,
	// two accounts — the rule that keeps `codex` off `openai`.
	//
	// Measured against the live endpoint 2026-08-20, because the third-party
	// catalog was wrong in both directions here: it lists 7 models where the
	// service serves 45, and 3 of those 7 (GLM-4.5, GLM-4.6,
	// Qwen3-30B-A3B-Instruct-2507) are not served at all. The picker is filled
	// by DiscoverOpenAICompatibleModels, which asks the endpoint; nothing in
	// this row should be taken from a table again.
	"modelscope": {
		canonical: "modelscope",
		// A real account with a free tier, and no endpoint a chat token can
		// read: the daily allowance is only visible once signed in to the hub.
		balanceKind: BalanceWebOnly,
		// QuotaNone, measured rather than assumed — the same mistake this
		// caught on gemini. GET /v1/models answers 200 without a token and
		// POST /v1/chat/completions answers 401 with one absent, and neither
		// response carries a single x-ratelimit-* header among the twelve it
		// sends. A row claiming QuotaOpenAIStd would put "the limit appears
		// once you chat" on a card that can never show one.
		quotaSource:    QuotaNone,
		aliases:        []string{"modelscope", "model-scope", "modelscope-cn"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://api-inference.modelscope.cn/v1",
		envKeys:        []string{"MODELSCOPE_API_KEY"},
		apiKeyURL:      "https://modelscope.cn/my/myaccesstoken",
		// No FallbackModel, for the reason the `ollama` row has none: there is
		// a server that can be asked. GET /v1/models here answers 200 with no
		// token at all, so ResolveDefaultModel's second step — ask the
		// endpoint — cannot fail while the provider works, and a name written
		// in here would only ever be reached with the network down, when
		// nothing else works either. Measured 2026-08-20, and it is the whole
		// justification: on a provider that gates its list behind auth (xAI
		// answers 403 until a team has credits) an empty fallback would leave
		// a new user staring at a blank picker.
		//
		// No thinking dial either. The hub does serve -Thinking checkpoints,
		// but they are separate model ids rather than a setting on one, which
		// is the same stance the `qwen` row takes and the reason Reasoning
		// stays false: a picker offering levels that go nowhere teaches the
		// user that the controls lie.
		modelDefaults: ModelDefaults{},
		capabilities:  Capabilities{ToolCalling: true},
	},
	// NVIDIA's own inference endpoint, and the widest free tier Aetox reaches:
	// of the models it serves that any source describes, 32 answer tool calls
	// at zero cost — nearly twice what OpenRouter's free tier offers, and seven
	// of OpenRouter's are these same Nemotrons through a middleman.
	//
	// The draw is not Nemotron though. This one endpoint serves GLM-5.2 and
	// MiniMax-M3 at a million tokens of context, Kimi K2.6, and gpt-oss — four
	// labs Aetox otherwise reaches only by paying each of them separately.
	//
	// Read the live list, not a table: models.dev describes 100 ids here and
	// the endpoint serves 57 of them. Among the 43 it gets wrong are every
	// Qwen id (NVIDIA serves no Qwen at all) and both unsuffixed DeepSeek V4
	// names, which is what a fallback written from that file would have picked.
	"nvidia": {
		canonical: "nvidia",
		// Free credits granted per account and countable only on their own
		// dashboard — a real balance, just not one a chat key can read.
		balanceKind: BalanceWebOnly,
		// QuotaNone, measured: GET /v1/models answers 200 with five headers and
		// POST /v1/chat/completions answers 401 with four, and no x-ratelimit-*
		// field appears in either. Same correction gemini needed.
		quotaSource:    QuotaNone,
		aliases:        []string{"nvidia", "nim", "nvidia-nim"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://integrate.api.nvidia.com/v1",
		envKeys:        []string{"NVIDIA_API_KEY"},
		apiKeyURL:      "https://build.nvidia.com/settings/api-keys",
		// No FallbackModel: GET /v1/models answers 200 here with no token
		// either, so the endpoint is the authority and nothing needs writing
		// down. This is the row where that matters most — the third-party
		// catalog gets 43 of NVIDIA's 100 listed ids wrong, so any name copied
		// from it had a better than even chance of being one nobody serves.
		//
		// The fetched-catalog step above it will not answer for this row and
		// that is fine: ModelCatalog.DefaultFor skips anything priced at zero,
		// which is most of what makes this endpoint worth having.
		//
		// One thing no model list can settle, learned the hard way on
		// 2026-08-20: a served id is not an invocable one. This endpoint
		// resolves a model to an NVCF function and a new account is refused
		// every one of them with a 404 "Not found for account", while
		// /v1/models and the web playground both keep working. The entitlement
		// is called "Public API Endpoints", NVIDIA does not self-serve it, and
		// accountAccessError (internal/model/account_access.go) is what turns
		// that 404 into a sentence naming help@build.nvidia.com. Expect a new
		// user of this row to meet that wall before they meet a model.
		modelDefaults: ModelDefaults{},
		// Reasoning stays false, and it is the interesting field on this row.
		// NVIDIA has no single thinking dial: gpt-oss models take
		// `reasoning_effort` (and only when their NIM was started with the
		// openai_gptoss parser, which is their deployment's business and not
		// visible from here), Nemotron 3.5 takes
		// `chat_template_kwargs.enable_thinking` nested inside extra_body, and
		// Nemotron Super 49B takes no field at all — its reasoning is switched
		// by what the system prompt says. Three mechanisms, two of which this
		// request struct cannot express and one of which is not a wire field.
		// A picker drawn over that would offer levels that go nowhere.
		capabilities: Capabilities{ToolCalling: true},
	},
	// Ollama's hosted tier, and the way up for someone whose own GPU ran out of
	// room. It exists as its own row rather than a setting on `ollama` because
	// the two share nothing but a name: a different host, an account, a token,
	// and a fixed served list instead of whatever the user happened to pull.
	//
	// RuntimeOpenAICompatible, and that is measured rather than preferred.
	// ollama.com does serve the native API — /api/tags answers 200 with the
	// real list — but OllamaProvider has no API key in it anywhere, not a
	// field and not a header, because the local server it was written for asks
	// for none. The OpenAI-compatible path needs no new client: the endpoint
	// returns OpenAI's own error envelope, and DiscoverOpenAICompatibleModels
	// already fills the picker from it.
	"ollama-cloud": {
		canonical:   "ollama-cloud",
		balanceKind: BalanceWebOnly,
		// QuotaNone, measured: fourteen headers on the 200 and thirteen on the
		// 401, all of them build stamps and trace ids, none a rate limit.
		quotaSource:    QuotaNone,
		aliases:        []string{"ollama-cloud", "ollamacloud", "ollama.com"},
		requiresAPIKey: true,
		runtime:        RuntimeOpenAICompatible,
		baseURL:        "https://ollama.com/v1",
		envKeys:        []string{"OLLAMA_API_KEY"},
		apiKeyURL:      "https://ollama.com/settings/keys",
		// No FallbackModel, the same as the local `ollama` row and for a reason
		// that survives the move to a host: /v1/models answers 200 here
		// without a token, so the served list is always readable and always
		// more current than anything typed here. Ollama's ids carry tags
		// (`deepseek-v4-flash:0731`), and a name written without one is the
		// exact shape of mistake that reads as "model not found" against a
		// server that is working perfectly.
		modelDefaults: ModelDefaults{},
		// No thinking dial: the served list is open-weights models whose
		// reasoning is baked into the checkpoint, and Ollama's OpenAI-compatible
		// surface documents no effort field to turn it with.
		capabilities: Capabilities{ToolCalling: true},
	},
}

var canonicalOrder = sortedCanonicalKeys()

func sortedCanonicalKeys() []string {
	out := make([]string, 0, len(catalog))
	for k := range catalog {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Resolution and lookup
// ---------------------------------------------------------------------------

// Normalize resolves any alias or canonical name to its canonical
// provider identifier. Returns the input unchanged if it is not a known
// alias.
func Normalize(name string) string {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		return "aetox"
	}
	for canonical, e := range catalog {
		for _, alias := range e.aliases {
			if key == alias {
				return canonical
			}
		}
	}
	return key
}

// Lookup returns the full Spec for a provider (by canonical or alias
// name). The second return value is false when the provider is unknown.
func Lookup(name string) (Spec, bool) {
	canonical := Normalize(name)
	e, ok := catalog[canonical]
	if !ok {
		return Spec{}, false
	}
	return Spec{
		Canonical:      canonical,
		Aliases:        append([]string{}, e.aliases...),
		RequiresAPIKey: e.requiresAPIKey,
		Runtime:        e.runtime,
		BaseURL:        e.baseURL,
		AltRuntime:     e.altRuntime,
		AltBaseURL:     e.altBaseURL,
		EnvKeys:        append([]string{}, e.envKeys...),
		ModelDefaults:  e.modelDefaults,
		Capabilities:   e.capabilities,
		BalanceKind:    e.balanceKind,
		QuotaSource:    e.quotaSource,
		AcceptsAPIKey:  !e.signInOnly,
		APIKeyURL:      e.apiKeyURL,
	}, true
}

// APIKeyURL returns where to go and create a key for this provider, or "" when
// there is nowhere to send the user: an unknown provider, a local runtime, or
// a sign-in provider that has no API key to give.
func APIKeyURL(name string) string {
	s, ok := Lookup(name)
	if !ok || !s.AcceptsAPIKey {
		return ""
	}
	return s.APIKeyURL
}

// AcceptsAPIKey reports whether pasting a key is a real way to authenticate
// with this provider. Unknown providers get true: a custom endpoint somebody
// added is far more likely to want a key than to have a sign-in flow Aetox
// has never heard of.
func AcceptsAPIKey(name string) bool {
	s, ok := Lookup(name)
	if !ok {
		return true
	}
	return s.AcceptsAPIKey
}

// BalanceKindFor reports what the provider can say about money. Unknown
// providers get BalanceWebOnly: "we have no way to read this" is the only
// honest answer for a name the catalog has never heard of.
func BalanceKindFor(name string) BalanceKind {
	s, ok := Lookup(name)
	if !ok {
		return BalanceWebOnly
	}
	return s.BalanceKind
}

// QuotaSourceFor reports which rate-limit dialect to read for a provider,
// or QuotaNone when there is none to read.
func QuotaSourceFor(name string) QuotaSource {
	s, ok := Lookup(name)
	if !ok {
		return QuotaNone
	}
	return s.QuotaSource
}

// SupportedProviders returns the canonical IDs of every registered
// provider, in sorted order.
func SupportedProviders() []string {
	return append([]string{}, canonicalOrder...)
}

// RequiresAPIKey reports whether the provider (by canonical or alias
// name) requires an API key to operate.
func RequiresAPIKey(name string) bool {
	s, ok := Lookup(name)
	return ok && s.RequiresAPIKey
}

// DefaultModel returns the static fallback model name for a provider, or
// "" for unknown providers. This is not authoritative — the actual model
// should come from user configuration or live model discovery first.
func DefaultModel(name string) string {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.ModelDefaults.FallbackModel
}

// RecommendedModels returns the optional short list of well-known models
// for a provider, or nil for unknown providers. This list is a static
// discovery hint, not a complete or guaranteed catalog.
func RecommendedModels(name string) []string {
	s, ok := Lookup(name)
	if !ok || len(s.ModelDefaults.RecommendedModels) == 0 {
		return nil
	}
	return append([]string{}, s.ModelDefaults.RecommendedModels...)
}

// DefaultBaseURL returns the standard API endpoint for a provider, or ""
// for unknown providers.
func DefaultBaseURL(name string) string {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.BaseURL
}

// Runtime returns the Runtime class for a provider, or "" if unknown.
func RuntimeFor(name string) Runtime {
	s, ok := Lookup(name)
	if !ok {
		return ""
	}
	return s.Runtime
}

// ---------------------------------------------------------------------------
// Validation and helpers
// ---------------------------------------------------------------------------

// ResolveAPIKey reads the first non-empty environment variable from the
// provider's configured EnvKeys list.
func ResolveAPIKey(name string) string {
	s, ok := Lookup(name)
	if !ok || len(s.EnvKeys) == 0 {
		return ""
	}
	for _, key := range s.EnvKeys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

// MenuLabel returns the display label used in provider-selection menus.
func MenuLabel(name string, keyFound bool) string {
	label := strings.TrimSpace(name)
	if label == "" {
		return "(unknown)"
	}
	if keyFound {
		return label + " (env key found)"
	}
	return label + " (needs key)"
}
