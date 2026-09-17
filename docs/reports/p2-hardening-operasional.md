# Laporan P2: Hardening & Kesiapan Operasional

- Tanggal: 2026-09-17
- Referensi: [`docs/architecture-production-ready.md`](../architecture-production-ready.md) (Fase 2), [`p0-perbaikan-blocker.md`](p0-perbaikan-blocker.md), [`p1-production-hardening.md`](p1-production-hardening.md), [`p1-rate-limit-client-ip.md`](p1-rate-limit-client-ip.md)
- Status: ✅ Semua item P2 selesai. Skor **4.4 / 5**; semua kriteria kritis (★) sudah ≥ 4.

## 1. Audit Ulang P0 & P1

Sebelum mengerjakan P2, semua item P0 dan P1 diperiksa ulang langsung di kode saat ini, bukan berdasarkan laporan sebelumnya.

| Pemeriksaan | Hasil |
|---|---|
| `go build`, `go vet`, `gofmt` | Bersih |
| `go test -race ./...` | 7 package ok |
| Integration test (Postgres 17 sekali pakai) | Lolos |
| `sqlc diff`, `docker compose config` | Bersih |
| `staticcheck` (semua check) | 1 temuan: `clientIP` tidak terpakai di `internal/middleware/logging.go` (= item P2 A2) |
| Pemeriksaan per item di kode (45 item P0 + P1) | **45/45 terpenuhi** |

Catatan: pemeriksaan otomatis sempat menandai B9 gagal. Ternyata jendela grep-nya terlalu pendek. Fungsi `decodeProductRequest` memang memanggil `req.Validate()`, dan test `create_blank_name` lolos.

## 2. Item yang Dikerjakan

### K3 — Arsitektur & Maintainability

| ID | Perbaikan | File |
|---|---|---|
| A1 | Hapus `contextKey`, `requestIDKey`, `GetRequestID` duplikat di middleware; sumber tunggal di `requestcontext` | `internal/middleware/request_id.go` |
| A2 | Hapus `clientIP` yang tidak terpakai | `internal/middleware/logging.go` |
| A3 | Hapus `requestcontext/error.go` (`WithError`/`GetError` tidak dipakai) | — |
| A4 | Hapus `apperror.Is` | `internal/platform/apperror/error.go` |
| A5 | Health handler sudah memakai `httpx.WriteJSON` sejak P1 | — |
| A7 | Tidak diubah, sesuai rekomendasi review (sqlc model di handler masih wajar untuk domain sekecil ini) | — |
| A8 | `FindByID` sudah sejak P1 | — |
| (temuan P1) | Wiring router dipisah ke `newRouter()` sehingga urutan middleware, route, dan error handler bisa diuji tanpa DB/OTLP | `internal/app/app.go`, `internal/app/router_test.go` |

### K4 — Error Handling & API Contract

| ID | Perbaikan | File |
|---|---|---|
| E4 | 404 → `NOT_FOUND`, 405 → `METHOD_NOT_ALLOWED` dalam format error JSON | `internal/app/app.go` |
| E5 | `Recovery` melacak apakah header sudah terkirim. Jika sudah, response tidak ditulis ulang (log mencatat `headers_already_sent`). `http.ErrAbortHandler` di-panic ulang sesuai kontrak `net/http`. | `internal/middleware/recovery.go` |
| X7 | `openapi.yaml` (OpenAPI 3.1): semua endpoint, skema, security API key, dan kode error. Lint dengan Redocly 2.3.0 (`redocly.yaml` mematikan 3 rule dengan alasan tertulis). | `openapi.yaml`, `redocly.yaml` |

### K5 — Konfigurasi

| ID | Perbaikan | File |
|---|---|---|
| C5 | `.env` hanya dibaca jika `APP_ENV` (dari environment asli) kosong atau `development`. File `.env` yang tidak ada tidak dianggap error, tapi error baca lainnya dilaporkan. Viper global diganti instance lokal `viper.New()`. | `internal/config/config.go` |
| C6, C7 | Sudah selesai di P1 (`Load` mengembalikan `nil` saat error; whitelist `APP_ENV`) | — |

### K6 — Database

| ID | Perbaikan | File |
|---|---|---|
| D5 | `DB_STATEMENT_TIMEOUT` (default 5s) dikirim sebagai runtime param `statement_timeout` ke setiap koneksi. Divalidasi harus < `APP_WRITE_TIMEOUT`. | `internal/database/postgres.go`, `internal/config/config.go` |
| D6 | `DROP TABLE IF EXISTS products;` | `db/migrations/0001_create_product_migration.down.sql` |
| D7 | Retry ping saat startup dengan exponential backoff (250ms → maks 5s), dibatasi `DB_CONNECT_TIMEOUT` (default 30s) | `internal/database/postgres.go` |

