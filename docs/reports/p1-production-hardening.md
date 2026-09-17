# Laporan P1: Production Hardening

- Tanggal: 2026-09-17
- Referensi: [`docs/architecture-production-ready.md`](../architecture-production-ready.md) (Fase 1), laporan sebelumnya [`p0-perbaikan-blocker.md`](p0-perbaikan-blocker.md)
- Status: ✅ Semua item P1 selesai. ⚠️ Ada breaking change (lihat bagian khusus di bawah)

## Ringkasan

Semua item P1 dari review arsitektur sudah dikerjakan, ditambah satu temuan dari laporan P0 (log 4xx di level ERROR). Dua masalah baru ditemukan dan langsung diperbaiki selama verifikasi: CVE HIGH pada gRPC dan konfigurasi collector yang deprecated.

Verifikasi dilakukan di stack Docker Compose terpisah (project `chip1verify`, dihapus di akhir) supaya tidak menyentuh DB dan container dev yang sedang berjalan:

- `go build`, `go vet`, `gofmt`, `go test -race`: bersih.
- Integration test ke Postgres 17: lolos.
- Migrasi up → down → up: bersih.
- `govulncheck`: 0 vulnerability yang terpanggil dari kode.
- Trivy image scan (HIGH/CRITICAL): 0 temuan.
- Uji end-to-end: auth, pagination, validasi, rate limit, propagasi trace, metrics di Prometheus, span per route di Jaeger, dan readiness drain. Semua sesuai.

**Skor naik dari ~2.3 ke 3.9 / 5.** Semua kriteria sudah ≥ 3, tapi dua kriteria kritis (K2 Testing dan K9 Security) masih 3.5, di bawah target 4. Detail di bagian Skor.

## ⚠️ Breaking Changes

| Perubahan | Dampak | Tindakan |
|---|---|---|
| Migrasi baru `0002_harden_products` (TIMESTAMPTZ, CHECK constraint, index) | App baru membaca kolom sebagai `timestamptz`. DB yang belum dimigrasi tidak didukung. | Jalankan `make migrate-up DATABASE_URL=...` sebelum deploy. Migrasi ini akan **gagal** jika ada baris dengan `price < 0` atau `name` kosong; bersihkan datanya dulu. |
| `GET /api/v1/products` sekarang mengembalikan `{"data":[...],"next_cursor":...}`, bukan array | Klien yang mengharapkan array akan rusak | Update klien |
| `price` wajib string desimal (`"12500.00"`), number ditolak 400 | Klien yang mengirim number akan rusak | Update klien |
| `created_at`/`updated_at` dalam format RFC 3339 (`2026-09-17T10:00:00Z`) | Parser tanggal di klien | Update klien |
| POST/PUT/DELETE wajib header `X-API-Key` di `staging`/`production` | Tanpa key: 401 | Set `API_KEYS` dan distribusikan key |
| Env baru dan aturan baru, di antaranya `OTEL_EXPORTER_OTLP_ENDPOINT` kini wajib diisi, dan `DB_SSL_MODE` default `require` | App gagal start jika env lama tidak memenuhi | Samakan `.env` dengan `.env.example` |
| Collector: exporter `otlp/jaeger` diganti `otlp_grpc/jaeger` | Butuh collector versi baru (terverifikasi di 0.160.0) | Pin image sudah diperbarui di compose |

## Item yang Dikerjakan

### K1 — Build & Correctness

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| B7 | `WriteError` memakai type assertion → apperror yang di-wrap jadi 500 | `errors.As` | `internal/platform/httpx/error.go` |
| B8 | Handler tidak memakai `DecodeJSON` (tanpa body limit & `DisallowUnknownFields`) | Semua write handler lewat `decodeProductRequest` → `httpx.DecodeJSON`. Body > 1MB → `413 REQUEST_BODY_TOO_LARGE` | `internal/product/handler.go`, `internal/platform/httpx/request.go` |
| B9 | Create tidak memanggil `Validate()` | Create dan Update memakai satu jalur validasi | `internal/product/handler.go` |
| B10 | Semua error validasi Update dipetakan jadi "invalid price" | `Validate()` mengembalikan `apperror.Validation` dengan pesan per field | `internal/product/dto.go` |
| B11 | Kontrak tipe `price` ambigu | Ditetapkan string desimal, didokumentasikan di README dan `http/product.http` | `internal/product/dto.go`, `README.md`, `http/product.http` |

