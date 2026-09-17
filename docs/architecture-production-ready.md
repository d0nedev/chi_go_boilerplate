# Review Arsitektur: Production Readiness — chi-product-api

Tanggal review: 2026-09-17
Cakupan: seluruh source di `cmd/`, `internal/`, `db/`, konfigurasi (`docker-compose.yml`, `otel-collector.yaml`, `sqlc.yaml`, `.env.example`), dan tooling repo.
Status perbaikan: P0 selesai ([laporan](reports/p0-perbaikan-blocker.md)), P1 selesai ([laporan](reports/p1-production-hardening.md), [rate limit](reports/p1-rate-limit-client-ip.md)), P2 selesai ([laporan](reports/p2-hardening-operasional.md)); skor terkini 4.4 / 5
Metode: baca kode end-to-end, `go build ./...`, `go vet ./...`, dan knowledge graph (`graphify-out/GRAPH_REPORT.md`).

---

## 1. Ringkasan Eksekutif

Fondasi arsitekturnya **bagus dan arahnya sudah benar**: layering handler → service → sqlc, error type terpusat (`apperror`), structured logging JSON (`slog`) dengan `request_id` + `trace_id`, OpenTelemetry tracing & metrics, graceful shutdown, readiness probe yang cek DB, konfigurasi via env dengan validasi, dan HTTP server timeouts.

Namun **saat ini belum production ready**. Blocker utamanya:

1. **Aplikasi tidak bisa di-build** (syntax error + signature mismatch di `internal/app/app.go`).
2. **Beberapa bug logika yang merusak data/observability**: harga produk salah saat di-serialize, create product selalu gagal, `tracing.RecordError` tidak pernah merekam error, wrapped `apperror` jadi 500.
3. **Tidak ada test sama sekali, tidak ada Dockerfile, CI, maupun proses migrasi yang jelas.**
4. **Endpoint `/api/v1/products/panic` terekspos**, tidak ada auth/rate limit.

**Skor keseluruhan: 1.8 / 5 — "Prototype yang terstruktur baik"**. Dengan menyelesaikan item P0 dan P1 (Bagian 5), skor realistis naik ke ~4/5.

---

## 2. Kriteria Production Ready

Setiap kriteria dinilai skala 0–5:

| Skor | Arti |
|---|---|
| 0 | Tidak ada |
| 1 | Ada tapi rusak / tidak berfungsi |
| 2 | Parsial, banyak gap |
| 3 | Cukup untuk staging, ada gap minor untuk prod |
| 4 | Production ready |
| 5 | Production ready + best practice |

Target minimal production: **semua kriteria ≥ 3, kriteria bertanda ★ (kritis) ≥ 4**.

| # | Kriteria | Yang diharapkan |
|---|---|---|
| K1 ★ | **Build & Correctness** | Build & vet bersih, tidak ada bug logika pada jalur utama, kontrak API konsisten |
| K2 ★ | **Testing** | Unit test service/handler, integration test DB (testcontainers), coverage jalur kritis, test dijalankan di CI |
| K3 | **Arsitektur & Maintainability** | Layer jelas, dependency bisa di-substitute untuk test, tidak ada dead/duplicate code, konsistensi pola |
| K4 ★ | **Error Handling & API Contract** | Error mapping konsisten, tidak bocor detail internal, format response & tipe data stabil, validasi input lengkap |
| K5 | **Konfigurasi & Secrets** | 12-factor, validasi lengkap (termasuk timeouts), default aman, secret tidak di file, DSN aman |
| K6 ★ | **Database & Data Integrity** | Migrasi versioned & otomatis di pipeline, constraint di DB, timezone benar, pagination, pool & statement timeout |
| K7 | **Observability** | Log terstruktur + korelasi, tracing end-to-end dengan propagation, metrics RED + backend, sampling sesuai env |
| K8 ★ | **Reliability & Lifecycle** | Graceful shutdown dengan drain, liveness/readiness benar, timeouts di semua layer, panic recovery benar |
| K9 ★ | **Security** | AuthN/AuthZ, rate limiting, body limit, security headers, TLS ke dependency, tidak ada endpoint debug, dependency scanning |
| K10 | **Performance & Scalability** | Stateless, pagination, pool tuning, tidak ada unbounded query, load test baseline |
| K11 ★ | **Delivery & Operasional** | Dockerfile multi-stage non-root, CI (lint/test/build/scan), versioning, README/runbook, API docs (OpenAPI) |

