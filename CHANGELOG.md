# Changelog

All modules are versioned together with [gorock-kit](https://github.com/arzab/gorock-kit): catalog `v0.5.0` works with core `v0.5.0`. Tags are per module (`rockhttp/v0.5.0`, `rockbun/v0.5.0`, …), as Go requires for a multi-module repository.

## Unreleased

## v0.5.0 — 2026-11-10

First tagged release of the catalog.

### Added

- **rockhttp** — HTTP App for rockhandlers on `net/http`: `Endpoint{Method, Path, Handler, Status}`, `App.Use`, binding from `path`/`query`/`header`/`form` tags (incl. embedded structs), JSON body and multipart files; `File` and `Redirect` results; `SetHeader`/`SetCookie`/`Request`; error kinds → HTTP status (Canceled 499, Timeout 504) with a JSON `ErrorBody` incl. `fields`. Built on gorock-kit `v0.5.0`: a handler that returns `rockhandlers.Done` answers with its Result; decoding errors reach the client as `invalid JSON body` / `invalid query` / `invalid path` / `invalid header` / `invalid form` with the field in `fields`, never with Go type names.

### Moved

- **rockfiber, rockbus, rockcron, rocktelebot, rockbun, rockredis, rocklog** — moved as-is from gorock-kit `v0.2.0` under `github.com/arzab/gorock-catalog/<module>`. `rockbun` and `rockredis` services are `Init` + `Stop` and fit `rockservices` directly. The Apps don't take rockhandlers handlers yet.