### K2 — Testing

| ID | Perbaikan | File |
|---|---|---|
| T2 | Tidak menambah interface baru. Test handler memakai fake `db.DBTX` (interface bawaan sqlc), jadi service tetap bisa diuji tanpa DB. | `internal/product/handler_test.go` |
| T3 | Integration test ke Postgres sungguhan via `TEST_DATABASE_URL`. Setiap test membuat schema `it_<timestamp>`, menjalankan semua migrasi up, lalu `DROP SCHEMA ... CASCADE`, jadi aman dijalankan ke DB mana pun. Di-skip jika env tidak diset. | `internal/product/integration_test.go` |

### K3 — Arsitektur

| ID | Perbaikan | File |
|---|---|---|
| A6 | Semua method service seragam: span dengan nama benar (`ProductService.List/FindByID/Create/Update/Delete`), `pgx.ErrNoRows` → 404, error lain → `apperror.Wrap(500)` + dicatat di span. Helper `notFoundOr` dan `failed` menghilangkan duplikasi. `FindById` diganti `FindByID`; `CreateProductRequest`/`UpdateProductRequest` yang identik digabung jadi `ProductRequest`. | `internal/product/service.go`, `internal/product/dto.go` |

### K4 — Error Handling & API Contract

| ID | Perbaikan |
|---|---|
| E1 | Pesan error harga statis (`"invalid product price"`); detail parser hanya disimpan di field `cause` pada log. |
| E2 | Timestamp `UTC().Format(time.RFC3339)`. |
| E3 | Validasi: `name` wajib dan ≤ 255 karakter (dihitung per rune/UTF-8); `price` cocok dengan `^\d{1,13}(\.\d{1,2})?$` (non-negatif, ≤ 2 desimal, muat di `NUMERIC(15,2)`). Nama di-trim sebelum disimpan. |
| (temuan P0) | `LogError`: 4xx dicatat di level `WARN` tanpa stack trace; 5xx tetap `ERROR` dengan stack trace. File: `internal/logging/error.go`. |

### K5 — Konfigurasi

File: `internal/config/config.go`, `.env.example`

| ID | Perbaikan |
|---|---|
| C1 | Default aman untuk timeout HTTP (5s/10s/10s/60s) dan validasi `> 0`. |
| C2 | `OTEL_TRACE_SAMPLE_RATE` default `0.1`; nilai efektif dicatat di log saat startup (`configuration loaded`). |
| C3 | DSN dirakit dengan `url.URL` + `url.UserPassword` + `net.JoinHostPort`, jadi password dengan karakter khusus dan host IPv6 aman. File: `internal/database/postgres.go`. |
| C4 | Aturan per environment: `APP_ENV` hanya boleh `development`/`staging`/`production`; di luar development wajib `API_KEYS`; di production `DB_SSL_MODE` wajib `require`/`verify-ca`/`verify-full` dan OTLP tidak boleh insecure. |
| — | `Validate()` sekarang mengumpulkan **semua** error sekaligus (`errors.Join`), jadi semua kesalahan konfigurasi terlihat dalam satu kali start. |

### K6 — Database