---

## 3. Scorecard

| # | Kriteria | Skor | Status |
|---|---|---|---|
| K1 ★ | Build & Correctness | **1** | ❌ Tidak bisa build, beberapa bug fungsional |
| K2 ★ | Testing | **0** | ❌ Tidak ada `_test.go` |
| K3 | Arsitektur & Maintainability | **3** | ⚠️ Struktur bagus, ada duplikasi & inkonsistensi |
| K4 ★ | Error Handling & API Contract | **2** | ⚠️ Fondasi bagus, implementasi tidak konsisten |
| K5 | Konfigurasi & Secrets | **2.5** | ⚠️ Validasi parsial, default berbahaya |
| K6 ★ | Database & Data Integrity | **2** | ⚠️ Tanpa pagination, tanpa timezone, migrasi manual |
| K7 | Observability | **2.5** | ⚠️ Setup lengkap tapi ada bug & tanpa propagator |
| K8 ★ | Reliability & Lifecycle | **3** | ⚠️ Shutdown ada, urutan middleware & drain kurang |
| K9 ★ | Security | **1** | ❌ Tanpa auth, rate limit; endpoint panic terbuka |
| K10 | Performance & Scalability | **2** | ⚠️ Stateless OK, query unbounded |
| K11 ★ | Delivery & Operasional | **0.5** | ❌ Tanpa Dockerfile/CI/Makefile, README bukan milik project |
| | **Rata-rata** | **1.8** | **Belum production ready** |

---

## 4. Temuan Detail

Label prioritas:
- **P0** — blocker, wajib sebelum deploy apapun.
- **P1** — wajib sebelum production.
- **P2** — sebaiknya ada, bisa menyusul setelah go-live.

### K1 — Build & Correctness

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| B1 | P0 | `internal/app/app.go:45` | Syntax error `Level: cfg.App.,` — seluruh binary tidak bisa di-build. | `Level: cfg.App.LogLevel,` |
| B2 | P0 | `internal/app/app.go:67` vs `internal/database/postgres.go:13` | `NewPostgresPool(ctx, cfg.App, cfg.DB)` dipanggil 3 argumen, fungsi hanya menerima `(ctx, cfg DBConfig)`. Error ini tertutup oleh B1. | Samakan signature: `NewPostgresPool(ctx, cfg.DB)`. |
| B3 | P0 | `internal/product/service.go:87` | `fmt.Sprintf("%0.2f", req.Price)` dengan `Price string` → menghasilkan `%!f(string=...)`, `Scan` gagal, **create product selalu error 500**. Terdeteksi `go vet`. | `price.Scan(req.Price)` seperti di `Update`, dan bungkus error jadi `apperror.Validation`. |
| B4 | P0 | `internal/product/dto.go:55` | `Price: product.Price.Int.String()` mengabaikan `Exp`. Harga `13000000.00` (Int=1300000000, Exp=-2) dikirim sebagai `"1300000000"` — **data salah ke klien**. | Gunakan `product.Price.Value()` / `Float64Value()` atau format dengan `Exp`, idealnya kirim string desimal `"13000000.00"`. |
| B5 | P0 | `internal/tracing/error.go:13` | Kondisi terbalik: `if err != nil { return }` — **error tidak pernah direkam ke span**. | `if err == nil { return }` |
| B6 | P0 | `internal/product/handler.go:117-124` | `UpdateProduct` menulis response **dua kali** (encode manual lalu `httpx.WriteJSON`) → body ganda + warning `superfluous WriteHeader`. | Hapus baris 117–118. |
| B7 | P1 | `internal/platform/httpx/error.go:23` | `err.(*apperror.Error)` type assertion, bukan `errors.As` → apperror yang di-wrap (`fmt.Errorf("...: %w")`) jadi 500. Tidak konsisten dengan `logging.LogError` yang pakai `errors.As`. | Ganti ke `errors.As(err, &appErr)`. |
| B8 | P1 | `internal/product/handler.go:73`, `:102` | Handler memakai `json.NewDecoder(r.Body)` langsung, bukan `httpx.DecodeJSON` → body limit 1MB & `DisallowUnknownFields` tidak berlaku. | Gunakan `httpx.DecodeJSON` dan bungkus error-nya ke `apperror.Validation`. |
| B9 | P1 | `internal/product/handler.go:68-87` | `CreateProduct` tidak memanggil `req.Validate()`. | Panggil validasi seperti Update. |
| B10 | P1 | `internal/product/handler.go:106-108` | Semua error validasi Update dipetakan jadi `"invalid price"`, walaupun yang kosong `name`. | Kembalikan `apperror.Validation(err.Error())`. |
| B11 | P1 | `http/product.http:16` | Contoh request mengirim `price` sebagai number, sementara DTO `string` → decode gagal. Kontrak API ambigu. | Tetapkan satu kontrak (disarankan string desimal) dan dokumentasikan di OpenAPI. |

