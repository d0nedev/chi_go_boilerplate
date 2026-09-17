# Laporan P2: Boilerplate Template dari Arsitektur chi-product-api

- Tanggal: 2026-09-17
- Referensi: [`p2-hardening-operasional.md`](p2-hardening-operasional.md), [`docs/architecture-production-ready.md`](../architecture-production-ready.md)
- Status: ✅ Selesai
- Hasil: template di `/home/danni/Workspace/go/go-chi-boilerplate` + script `scripts/new-service.sh`

## Ringkasan

Arsitektur ini dijadikan template untuk project lain dalam dua langkah:

1. **Merapikan bagian yang masih terikat ke project di `chi-product-api` sendiri**, supaya project ini tetap menjadi referensi yang berjalan dan identik dengan template.
2. **Membuat template generik** di folder terpisah, dengan script yang menghasilkan service baru dan langsung memverifikasinya.

Template diuji dengan menghasilkan dua service dari nol (`github.com/acme/order-api` dan `example.com/platform/billing-service`). `order-api` diverifikasi penuh: unit test, integration test, staticcheck, sqlc, OpenAPI, alert rules, Trivy, dan stack Docker Compose end-to-end.

Keputusan desain: **template repo + script copy**, bukan library bersama. Konsekuensinya, perbaikan di template tidak otomatis mengalir ke service yang sudah dibuat. Keuntungannya, setiap service bebas mengubah lapisan platform tanpa merusak service lain.

## 1. Perubahan di chi-product-api

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| T1 | Helper error `failed`/`notFoundOr` terkunci di package `product` padahal polanya generik | `apperror.NotFound(code, msg)`, `apperror.Internal(code, msg, err)` (stack trace menunjuk ke pemanggil), `database.IsNotFound(err)`, `tracing.Fail(span, err)`. Product service tinggal punya satu helper domain `mapError`. | `internal/platform/apperror/error.go`, `internal/database/errors.go`, `internal/tracing/error.go`, `internal/product/service.go` |
| T2 | `newRouter` bergantung langsung ke `product.Handler`, jadi menambah domain berarti mengubah router | `newRouter(cfg, logger, health, apiRoutes ...func(chi.Router))`. Wiring semua domain dipindah ke `modules()` di satu file. Test router memakai `modules()` yang asli sehingga wiring ikut teruji. | `internal/app/app.go`, `internal/app/modules.go`, `internal/app/router_test.go` |
| T3 | Belum ada contoh transaksi | **Tanpa helper baru**: `pgx.BeginFunc` (sudah ada di pgx) sudah menangani commit/rollback/panic. Ditambahkan integration test yang sekaligus menjadi contoh pola `pgx.BeginFunc` + `queries.WithTx`: rollback saat error, rollback saat panic, commit saat sukses. | `internal/product/transaction_integration_test.go` |
| T4 | Nama tracer `chi-product-api/product`, alert `ChiProductAPI*`, dan recording rule `chi:*` spesifik project | Tracer memakai import path package (`chi-product-api/internal/product`); alert jadi `Service*` dengan label `service: <nama>`; recording rule jadi `service:*`. Anchor runbook disesuaikan. | `internal/app/modules.go`, `prometheus-alerts.yml`, `prometheus-alerts_test.yml`, `docs/runbook.md` |
| — | Dokumentasi | README: letak `modules.go`, pola error dan transaksi, rujukan ke template | `README.md` |

**Perubahan yang terlihat dari luar:** hanya nama alert (`ChiProductAPIHighErrorRate` → `ServiceHighErrorRate`, dst.) dan nama recording rule (`chi:` → `service:`). Dashboard atau routing notifikasi yang merujuk nama lama perlu diperbarui. API dan perilaku HTTP tidak berubah.

## 2. Template `go-chi-boilerplate`

### Isi

Salinan `chi-product-api` setelah langkah 1, dengan pengecualian:

| Tidak disalin | Alasan |
|---|---|
| `.git`, `graphify-out/`, `.claude/`, `CLAUDE.md`, `.env` | Spesifik mesin/tool, atau berisi secret |
| `docs/reports/`, `docs/architecture-production-ready.md` | Riwayat project ini, bukan milik template |
| `LICENSE` | Isinya lisensi golang-migrate; lisensi template belum diputuskan |

Token nama diganti: `chi-product-api` → `go-chi-boilerplate`, `chi_product` → `go_chi_boilerplate`, lalu `gofmt -w` (urutan import berubah setelah rename). Sisa token lama: 0.

Penyesuaian khusus template:
- **README**: pengantar template, kebutuhan (Go 1.27+ karena `uuid` stdlib), "Membuat Service Baru", tabel "Platform vs Domain", checklist "Menambah Domain", "Menghapus Domain Contoh", dan "Pola Transaksi". Bagian pembuatan service ditandai `<!-- template-only -->` sehingga tidak ikut ke service hasil generate.
- **AGENTS.md**: bagian graphify dan rujukan ke dokumen review dihapus; ditambah konvensi kode (pola domain, `make sqlc`, jangan ubah migrasi lama, `make lint/test`). Aturan laporan dipertahankan.
- **`.gitignore`**: entri `graphify-out` dihapus.

### Script `scripts/new-service.sh`

```bash
scripts/new-service.sh github.com/acme/order-api                       # folder ./order-api
SERVICE_NAME=billing scripts/new-service.sh example.com/x/billing-service ./billing-svc
```

