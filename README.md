# gorock-catalog

Components built by the [GOROCK](https://arzab.github.io/gorock/) rules on top of [gorock-kit](https://github.com/arzab/gorock-kit) — the kit keeps only the core (`rockengine`, `rockconfig`, `rockservices`, `rockhandlers`), everything technology-specific lives here.

| Module | What it is | GOROCK place | Status |
|---|---|---|---|
| [`rockhttp`](./rockhttp) | HTTP transport for [rockhandlers](https://github.com/arzab/gorock-kit/tree/main/rockhandlers) on `net/http`: endpoints, binding, files, redirects, error mapping | `cmd/apps` | **experimental**, the recommended HTTP app |
| `rockfiber` | HTTP App on Fiber v2 (older model, without rockhandlers) | `cmd/apps` | as moved from kit v0.2.0 |
| `rockbus` | in-process event bus App | `cmd/apps` | as moved from kit v0.2.0 |
| `rockcron` | scheduler App (robfig/cron) | `cmd/apps` | as moved from kit v0.2.0 |
| `rocktelebot` | Telegram bot App (telebot.v3) | `cmd/apps` | as moved from kit v0.2.0 |
| `rockbun` | PostgreSQL service (bun) | `pkg/services` | as moved from kit v0.2.0 |
| `rockredis` | Redis service (go-redis) | `pkg/services` | as moved from kit v0.2.0 |
| `rocklog` | logger on log/slog | `pkg/infra` | as moved from kit v0.2.0 |

Modules marked "as moved" are the gorock-kit `v0.2.0` code under new module paths (`github.com/arzab/gorock-catalog/<module>`); they will be reshaped to the new core (services as `rockservices.Service`, apps on rockhandlers) step by step.

```sh
go get github.com/arzab/gorock-catalog/rockhttp
```

All modules share one version; tags are per module (`rockhttp/v0.1.0`, …).

## Development

```sh
bash scripts/check.sh all        # build + vet + test every module
bash scripts/check.sh tidy-check
```

## License

MIT
