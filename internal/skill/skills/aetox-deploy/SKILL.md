---
name: aetox-deploy
description: ก่อนปล่อยของและตอนเกิดเหตุ, checklist ก่อน deploy (migration, feature flag, rollback trigger, ยืนยัน CI), incident response (เกณฑ์ SEV1-4, สื่อสารระหว่างเหตุ, เขียน postmortem) และ git workflow ปลอดภัย (แยกงานด้วย worktree, ปิด branch ให้จบสะอาด) ทริกด้วย "จะ deploy", "production ล่ม", "ปิดงานยังไง"
source: https://github.com/anthropics/knowledge-work-plugins (deploy-checklist, incident-response) + https://github.com/obra/superpowers (git worktrees, finishing a branch)
license: Apache-2.0 (Anthropic) + MIT (obra, LICENSE-superpowers)
copyright: Copyright Anthropic, PBC (Apache-2.0) และ Copyright (c) 2025 Jesse Vincent (MIT)
---

# Shipping and the things around it

Four guides for the moments where a mistake is expensive: the last check before
a release, the first hour of an incident, and the git discipline that keeps
parallel work and branch-closing from becoming their own outage. Open the one the
moment calls for.

- `references/deploy-checklist.md`, the pre-deploy pass: migrations, feature
  flags, CI status and approvals, and the rollback triggers written down *before*
  the deploy, not during it.
- `references/incident-response.md`, when something is down: severity (SEV1–4),
  triage, the status update while it is still burning, and the blameless
  postmortem after.
- `references/git-worktrees.md`, isolated git worktrees so parallel work (or a
  parallel agent) never fights over one working tree.
- `references/finishing-branch.md`, closing a branch safely: verify tests pass,
  then merge / open a PR / keep / discard, and clean up after.

The deploy and incident halves are adapted from Anthropic's engineering plugin
(Apache-2.0); the two git guides from obra/superpowers (MIT). Each is kept whole.
