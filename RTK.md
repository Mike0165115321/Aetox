# RTK Usage Rules

## Safe filtering policy

Only prefix **noise commands** with `rtk`. Never use rtk on **evidence commands**.

### Evidence commands — always run raw:
- tsc, typecheck, next build
- test, vitest, jest, cargo test, go test
- eslint, lint, ruff
- git diff, git show

### Noise commands — safe to rtk:
- ls, tree, find
- git status
- pnpm install, npm install, bun install
- pip list, npm list, pnpm list

## Examples

```bash
# rtk ok (noise):
rtk git status
rtk pnpm install

# rtk NEVER (evidence):
go test ./...
git diff
```