| ID | Perbaikan | File |
|---|---|---|
| D1 | Pagination keyset `(created_at, id) DESC`; mengambil `limit+1` baris untuk menentukan `next_cursor`. Cursor berupa base64url dari `unixmicro:uuid` (presisi mikrodetik sama dengan Postgres). `limit` 1–100, default 20. | `db/queries/product.sql`, `internal/product/{dto,service,handler}.go` |
| D2 | `TIMESTAMP` → `TIMESTAMPTZ`; data lama diinterpretasikan sebagai UTC. | `db/migrations/0002_harden_products.up.sql` |
| D3 | `CHECK (price >= 0)`, `CHECK (length(btrim(name)) > 0)`, dan index `(created_at DESC, id DESC)` untuk pagination. | idem |
| D4 | Migrasi dijalankan oleh service `migrate` di compose (app menunggu migrasi selesai), `make migrate-up/down` via image `migrate/migrate:v4.20.1`, dan di CI dijalankan up → down → up. | `docker-compose.yml`, `Makefile`, `.github/workflows/ci.yml` |

Migrasi `0001` tidak diubah karena kemungkinan sudah diterapkan di DB yang ada.

### K7 — Observability

| ID | Perbaikan | File |
|---|---|---|
| O2 | `otel.SetTextMapPropagator(TraceContext + Baggage)`, sehingga header `traceparent` dari upstream diteruskan. | `internal/tracing/tracing.go` |
| O3 | Middleware `RouteTag`: setelah routing, span diberi nama `GET /api/v1/products/{id}`, atribut `http.route` ditambahkan, dan label route dipasang ke metrics otelhttp via `Labeler`. | `internal/middleware/route.go` |
| O4 | Collector mengekspor metrics ke exporter `prometheus` (`:8889`), lalu discrape oleh service Prometheus (`:9090`). | `otel-collector.yaml`, `prometheus.yml`, `docker-compose.yml` |
| O5 | `OTEL_EXPORTER_OTLP_INSECURE` bisa dikonfigurasi (default `true` hanya di development). Resource juga mendapat atribut `deployment.environment.name`, dipakai bersama oleh traces dan metrics. | `internal/tracing/tracing.go`, `internal/metrics/metric.go` |

### K8 — Reliability

| ID | Perbaikan | File |
|---|---|---|
| R1 | Urutan middleware sekarang `RequestID → Logging → RouteTag → Recovery`, jadi panic yang di-recover tetap tercatat di access log dengan status 500. | `internal/app/app.go` |
| R2 | Saat SIGTERM: `Health.StartDraining()` membuat `/ready` langsung 503, lalu tunggu `APP_SHUTDOWN_DRAIN_DELAY` (default 5s), baru `server.Shutdown`. | `internal/health/handler.go`, `cmd/server/main.go` |

### K9 — Security

| ID | Perbaikan | File |
|---|---|---|
| S2 | Middleware `APIKey` untuk POST/PUT/DELETE (header `X-API-Key`). Mendukung banyak key untuk rotasi. Perbandingan constant-time atas hash SHA-256, jadi panjang key tidak bocor lewat timing. GET tetap publik. | `internal/middleware/auth.go`, `internal/product/routes.go` |
| S3 | Rate limit per IP dengan `go-chi/httprate` v0.16.0 di `/api/v1` (probe `/health` & `/ready` tidak dibatasi). Melebihi batas → `429 RATE_LIMITED` dalam format error JSON. | `internal/app/app.go` |
| S4 | Body limit 1MB berlaku di semua write endpoint (lihat B8). | — |
| S6 | `govulncheck` v1.8.0 dan Trivy 0.66.0 (HIGH/CRITICAL, fixed-only) dijalankan di CI. | `.github/workflows/ci.yml`, `Makefile` |
| S7 | Dicakup oleh C3, C4, dan O5. | — |

### K10 — Performance

| ID | Perbaikan |
|---|---|
| P1 | Dicakup D1 (pagination + index). |

### K11 — Delivery

| ID | Perbaikan | File |
|---|---|---|
| X3 | CI GitHub Actions dengan 4 job. **lint**: gofmt, vet, govulncheck. **sqlc**: `sqlc diff`. **test**: Postgres service, migrasi up/down/up, `go test -race` termasuk integration, coverage. **image**: build + Trivy, hanya jalan jika tiga job lain lolos. | `.github/workflows/ci.yml` |
| X4 | README project menggantikan README golang-migrate: arsitektur, API & kontrak data, cara run, konfigurasi, development. | `README.md` |
| X5 | Compose lengkap: `postgres` (healthcheck, port host 55432 agar tidak bentrok dengan Postgres lokal), `migrate`, `app`, `otel-collector`, `jaeger`, `prometheus`. Semua image di-pin. | `docker-compose.yml` |

