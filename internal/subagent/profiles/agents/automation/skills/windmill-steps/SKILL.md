---
name: windmill-steps
description: How a Windmill flow and its steps are shaped — the path rules the server enforces, module ids and how results are referenced, input_transforms, branches and loops, and where failure is handled. Use when designing, reviewing, or repairing a Windmill flow.
---

# Flow และ step ของ Windmill

## The workspace comes first

Nothing here is reachable without one, and the workspace **id** is not the name
shown in Windmill's own interface. `windmill` action `workspaces` returns the
real one; using the display name gets a 404 that reads like a permissions
problem, and the next hour goes on checking the token.

Then the habit that holds on every engine: `windmill` action `read` an existing flow
before writing one that uses a step you are unsure of. There is no registry of
step types to consult here — a step is code, or a reference to something
runnable — so the arguments a `script` step takes are that script's own, and the
only accurate copy of them is on the instance in front of you. If nothing there
uses it, say so before you build rather than after.

## A flow is a path, a summary, and modules

```json
{
  "path": "u/mike/ใบเสร็จรายเดือน",
  "summary": "รวมใบเสร็จจากอีเมลแล้วสรุปเป็นไฟล์เดียว",
  "value": { "modules": [] }
}
```

**The path is validated by the server and its error message is not helpful, so
get it right before sending.** It must match
`^[ufg](/[\p{Alphabetic}\p{Nd}_-]+){2,}$` — one leading letter, then at least
two `/`-separated segments, no dots, no spaces, 255 characters at most. Thai
letters are fine; a space is not.

The leading letter is a decision, not a prefix:

- `u/<username>/…` is **that person's private space.** Convenient, and invisible
  to their colleagues.
- `f/<folder>/…` is shared — but the folder has to exist already and the user
  has to have write access. If not, the refusal arrives as a permission error
  from the database, which reads like something else entirely.
- `g/…` is a group.

Ask which one they want when the automation is meant to outlive their own use of
it. Do not quietly default to private for something the team will depend on.

## A module is an id and a value

```json
{ "id": "a", "summary": "อ่านใบเสร็จ", "value": { "type": "rawscript", "...": "..." } }
```

**The `id` is how every later step refers to this one's output** — `results.a` in
an expression. Short ids are the convention. Change one and every reference to
it breaks.

Nine kinds of `value.type`, and the required fields are worth knowing because a
missing one is a rejection:

| type | what it is | required beyond `type` |
|:--|:--|:--|
| `rawscript` | code written inline | `content`, `language`, `input_transforms` |
| `script` | a script in the workspace, by path | `path`, `input_transforms` |
| `flow` | another flow as a sub-flow | `path`, `input_transforms` |
| `forloopflow` | iterate a list | `modules`, `iterator`, `skip_failures` |
| `whileloopflow` | repeat until a condition | `modules`, `skip_failures` |
| `branchone` | first matching branch wins | `branches`, `default` |
| `branchall` | every branch runs | `branches` |
| `identity` | passes input through, for debugging | — |
| `aiagent` | an agent step with tools | `input_transforms`, `tools` |

`branchone` requires a `default` even when you think no case can miss. Each
entry of `branches` is `{expr, modules}`; `branchall` takes an optional
`parallel`.

## input_transforms is where the wiring lives

Windmill has no lines between boxes. A step's inputs are a map from the argument
name to where the value comes from:

```json
"input_transforms": {
  "email": { "type": "static", "value": "sales@example.com" },
  "rows":  { "type": "javascript", "expr": "results.a.items" }
}
```

`static` is a literal. `javascript` is an expression with `results.<id>`,
`flow_input.<field>` and — inside a loop — `flow_input.iter.value` in scope.

**The keys must be exactly the arguments the step takes.** For a `rawscript`
that is the parameters of the `main` function you are writing in `content`, so
you control both and can keep them honest. For `script` and `flow` it is
somebody else's signature, and guessing at it is the most likely way a flow you
wrote fails on its first run.

## Failure, and the thing not to do

`failure_module` on the flow is the try/catch for the whole run — one place that
learns something went wrong. Per-step there is `continue_on_error`, `retry`,
`skip_if`, `timeout`, and `suspend` when a human should approve before it
carries on. A flow with no failure handling is fine to build and dishonest to
hand over without saying so.

**Never fix a flow by deleting it and creating it again.** Update it in place.
Deleting discards its history and orphans any schedule pointing at it, and the
user finds out when the thing stops running on Monday.
