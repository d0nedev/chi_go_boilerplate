# Laporan P0: Perbaikan Blocker Production Readiness

- Tanggal: 2026-09-17
- Referensi: [`docs/architecture-production-ready.md`](../architecture-production-ready.md), bagian 5 "Fase 0"
- Status: ✅ Semua item P0 selesai

## Ringkasan

Semua 10 item P0 dari review arsitektur sudah dikerjakan. Aplikasi sekarang bisa di-build, bug di jalur CRUD produk sudah diperbaiki, endpoint debug dihapus, repo sudah berupa git repository, sudah ada unit test untuk area yang diperbaiki, dan sudah ada Dockerfile production.

Verifikasi: `go build`, `go vet`, `go test -race` bersih; image Docker ter-build (37MB, distroless non-root); uji end-to-end terhadap Postgres lokal untuk create → get → update → delete berhasil.

## Item yang Dikerjakan

| ID | Masalah | Perbaikan | File |
|---|---|---|---|
| B1 | Syntax error `Level: cfg.App.,` — build gagal | `Level: cfg.App.LogLevel` | `internal/app/app.go` |
| B2 | `NewPostgresPool` dipanggil 3 argumen, fungsinya menerima 2 | `NewPostgresPool(ctx, cfg.DB)` | `internal/app/app.go` |
| B3 | `fmt.Sprintf("%0.2f", string)` → create selalu 500 | `price.Scan(req.Price)`; harga tidak valid → `400 INVALID_PRODUCT_PRICE` (detail parser hanya masuk log, tidak bocor ke klien) | `internal/product/service.go` |
| B4 | `Price.Int.String()` mengabaikan eksponen → `13000000.00` dikirim sebagai `"1300000000"` | Helper `numericString` pakai `pgtype.Numeric.Value()` → string desimal yang benar | `internal/product/dto.go` |
| B5 / O1 | `RecordError`: kondisi `if err != nil { return }` terbalik → error tidak pernah masuk span | `if err == nil { return }` | `internal/tracing/error.go` |
| B6 | `UpdateProduct` menulis body JSON dua kali | Hapus encode manual, tinggal satu `httpx.WriteJSON` | `internal/product/handler.go` |
| S1 | `GET /api/v1/products/panic` terbuka untuk publik | Route dihapus, contoh request di `.http` juga dihapus | `internal/product/routes.go`, `http/product.http` |
| X1 | Bukan git repository | `git init` + `.gitignore` (`.env`, `graphify-out/`, binary, coverage). Belum ada commit. | `.gitignore` |
| X2 | Tidak ada Dockerfile | Multi-stage: `golang:1.27.1` → `distroless/static-debian12:nonroot`, `CGO_ENABLED=0`, `-trimpath -s -w`, user non-root. Ditambah `.dockerignore` (termasuk `.env`). | `Dockerfile`, `.dockerignore` |
| T1 | Tidak ada test | Test untuk semua bug di atas (lihat bagian Test) | `internal/product/*_test.go`, `internal/tracing/error_test.go` |

Perubahan tambahan yang tidak mengubah perilaku: `gofmt` pada `internal/app/app.go` (method `Shutdown` sebelumnya pakai indentasi spasi), import `fmt`/`net/http` yang tidak terpakai dihapus.

## Test

| Test | Menangkap bug |
|---|---|
| `TestCreateProductReturnsDecimalPrice` | B3, B4 |
| `TestCreateProductInvalidPriceReturns400` | B3 (error mapping) |
| `TestUpdateProductWritesSingleJSONBody` | B6, B4 |
| `TestPanicRouteRemoved` | S1 |
| `TestNumericString` | B4 |
| `TestRecordError` (4 case: nil, error biasa, apperror 4xx, apperror 5xx) | B5 |

Test handler memakai fake `db.DBTX` (interface hasil generate sqlc), jadi tidak butuh database dan tidak perlu menambah interface baru di kode produksi.

**Validasi test:** kondisi lama di `tracing/error.go` sempat dikembalikan sementara, dan `TestRecordError` gagal di 4 case. Setelah perbaikan dipasang lagi, test lolos. Artinya test benar-benar mendeteksi bug tersebut.

## Bukti Verifikasi

```
$ go build ./...        # OK
$ go vet ./...          # OK (sebelumnya: printf %0.2f wrong type)
$ go test -race ./...
ok  chi-product-api/internal/product
ok  chi-product-api/internal/tracing

$ docker build -t chi-product-api:p0 .   # OK, 37MB
$ docker run --env-file .env.example -e DB_HOST=127.0.0.1 -e DB_PORT=1 chi-product-api:p0
ERROR failed to initialize application error="initialize database: ping postgres: ... connection refused"
# binary jalan, config ter-load, gagal dengan jelas saat DB tidak tersedia
```

Uji end-to-end (binary lokal, `APP_PORT=18080`, Postgres lokal; produk uji dihapus lagi di akhir):

| Langkah | Hasil |
|---|---|
| `GET /ready` | `200 {"status":"ready"}` |
| `POST /api/v1/products` `{"price":"13000000.00"}` | `201`, `"price":"13000000.00"` ✅ (sebelumnya 500) |
| `GET /api/v1/products/{id}` | `200`, harga benar |
| `PUT /api/v1/products/{id}` `{"price":"27000000.50"}` | `200`, satu body JSON, `"price":"27000000.50"` ✅ |
| `POST` dengan `"price":"abc"` | `400 INVALID_PRODUCT_PRICE` |
| `GET /api/v1/products/panic` | `400 VALIDATION_ERROR` (route panic sudah tidak ada) |
| `DELETE /api/v1/products/{id}` | `204` |
| `GET` setelah delete | `404 PRODUCT_NOT_FOUND` |
| SIGTERM | Log `shutdown signal received` → `server stopped`; tidak ada `superfluous WriteHeader` |

## Temuan Baru Selama Pengerjaan

| Temuan | Prioritas usulan |
|---|---|
| `logging.LogError` mencatat error 4xx (validasi, not found) di level `ERROR`. Ini membuat alert berbasis log jadi berisik. Sebaiknya 4xx di level `WARN`. | P1 (masuk K4/K7) |
| `created_at`/`updated_at` masih format `time.String()`, bukan RFC 3339 (sudah tercatat sebagai E2). | P1 |

## Di Luar Cakupan (tetap terbuka)

Semua item P1 dan P2 di review arsitektur, terutama:
- B7–B11: `errors.As` di `WriteError`, `DecodeJSON` belum dipakai handler, `Create` belum memanggil `Validate()`, kontrak tipe `price`.
- E1: `Update` masih menaruh `err.Error()` di pesan error ke klien.
- R1–R2: urutan middleware Recovery/Logging, readiness drain.
- S2–S3, S6: auth, rate limit, vulnerability scanning.
- X3–X5: CI, README project, `docker-compose` lengkap.

## Skor Setelah P0

| # | Kriteria | Sebelum | Sesudah |
|---|---|---|---|
| K1 ★ | Build & Correctness | 1 | 3 |
| K2 ★ | Testing | 0 | 1.5 |
| K7 | Observability | 2.5 | 3 |
| K9 ★ | Security | 1 | 1.5 |
| K11 ★ | Delivery & Operasional | 0.5 | 2 |
| | **Rata-rata keseluruhan** | **1.8** | **~2.3** |

Masih **belum production ready**. Langkah berikutnya: Fase 1 (P1).