### K2 — Testing

| ID | Prio | Temuan | Rekomendasi |
|---|---|---|---|
| T1 | P0 | Tidak ada satupun `_test.go`. Bug B3–B6 akan tertangkap oleh test paling sederhana. | Minimal: table-driven test untuk `dto` (validasi + konversi harga), `apperror`, `httpx.DecodeJSON`/`WriteError`, middleware (recovery, request ID). |
| T2 | P1 | `Service` bergantung pada `*db.Queries` konkret → tidak bisa di-mock. | Definisikan interface kecil di sisi consumer (`package product`: `type store interface { GetProducts(...); ... }`), **atau** integration test dengan Postgres via testcontainers (disarankan untuk layer data). |
| T3 | P1 | Tidak ada integration/e2e test handler → DB. | `httptest` + testcontainers-go Postgres, jalankan migrasi sebelum test. |

### K3 — Arsitektur & Maintainability

**Yang sudah baik:** pemisahan `platform/` (httpx, apperror) dari domain `product/`; composition root tunggal di `app.New`; sqlc untuk type-safe query; `httpx.Handle` pattern (handler return error) yang bersih.

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| A1 | P2 | `internal/middleware/request_id.go:10-37` | `contextKey`, `requestIDKey`, `GetRequestID` duplikat dari `requestcontext` dan tidak dipakai (dead code, key berbeda → rawan salah pakai). | Hapus; pakai `requestcontext` saja. |
| A2 | P2 | `internal/middleware/logging.go:97` | `clientIP` duplikat dari `logging/request.go:34`, tidak dipakai. | Hapus. |
| A3 | P2 | `internal/requestcontext/error.go` | `WithError`/`GetError` tidak dipakai di mana pun. | Hapus sampai dibutuhkan. |
| A4 | P2 | `internal/platform/apperror/error.go:97` | `apperror.Is` hanya membungkus `errors.Is` tanpa nilai tambah. | Hapus, pakai `errors.Is`. |
| A5 | P2 | `internal/health/handler.go:48` | `writeJSON` duplikat `httpx.WriteJSON`. | Reuse `httpx.WriteJSON`. |
| A6 | P1 | `internal/product/service.go` | Inkonsistensi: hanya `FindAll`/`FindById` yang punya span & error mapping; `Create` mengembalikan error mentah; `Delete`/`Update` tanpa span. Span `FindById` salah nama (`ProductService.GetProducts`). | Seragamkan pola per method: span + mapping `pgx.ErrNoRows` → 404, lainnya → `apperror.Wrap(500)`. |
| A7 | P2 | `internal/product/handler.go`, `dto.go` | Tipe sqlc (`db.Product`) bocor ke handler. Untuk service sekecil ini masih bisa diterima. | Biarkan dulu; pisahkan domain model hanya jika logika bisnis bertambah. |
| A8 | P2 | `FindById` | Penamaan tidak idiomatik Go. | `FindByID`. |

### K4 — Error Handling & API Contract

**Yang sudah baik:** format error seragam `{"error":{"code","message"}}`, error internal disembunyikan dari klien, stack trace hanya di log.

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| E1 | P1 | `internal/product/service.go:106` | Pesan error menyertakan `err.Error()` dari parser pgtype → detail internal bocor ke klien. | Pesan statis `"invalid product price"`; detail cukup di log (`apperror.Wrap`). |
| E2 | P1 | `internal/product/dto.go:56-57` | `time.Time.String()` → format `2026-09-17 10:00:00 +0000 UTC`, bukan RFC 3339. | `Format(time.RFC3339)` atau langsung `time.Time` dengan JSON tag. |
| E3 | P1 | `dto.go` | Tidak ada validasi batas: panjang `name` ≤ 255 (kolom `VARCHAR(255)` → error DB jadi 500), harga negatif, presisi > 2 desimal, overflow `NUMERIC(15,2)`. | Validasi di DTO → 400, dan constraint di DB sebagai jaring pengaman. |
| E4 | P2 | Router | 404/405 dari chi mengembalikan plain text, tidak sesuai format error JSON. | `router.NotFound(...)` & `router.MethodNotAllowed(...)` pakai `httpx.WriteError`. |
| E5 | P2 | `middleware/recovery.go:40` | Jika panic terjadi setelah header ditulis, `WriteInternalServerError` memicu `superfluous WriteHeader`. | Terima sebagai known limitation, atau cek flag header via wrapper. |