## Masalah Baru yang Ditemukan & Diperbaiki

| Temuan | Perbaikan |
|---|---|
| **CVE-2026-84445 (HIGH)** di `google.golang.org/grpc` v1.83.1 (dependency tidak langsung dari exporter OTLP), terdeteksi Trivy. `govulncheck` menyatakan jalur kodenya tidak terpanggil, tapi image scanner tetap menandainya. | Upgrade ke v1.83.2. Trivy sekarang 0 temuan. |
| Collector 0.160.0 memberi warning deprecation: alias exporter `otlp` dan `resource_to_telemetry_conversion`. | Exporter diganti `otlp_grpc/jaeger`; opsi deprecated dihapus (atribut resource tetap tersedia lewat metric `target_info`). Setelah restart: 0 warning, trace tetap masuk. |

## Test

| Package | Test baru/diubah | Coverage |
|---|---|---|
| `product` | Validasi write endpoint (9 case: harga invalid/negatif/number, nama kosong, unknown field, body kosong, body > 1MB, id invalid), query list invalid, `ProductRequest.Validate` (12 case), cursor round-trip + cursor rusak, RFC 3339. Integration: lifecycle lengkap + pagination 2 halaman + constraint DB. | 58.4% (unit saja) |
| `middleware` | API key (5 case + disabled), nama span dari route pattern, panic tetap tercatat di access log sebagai 500 | 84.5% |
| `config` | `Validate` per environment (6 case), `splitList` | 45.5% |
| `logging` | Level log berdasarkan status (WARN untuk 4xx, ERROR untuk 5xx/unknown) | 87.5% |
| `httpx` | `WriteError` meng-unwrap apperror; error tak dikenal tidak membocorkan detail | 36.5% |
| `health` | `/ready` 503 saat draining | 33.3% |

## Bukti Verifikasi

```
$ go build ./... && go vet ./... && test -z "$(gofmt -l cmd internal)"   # OK
$ go test -race ./...                                                     # 7 package ok
$ make test-integration DATABASE_URL=postgres://...:55433/...
--- PASS: TestIntegrationProductLifecycle
--- PASS: TestIntegrationDatabaseConstraints
$ migrate down -all && migrate up                                         # 2/d, 1/d, 1/u, 2/u
$ make vuln          # Your code is affected by 0 vulnerabilities.
$ trivy image ...    # debian: 0, gobinary: 0 (setelah upgrade grpc)
$ sqlc diff          # bersih
$ docker compose config -q                                                # OK
```

Uji end-to-end di stack compose (`APP_ENV=staging`, `API_KEYS=verify-key`, `RATE_LIMIT_REQUESTS_PER_MINUTE=30`):

| Skenario | Hasil |
|---|---|
| `POST` tanpa `X-API-Key` | `401 UNAUTHORIZED` |
| 3 × `POST` dengan key, lalu `GET ?limit=2` | 2 item terbaru + `next_cursor` |
| `GET ?limit=2&cursor=...` | 1 item sisa, `next_cursor: null` |
| `POST` `"price":"1.999"` | `400 VALIDATION_ERROR` dengan pesan jelas |
| Request dengan `traceparent: 00-4bf92f35...` | Log app mencatat `trace_id = 4bf92f3577b34da6a3ce929d0e0e4736` (trace upstream diteruskan) |
| 30 request beruntun (limit 30/menit, termasuk request sebelumnya) | 22 × 200, 8 × 429 |
| `/health` setelah kena rate limit | 200 (probe tidak dibatasi) |
| Prometheus `http_server_request_duration_seconds_count` | Label `http_route` = `/api/v1/products` dan `/api/v1/products/{id}` |
| Jaeger operations | `GET /api/v1/products`, `GET /api/v1/products/{id}`, `ProductService.List`, `ProductService.FindByID` |
| `docker stop` (SIGTERM, drain 1s) sambil polling `/ready` | `200` → `503` ×6 → koneksi ditutup; log `server stopped` |

