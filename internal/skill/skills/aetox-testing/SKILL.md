---
name: aetox-testing
description: ออกแบบกลยุทธ์และแผนทดสอบ — testing pyramid, กลยุทธ์รายชั้น (API, data pipeline, frontend, infra), จัดลำดับ critical path/edge/security บวกวินัย TDD (red-green-refactor) และกับดักเทสต์ที่ควรเลี่ยง ทริกด้วย "ควรเทสต์ยังไง", "test plan", "เขียนเทสต์ให้"
source: https://github.com/anthropics/knowledge-work-plugins (engineering/testing-strategy) + https://github.com/obra/superpowers (test-driven-development)
license: Apache-2.0 (testing-strategy) + MIT (TDD, LICENSE-superpowers)
copyright: Copyright Anthropic, PBC (Apache-2.0) และ Copyright (c) 2025 Jesse Vincent (MIT)
---

# Testing Strategy

Design effective testing strategies balancing coverage, speed, and maintenance.

## Testing Pyramid

```
        /  E2E  \         Few, slow, high confidence
       / Integration \     Some, medium speed
      /    Unit Tests  \   Many, fast, focused
```

## Strategy by Component Type

- **API endpoints**: Unit tests for business logic, integration tests for HTTP layer, contract tests for consumers
- **Data pipelines**: Input validation, transformation correctness, idempotency tests
- **Frontend**: Component tests, interaction tests, visual regression, accessibility
- **Infrastructure**: Smoke tests, chaos engineering, load tests

## What to Cover

Focus on: business-critical paths, error handling, edge cases, security boundaries, data integrity.

Skip: trivial getters/setters, framework code, one-off scripts.

## Output

Produce a test plan with: what to test, test type for each area, coverage targets, and example test cases. Identify gaps in existing coverage.

## Writing the tests, not only planning them

The strategy above decides *what* and *how much* to test. For the discipline of
actually writing them, two references adapted from obra/superpowers (MIT):

- `references/tdd.md` — the RED-GREEN-REFACTOR loop: a failing test first, the
  smallest code that passes, then refactor.
- `references/writing-good-tests.md` — the test anti-patterns to avoid, so a
  suite stays worth keeping.