### K5 — Konfigurasi & Secrets

**Yang sudah baik:** env-based, validasi fail-fast, `LOG_LEVEL` di-parse ketat.

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| C1 | P1 | `config.go:78-81`, `Validate()` | HTTP timeouts tidak divalidasi. Jika env kosong → `0` = **tanpa timeout** (rentan Slowloris). | Default aman (`viper.SetDefault`) + validasi `> 0`. |
| C2 | P1 | `config.go:59` | `OTEL_TRACE_SAMPLE_RATE` kosong → `0` → **tracing mati diam-diam**. | Set default (mis. `0.1` prod) dan log nilai efektif saat startup. |
| C3 | P1 | `database/postgres.go:18` | DSN dirakit dengan `Sprintf` tanpa escaping → password dengan `@`, `/`, `:` merusak DSN. | Gunakan `url.URL{User: url.UserPassword(...)}` atau key/value DSN. |
| C4 | P1 | `.env.example:16`, config | `DB_SSL_MODE=disable` dan tidak ada validasi per environment. | Validasi `APP_ENV=production` ⇒ `sslmode` ∈ {`require`,`verify-ca`,`verify-full`}. |
| C5 | P2 | `config.go:52` | Selalu membaca `.env` dari working directory, termasuk di production. Viper global state. | Baca `.env` hanya bila ada/dev; di prod secret dari env/secret manager (Vault, K8s Secret). |
| C6 | P2 | `config.go:62` | Return `&Config{}` + error, tidak konsisten dengan path lain (`nil`). Validasi sample rate dobel. | Return `nil`; hapus cek duplikat. |
| C7 | P2 | `AppConfig.Env` | Tidak divalidasi (typo `prodution` lolos). | Whitelist nilai. |

### K6 — Database & Data Integrity

**Yang sudah baik:** sqlc + pgx v5, pool dikonfigurasi, ping saat startup, UUID PK, `NUMERIC` untuk uang (bukan float).

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| D1 | P1 | `db/queries/product.sql:1-9` | `GetProducts` tanpa `LIMIT` → unbounded, akan membebani memori & DB seiring data tumbuh. | Pagination (keyset `created_at,id` disarankan, atau limit/offset dengan batas maks). |
| D2 | P1 | `migrations/0001...up.sql:5-6` | `TIMESTAMP` tanpa timezone. | `TIMESTAMPTZ`. |
| D3 | P1 | `up.sql` | Tidak ada `CHECK (price >= 0)`, `CHECK (length(trim(name)) > 0)`. | Tambah constraint. |
| D4 | P1 | Repo | Tidak ada mekanisme menjalankan migrasi (README berisi dokumentasi golang-migrate, bukan project). Tidak jelas kapan/siapa menjalankannya saat deploy. | Tambah target `make migrate-up` (golang-migrate CLI) dan jalankan sebagai job terpisah (init container / CI step) sebelum rollout. |
| D5 | P2 | Pool | Tidak ada `statement_timeout` / query timeout per request. Query lambat hanya dibatasi context request. | Set `statement_timeout` di `ConnConfig.RuntimeParams` atau context timeout per query. |
| D6 | P2 | `down.sql` | `DROP TABLE products;` tanpa `IF EXISTS`. | `DROP TABLE IF EXISTS products;` |
| D7 | P2 | `postgres.go:44` | Ping timeout hardcoded 5s, tanpa retry saat DB belum siap (umum di container orchestration). | Retry dengan backoff terbatas, atau biarkan crash-loop + readiness (terima jika platform K8s). |

### K7 — Observability

