# Contributing to SluiceOps

Thanks for helping! Issues and pull requests in German or English are welcome.

## Ground rules

- **Never lose a byte of the user's config.** The line-preserving editor in
  `internal/cfgfile` is the heart of the project. Every edit operation must only
  touch the lines it is about. Add a test that shows the exact diff
  (see `edit_test.go`) for every new operation.
- **Reversible.** Disabling and enabling something again must restore the
  original file exactly.
- **Validate, then write.** Changes always go through `app.Apply`
  (diff → `haproxy -c` → write → history → reload/runtime).
- Keep dependencies minimal, both in Go and in the web UI.
- UI texts exist in German and English (`web/src/i18n`).

## Development setup

Requirements: Go 1.26, Node 22, optionally `haproxy` (tests against a real
HAProxy are skipped without it).

```bash
make ui && make build
make test
make run   # read-only demo against testdata/realworld.cfg
```

`testdata/realworld.cfg` is an anonymised real-world configuration with lots of
comments and commented-out services. Use it to check that changes keep such
files intact. Please never commit real host names, IPs or secrets.

## Pull requests

- `gofmt`, `go vet` and `go test ./...` must pass, the UI must build
  (`npm run build`).
- Describe what changes for users and include screenshots for UI changes.
- By contributing you agree that your contribution is licensed under the
  AGPL-3.0.
