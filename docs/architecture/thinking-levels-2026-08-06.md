# The Thinking Dial: One Table, and Only Levels That Exist (2026-08-06)

Reference for the thinking-depth control — the picker in the composer and the
field it turns into on the wire. Read this before adding a provider, adding a
level, or changing what the menu offers; the arrangement it replaced was wrong
in four different ways at once, and each way looked fine from where it was
written.

Live code: [`thinking_capabilities.go`](../../internal/model/thinking_capabilities.go),
[`anthropic.go`](../../internal/model/anthropic.go),
[`openai_compatible.go`](../../internal/model/openai_compatible.go),
[`bootstrap.go`](../../internal/bootstrap/bootstrap.go),
[`Chat.svelte`](../../desktop/frontend/src/lib/Chat.svelte).

---

## The rule

**A level is offered only if the endpoint has it, it changes what the model
does, and the user can choose something else instead.**

Three clauses, and every one of them was broken by real code in this repo:

| Clause | What broke it |
|---|---|
| the endpoint has it | Seven providers with no thinking field at all drew a full four-entry picker |
| it changes what the model does | DeepSeek listed `medium` and `xhigh`, which the service folds onto `high` |
| the user can choose something else | Models with exactly one level drew a dropdown with one row |

---

## What the picker did before

Four places answered "which levels does this provider have", in three files,
and no two of them agreed. For DeepSeek:

| Place | Said |
|---|---|
| the capability table | `off, high, max` |
| the normalizer's fallback | folded `low`/`medium` → `high`, `xhigh` → `max` |
| the Anthropic-format effort mapper | `low, medium, high, xhigh, max` |
| the OpenAI-format effort mapper | `high, max` |
| **the API** | `low, medium, high, xhigh, ultra, max` |

`low` — a real, distinct depth — was thrown away twice: the picker never
offered it, and the normalizer rewrote any that arrived. Meanwhile two mappers
knew names the picker could never produce, so those branches were dead.

Now `ThinkingCapabilities` carries both halves — the levels **and** what each one
is called on the wire — and every mapper reads it. A level that cannot be sent
cannot be shown, and `TestEveryOfferedLevelHasAWireValue` fails the build if one
appears.

## The bug underneath all of it

None of that mattered, because **the level never reached the request at all** on
the desktop. `bootstrap.Engine` built the sub-agent tools with `cfg.ThinkLevel`
and then built the app without it, so `app.thinkLevel` resolved an empty string —
which `think.NormalizeLevel` reads as `medium`. Every desktop turn went out at a
depth nobody chose, `off` included.

The CLI passed it and was never affected. Sub-agents honoured a setting the main
agent ignored.

Every unit test passed throughout: each link was correct and the break was in
the wiring between them. The test that catches it now
([`thinklevel_wire_test.go`](../../internal/bootstrap/thinklevel_wire_test.go))
asserts against the **payload**, not against any function's return value — it is
the only thing that can tell "the user picked off" from "the request said off".

## Where each provider's dial comes from

| Provider | Levels | Carried by | Source |
|---|---|---|---|
| deepseek | `off low high max` | `thinking` block + `output_config.effort` | API enum, measured 2026-08-06 |
| anthropic | `off low medium high xhigh max` | `thinking` + `output_config.effort` | Anthropic effort ladder |
| openai | per family (`gpt-5.2`: `none…xhigh`) | `reasoning_effort` | OpenAI chat docs |
| gemini | `none minimal low medium high` | `reasoning_effort` | Gemini OpenAI-compat docs |
| groq | per family | `reasoning_effort` | Groq reasoning docs |
| openrouter | `none…xhigh` | `reasoning` object | OpenRouter docs |
| codex | `off low medium high xhigh` | Responses `reasoning.effort` | Responses API |
| **kimi** | `low high max` — **no off** | `reasoning_effort` | K3 always thinks |
| **minimax** | `off on` (M3) · `on` (M2.x) | `thinking` block only | no effort field exists |
| everything else | **none** | — | the API has no such field |

A model this table does not recognise gets **no dial**, even on a provider that
has one. Both OpenAI's and Groq's catalog defaults — `gpt-4o-mini`,
`llama-3.3-70b-versatile` — are not reasoning models, and the old guess did not
merely draw a fake menu: it put the guessed effort on the wire, at a model that
does not take it.

The cost is that a genuinely new model has no dial until it is added here. That
is the right way round. A missing control is something the user can see and
report; an inert one is not.

## Two things measurement said that the docs did not

Measured against the live API, 5–6 runs per level, two problems, three request
shapes:

- **DeepSeek's `low` spends 2–3× MORE tokens than `high`.** Consistently, on
  every shape tested. The ladder is not ordered the way the names read. The
  names are still the API's, spelled its way — inventing our own would only be a
  second vocabulary to keep in step with theirs.
- **`medium` measures identically to `high`** (2,679 thinking chars against
  3,002), which is what the docs claim and worth having confirmed.

`ultra` is in the API's own error enum and in no documentation anywhere. It is
reachable as an alias, never offered as a choice.

## Adding a provider

1. Catalog entry in [`internal/provider/catalog.go`](../../internal/provider/catalog.go).
   `Capabilities.Reasoning` is the gate — false means no dial, everywhere, and
   nothing else needs saying.
2. A resolver in `thinking_capabilities.go` returning the levels **and** their
   wire values. Unknown models must fall through to `conservativeFallback`.
3. If the dial is a switch carried in a `thinking` block rather than an effort
   string, add its runtime to `ThinkingBlockType`.
4. Add the provider to `thinkingProviders` in
   [`thinking_wire_consistency_test.go`](../../internal/model/thinking_wire_consistency_test.go)
   and add a payload case to `thinklevel_wire_test.go`.

Do **not** add a model list. Names come from the provider's own `/models`
endpoint; the catalog's single `FallbackModel` is the cold-start fallback for
when discovery cannot run, not the source the picker reads.

## Unverified

Kimi and MiniMax are documented, not measured — there is no key for either on
the machine this was written on. DeepSeek is the standing reminder that those
are different things.