**Yang sudah baik:** JSON log dengan `service`, `env`, `request_id`, `trace_id`, `span_id`; access log dengan level berdasarkan status; `otelhttp` + `otelpgx` untuk auto-instrumentation; batch exporter; `ParentBased` sampler.

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| O1 | P0 | `tracing/error.go:13` | (= B5) error tidak pernah direkam ke span. | Lihat B5. |
| O2 | P1 | `tracing/tracing.go` | Tidak ada `otel.SetTextMapPropagator(...)` → header `traceparent` dari upstream **diabaikan**, trace terputus antar service. | `otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))` |
| O3 | P1 | `main.go:32` | Semua span HTTP bernama sama (`chi-product-api`) karena route pattern chi tidak dipakai. | Set nama span/atribut `http.route` dari `chi.RouteContext(r.Context()).RoutePattern()` setelah routing (middleware kecil). |
| O4 | P1 | `otel-collector.yaml:28-31` | Pipeline metrics hanya ke `debug` exporter → **metrics tidak tersimpan di mana pun**. | Tambah backend (Prometheus exporter / remote write, atau vendor). |
| O5 | P1 | `tracing.go:22`, `metric.go:22` | `WithInsecure()` hardcoded. | Konfigurable; TLS di production. |
| O6 | P2 | Resource | Hanya `service.name`. Tidak ada `service.version`, `deployment.environment`. | Tambah atribut; version dari `-ldflags` build. |
| O7 | P2 | `logging/request.go:21` | Log `query` string mentah → berpotensi mencatat PII/token. | Redaksi atau hapus. |
| O8 | P2 | `middleware/request_id.go:17` | `X-Request-ID` dari klien diterima tanpa validasi panjang/karakter → log injection / log bloat. | Validasi (mis. ≤ 64 char, `[A-Za-z0-9-]`), selain itu generate baru. |
| O9 | P2 | Access log | `/health` & `/ready` di-log dan di-trace setiap probe → noise & biaya. | Skip logging/tracing untuk probe endpoint. |
| O10 | P2 | `docker-compose.yml` | Image `latest`. | Pin versi. |

### K8 — Reliability & Lifecycle

**Yang sudah baik:** `http.Server` timeouts, signal handling SIGTERM/SIGINT, `server.Shutdown` lalu flush telemetry lalu close pool (urutan benar), liveness vs readiness terpisah, readiness cek DB dengan timeout 2s.

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| R1 | P1 | `app.go:82-84` | Urutan middleware: `RequestID → Recovery → Logging`. Panic di handler melewati `Logging` tanpa tercatat access log-nya (Logging berada **di dalam** Recovery). | `RequestID → Logging → Recovery`, sehingga access log tetap mencatat status 500 hasil recovery. |
| R2 | P1 | `main.go:80-95` | Tidak ada fase drain: saat SIGTERM, `/ready` tetap 200 hingga server ditutup → load balancer masih mengirim traffic. | Set flag shutting-down → `/ready` 503, tunggu beberapa detik (sesuai periode probe), baru `server.Shutdown`. |
| R3 | P2 | `main.go:83` | Timeout shutdown 10s hardcoded, dipakai bersama untuk server + telemetry. | Jadikan config; pastikan < `terminationGracePeriodSeconds`. |
| R4 | P2 | `main.go:19`, `metric.go:21`, `tracing.go:21` | Init memakai `context.Background()` tanpa timeout; exporter gRPC bersifat lazy sehingga OK, tapi `app.New` tidak punya batas waktu total. | Context dengan timeout untuk startup. |
| R5 | P2 | `main.go:71-78` | Jika server gagal start (port dipakai), aplikasi tetap exit dengan kode 0. | `os.Exit(1)` setelah cleanup pada path error. |

### K9 — Security

| ID | Prio | Lokasi | Temuan | Rekomendasi |
|---|---|---|---|---|
| S1 | P0 | `product/routes.go:20-22` | `GET /api/v1/products/panic` terekspos publik. | Hapus (pindahkan ke test). |
| S2 | P1 | Router | Tidak ada autentikasi/otorisasi untuk operasi write (POST/PUT/DELETE). | JWT/OIDC middleware atau API gateway; minimal pisahkan read vs write. |
| S3 | P1 | Router | Tidak ada rate limiting. | `httprate` (go-chi) atau di gateway/ingress. |
| S4 | P1 | Handler | Body limit tidak berlaku (B8). | Lihat B8, atau middleware global `http.MaxBytesHandler`. |
| S5 | P2 | Router | Tidak ada security headers (`X-Content-Type-Options: nosniff`, dll.) dan kebijakan CORS. | Middleware kecil / di ingress. |
| S6 | P1 | CI | Tidak ada `govulncheck` / dependency scanning / image scanning. | Tambah ke CI. |
| S7 | P1 | C3, C4, O5 | DSN tidak di-escape, SSL DB disable, OTLP insecure. | Lihat item terkait. |

