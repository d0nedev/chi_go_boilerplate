# P1 — Repo Menjadi Template gonew

Tanggal: 2026-09-17. Referensi: `README.md` bagian "Membuat Service Baru".

## Item

| ID | Masalah | Perbaikan | File |
|----|---------|-----------|------|
| G1 | Module `chi-product-api` ditolak gonew: `malformed module path "chi-product-api": missing dot in first path element` | Module `github.com/d0nedev/chi_go_boilerplate`, semua import diperbarui, `gofmt -w` | `go.mod`, `cmd/**`, `internal/**` |
| G2 | gonew hanya mengganti import path; nama `chi-product-api`/`chi_product` di compose, deploy, CI, Makefile, alert, dashboard, OpenAPI, runbook, dan README tidak ikut berubah | Nama netral `chi-go-boilerplate` (service) dan `chi_go_boilerplate` (DB), lalu satu langkah `perl` di README untuk mengganti keduanya | seluruh file non-Go, `grafana/dashboards/chi-go-boilerplate.json` |
| G3 | Nama tracer berupa string `"chi-product-api/internal/product"` tidak di-rewrite gonew | `tp.Tracer("product")` | `internal/app/modules.go` |
| G4 | Laporan historis dan review arsitektur chi-product-api ikut tersalin ke service baru | Dihapus dari tree (tetap ada di git history, commit `e178a21`) | `docs/reports/*`, `docs/architecture-production-ready.md`, `AGENTS.md` |
| G5 | `.claude/settings.json` berisi hook dengan path absolut `/home/danni/...`; di mesin lain setiap tool call gagal | Dipindah ke `.claude/settings.local.json` (lokal, di-ignore) | `.claude/`, `.gitignore` |
| G6 | README spesifik project; tabel config masih menyebut OTLP insecure wajib `false` di production | Intro template, bagian "Membuat Service Baru" bertanda `template-only` (otomatis dihapus di service baru), tabel diperbaiki | `README.md` |

## Test

Tidak ada test Go baru (rename mekanis).

**Uji end-to-end gonew:** template dikemas jadi module zip `v0.2.0` (file yang ter-track + belum di-commit, tanpa file yang di-ignore) di GOPROXY `file://` lokal. Blok perintah README dijalankan apa adanya (hanya `make test` diganti `go test ./...`) untuk `github.com/acme/order-api`.

Bug yang tertangkap: judul README service baru menjadi `# order_api` karena judul template memakai token snake_case. Judul template diganti `# chi-go-boilerplate`.

## Verifikasi

Template:
- `go build`, `go vet`, `gofmt -l` — bersih
- `go test -race ./...` + integration — 86 passed
- `sqlc diff` — bersih
- `docker compose config -q` (dev dan `deploy/`) — valid
- `make alerts-test` — `SUCCESS: 8 rules found`, unit test SUCCESS
- `make openapi-lint` — exit 0

Service hasil gonew (`order-api`):
- `module github.com/acme/order-api`
- grep `chi-go|boilerplate|d0nedev|chi-product` — 0 hasil
- `gofmt -l` bersih, `sqlc diff` bersih
- `go test -race ./...` + integration — 86 passed
- `docker build` — sukses
- compose: `APP_SERVICE_NAME: order-api`, `POSTGRES_DB: order_api`; alert `exported_job="order-api"`
- Bagian `template-only` di README terhapus; `git commit` awal sukses

## Temuan baru

- Tag `v0.1.0` sudah di-push di commit `e178a21` dengan module lama; tag itu tidak bisa dipakai gonew. Rilis template berikutnya memakai `v0.2.0`; `v0.1.0` jangan digeser.
- Dev stack lokal: nama DB compose berubah menjadi `chi_go_boilerplate`, sedangkan volume `pgdata` lama berisi DB `chi_product`. Perlu `docker compose down -v` sebelum `make up`.

## Di luar cakupan / terbuka

- LICENSE: MIT atas nama `danniyp` (nama git user). Ganti nama pemegang hak cipta bila perlu.
- Commit, push, dan tag `v0.2.0` (dilakukan pemilik repo).
- Template lama `../go-chi-boilerplate` (bukan repo git, struktur lama) sudah digantikan repo ini; belum dihapus.
- Nama repo GitHub `chi_go_boilerplate` harus sama dengan module path.
