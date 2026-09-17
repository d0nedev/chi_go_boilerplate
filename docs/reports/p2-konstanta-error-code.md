# P2 — Konstanta Error Code

Tanggal: 2026-09-17. Referensi: `docs/architecture-production-ready.md` (K4), `openapi.yaml`.

## Item

| ID | Masalah | Perbaikan | File |
|----|---------|-----------|------|
| EC1 | Error code API berupa string literal tersebar di 7 file; typo tidak tertangkap compiler, daftar code tidak terlihat di satu tempat | Const code umum di `apperror`, const code domain di package domain | `internal/platform/apperror/codes.go`, `internal/product/errors.go`, `app.go`, `httpx/error.go`, `httpx/request.go`, `middleware/auth.go`, `middleware/ratelimit.go`, `apperror/error.go`, `product/service.go`, `README.md` |

Nilai string tidak berubah, jadi kontrak API dan `openapi.yaml` tetap sama. Message dibiarkan di call site karena isinya kontekstual.

## Test

Tidak ada test baru. Test yang ada sengaja tetap memakai string literal (mis. `"NOT_FOUND"`) supaya perubahan nilai const yang tidak sengaja ikut gagal di test.

## Verifikasi

- `go build ./...`, `go vet ./...`, `gofmt -l` — bersih
- `go test -race ./...` + integration (`TEST_DATABASE_URL`) — semua ok
- grep literal `"[A-Z]+_[A-Z_]+"` di kode non-test: hanya nama env var di `config.go`

## Temuan baru

Tidak ada.

## Di luar cakupan

- Sinkronisasi otomatis daftar code dengan `openapi.yaml`.