### K10 — Performance & Scalability

| ID | Prio | Temuan | Rekomendasi |
|---|---|---|---|
| P1 | P1 | Unbounded list (D1). | Pagination. |
| P2 | P2 | Tidak ada baseline performa. | Load test sederhana (k6/vegeta) untuk menentukan `DB_MAX_CONNS`, timeouts, dan resource limit container. |
| P3 | — | Aplikasi stateless, pool terkonfigurasi. | ✅ Sudah siap horizontal scaling. |

### K11 — Delivery & Operasional

| ID | Prio | Temuan | Rekomendasi |
|---|---|---|---|
| X1 | P0 | Project bukan git repository. | `git init`, `.gitignore` (termasuk `.env`, `graphify-out/`). |
| X2 | P0 | Tidak ada `Dockerfile`. | Multi-stage build, `CGO_ENABLED=0`, base distroless/`scratch`, user non-root, inject version via `-ldflags`. |
| X3 | P1 | Tidak ada CI. | Pipeline: `gofmt`/`golangci-lint` → `go vet` → `go test -race` → `govulncheck` → `sqlc diff` (pastikan generated code sinkron) → build image → scan image. |
| X4 | P1 | `README.md` berisi dokumentasi golang-migrate, bukan project ini. | README project: arsitektur singkat, env vars, cara run lokal, migrasi, test. |
| X5 | P1 | `docker-compose.yml` hanya Jaeger + collector, tanpa Postgres & app. | Tambah `postgres` (dengan healthcheck), service `migrate`, dan `app` agar `docker compose up` bisa dipakai end-to-end. |
| X6 | P2 | Tidak ada `Makefile`/taskfile. | Target: `run`, `test`, `lint`, `sqlc`, `migrate-up/down`. |
| X7 | P2 | Tidak ada OpenAPI spec. | `openapi.yaml` sebagai kontrak (menyelesaikan ambiguitas B11). |
| X8 | P2 | Tidak ada runbook / SLO / alerting. | Definisikan SLO (mis. availability, p95 latency) dan alert berbasis metrics RED. |

---

## 5. Roadmap Perbaikan

### Fase 0 — Bisa jalan dengan benar (P0)
1. Perbaiki build: B1, B2.
2. Perbaiki bug fungsional: B3, B4, B5, B6.
3. Hapus endpoint panic: S1.
4. `git init` + `.gitignore`: X1.
5. Unit test minimal untuk area yang baru diperbaiki: T1.
6. Dockerfile: X2.

**Target skor setelah fase 0: ~2.8/5**

### Fase 1 — Siap production (P1)
1. Konsistensi error & kontrak API: B7–B11, E1–E3, A6.
2. Config aman: C1–C4.
3. Database: D1–D4 (pagination, `TIMESTAMPTZ`, constraint, proses migrasi).
4. Observability: O2–O5 (propagator, span name per route, backend metrics, TLS).
5. Reliability: R1, R2 (urutan middleware, readiness drain).
6. Security: S2, S3, S6.
7. Testing: T2, T3 (integration test dengan testcontainers).
8. Delivery: X3–X5 (CI, README, compose lengkap).

**Target skor setelah fase 1: ~4/5 (production ready)**

### Fase 2 — Hardening (P2)
Cleanup dead code (A1–A5), E4–E5, C5–C7, D5–D7, O6–O10, R3–R5, S5, P2, X6–X8.

---

## 6. Checklist Go-Live

- [ ] `go build ./...` dan `go vet ./...` bersih
- [ ] `go test -race ./...` hijau, termasuk integration test DB
- [ ] `golangci-lint` & `govulncheck` bersih di CI
- [ ] Endpoint debug (`/panic`) dihapus
- [ ] Create/Update/Get produk mengembalikan harga yang benar (test)
- [ ] Error di-record ke span; trace tersambung dari upstream (`traceparent`)
- [ ] Semua timeouts & sample rate punya default aman dan tervalidasi
- [ ] DB: `sslmode` aman, migrasi otomatis di pipeline, `TIMESTAMPTZ`, constraint, pagination
- [ ] Auth untuk write endpoint, rate limit aktif
- [ ] Readiness mengembalikan 503 saat shutdown sebelum server berhenti
- [ ] Docker image non-root, versi di-pin, ter-scan
- [ ] Metrics tersimpan di backend + dashboard RED + alert dasar
- [ ] README & OpenAPI sesuai implementasi
