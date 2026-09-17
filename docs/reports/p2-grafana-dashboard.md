# P2 — Grafana untuk Observability Lokal

Tanggal: 2026-09-17. Referensi: `docs/architecture-production-ready.md` (K7), `docs/runbook.md`.

## Item

| ID | Masalah | Perbaikan | File |
|----|---------|-----------|------|
| G1 | Metrics hanya bisa dilihat lewat Prometheus UI, traces lewat Jaeger UI; tidak ada dashboard | Service `grafana` di compose dengan datasource Prometheus + Jaeger dan dashboard `chi-product-api` yang di-provision dari file | `docker-compose.yml`, `grafana/provisioning/**`, `grafana/dashboards/chi-product-api.json`, `README.md` |

Panel dashboard: requests/detik per route, rasio 5xx, latency p95 per route, response per status code, pemakaian DB pool, DB query p95. Query memakai metric dan label yang sama dengan `prometheus-alerts.yml`.

## Test

Tidak ada test Go. Grafana tidak memengaruhi kode aplikasi.

## Verifikasi

- `docker compose config -q` — valid
- `GET /api/health` Grafana — `database: ok`, versi 12.2.0
- Health datasource Prometheus: `Successfully queried the Prometheus API.`; Jaeger: `Data source is working`
- Dashboard `chi-product-api` ter-provision di `/d/chi-product-api/chi-product-api`
- Query panel RPS per route mengembalikan series `/api/v1/products` dan `/api/v1/products/{id}`

## Temuan baru

Tidak ada.

## Di luar cakupan

- Grafana tanpa login (anonymous Admin) hanya untuk dev lokal; jangan dipakai di environment bersama.
- Link trace-to-metrics/exemplar dan log backend (Loki) belum ada; log masih di stdout container.
