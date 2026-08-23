# Motion for Generated Work

Aetox's product UI already has a strict, settled motion philosophy — see `DESIGN.md` at the repository root, §4. That document is about screens Aetox itself renders (settings, onboarding, chat). This file is the same discipline applied to what this design system *generates for a user* (decks, artifacts, dashboards) — a distinct surface with its own review needs.

## Carry the same two questions

`DESIGN.md`'s test — does it answer "does this feel responsive" or "what is happening right now," and nothing else — applies here unchanged, and is repeated in full because it's the filter that catches almost everything else in this file: anything that doesn't answer one of those two questions is excess. No animation exists purely to look polished on load.

## Audit posture, not just build guidance

Treat a piece of finished motion as something to *review*, not just something to have written correctly the first time. Default to flagging; approval is earned. Concrete triggers to flag automatically:

- `transition: all` (never scope a transition to "everything" — name the properties)
- An entrance that scales from `0`
- `ease-in` used for anything the user initiates (see the table below — `ease-in` is for exits and off-screen movement only)
- Any unjustified duration over 300ms
- Motion with no `prefers-reduced-motion` fallback

## Easing, chosen by what's happening — not by habit

| Situation | Easing |
|---|---|
| User-initiated enter/exit (a panel opening, a toast arriving) | `ease-out` |
| Movement that stays on screen (reordering, resizing) | `ease-in-out` |
| Hover, color changes | `ease` |
| A literal constant-speed effect (a marquee, a progress fill tied to real elapsed time) | `linear` |
| Actively avoid | `ease-in` alone for anything the user is watching arrive — it starts slow and ends fast, which reads as sluggish for an entrance |

## How often a moment happens changes whether it should animate at all

An interaction that fires rarely can afford a delightful, slightly longer moment. One that fires constantly cannot — repetition turns charm into friction. As a rough gate: something a user triggers less than monthly can carry real personality in its motion; something triggered dozens of times a session should be fast and nearly instant; anything keyboard-initiated in a power-user flow should generally not animate at all, since the whole point of the keyboard path was to skip waiting.

## Reduced motion means gentler, not necessarily zero

`DESIGN.md` already requires every state to be distinguishable without motion at all — that rule doesn't change. Within that constraint, a `prefers-reduced-motion` response that keeps a plain opacity fade while dropping scale/transform/parallax is a legitimate middle ground, not a violation — the requirement is that meaning survives without motion, not that literally nothing may move.

## Related

- `DESIGN.md` (repository root, §4) — the canonical token names (`--dur-press`, `--dur-arrive`, `--dur-settle`, `--dur-hold-success`, `--dur-ground`) and the rule that a millisecond number is written once, there, and referenced everywhere else by name.
