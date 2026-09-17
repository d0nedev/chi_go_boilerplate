# Runbook: chi-product-api

Setiap alert di `prometheus-alerts.yml` punya `annotations.runbook` yang mengarah ke bagian di dokumen ini.

## SLO

| SLO | Target | Pengukuran | Error budget (30 hari) |
|---|---|---|---|
| Availability | 99.9% request tanpa 5xx | `service:http_errors_5xx:rate5m / service:http_requests:rate5m` | ~43 menit setara 100% error |
| Latency | p95 < 300ms per route | `service:http_latency_p95:5m` | — |

Baseline load test lokal (lihat `loadtest/products.js`): pada 3000 req/s, p95 baca 3.6ms dan tulis 7.95ms, dengan 0% error. Angka ini diambil dari satu laptop di mana app, Postgres, dan generator beban berjalan di host yang sama, jadi hanya berguna sebagai pembanding antar versi, bukan sebagai estimasi kapasitas production.

## Sinyal Umum

| Kebutuhan | Tempat |
|---|---|
| Metrics | Prometheus. Label `http_route`, `http_response_status_code`, `service_version`, `deployment_environment_name` |
| Trace | Jaeger, service `chi-product-api`. Nama span `GET /api/v1/products/{id}` dan `ProductService.*` |
| Log | JSON stdout. Korelasi lewat `request_id` (juga di header `X-Request-ID`) dan `trace_id` |
| Konfigurasi efektif | Log `configuration loaded` saat startup |

## ServiceHighErrorRate

Rasio 5xx di atas 1.4% selama 5 menit, artinya error budget bulanan habis sekitar 14× lebih cepat dari seharusnya.

1. Cari route mana yang gagal: `sum by (http_route, http_response_status_code) (rate(http_server_request_duration_seconds_count{http_response_status_code=~"5.."}[5m]))`.
2. Cek apakah bertepatan dengan rilis: pisahkan per `service_version`. Jika error hanya ada di versi baru, rollback.
3. Cari log `"level":"ERROR"` dan periksa `error_code`:
   - `PRODUCT_*_FAILED`: error dari database. Cek `/ready`, koneksi Postgres, dan apakah query dibatalkan oleh `statement_timeout` (`canceling statement due to statement timeout`).
   - `panic recovered`: bug. Lihat `stack_trace`, lalu rollback.
4. Buka trace contoh lewat `trace_id` dari log untuk melihat span DB yang gagal.

## ServiceHighLatency

p95 sebuah route di atas 300ms selama 10 menit.

1. Di Jaeger, urutkan trace route tersebut berdasarkan durasi. Bandingkan span `pool.acquire` dengan span query.
   - `pool.acquire` lama: pool DB penuh. Pertimbangkan menaikkan `DB_MAX_CONNS` (perhatikan `max_connections` Postgres dikali jumlah replika) atau menambah replika.
   - Query lama: cek `pg_stat_statements` dan rencana eksekusi. Untuk list, pastikan index `products_created_at_id_idx` dipakai.
2. Pastikan tidak ada klien yang meminta `limit=100` secara berlebihan.

## ServiceNoTraffic

Tidak ada metric request selama 15 menit.

1. Cek app hidup: `/health`, status pod/container, restart loop.
2. Jika app hidup dan menerima request (lihat access log), berarti masalahnya di pipeline telemetry: cek log collector dan `OTEL_EXPORTER_OTLP_ENDPOINT`.
3. Di environment dengan trafik rendah (malam/staging), alert ini bisa normal. Sesuaikan atau matikan per environment.

## ServiceTelemetryPipelineDown

Prometheus tidak bisa scrape OTel collector. **Semua alert lain buta** selama kondisi ini.

1. Cek container/pod collector dan log-nya.
2. Validasi konfigurasi: `otelcol validate --config=otel-collector.yaml`.

## Operasi Rutin

### Deploy

1. Jalankan migrasi **sebelum** rollout: `make migrate-up DATABASE_URL=...`. Migrasi harus kompatibel dengan versi app yang sedang berjalan.
2. Rollout. App menunggu DB hingga `DB_CONNECT_TIMEOUT` (default 30s).
3. Build image dengan versi: `docker build --build-arg VERSION=$(git describe --tags --always) .`

### Shutdown / Rolling Restart

Saat SIGTERM: `/ready` langsung 503, lalu tunggu `APP_SHUTDOWN_DRAIN_DELAY`, lalu request yang sedang berjalan diselesaikan dalam `APP_SHUTDOWN_TIMEOUT`, lalu telemetry di-flush. Pastikan `drain delay + shutdown timeout` < grace period orchestrator (Kubernetes default 30s). Exit code 0 berarti berhenti bersih; exit code 1 berarti server gagal atau shutdown HTTP melebihi timeout.

### Rotasi API Key

1. Tambahkan key baru: `API_KEYS=old,new`, lalu deploy.
2. Pindahkan semua klien ke key baru.
3. Hapus key lama: `API_KEYS=new`, lalu deploy.

### Batasan yang Diketahui

- Rate limit disimpan di memori per instance. Dengan N replika, limit efektif adalah N × `RATE_LIMIT_REQUESTS_PER_MINUTE`.
- Tidak ada Alertmanager di `docker-compose.yml`. Alert hanya terlihat di UI Prometheus (`/alerts`). Routing notifikasi (pager/Slack) dikonfigurasi di platform monitoring masing-masing environment.
