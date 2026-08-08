---
name: repo-standards
description: What a repository needs before it is fit for other people — the files, the settings, and the order to add them in. Use when setting up a new repository, publishing an existing project, or auditing one that grew without a plan.
---

# A repository other people can use

The question this answers is not "what files does GitHub support" — it is
"what is missing before someone who is not you can use, trust, or contribute to
this". Those are different lists, and the second one is short.

Work in the order below. It is ordered by what breaks first without it, not by
what is most often talked about.

## 1. The four that decide whether anyone stays

**`README.md`** — the only file most visitors will ever open. It has to answer,
above the fold: what this is, who it is for, and how to run it. A README that
opens with a badge wall and a table of contents has spent its most valuable
screen on navigation for a page nobody has decided to read yet.

Put the smallest thing that works first — install, then one command that
produces visible output. A user who gets one thing working will read the rest.

**`LICENSE`** — without one, the default is *all rights reserved*: nobody may
legally use it, including the person who starred it intending to. This is the
single most common gap in otherwise finished projects.

Choose by what you want back: MIT and Apache-2.0 let anyone use it commercially
(Apache-2.0 also grants patent rights explicitly, which is why companies prefer
it); GPL-3.0 requires derivative works to stay open; AGPL-3.0 extends that to
software served over a network. Do not invent one, and do not leave the
copyright line as a template placeholder.

**`.gitignore`** — the difference between a clean history and one with a
committed `.env` in it. Start from GitHub's template for the language, then add
what this project actually produces. Check it *before* the first commit: a
secret removed in a later commit is still in the history, and the fix is to
rotate the secret, not to delete the file.

**A description and topics on the repository itself.** Not files, and the
easiest to skip — they are how the repository is found at all.

## 2. Once someone else might contribute

**`CONTRIBUTING.md`** — how to set up, how to run the tests, what a good pull
request looks like here, and how long review takes. It is written for exactly
one person: someone willing to help who does not want to guess and be told off.

**Issue and pull request templates** — `.github/ISSUE_TEMPLATE/` (bug report
and feature request as separate forms) and `.github/pull_request_template.md`.
Their real job is not tidiness; it is to make the reporter tell you the version
and the reproduction steps in the first message instead of the fourth.

**`CODEOWNERS`** in `.github/` — maps paths to reviewers, and auto-requests
them. Worth adding the moment more than one person reviews, and worth *not*
adding while it would just be your own name on every line.

**`SECURITY.md`** — where to report a vulnerability privately. Without it, the
polite finder opens a public issue describing exactly how to exploit you.
Enable private vulnerability reporting in the repository settings alongside it.

**`CODE_OF_CONDUCT.md`** — the Contributor Covenant is the standard text and
adding it is copying it. It matters when it matters, and by then it is late.

## 3. What makes the repository defend itself

**CI on every pull request** — `.github/workflows/`. One workflow that runs
build, tests and lint on push and pull_request is worth more than five that do
clever things. Pin actions to a version, and give the workflow the narrowest
`permissions:` it can do its job with rather than the default.

**Branch protection on the default branch** — require the checks to pass,
require a review, and forbid force-push. This is a setting, not a file, and it
is what turns the CI above from a suggestion into a rule. A repository with CI
and no protection is one tired evening away from a broken main.

**Dependabot** — `.github/dependabot.yml` for the package ecosystem and for
`github-actions`. Security updates are on by default; version updates are not.

**Secret scanning and push protection** — on for public repositories, worth
switching on for private ones. It stops the commit rather than reporting it.

## 4. When it is released rather than just published

- **Semantic versioning and tags** — `v1.4.0`. Tag the commit, and let the
  release note say what changed for a *user*, not what changed in the diff.
- **`CHANGELOG.md`**, or release notes doing that job. Pick one. Two places
  answering "what changed in 1.4" is two places to forget.
- **A release with artefacts** if people install it rather than clone it.

## The audit, when you are handed one that grew

Check in this order and report what is missing, worst first:

1. Is there a `LICENSE`? — if not, nothing else matters yet.
2. Does the README say what it is and how to run it, in the first screen?
3. Is anything secret in the history? A committed `.env`, a key in a config.
4. Does CI run on pull requests, and is the default branch protected?
5. Is there a private way to report a vulnerability?
6. Can a newcomer find out how to contribute without asking?

Report it as that list, with the two or three that actually matter for *this*
repository named first. A checklist returned in full, all items weighted the
same, is a checklist the owner will skim and act on none of.