1. Validasi argumen (module path, nama service kebab-case, target belum ada) dan tool (`go`, `gofmt`, `perl`, `tar`).
2. Salin template tanpa `.git`, `scripts/`, `.env`, `bin`, `graphify-out`.
3. Hapus bagian `template-only` di README.
4. Ganti token: import path `"go-chi-boilerplate/` → `"<module>/`, lalu `go-chi-boilerplate` → nama service, dan `go_chi_boilerplate` → nama DB (`-` → `_`). Memakai `perl` supaya jalan di Linux dan macOS.
5. `go mod edit -module`, `gofmt -w`, lalu pastikan tidak ada token tersisa.
6. `go build`, `go vet`, `go test` (lewati dengan `SKIP_VERIFY=1`).

`gonew` belum bisa dipakai sebelum template dipublikasikan sebagai module. Setelah dipublikasikan pun, `gonew` hanya mengganti import path; nama service dan DB tidak ikut diganti. Hal ini dicatat di README template.

## 3. Bukti Verifikasi

### chi-product-api (setelah refactor)

```
go build, go vet, gofmt, staticcheck (semua check)         # bersih
go test -race ./... (+ integration, Postgres 17)          # 9 package ok
  --- PASS: TestIntegrationTransactionPattern
make alerts-test                                           # SUCCESS: 7 rules; unit test SUCCESS
```

### Template go-chi-boilerplate

```
go build, go vet, gofmt, staticcheck                       # bersih
go test -race ./... (+ integration)                        # 9 package ok
sqlc diff, docker compose config, make openapi-lint        # bersih / valid
make alerts-test                                           # SUCCESS
```

### Service hasil generate: `github.com/acme/order-api`

| Pemeriksaan | Hasil |
|---|---|
| `go.mod` | `module github.com/acme/order-api` |
| Token template tersisa | 0 |
| Bagian "Membuat Service Baru" di README | Terhapus |
| Folder `scripts/` | Tidak ikut |
| Import & tracer | `github.com/acme/order-api/internal/...` |
| Compose | `APP_SERVICE_NAME: order-api`, `POSTGRES_DB: order_api`, `DB_NAME: order_api` |
| `go build`/`vet`/`test` (oleh script) | Lolos |
| `gofmt -l`, staticcheck | Bersih |
| Integration test (4) | Lolos: statement timeout, lifecycle, constraints, transaksi |
| `sqlc diff`, `openapi-lint`, `alerts-test` | Bersih / valid / SUCCESS |
| Trivy (HIGH/CRITICAL) | debian 0, gobinary 0 |

Stack Docker Compose `order-api` (`APP_ENV=staging`, build dari nol):

| Skenario | Hasil |
|---|---|
| Semua service | migrate exit 0; app, postgres, collector, jaeger, prometheus running |
| `GET /ready` | `{"status":"ready"}` |
| `POST` tanpa key / dengan key | 401 / 201, `"price":"99.50"` |
| `GET /api/v1/products?limit=5` | 1 item, `next_cursor: null` |
| Prometheus | Series `exported_job="order-api"`, `http_route=/api/v1/products`, `service_version=gen-verify` |
| Alert rules | 7 rule, semua `health: ok`, label `service=order-api` |
| Jaeger | Service `order-api` |
| `docker stop` | Exit 0 |

### Kasus Error pada Script

| Input | Hasil |
|---|---|
| `SERVICE_NAME=Bad_Name` | Exit 1, `service name must be lowercase kebab-case`, tidak ada folder tersisa |
| Target sudah ada | Exit 1, `target already exists` |
| Tanpa argumen | Exit 1, usage |
| `SERVICE_NAME=billing` + module `example.com/platform/billing-service` + target `./billing-svc` | Service `billing`, DB `billing`, alert `exported_job="billing"`, build lolos |

Semua resource verifikasi (stack compose, container Postgres test, image, folder hasil generate) sudah dihapus. Container dev `chi-product-api-jaeger-1` dan `chi-product-api-otel-collector-1` tidak disentuh.

## 4. Temuan Baru

| Temuan | Prioritas usulan |
|---|---|
| Blok import mencampur package lokal dan stdlib dalam satu grup (gaya bawaan project). `gofmt` menerimanya, tetapi `goimports` akan mengelompokkan ulang. Belum ada linter yang menegakkan pengelompokan import. | P2 (tambah `goimports -local <module>` atau `golangci-lint` di CI) |
| Checklist "Menambah Domain" di README template belum diuji dengan benar-benar menambah domain kedua. Yang diverifikasi: potongan kode di README cocok dengan nama variabel di `modules.go`. | P2 |

## 5. Di Luar Cakupan / Keputusan Terbuka

- **Template belum menjadi git repository dan belum dipublikasikan.** Jalankan `git init` dan push ke remote pilihan Anda. Setelah itu nama module template bisa disesuaikan dengan path repo (mis. `github.com/<org>/go-chi-boilerplate`) agar `gonew` juga bisa dipakai.
- **Lisensi template** belum ditentukan.
- **Perbaikan di template tidak otomatis masuk ke service turunan.** Untuk sinkronisasi, bandingkan diff antar versi template secara manual.
- Keputusan terbuka dari laporan sebelumnya tetap berlaku untuk setiap service turunan: strategi auth, `TRUSTED_PROXIES`, CI di remote.
