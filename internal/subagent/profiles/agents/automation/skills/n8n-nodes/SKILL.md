---
name: n8n-nodes
description: How an n8n node is actually shaped — the fields it must carry, why a wrong `type` or `typeVersion` saves cleanly and breaks at run time, how expressions are written, how a step fails without taking the run with it. Use when adding, wiring, or repairing a node on n8n, or when a workflow saved successfully and does not work.
---

# โหนดของ n8n

Your own instructions already carry the four rules that make a whole workflow
survive: read before you write, `connections` is keyed by node name with a
doubled array, an update replaces everything, and a workflow is created switched
off. This is the layer below that — the inside of one node.

Everything here exists because of one property of the API: **`parameters` is
validated as an opaque object.** A node type that is not installed, a version
that does not exist, a parameter you invented — all of them save with a 200 and
fail on the first run. Nothing in this file can be checked by writing it. Only
by reading something real.

## The fields one node carries

```json
{
  "name": "ดึงใบเสร็จจากอีเมล",
  "type": "n8n-nodes-base.gmail",
  "typeVersion": 2.1,
  "position": [460, 300],
  "parameters": { "operation": "getAll", "returnAll": false, "limit": 20 },
  "credentials": { "gmailOAuth2": { "id": "7", "name": "Gmail ของบริษัท" } }
}
```

**`name` is the node's identity, not a label.** `connections` refers to it,
expressions in other nodes refer to it. Two nodes with the same name is a
corrupt graph, and renaming one silently breaks every reference to it — so if
you rename, fix the wiring and the expressions in the same write.

**`type` is an exact string and it is instance-specific.** Core nodes are
`n8n-nodes-base.<something>`; the AI ones are `@n8n/n8n-nodes-langchain.<something>`;
community nodes carry their package name. Case matters. A type that is not
installed on *this* instance is accepted at write time and is a broken node
afterwards. Copy it from a node that already exists rather than reconstructing
it.

**`typeVersion` decides what the parameters are called.** The same node at
version 1 and version 2 takes different fields. Copy the version off a real node
of that type on this instance; do not reach for the highest number you have
heard of, and do not leave it out.

**`position` is `[x, y]` and matters more than it looks.** It is only the canvas,
but the user opens that canvas when something is wrong. Left to right in
execution order, roughly 200 apart. Nodes without positions land on top of each
other and the automation becomes unreadable at exactly the wrong moment.

**`credentials` points at a credential that already exists.** You cannot create
one and must never put a secret in `parameters`. If a node needs an account the
user has not connected, that is a sentence you say to them — not a field you
fill with something plausible.

## Expressions: the `=` is not decoration

A parameter that computes something is a string that **starts with `=`**:

```json
"url": "=https://api.example.com/orders/{{ $json.order_id }}"
```

Without the leading `=` the whole thing is a literal, and the workflow will
cheerfully request a URL with `{{ $json.order_id }}` in it. This is the single
most common defect in a generated workflow after the wiring.

Inside `{{ }}`: `$json` is the current item, `$('ชื่อโหนด').item.json` reaches a
named earlier node, `$now` is the time. A node runs **once per item**, not once
— so an expression that assumes a single record is a bug that only shows up on
the day two arrive.

## Nothing runs without a trigger

A workflow with no trigger node cannot be switched on at all.

**A Manual Trigger is a node, and it is still not an activatable trigger.** This
is the trap, because the graph looks complete: n8n counts only webhook, schedule
and polling nodes, so `n8n` action `activate` on a manual-trigger workflow comes
back with *"has no trigger node"* about a workflow that visibly has one.

What to do with that is the part worth knowing: **nothing**. Activation means
"run by itself from now on". A workflow built to be tested by hand is not
supposed to do that — it is run with the Execute button in the editor, and it is
**finished the moment it saved**. Do not call activate on one, and if the
refusal already came back, do not report the job as failed and do not go adding
a schedule nobody asked for: say it saved, and say where to press Execute.

Whichever real trigger you pick, it has required parameters that decide *when*
it fires: a webhook has a path and an HTTP method, a schedule has a rule, a
poller has an interval. Getting these from a real workflow is worth more than
getting them nearly right from memory.

## Make the step fail the way the user would want

These are node fields, and they are the difference between an automation that
degrades and one that stops dead at 3am:

- **`onError`** — `"stopWorkflow"` (the default), `"continueRegularOutput"` to
  carry on with whatever came out, or `"continueErrorOutput"` to send failures
  down a second output. With that last one the node gains an error port, and it
  is `main` index **1** in `connections` — the second entry of the outer array.
- **`retryOnFail": true`** with `maxTries` and `waitBetweenTries` (ms). This is
  the right answer for a flaky network call and the wrong one for a request that
  is rejected — retrying a 401 four times is four 401s.
- **`alwaysOutputData`** keeps an empty result from silently ending the branch.

Decide this per node and say which nodes you decided it for. A workflow where
every step is `stopWorkflow` is a workflow that will stop; one where every step
is `continueRegularOutput` fails silently, which is worse.

## Before you hand it over

Read the workflow back. Then check three things by eye, because the API checked
none of them: every name in `connections` is a node that exists, every node has
a `typeVersion` you copied rather than chose, and every computed parameter
starts with `=`.

Then say what you could not check. You saw it save. You did not see it run.
