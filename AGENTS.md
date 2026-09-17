# AGENTS.md

Instruksi untuk AI coding agent yang bekerja di repository ini.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).


## Laporan (Reports)

- Setiap laporan pekerjaan **wajib** ditulis ke `docs/reports/<prioritas>-<nama-report>.md`.
  - `<prioritas>`: level prioritas item yang dikerjakan, huruf kecil: `p0`, `p1`, atau `p2`.
  - `<nama-report>`: kebab-case, singkat, deskriptif. Contoh: `docs/reports/p0-perbaikan-blocker.md`.
- Jangan menulis laporan di luar `docs/reports/`, dan jangan menimpa laporan yang sudah ada. Buat file baru.
- Isi minimal laporan:
  1. Tanggal dan referensi ke dokumen/issue terkait.
  2. Tabel item yang dikerjakan: ID, masalah, perbaikan, file.
  3. Test yang ditambahkan dan bug yang ditangkap.
  4. Bukti verifikasi: output `go build`, `go vet`, `go test`, dan uji manual/e2e bila ada.
  5. Temuan baru selama pengerjaan.
  6. Item yang di luar cakupan dan masih terbuka.

## Dokumen Terkait

- Operasional, deploy, dan alert: `docs/runbook.md`
