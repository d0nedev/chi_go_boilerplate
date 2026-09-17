# P1 — Penilaian Ulang Production Readiness

Tanggal: 2026-09-17. Referensi: `docs/architecture-production-ready.md`, laporan P0/P1/P2 di `docs/reports/`.
Cakupan: working tree setelah restrukturisasi `internal/platform` (termasuk perubahan belum di-commit: `middleware/timeout.go`, `tracing.go`, `postgres.go`, alert rules).

## Verdict

**Kode: production ready (4.3 / 5).** Semua kriteria ≥ 4 kecuali K11. **Go-live belum**: masih ada 4 item non-kode yang wajib (lihat bawah).

## Verifikasi

| Cek | Hasil |
|---|---|
| `go build ./...`, `go vet ./...`, `gofmt -l` | bersih |
| `go test -race ./...` + integration (`TEST_DATABASE_URL` ke Postgres compose) | 85 passed, 14 package, coverage 67.8% |
| `govulncheck` | No vulnerabilities found |
| `sqlc diff` (path output baru) | bersih |
| Smoke test stack compose | `/health` 200, `/ready` 200, list 200, create 201, harga negatif 400. Data test dihapus (204). |
| CI GitHub | **tidak bisa dicek** (`gh` tidak terpasang) |

## Scorecard

| # | Kriteria | P2 | Sekarang | Catatan |
|---|---|---|---|---|
| K1 ★ | Build & Correctness | 5 | 5 | |
| K2 ★ | Testing | 4 | 4 | Integration test lolos lokal; CI remote belum terbukti hijau; tanpa contract test OpenAPI |
| K3 | Arsitektur | 4.5 | 4.5 | `internal/` = app / domain / platform |
| K4 ★ | Error & API Contract | 4.5 | 4.5 | |
| K5 | Konfigurasi & Secrets | 4.5 | 4.5 | `DB_USER` tidak divalidasi wajib (minor) |
| K6 ★ | Database | 4.5 | 4.5 | Migrasi manual via runbook, bukan step pipeline |
| K7 | Observability | 4.5 | 4.5 | |
| K8 ★ | Reliability | 4.5 | 4.5 | Lihat temuan T3 |
| K9 ★ | Security | 4 | 4 | API key bersama, GET publik |
| K10 | Performance | 4 | 4 | Rate limit per instance |
| K11 ★ | Delivery | 4.5 | **3.5** | Tanpa CD/registry push, LICENSE salah, CI belum terbukti |
| | **Rata-rata** | 4.4 | **4.3** | |

K11 diturunkan: penilaian P2 terlalu optimis untuk item yang belum pernah jalan di luar mesin lokal.

## Wajib sebelum go-live

1. **CI hijau di GitHub.** Remote `origin` sudah ada; pastikan workflow benar-benar jalan dan lulus.
2. **LICENSE** masih lisensi golang-migrate (Matthias Kadenbach / Dale Hui). Ganti dengan lisensi project.
3. **Delivery**: job `image` hanya build + scan, tidak push ke registry; tidak ada manifest deploy; migrasi dijalankan manual (`docs/runbook.md` §Deploy).
4. **Keputusan auth**: API key bersama untuk write, semua GET publik tanpa auth. Pastikan sesuai klien sebenarnya; set `TRUSTED_PROXIES` sesuai load balancer.

## Temuan baru

| ID | Temuan | Dampak | Saran |
|---|---|---|---|
| T1 | Access log menyimpan `request_body` (≤2KB) untuk semua 4xx/5xx | PII masuk log saat domain berisi data pribadi | Redaksi/hapus sebelum menambah domain dengan PII |
| T2 | Log tidak mencatat API key mana yang dipakai | Tidak ada audit trail per klien | Log fingerprint (prefix hash) key |
| T3 | `/ready` ping DB | DB blip singkat membuat semua pod keluar dari LB bersamaan | Terima sebagai desain, atau readiness hanya drain + liveness DB terpisah |
| T4 | Base image Dockerfile tidak di-pin digest | Build tidak reproducible | Pin `@sha256:` |

## Di luar cakupan

- Contract test OpenAPI vs respons.
- Rate limit terdistribusi (Redis) — tunggu jumlah replika diketahui.
