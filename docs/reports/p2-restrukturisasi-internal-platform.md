# P2 — Restrukturisasi `internal/` ke `internal/platform`

Tanggal: 2026-09-17. Referensi: `docs/architecture-production-ready.md`.

## Item

| ID | Masalah | Perbaikan | File |
|----|---------|-----------|------|
| R1 | `internal/` dipenuhi package utilitas sistem, business logic sulit ditemukan | Pindah `config`, `database`, `health`, `logging`, `metrics`, `middleware`, `requestcontext`, `tracing` ke `internal/platform/` (via `git mv`), update import path | `internal/**`, `sqlc.yaml`, `README.md` |

Hasil: `internal/` sekarang hanya `app/` (wiring), `product/` (domain), `platform/` (infra). Feature baru = folder baru di `internal/<domain>` + daftar di `internal/app/modules.go`.

## Test

Tidak ada test baru (murni pindah package, tanpa perubahan logika).

## Verifikasi

- `go build ./...` — ok
- `go vet ./...` — ok
- `go test ./...` — 80 passed in 14 packages
- `gofmt -l internal cmd` — bersih

## Temuan baru

- Output sqlc sekarang `internal/platform/database/sqlc` (`sqlc.yaml` sudah diupdate).

## Di luar cakupan

- Referensi path lama di laporan historis `docs/reports/*` dan `docs/architecture-production-ready.md` dibiarkan (catatan historis).