## Yang Belum Terverifikasi

- **Workflow GitHub Actions belum pernah dijalankan** karena repo belum punya remote. Yang sudah dicek: YAML valid, tag action (`checkout@v7`, `setup-go@v7`, `setup-sqlc@v5`) dan image (`trivy:0.66.0`, `migrate:v4.20.1`, `postgres:17.6-alpine`) memang ada, dan setiap perintahnya sudah dijalankan manual secara lokal.
- **Migrasi `0002` belum diterapkan ke DB dev lokal** (`localhost:5432`). Saya sengaja tidak menyentuhnya. Jalankan `make migrate-up DATABASE_URL=...` sebelum menjalankan app secara lokal.

## Temuan Baru (belum dikerjakan)

| Temuan | Prioritas usulan |
|---|---|
| Probe `/ready` yang mengembalikan 503 (saat drain atau DB down) tercatat di level `ERROR` dan ikut di-trace, sehingga berisik. Sudah tercakup di O9. | P2 |
| Rate limit memakai `RemoteAddr`. Di belakang load balancer semua klien akan berbagi satu IP. Perlu `KeyByRealIP` dengan daftar proxy tepercaya sesuai infrastruktur. | P1 saat deploy di belakang LB |
| `LICENSE` berisi lisensi MIT milik golang-migrate (Matthias Kadenbach), bukan milik project ini. | Keputusan pemilik repo |
| Tidak ada test untuk `app.New` (wiring router). Urutan middleware hanya diuji lewat komposisi yang sama di test middleware. | P2 |

## Di Luar Cakupan (tetap terbuka)

Item P2 di review arsitektur: dead code (A1–A5), 404/405 JSON (E4), `.env` di production (C5), `statement_timeout` (D5), atribut `service.version` (O6), redaksi query string (O7), validasi `X-Request-ID` (O8), noise probe (O9), timeout shutdown & exit code (R3–R5), security headers/CORS (S5), load test (P2), OpenAPI (X7), SLO/alert (X8).

Keputusan desain yang perlu dikonfirmasi pemilik: **API key** dipilih untuk S2 karena paling sederhana dan tidak butuh identity provider. Jika klien adalah pengguna akhir atau butuh otorisasi per peran, ganti dengan JWT/OIDC.

## Skor Setelah P1

| # | Kriteria | P0 | P1 | Catatan |
|---|---|---|---|---|
| K1 ★ | Build & Correctness | 3 | **4.5** | |
| K2 ★ | Testing | 1.5 | **3.5** | Belum 4: CI belum pernah jalan; wiring `app.New` belum diuji |
| K3 | Arsitektur & Maintainability | 3 | **3.5** | Dead code P2 masih ada |
| K4 ★ | Error Handling & API Contract | 2 | **4** | |
| K5 | Konfigurasi & Secrets | 2.5 | **4** | |
| K6 ★ | Database & Data Integrity | 2 | **4** | |
| K7 | Observability | 3 | **4** | |
| K8 ★ | Reliability & Lifecycle | 3 | **4** | |
| K9 ★ | Security | 1.5 | **3.5** | Belum 4: API key (bukan identity per user), rate limit belum sadar proxy, tanpa security headers |
| K10 | Performance & Scalability | 2 | **3.5** | Belum ada load test |
| K11 ★ | Delivery & Operasional | 2 | **4** | |
| | **Rata-rata** | **~2.3** | **3.9** | |

**Kesimpulan:** siap untuk **staging**. Untuk production, selesaikan dulu:
1. Jalankan CI di remote dan pastikan hijau.
2. Tetapkan strategi auth (API key atau JWT/OIDC) dan kunci rate limit sesuai topologi load balancer.
3. Terapkan migrasi `0002` di setiap environment.
