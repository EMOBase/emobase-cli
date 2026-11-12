# emobase-cli

`emobase` is the cross-repository control point for an EMOBase deployment. It
scaffolds each stack with that repository's own `init.sh`, validates the
configuration shared between the stacks, and runs them in dependency order.

It is **not** a fourth source of configuration. Every checkout keeps its own
`.env` and its own secrets:

| Checkout | Role | Owns |
| --- | --- | --- |
| `emobase-infra` | shared infrastructure | MySQL root password, Keycloak public/internal URLs, the shared Docker network, the shared realm and client |
| `emobase-portal-deploy` | multi-species portal | `PUBLIC_SERVER`, portal secrets, genomics database settings |
| `emobase-deploy-<species>` | one species site | dedicated realm credentials (when used), Directus database, HTTP port, Compose project name |

One `emobase-deploy` clone exists per species. The CLI never stores secrets,
never copies them between checkouts, and never rewrites a file it does not need
to touch.

## Install

Prebuilt binaries are available for Linux and macOS (`amd64` and `arm64`) on
the [releases page](https://github.com/EMOBase/emobase-cli/releases). Building
from source requires Go 1.25 or newer.

The deployment host needs Docker with Compose, `git`, and `python3`.

### Latest build from `main` (rolling)

Every push to `main` publishes a `latest` release with stable asset names. It
changes with each commit, so use a tagged release for production
reproducibility. `emobase --version` reports `main-<commit>`.

#### Linux server

```sh
case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)"; exit 1 ;;
esac
BASE="https://github.com/EMOBase/emobase-cli/releases/download/latest"

curl -fsSLO "$BASE/emobase_linux_${ARCH}.tar.gz"
curl -fsSLO "$BASE/checksums.txt"
grep "linux_${ARCH}" checksums.txt | sha256sum -c -
tar -xzf "emobase_linux_${ARCH}.tar.gz" emobase
sudo install -m 0755 emobase /usr/local/bin/emobase
emobase --version
```

#### macOS

```sh
case "$(uname -m)" in
  x86_64) ARCH=amd64 ;;
  arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)"; exit 1 ;;
esac
BASE="https://github.com/EMOBase/emobase-cli/releases/download/latest"

curl -fsSLO "$BASE/emobase_darwin_${ARCH}.tar.gz"
curl -fsSLO "$BASE/checksums.txt"
grep "darwin_${ARCH}" checksums.txt | shasum -a 256 -c -
tar -xzf "emobase_darwin_${ARCH}.tar.gz" emobase
./emobase --version
sudo install -m 0755 emobase /usr/local/bin/emobase
```

### Tagged release (pinned)

#### Linux server

```sh
VER=0.1.1
ARCH=amd64        # uname -m: x86_64 -> amd64, aarch64/arm64 -> arm64
BASE="https://github.com/EMOBase/emobase-cli/releases/download/v${VER}"

curl -fsSLO "$BASE/emobase_${VER}_linux_${ARCH}.tar.gz"
curl -fsSLO "$BASE/checksums.txt"
grep "linux_${ARCH}" checksums.txt | sha256sum -c -
tar -xzf "emobase_${VER}_linux_${ARCH}.tar.gz" emobase
sudo install -m 0755 emobase /usr/local/bin/emobase
emobase --version
```

#### macOS

```sh
VER=0.1.1
ARCH=arm64        # arm64 on Apple Silicon, amd64 on Intel
BASE="https://github.com/EMOBase/emobase-cli/releases/download/v${VER}"

curl -fsSLO "$BASE/emobase_${VER}_darwin_${ARCH}.tar.gz"
curl -fsSLO "$BASE/checksums.txt"
grep "darwin_${ARCH}" checksums.txt | shasum -a 256 -c -
tar -xzf "emobase_${VER}_darwin_${ARCH}.tar.gz" emobase
./emobase --version
sudo install -m 0755 emobase /usr/local/bin/emobase
```

To install without sudo, use `install -D -m 0755 emobase ~/.local/bin/emobase`
in either section.

### Build from source (local machine)

```sh
git clone https://github.com/EMOBase/emobase-cli.git
cd emobase-cli
make build            # writes ./dist/emobase
./dist/emobase --version
```

Put it on your `PATH` with one of:

```sh
make install-user                        # ~/.local/bin/emobase, no sudo
make install                             # /usr/local/bin/emobase, may need sudo
PREFIX=/opt/homebrew make install        # Homebrew prefix on Apple Silicon
```

`~/.local/bin` is not always on `PATH`; add it in `~/.zshrc` if needed:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

### Build and copy to a server (offline or unreleased)

Cross-compile the Linux binaries on your machine:

```sh
make build-linux      # dist/emobase-linux-amd64 and dist/emobase-linux-arm64
```

Pick the architecture matching the server (`uname -m`: `x86_64` → `amd64`,
`aarch64`/`arm64` → `arm64`), copy it over, and install it system-wide:

```sh
scp dist/emobase-linux-amd64 you@server:/tmp/emobase
ssh you@server 'sudo install -m 0755 /tmp/emobase /usr/local/bin/emobase && emobase --version'
```

Or install for one user without sudo:

```sh
ssh you@server 'install -D -m 0755 /tmp/emobase ~/.local/bin/emobase'
```

Then run it from the directory that holds the checkouts, or point at it
explicitly:

```sh
cd /srv/emobase && emobase doctor
emobase --workspace /srv/emobase doctor
```

## Quick start

Put the checkouts side by side. Directory names may differ; each checkout is
identified by its `compose.yml`.

```
work/
├── emobase-infra/
├── emobase-portal-deploy/
├── emobase-deploy-tcas/
└── emobase-deploy-dmel/
```

```sh
cd work

# 1. Scaffold. Each command asks for its values and shows the repository
#    default; press Enter to accept it.
emobase create infra
emobase create portal
emobase create species       # the species code is required, for example Tcas

# Add another species. It gets its own checkout, port and Directus database.
emobase create species

# 2. Validate and repair the cross-repository contract.
emobase doctor

# 3. Bring everything up in the documented order.
emobase bootstrap
```

The species code has no default: you must type it, for example `Tcas`.

`emobase` with no arguments prints a dashboard of the discovered workspace.
`--workspace DIR` points at another directory; when run from inside a checkout,
the parent directory is used automatically.

Every question can also be answered by a flag, which is useful for automation.
A value passed as a flag skips only that question. `--yes` accepts every
default without prompting:

```sh
# Local trial without questions.
emobase --yes create infra --local
emobase --yes create portal --local
emobase --yes create species --species Tcas --local --clone
emobase bootstrap

# Server deployment: answer only the URL questions.
emobase create infra \
  --keycloak-public-url https://auth.example.org/keycloak

emobase create portal --public-server https://portal.example.org
emobase create species --species Tcas \
  --public-server https://tcas.example.org \
  --clone
emobase bootstrap
```

`create portal` and `create species` create or update the shared Keycloak realm
when infra is already running. If it is not, `emobase up` or
`emobase bootstrap` does it after starting infra.

## Changing public URLs

Use `emobase domain` after DNS and TLS work. It changes the browser-facing
URLs, the Keycloak issuer, the dependent `.env` files, and the realm origins in
one transaction.

```sh
emobase domain            # prompts with the current values as defaults
emobase domain plan       # show the changes without applying them
emobase domain status     # show the configured URLs
```

Non-interactive form:

```sh
emobase domain apply --yes \
  --auth https://auth.example.org/keycloak \
  --portal https://portal.example.org \
  --species Tcas=https://tcas.example.org
```

Pass `--species REF=URL` once per changed species. Values not passed keep their
current setting. Changing the Keycloak URL invalidates existing login sessions;
changing the portal URL updates every species `PORTAL_SERVER`.

The command does not configure DNS, TLS, or the reverse proxy, and it refuses
to apply changes while Keycloak is down. Old realm origins are left in place;
remove them in the Keycloak admin console if the cutover is permanent.

## Commands

| Command | What it does |
| --- | --- |
| `emobase` | Workspace dashboard: stacks, shared URLs, findings |
| `emobase doctor [--fix] [--yes] [--live] [--json]` | Static (and optionally runtime) cross-repo checks; `--fix` applies safe repairs |
| `emobase status` | Live containers, Docker network, MySQL databases, realms and HTTP status |
| `emobase env matrix [--json]` | One row per shared key, one column per stack; secrets masked |
| `emobase domain [plan\|apply\|status]` | Review and update public URLs, the Keycloak issuer and realm origins |
| `emobase species list` | List discovered species checkouts |
| `emobase species add ...` | Same as `emobase create species` |
| `emobase species setup <ref>` | Create DB, register origin, initialise Directus, start |
| `emobase species remove <ref> [--purge --yes]` | Show (or execute) teardown commands |
| `emobase create infra\|portal\|species` | Ask for values, then run the repository's `init.sh`; flags and `--yes` support automation |
| `emobase up [--only infra\|portal\|species] [--pull]` | Start in dependency order |
| `emobase down [--only ...]` | Stop dependants first |
| `emobase update [--only ...] [--no-git] [--skip-migrations]` | Pull, migrate, restart |
| `emobase bootstrap [--only ...] [--dry-run]` | Full ordered bring-up from existing `.env` files |

Use `--dry-run` on `create`, `up`, `down`, `update` and `bootstrap` to print
every command instead of running it. `--no-color`, `NO_COLOR` and `--json` are
global.

## What `doctor` checks

- Every stack has a `.env` and every required key is non-empty.
- Shared keys match the authoritative `emobase-infra` values
  (`MYSQL_ROOT_PASSWORD`, `KEYCLOAK_PUBLIC_URL`, `KEYCLOAK_INTERNAL_URL`,
  `EMOBASE_INFRA_NETWORK`, and the shared realm/client keys when used).
- Species `PORTAL_SERVER` values point at the local portal.
- `HTTP_PORT`, `PUBLIC_SERVER`, Directus database names and Compose project
  names are unique across stacks.
- URLs have no trailing slash.
- `INTERNAL_KEYCLOAK_ISSUER`/`DIRECTUS_KEYCLOAK_ISSUER_URL` end with
  `/realms/<realm>`.
- Container-facing URLs (`*_INTERNAL_URL`, `INTERNAL_*`) do not use `localhost`.
- A localhost Keycloak URL uses the published `KEYCLOAK_HOST_PORT`.
- A dedicated-realm species owns its client ID and secret.
- `COMPOSE_PROJECT_NAME` is set and unique.
- No `change-me` placeholders remain.

`--live` additionally checks the Docker daemon, the shared network, container
health, MySQL databases, Keycloak realm reachability, registered redirect URIs
and portal/species HTTP endpoints.

### Fix classes

Findings advertise what it takes to resolve them:

| Class | Behaviour |
| --- | --- |
| `safe` | Applied by `doctor --fix`. Fills missing shared values, normalises trailing slashes, adds a derived Compose project name. |
| `choose` | Applied only by `doctor --fix --yes`. A real conflict, for example a mismatched MySQL password. Accepts infra's value. |
| `destructive` | Never applied automatically. Changing a Keycloak issuer/origin after the realm was created needs a manual realm update or a reset. `doctor` only reports it. |

`.env` files are edited in place, preserving comments and ordering. A `<file>.env.bak`
copy of the previous contents is kept before each save.

## JSON output

`--json` works with the dashboard, `doctor`, `status`, `env matrix` and
`species list`. Examples:

```sh
emobase doctor --json | jq '.findings[] | select(.severity == "error")'
emobase env matrix --json | jq -r '.headers, .rows[]'
emobase species list --json | jq -r '.[] | "\(.species)\t\(.url)"'
```

Exit codes: `0` success, `1` failed checks or command error.

Runtime checks degrade gracefully: if Docker is not running, `doctor --live`
reports a single warning instead of failing.

## How it decides what to run

No registry file is needed. Each immediate child of the workspace is classified
from its `compose.yml`:

- `keycloak` **and** `mysql` services → infrastructure
- `genomicsservice` service → portal
- `directus` service → species

Exactly one infra and one portal are allowed; species are unlimited. A checkout
is identified by its directory name, `MAIN_SPECIES`, or Compose project name, so
`emobase species setup tcas`, `... Tcas` and `... emobase-deploy-tcas` all work.

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
matrix, species list and runtime status, including empty, warning, conflict and
Docker-unavailable states.

Layout:

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
git tag v0.1.2
git push origin v0.1.2
gh run watch --workflow=release.yml
```

Latest main build:

```sh
git push origin main
gh run watch --workflow=latest.yml
```

To test the GoReleaser configuration locally without publishing:

```sh
brew install goreleaser
goreleaser release --snapshot --clean   # writes dist/
```
