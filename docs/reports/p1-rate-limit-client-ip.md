# Laporan P1: Migrasi API Rate Limit yang Deprecated & IP Klien di Belakang Proxy

- Tanggal: 2026-09-17
- Referensi: [`p1-production-hardening.md`](p1-production-hardening.md) (item S3 dan temuan "rate limit memakai `RemoteAddr`"), [`docs/architecture-production-ready.md`](../architecture-production-ready.md)
- Status: ✅ Selesai

## Ringkasan

Blok `router.Route("/api/v1", ...)` di `internal/app/app.go` memakai dua API `httprate` v0.16.0 yang deprecated. **Tidak ada API chi yang deprecated.** Hasil scan `staticcheck -checks SA1019` untuk seluruh repo:

```
internal/app/app.go:113:9:  github.com/go-chi/httprate.Limit is deprecated: Use LimitBy(requestLimit, windowLength, keyFn, options...) instead ...
internal/app/app.go:116:26: github.com/go-chi/httprate.KeyByIP is deprecated: KeyByIP keys off r.RemoteAddr ... behind a reverse proxy ... every client sharing that proxy lands in one rate-limit bucket.
```

Keduanya diganti dengan API yang direkomendasikan. Sekaligus, temuan P1 "rate limit tidak sadar proxy" juga selesai.

Selama verifikasi ditemukan celah pada pola yang direkomendasikan dokumentasi `httprate`: `chi/middleware.ClientIPFromXFF` **hanya membaca header `X-Forwarded-For` dan tidak memeriksa `RemoteAddr`**. Akibatnya, klien yang bisa mengakses server secara langsung dapat memalsukan IP dan menghindari rate limit. Celah ini sudah ditutup.

## Perubahan

| Sebelum (deprecated) | Sesudah | File |
|---|---|---|
| `httprate.Limit(n, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP), ...)` inline di `app.go` | `middleware.RateLimit(n)` → `httprate.LimitBy(n, time.Minute, clientIPKey, WithLimitHandler, WithErrorHandler)` | `internal/middleware/ratelimit.go`, `internal/app/app.go` |
| `KeyByIP` (selalu `RemoteAddr`) | `clientIPKey`: `httprate.CanonicalizeIP(chimiddleware.GetClientIP(ctx))` (IPv6 dikelompokkan per /64). Jika IP tidak dapat ditentukan → `400 CLIENT_IP_UNRESOLVED` (fail-closed, bukan satu bucket global). | `internal/middleware/ratelimit.go` |
| — | `middleware.ClientIP(trustedProxies)` dipasang paling awal di router. Tanpa `TRUSTED_PROXIES` → `ClientIPFromRemoteAddr`. Dengan `TRUSTED_PROXIES` → `ClientIPFromXFF`, **hanya jika peer TCP berada di CIDR tepercaya**; selain itu tetap `RemoteAddr`. | `internal/middleware/ratelimit.go`, `internal/app/app.go` |
| — | Env `TRUSTED_PROXIES` (daftar CIDR dipisah koma), divalidasi dengan `netip.ParsePrefix` sehingga `MustParsePrefix` tidak panic saat startup. Nilainya dicatat di log `configuration loaded`. | `internal/config/config.go`, `.env.example`, `README.md` |
| Log `remote_ip` selalu dari `RemoteAddr` | Memakai IP klien hasil resolusi yang sama dengan key rate limit (fallback `RemoteAddr`) | `internal/logging/request.go` |

Blok router di `app.go` sekarang:

```go
router.Use(middleware.ClientIP(cfg.App.TrustedProxies))
router.Use(middleware.RequestID)
...
router.Route("/api/v1", func(r chi.Router) {
	r.Use(middleware.RateLimit(cfg.RateLimit.RequestsPerMinute))

	product.RegisterRoutes(r, productHandler, logger, middleware.APIKey(cfg.Auth.APIKeys))
})
```

## Test

| Test | Yang diverifikasi |
|---|---|
| `TestRateLimitPerRemoteAddr` | Bucket per IP; respons 429 dalam format JSON `RATE_LIMITED` |
| `TestRateLimitIgnoresXFFWithoutTrustedProxies` | Tanpa `TRUSTED_PROXIES`, XFF palsu tidak memberi bucket baru |
| `TestRateLimitBehindTrustedProxy` | Klien berbeda di belakang proxy yang sama mendapat bucket masing-masing |
| `TestXFFFromUntrustedPeerIsIgnored` | **Celah spoofing**: peer yang tidak tepercaya dengan XFF berganti-ganti tetap kena limit |
| `TestRateLimitRejectsUnresolvedClientIP` | Proxy tepercaya tanpa XFF → `400 CLIENT_IP_UNRESOLVED` |
| `TestClientIPUsedForLogging` | IP dari XFF tersedia di context untuk logging |
| `TestValidate` (2 case baru) | `TRUSTED_PROXIES` bukan CIDR ditolak; IPv4 + IPv6 CIDR diterima |

## Bukti Verifikasi

```
$ go run honnef.co/go/tools/cmd/staticcheck@latest -checks SA1019 ./...
SA1019 clean
$ go vet ./... && go test -race ./...    # semua ok
```

Smoke test binary (`RATE_LIMIT_REQUESTS_PER_MINUTE=3`, endpoint yang tidak menyentuh DB):

| Skenario | Hasil |
|---|---|
| Tanpa `TRUSTED_PROXIES`, 5 request dengan XFF berbeda | `400 400 400 429 429` (XFF diabaikan) |
| `TRUSTED_PROXIES=127.0.0.1/32`, peer `127.0.0.1`, 5 klien XFF berbeda | `400 ×5` (bucket per klien) |
| `TRUSTED_PROXIES=127.0.0.1/32`, peer `::1` (tidak tepercaya), 5 XFF palsu | `400 400 400 429 429` (spoofing ditolak) |
| Peer tepercaya tanpa XFF | `400 CLIENT_IP_UNRESOLVED` |

Catatan: sebelum perbaikan spoofing, skenario peer `::1` + XFF palsu **lolos** (setiap XFF mendapat bucket baru). Inilah yang memicu perbaikan.

## Dampak & Tindakan Deploy

- Tanpa `TRUSTED_PROXIES`, perilaku sama seperti sebelumnya (per `RemoteAddr`).
- **Di belakang load balancer/CDN**, isi `TRUSTED_PROXIES` dengan CIDR LB/CDN. Verifikasi dengan melihat `remote_ip` di access log: nilainya harus IP klien, bukan IP LB.
- Jika LB meneruskan request tanpa `X-Forwarded-For`, klien akan mendapat `400 CLIENT_IP_UNRESOLVED`. Ini disengaja (fail-closed) supaya salah konfigurasi langsung terlihat.

## Temuan Baru

| Temuan | Prioritas usulan |
|---|---|
| Rate limiter memakai counter in-memory, sehingga limit berlaku **per instance**. Dengan N replika, limit efektif menjadi N × `RATE_LIMIT_REQUESTS_PER_MINUTE`. Solusinya `httprate-redis` atau rate limit di gateway. | P2 (P1 jika limit harus presisi) |

## Di Luar Cakupan

Item P2 lain di review arsitektur tidak berubah.