### K7 — Observability

| ID | Perbaikan | File |
|---|---|---|
| O6 | `var version` diisi lewat `-ldflags -X main.version` (`ARG VERSION` di Dockerfile, `github.sha` di CI). Masuk ke resource `service.version`, ke log startup, dan ke label metrics `service_version` + `deployment_environment_name` (via `resource_constant_labels` di collector). | `cmd/server/main.go`, `internal/tracing/tracing.go`, `internal/metrics/metric.go`, `Dockerfile`, `otel-collector.yaml` |
| O7 | Query string mentah tidak lagi dicatat di log | `internal/logging/request.go` |
| O8 | `X-Request-ID` dari klien hanya dipakai jika cocok `^[A-Za-z0-9._-]{1,64}$`; selain itu diganti UUID baru (mencegah log injection) | `internal/middleware/request_id.go` |
| O9 | Probe `/health` dan `/ready` tidak di-trace (`otelhttp.WithFilter`) dan hanya di-log saat gagal, di level `WARN` (bukan `ERROR`) | `internal/app/app.go`, `internal/middleware/logging.go` |
| O10 | Image di compose sudah di-pin sejak P1 | — |
| — | Wrapper `responseWriter` mendapat `Unwrap()` sehingga `http.ResponseController` (Flush, deadline) tetap berfungsi melewati middleware | `internal/middleware/logging.go` |

### K8 — Reliability

| ID | Perbaikan | File |
|---|---|---|
| R3 | `APP_SHUTDOWN_TIMEOUT` (default 10s), divalidasi > 0 | `internal/config/config.go`, `cmd/server/main.go` |
| R4 | Satu-satunya langkah startup yang bisa menggantung (koneksi DB) kini dibatasi `DB_CONNECT_TIMEOUT`. Exporter OTLP bersifat lazy. | `internal/database/postgres.go` |
| R5 | `main` → `run() int`. Exit 1 jika server gagal (mis. port dipakai) atau shutdown HTTP gagal. Kegagalan flush telemetry hanya dicatat di log (lihat bagian 3). | `cmd/server/main.go` |

### K9 — Security

