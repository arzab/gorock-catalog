# gorock-catalog

Integrations and apps built by the GOROCK rules, moved out of [gorock-kit](https://github.com/arzab/gorock-kit) so that the kit keeps only its core (`rockengine`, `rockconfig`).

**Status: parked.** The code is moved as-is (gorock-kit commit `5ff98ba`, released there as `v0.2.0`) and builds and passes its tests, but its shape will change: the plan is a catalog of components you copy into your project (`pkg/services/...`, `cmd/apps/...`) instead of importing them as modules, and an HTTP app on `net/http` instead of Fiber.

| Module | What it is | GOROCK place |
|---|---|---|
| `rockfiber` | HTTP App on Fiber v2: endpoints, params, error handling | `cmd/apps` |
| `rockbus` | in-process event bus App | `cmd/apps` |
| `rockcron` | scheduler App (robfig/cron) | `cmd/apps` |
| `rocktelebot` | Telegram bot App (telebot.v3) | `cmd/apps` |
| `rockbun` | PostgreSQL service (bun) | `pkg/services` |
| `rockredis` | Redis service (go-redis) | `pkg/services` |
| `rocklog` | logger on log/slog | `pkg/infra` |

Until the catalog is published, use the gorock-kit `v0.2.0` tags (`github.com/arzab/gorock-kit/<module>@v0.2.0`) — they stay available.

## Development

```sh
bash scripts/check.sh all        # build + vet + test every module
bash scripts/check.sh tidy-check
```
