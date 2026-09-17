# P1 — Deploy Production ke VPS + Observability Grafana Cloud

Tanggal: 2026-09-17. Referensi: `docs/reports/p1-penilaian-ulang-production-ready.md` (item wajib no. 3 "Delivery"), `docs/runbook.md`.

Target: VPS 2 vCPU / 8 GB RAM / 100 GB, minimalis tapi reliable.

## Keputusan

- **Observability tidak di-host di VPS.** Metrics, traces, dan log dikirim ke Grafana Cloud lewat satu OTel Collector. Alasan: kalau VPS mati, alert harus tetap jalan; stack LGTM self-hosted menambah 5 komponen stateful.
- **Caddy** untuk HTTPS otomatis dan reverse proxy.
- **Migrasi otomatis** lewat service `migrate` sebelum app start.
- **Satu replika app**: rilis tidak zero-downtime (jeda beberapa detik), diterima demi kesederhanaan.

## Item

| ID | Masalah | Perbaikan | File |
|----|---------|-----------|------|
| D1 | Tidak ada konfigurasi deploy production | Compose production: Caddy, app, migrate, Postgres (TLS self-signed), OTel Collector contrib; limit memori, rotasi log docker, `GOMEMLIMIT`, IP Caddy tetap untuk `TRUSTED_PROXIES` | `deploy/docker-compose.yml`, `deploy/Caddyfile`, `deploy/.env.example` |
| D2 | Log hanya di stdout | Collector membaca log JSON docker (hanya container berlabel `service`), parse JSON slog, set severity dan trace context, kirim ke Loki. Metrics dan traces ke Mimir/Tempo | `deploy/otel-collector.yaml` |
| D3 | Tidak ada backup DB | `pg_dump` harian, retensi 7 hari, salinan off-site opsional via rclone, file ditulis atomik (`.partial` lalu `mv`) | `deploy/backup.sh` |
| D4 | Validasi config menolak `OTEL_EXPORTER_OTLP_INSECURE=true` di production, padahal collector ada di network privat yang sama | Aturan dihapus; default tetap `false` di luar development, jadi plaintext harus diset eksplisit | `internal/platform/config/config.go`, `config_test.go`, `.env.example` |
| D5 | `.dockerignore` hanya mengecualikan `.env` di root; `deploy/.env` (secret) akan ikut ke build context | Pola `**/.env`, `**/.env.*`, `deploy/backups` | `.dockerignore`, `.gitignore` |
| D6 | Langkah deploy, rilis, rollback, restore, dan penyesuaian label alert untuk Grafana Cloud belum ada | Runbook bagian Deploy ditulis ulang | `docs/runbook.md`, `README.md` |

## Test

- `config_test.go`: kasus baru `production allows explicit plaintext to local collector`.

**Bug yang tertangkap saat uji stack lokal:**
- Log collector ikut terbaca oleh collector sendiri, jadi terjadi loop (±23 ribu record dalam beberapa detik dengan exporter debug). Di production ini akan memperbesar error export. Perbaikan: container collector tidak diberi label `service`.
- Nama receiver `filelog` deprecated di 0.160.0, diganti `file_log`.

## Verifikasi

Stack `deploy/` dijalankan lokal (project `chi-product-api-prod`, exporter diganti `debug`, Caddy di port 18443, `DOMAIN=localhost`), lalu dibongkar (`down -v`).

| Cek | Hasil |
|---|---|
| `docker compose config -q` | valid |
| `otelcol-contrib validate` pada config production asli (otlphttp + basicauth) | valid |
| Migrasi dengan `sslmode=require` | `2/u harden_products`, exit 0 |
| Koneksi app ke Postgres | `pg_stat_ssl.ssl = t` |
| `GET /ready` via Caddy HTTPS | 200 |
| `POST` tanpa key / dengan key | 401 / 201 |
| `X-Forwarded-For: 6.6.6.6` palsu | diabaikan; `remote_ip` = peer dari Caddy |
| Log record di collector | `service.name=chi-product-api`, `SeverityText` WARN/INFO, `Trace ID`/`Span ID` terisi, field JSON jadi attributes |
| `backup.sh` + `pg_restore` ke DB terpisah | dump ok, tabel `products` ter-restore |
| `docker compose stop app` | drain 2s, `server stopped`, exit 0 |
| Memori idle | collector 63 MiB / 256, caddy 13 / 128, postgres 113 MiB / 3 GiB |
| `go build`, `go vet`, `go test -race` + integration | lolos |

Pengiriman ke Grafana Cloud yang sebenarnya **belum diuji** (butuh kredensial).

## Temuan baru

- Metrics via OTLP ke Grafana Cloud memakai label `job`/`instance`, sedangkan `prometheus-alerts.yml` dan dashboard memakai `exported_job`/`exported_instance` (hasil scrape exporter Prometheus). Runbook memberi perintah `sed` untuk import. Alert `ServiceTelemetryPipelineDown` tidak berlaku di Grafana Cloud.

## Di luar cakupan / terbuka

- Uji end-to-end ke Grafana Cloud dengan kredensial asli, termasuk import alert, dashboard, dan uptime check.
- Registry image dan CD otomatis. Saat ini build dilakukan di VPS dari `git pull`.
- Hardening host (SSH, fail2ban) hanya disebut di runbook, tidak diotomasi.
- Zero-downtime deploy (butuh 2 replika app dan upstream Caddy dengan health check).
- LICENSE dan CI hijau di GitHub (dari laporan sebelumnya).
