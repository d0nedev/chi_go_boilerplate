# chi-go-boilerplate

Template REST API Go production-ready: [chi](https://github.com/go-chi/chi), PostgreSQL ([pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev)), OpenTelemetry, dan deploy VPS. Domain `product` adalah contoh; ganti atau hapus sesuai kebutuhan.

Kebutuhan: Go 1.27+ (memakai package `uuid` dari stdlib), Docker, [sqlc](https://sqlc.dev) untuk regenerate query.

<!-- template-only:start -->
## Membuat Service Baru

Contoh untuk service `order-api` dengan module `github.com/acme/order-api`:

```bash
go run golang.org/x/tools/cmd/gonew@latest github.com/d0nedev/chi_go_boilerplate@latest github.com/acme/order-api
cd order-api

# gonew hanya mengganti import path Go. Nama service, DB, alert, dan dashboard diganti di sini.
perl -0pi -e 's/<!-- template-only:start -->.*?<!-- template-only:end -->\n//s' README.md
grep -rlE 'chi-go-boilerplate|chi_go_boilerplate' . | xargs perl -pi -e 's/chi-go-boilerplate/order-api/g; s/chi_go_boilerplate/order_api/g'
mv grafana/dashboards/chi-go-boilerplate.json grafana/dashboards/order-api.json
rm -f docs/reports/*.md
gofmt -w cmd internal

go build ./... && make test
git init && git add -A && git commit -m "init from chi_go_boilerplate"
```

Aturan nama: `chi-go-boilerplate` diganti nama service (kebab-case), `chi_go_boilerplate` diganti nama database (snake_case).

Jika repo template private: `go env -w GOPRIVATE=github.com/d0nedev` dan pastikan git bisa mengakses `https://github.com/d0nedev/...`.

Perbaikan di template tidak mengalir otomatis ke service yang sudah dibuat; bandingkan diff antar tag template secara manual.

<!-- template-only:end -->
## Arsitektur

```
cmd/server            entrypoint: HTTP server, graceful shutdown + readiness drain
internal/app          composition root: config, telemetry, DB pool, router (app.go); wiring domain (modules.go)
internal/product      domain produk: handler -> service -> sqlc
internal/platform     utilitas sistem (bukan business logic):
  config              konfigurasi dari env / .env, validasi per environment
  database            pool Postgres + kode hasil generate sqlc
  middleware          request ID, access log, route tag (OTel), recovery, API key
  apperror, httpx     error terstruktur, JSON request/response
  health              /health (liveness) dan /ready (readiness + drain)
  logging, tracing, metrics, requestcontext  slog JSON, OTel traces & metrics via OTLP gRPC
db/migrations         migrasi golang-migrate (juga schema sumber sqlc)
db/queries            query SQL untuk sqlc
```

Urutan middleware: `ClientIP -> RequestID -> SecurityHeaders -> Logging -> RouteTag -> Recovery`, dengan rate limit (per IP klien) dan API key di `/api/v1`. Probe `/health` dan `/ready` tidak di-trace, dan hanya di-log saat gagal.

## API

Kontrak lengkap: [`openapi.yaml`](openapi.yaml). Base path: `/api/v1`. Semua error, termasuk 404/405, berformat `{"error":{"code":"...","message":"..."}}`.

| Method | Path | Auth | Keterangan |
|---|---|---|---|
| GET | `/products?limit=20&cursor=...` | - | Pagination keyset. `limit` 1–100 (default 20). Response `{"data":[...],"next_cursor":"..."\|null}` |
| GET | `/products/{id}` | - | |
| POST | `/products` | `X-API-Key` | Body `{"name":"...","price":"12500.00"}` |
| PUT | `/products/{id}` | `X-API-Key` | Body sama dengan POST |
| DELETE | `/products/{id}` | `X-API-Key` | 204 |
| GET | `/health` | - | Liveness |
| GET | `/ready` | - | Readiness (ping DB; 503 saat drain) |

Kontrak data:
- `price` adalah **string desimal** non-negatif, maksimal 2 angka di belakang koma (`NUMERIC(15,2)`).
- `name` wajib diisi, maksimal 255 karakter.
- `created_at`/`updated_at` dalam format RFC 3339 UTC.
- Rate limit per IP (`RATE_LIMIT_REQUESTS_PER_MINUTE`), melebihi batas mendapat `429 RATE_LIMITED`.

Contoh request: [`http/product.http`](http/product.http).

## Menjalankan

### Semua via Docker Compose

```bash
make up          # postgres, migrate, app, otel-collector, jaeger, prometheus
```

- API: http://localhost:8080
- Jaeger: http://localhost:16686
- Prometheus: http://localhost:9090
- Grafana: http://localhost:3000 (dashboard `chi-go-boilerplate`, datasource Prometheus + Jaeger, tanpa login — dev only)
- Postgres: `localhost:55432` (postgres/postgres)

Set `API_KEYS=...` di shell sebelum `make up` untuk mengaktifkan auth di compose.

### App lokal

```bash
cp .env.example .env     # sesuaikan DB_*
make migrate-up DATABASE_URL='postgres://user:pass@localhost:5432/chi_go_boilerplate?sslmode=disable'
make run
```

## Konfigurasi

Lihat [`.env.example`](.env.example). Aturan validasi penting:

| Env | Aturan |
|---|---|
| `APP_ENV` | `development`, `staging`, atau `production` |
| `API_KEYS` | Wajib di luar `development`. Dipisah koma untuk rotasi key. |
| `DB_SSL_MODE` | Default `require`; di `production` wajib `require`/`verify-ca`/`verify-full` |
| `OTEL_EXPORTER_OTLP_INSECURE` | Default `true` hanya di `development`, `false` di luar itu. Set `true` hanya untuk collector di network privat yang sama (lihat `deploy/`) |
| `OTEL_TRACE_SAMPLE_RATE` | 0–1, default `0.1` |
| `APP_*_TIMEOUT` | Default aman (5s/10s/10s/60s), harus > 0 |
| `APP_SHUTDOWN_DRAIN_DELAY` | Default `5s`; set lebih besar dari periode readiness probe |
| `APP_SHUTDOWN_TIMEOUT` | Default `10s`. `drain delay + timeout` harus < grace period orchestrator |
| `DB_CONNECT_TIMEOUT` | Default `30s`; lama retry koneksi DB pertama saat startup |
| `DB_STATEMENT_TIMEOUT` | Default `5s`; batas per query di sisi Postgres, harus < `APP_WRITE_TIMEOUT` |
| `.env` | Hanya dibaca jika `APP_ENV` kosong atau `development` |
| `TRUSTED_PROXIES` | CIDR proxy/LB di depan app, dipisah koma. Kosong = IP klien dari koneksi TCP. `X-Forwarded-For` hanya dipercaya jika koneksi datang dari CIDR ini. **Wajib diisi di belakang load balancer**, kalau tidak semua klien berbagi satu bucket rate limit. |

## Development

```bash
make test               # unit test (+ race detector)
make test-integration   # butuh Postgres; tiap test memakai schema terisolasi lalu dihapus
make lint               # gofmt + go vet
make vuln               # govulncheck
make sqlc               # regenerate setelah mengubah db/queries atau migrasi
make openapi-lint       # validasi openapi.yaml
make alerts-test        # promtool check + unit test alert rules
make loadtest BASE_URL=... API_KEY=... RATE=200   # k6, butuh stack berjalan
```

Domain baru: ikuti pola `internal/product` dan daftarkan route-nya di `internal/app/modules.go`. Error code umum ada di `internal/platform/apperror/codes.go`; code khusus domain ditaruh di `<domain>/errors.go` (contoh: `internal/product/errors.go`). Error query dipetakan dengan `database.IsNotFound` → `apperror.NotFound`, selain itu `tracing.Fail(span, apperror.Internal(...))`. Untuk beberapa query atomik pakai `pgx.BeginFunc` + `queries.WithTx` (contoh teruji: `internal/product/transaction_integration_test.go`).

Migrasi baru: tambahkan pasangan `db/migrations/000N_nama.up.sql` dan `.down.sql`, lalu `make sqlc`. Jangan mengubah migrasi yang sudah pernah dijalankan.

CI (`.github/workflows/ci.yml`) menjalankan lint, govulncheck, `sqlc diff`, lint OpenAPI, test alert rules, migrasi up/down/up, test dengan Postgres, build image (dengan `VERSION`), dan scan Trivy.

## Operasional

Deploy production ke VPS (Caddy + Postgres + OTel Collector, observability ke Grafana Cloud): folder [`deploy/`](deploy/), langkah lengkap di [`docs/runbook.md`](docs/runbook.md#deploy-vps-deploy).

SLO, arti setiap alert, dan langkah penanganannya ada di [`docs/runbook.md`](docs/runbook.md). Alert rules: [`prometheus-alerts.yml`](prometheus-alerts.yml), dimuat oleh Prometheus di compose (http://localhost:9090/alerts).
