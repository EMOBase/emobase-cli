# Contributing

## Development

```sh
make test     # go test ./...
make vet      # go vet ./...
make fmt      # gofmt -w .
make build
```

### UI preview

`cmd/preview` renders each UI case from fixtures, without a real workspace or
Docker. It is a development tool and is not part of the release binary.

```sh
make preview                         # list every case
make preview-case CASE=doctor-findings
make preview-all                     # render every case
```

Flags: `--no-color`, `--all`, `--help`. Cases cover the dashboard, doctor,
container image list/set/reset states, species list and runtime status,
including empty, warning, conflict and Docker-unavailable states.

### Layout

```
cmd/emobase          entry point
cmd/preview          dev-only UI case browser
internal/envfile     comment-preserving dotenv parser/editor
internal/workspace   signature-based checkout discovery
internal/spec        required keys, shared-key authority, masking
internal/checks      cross-repo checks and safe/choose/destructive fixes
internal/live        Docker, MySQL, HTTP probes
internal/keycloak    minimal Keycloak admin client and redirect matching
internal/scaffold    command runner for the repositories' own scripts
internal/cli         cobra commands and rendering
internal/ui          colour and tables
```

## Releasing

There are two channels:

- **Latest main build** — every push to `main` runs
  `.github/workflows/latest.yml`. It tests the commit, builds all four
  platforms, and moves the `latest` tag to publish stable asset names such as
  `emobase_linux_amd64.tar.gz` and `checksums.txt`. The `latest` release is
  marked as a pre-release so tagged versions remain GitHub's "Latest".
- **Tagged release** — pushing a `v*` tag runs `.github/workflows/release.yml`
  with GoReleaser (`.goreleaser.yaml`) and publishes versioned assets.

Tagged release:

```sh
git tag v0.2.0
git push origin v0.2.0
gh run watch "$(gh run list --workflow=release.yml --limit 1 \
  --json databaseId --jq '.[0].databaseId')"
```

Latest main build:

```sh
git push origin main
gh run watch "$(gh run list --workflow=latest.yml --limit 1 \
  --json databaseId --jq '.[0].databaseId')"
```

To test the GoReleaser configuration locally without publishing:

```sh
brew install goreleaser
goreleaser release --snapshot --clean   # writes dist/
```
