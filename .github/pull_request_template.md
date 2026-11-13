## Apa yang berubah
<!-- 1–3 kalimat. Tautkan butir TODO, misal: TODO 3.1 -->

## Checklist
- [ ] `make test-all` hijau (atau test repo ini: `go test -race ./...` / `pnpm test && pnpm typecheck && pnpm test:e2e`)
- [ ] Kontrak API ke frontend tidak berubah, atau `docs/openapi/v1.yaml` + `docs/architecture/api-v1-contract.md` ikut di-update
- [ ] Status fitur di `docs/product/features.md` di-update (bila fitur berubah)
- [ ] Butir di `docs/TODO.md` dicentang dengan hash commit (bila menyelesaikan butir)
- [ ] Tidak ada token, API key, password, atau nomor pelanggan asli di kode, log, atau dokumen