| ID | Perbaikan | File |
|---|---|---|
| S5 | Middleware `SecurityHeaders`: `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, `Content-Security-Policy: default-src 'none'; frame-ancestors 'none'`, `Cache-Control: no-store`. HSTS sengaja diserahkan ke TLS terminator. **CORS sengaja tidak ditambahkan**: tanpa header CORS, browser menolak request lintas origin, dan itu default yang aman sampai ada klien browser. | `internal/middleware/security.go` |

### K10 — Performance

| ID | Perbaikan | File |
|---|---|---|
| P2 | Skrip k6 dengan skenario 90% baca (list/get) dan 10% tulis, arrival rate konstan, plus threshold (error < 1%, p95 baca < 300ms, p95 tulis < 500ms). Hasil di bagian 4. | `loadtest/products.js`, `Makefile` |

### K11 — Delivery & Operasional

| ID | Perbaikan | File |
|---|---|---|
| X6 | Target baru: `openapi-lint`, `alerts-test`, `loadtest` | `Makefile` |
| X7 | Lihat K4 | — |
| X8 | SLO (availability 99.9%, p95 < 300ms), 3 recording rule + 4 alert (`HighErrorRate` berbasis burn rate, `HighLatency`, `NoTraffic`, `TelemetryPipelineDown`), dan unit test alert via `promtool`. Rules dimuat Prometheus di compose. Runbook berisi langkah per alert, deploy, shutdown, dan rotasi API key. | `prometheus-alerts.yml`, `prometheus-alerts_test.yml`, `prometheus.yml`, `docker-compose.yml`, `docs/runbook.md` |
| — | CI: job baru `contracts` (lint OpenAPI + `promtool check/test rules`); build image membawa `VERSION` | `.github/workflows/ci.yml` |

## 3. Masalah yang Ditemukan Saat Verifikasi & Diperbaiki

| Temuan | Perbaikan |
|---|---|
| **SIGTERM menghasilkan exit code 1** ketika collector OTLP tidak bisa dihubungi, karena error flush telemetry ikut mengubah exit code. Orchestrator akan membaca shutdown normal sebagai crash. | Error `application.Shutdown` (flush telemetry) hanya dicatat di log. Diverifikasi: SIGTERM → exit 0, baik saat collector mati maupun hidup. |
| `target_info` tidak diekspos exporter Prometheus di collector, jadi `service.version` tidak terlihat di metrics (di trace sudah ada). | Tambah `resource_constant_labels.included` di exporter `prometheus`. Format opsinya ditemukan lewat `otelcol validate`, lalu diverifikasi label muncul di `/metrics`. |
| Query `pg_settings` pertama saya untuk memverifikasi `statement_timeout` keliru: yang terbaca adalah sesi `psql` sendiri, bukan sesi app. | Diganti integration test yang membuka pool via `NewPostgresPool`, memeriksa `SHOW statement_timeout` = `1234ms`, dan memastikan `pg_sleep(2)` dibatalkan. |
| `license: Proprietary` sempat saya tulis di `openapi.yaml` tanpa dasar. | Dihapus. Rule `info-license` dimatikan dengan catatan bahwa lisensi belum diputuskan. |

## 4. Bukti Verifikasi

### Otomatis

```
$ go build ./... && go vet ./... && gofmt -l cmd internal        # bersih
$ staticcheck ./...                                              # bersih (semua check)
$ TEST_DATABASE_URL=... go test -race -coverprofile ./...        # 9 package ok, total coverage 65.5%
$ govulncheck ./...                                              # No vulnerabilities found.
$ sqlc diff                                                      # bersih
$ make openapi-lint                                              # valid
$ make alerts-test                                               # SUCCESS: 7 rules found; unit test SUCCESS
$ trivy image (HIGH,CRITICAL)                                    # debian 0, gobinary 0
```

| Package | Coverage | Test baru P2 |
|---|---|---|
| `app` | 34.5% | 404/405/401/400 berformat JSON; security headers; `X-Request-ID` tidak valid diganti; rate limit hanya di API (probe tidak); probe sukses tidak di-log, probe gagal di-log WARN |
| `config` | 96.4% | `Load()` tanpa `.env`; `.env` dibaca di development dan diabaikan di production; `DB_STATEMENT_TIMEOUT` < `APP_WRITE_TIMEOUT` |
| `database` | 90.7% | Retry ping sukses di percobaan ke-3; menyerah saat deadline; **integration**: `statement_timeout` aktif dan membatalkan query |
| `middleware` | 87.0% | Validasi `X-Request-ID` (5 case termasuk newline injection); recovery tidak menulis ulang response yang sudah terkirim; `ErrAbortHandler` diteruskan; `ResponseController.Flush` lewat wrapper |

### Stack Compose (`chip2verify`, dihapus setelah selesai)

| Skenario | Hasil |
|---|---|
| `GET /nope`, `PATCH /api/v1/products` | `{"error":{"code":"NOT_FOUND",...}}`, `{"error":{"code":"METHOD_NOT_ALLOWED",...}}` |
| `GET /health` dengan `X-Request-ID: evil id` | Header keamanan lengkap; `X-Request-Id` diganti UUID |
| Operasi di Jaeger setelah memanggil probe | Tidak ada `/health` maupun `/ready` |
| Proses resource di Jaeger | `service.version=p2-verify`, `deployment.environment.name=staging` |
| Label metrics di collector | `service_version="p2-verify"`, `deployment_environment_name="staging"` |
| Prometheus `/api/v1/rules` | 7 rule, semua `health: ok` |
| `docker stop` app | Exit code 0 |

### Binary Lokal

| Skenario | Hasil |
|---|---|
| Instance kedua di port yang sama | Exit 1, log `server failed` |
| SIGTERM, collector tidak dapat dihubungi | Exit 0 (sebelum perbaikan: 1) |
| DB tidak dapat dihubungi, `DB_CONNECT_TIMEOUT=2s` | Exit 1 setelah ~2s: `gave up: context deadline exceeded` |
| Build dengan `-X main.version=local-test` | Log `"version":"local-test"` |

### Load Test k6 (baseline)

Konfigurasi: stack compose lokal, `DB_MAX_CONNS=10`, rate limit dinaikkan, 90% baca / 10% tulis. App, Postgres, dan k6 berjalan di satu laptop (8 core, 15 GB).

| Target rate | Durasi | Request | Error | p95 baca | p95 tulis | Maks |
|---|---|---|---|---|---|---|
| 200/s | 60s | 12.201 | 0% | 1.67ms | 4.15ms | 8.8ms |
| 1000/s | 30s | 30.201 | 0% | 1.81ms | 4.73ms | 56.9ms |
| 3000/s | 30s | 90.161 | 0% | 3.60ms | 7.95ms | 58.1ms |

Pada 3000/s, k6 mencatat 41 dropped iteration. Itu batas VU di sisi generator beban, bukan error dari app. **Titik jenuh app belum tercapai.** Angka ini hanya baseline untuk membandingkan antar versi, bukan estimasi kapasitas production, karena generator beban, app, dan DB berbagi host yang sama tanpa latensi jaringan.

## 5. ⚠️ Perubahan Perilaku

| Perubahan | Dampak |
|---|---|
| `.env` diabaikan jika `APP_ENV` = `staging`/`production` di environment | Deployment yang selama ini bergantung pada file `.env` harus memindahkan nilainya ke env/secret manager |
| Env baru dengan default: `APP_SHUTDOWN_TIMEOUT=10s`, `DB_CONNECT_TIMEOUT=30s`, `DB_STATEMENT_TIMEOUT=5s` | Query > 5s kini dibatalkan Postgres (`PRODUCT_*_FAILED` 500) |
| Header `Cache-Control: no-store` di semua respons | Tidak ada caching HTTP; ubah per route jika nanti butuh caching |
| `X-Request-ID` non-alfanumerik dari upstream diganti | Korelasi dengan sistem upstream yang memakai format lain akan putus; perluas regex jika perlu |
| `App.Router` → `App.Handler` (sudah dibungkus otelhttp) dan `app.New(ctx, version)` | Hanya berdampak ke kode yang memanggil `app.New` langsung (saat ini hanya `main`) |
| `otel-collector.yaml` memakai `resource_constant_labels` | Butuh collector yang mendukung opsi ini (terverifikasi di 0.160.0). Container collector yang sedang berjalan di mesin dev memakai file yang sama dan akan memakai konfigurasi baru saat di-restart. |

## 6. Yang Belum Terverifikasi / Keputusan Terbuka

- **Workflow CI belum pernah dijalankan di GitHub** karena repo belum punya remote. Semua perintahnya sudah dijalankan manual secara lokal.
- **Migrasi `0002` belum diterapkan ke DB dev lokal** (`localhost:5432`).
- **Strategi auth**: API key vs JWT/OIDC (dari P1).
- **`LICENSE`** masih berisi lisensi golang-migrate.
- **Rate limit per instance** (in-memory) didokumentasikan sebagai batasan di runbook. Belum ditambahkan backend terdistribusi (Redis) karena jumlah replika belum diketahui.
- **Alertmanager** tidak disertakan di compose; routing notifikasi tergantung platform monitoring tiap environment.
- **Kontrak `openapi.yaml` vs implementasi** belum diuji otomatis (tidak ada test yang memvalidasi respons terhadap skema). Saat ini spesifikasi ditulis manual berdasarkan kode.

## 7. Skor Setelah P2

| # | Kriteria | P1 | P2 | Catatan |
|---|---|---|---|---|
| K1 ★ | Build & Correctness | 4.5 | **5** | staticcheck semua check bersih |
| K2 ★ | Testing | 3.5 | **4** | Wiring router & DB diuji, alert rules diuji, coverage 65.5%. Belum 4.5: CI belum pernah jalan; tidak ada contract test OpenAPI |
| K3 | Arsitektur & Maintainability | 3.5 | **4.5** | Dead code bersih |
| K4 ★ | Error Handling & API Contract | 4 | **4.5** | 404/405 JSON, OpenAPI |
| K5 | Konfigurasi & Secrets | 4 | **4.5** | `.env` tidak dipakai di production |
| K6 ★ | Database & Data Integrity | 4 | **4.5** | statement timeout, retry koneksi |
| K7 | Observability | 4 | **4.5** | versi di traces & metrics, probe tidak berisik |
| K8 ★ | Reliability & Lifecycle | 4 | **4.5** | exit code benar, timeout shutdown dapat dikonfigurasi |
| K9 ★ | Security | 3.5 | **4** | Security headers, validasi request ID. Belum 4.5: auth berbasis API key |
| K10 | Performance & Scalability | 3.5 | **4** | Baseline load test. Belum 4.5: rate limit per instance, belum diuji di infrastruktur nyata |
| K11 ★ | Delivery & Operasional | 4 | **4.5** | OpenAPI, runbook, SLO, alerts |
| | **Rata-rata** | **3.9** | **4.4** | |

**Kesimpulan:** berdasarkan kriteria di review arsitektur (semua ≥ 3, semua ★ ≥ 4), layanan ini **memenuhi syarat production ready**, dengan tiga syarat sebelum go-live:
1. CI dijalankan di remote dan hijau.
2. Migrasi `0002` diterapkan di setiap environment sebelum rollout.
3. Keputusan auth (API key vs JWT/OIDC) dan `TRUSTED_PROXIES` disesuaikan dengan topologi deploy.
