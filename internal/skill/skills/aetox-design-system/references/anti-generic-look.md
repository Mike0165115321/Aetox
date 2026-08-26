# Avoiding the Generic AI Look

This system exists partly to prevent one specific failure: work that is technically correct, tokens resolve, contrast passes, the grid lines up, and still looks like the default output of any model asked the same kind of question. The standard for this system is that it should not read as second to anything else being generated right now.

## The clustered defaults

Unprompted AI-generated design converges on a handful of recognisable looks, cream-and-terracotta, near-black-and-acid, broadsheet hairlines, the big-number hero, decorative 01/02/03 numbering, and the failure is reaching for one *by default*, without a reason. None is wrong when a brief actually calls for it.

The named list, kept current, with a hard rule and a swap-in for each, lives in one place: the `aetox-anti-slop` skill. This system's job is to send you there before generating, not to keep a second copy of the list that quietly drifts from it.

## The genericness check

Already written into `aetox-design`'s "think twice before drawing" step for logos and identity work, the same check applies to anything this system generates: for each design decision, ask whether a different-but-similar brief would produce the same answer. If yes, that decision was a reflex, not a choice, and it gets revisited before building.

## Lock the direction before generating, not after

Where a brief or a user gives an actual constraint, a specific palette, a specific reference, a specific mood, treat that as fixed and generate everything else around it, rather than generating freely and hoping the result matches. A locked constraint that gets silently overridden partway through is the most common way a stated direction gets lost.

## What this buys, concretely

A design-token system on its own only guarantees *consistency*, every piece looking like every other piece this system made. It says nothing about whether that shared look is distinctive. This file is what closes that second gap: consistency is necessary but not sufficient for standing out.

## Related

- `aetox-design`'s "think twice before drawing" section, the same check, written for logo/identity work specifically.
