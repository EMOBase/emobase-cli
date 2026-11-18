# emobase-cli

`emobase` is the cross-repository control point for an EMOBase deployment. It
scaffolds each stack with that repository's own `init.sh`, validates the
configuration shared between the stacks, and runs them in dependency order.

It is **not** a fourth source of configuration. Every checkout keeps its own
`.env` and its own secrets:

| Checkout | Role | Owns |
| --- | --- | --- |
| `infra` | shared infrastructure | MySQL root password, Keycloak admin credentials, Keycloak public/internal URLs, the shared Docker network |
| `portal` | multi-species portal | `PUBLIC_SERVER`, portal realm/client/seed credentials, genomics database settings |
| `<Species>` | one species site | joined or new realm credentials, Directus database, HTTP port, Compose project name |

`emobase create` uses these names and prompts for the directory, so a deployment
can rename a checkout without breaking discovery (each checkout is identified by
its `compose.yml`, not its path). One species clone exists per species. The CLI
never stores secrets, never copies them between checkouts, and never rewrites
a file it does not need to touch.

## Install

Install the latest stable release:

```sh
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh | sh
```

For the rolling `main` build, manual downloads, or building from source, see
[INSTALL.md](INSTALL.md).

## Quick start

Put the checkouts side by side. Directory names may differ; each checkout is
identified by its `compose.yml`.

```
work/
├── infra/
├── portal/
├── Tcas/
└── Dmel/
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

`create` only writes `.env` files; it does not start containers. The species
setup prompt appears only when infra is running. Use `emobase bootstrap` to
start everything, or `emobase up --only infra` before running setup.

`emobase` with no arguments prints a dashboard of the discovered workspace.
`--workspace DIR` points at another directory; when run from inside a checkout,
the parent directory is used automatically.

Without `--local`, URL questions default to the host's primary IPv4 address
(the address used for the default route). If the server is behind NAT, enter
its public IP or domain instead. `--local` uses `localhost` for a same-machine
trial.

Every question can also be answered by a flag, which is useful for automation.
A value passed as a flag skips only that question. `--yes` accepts every
default without prompting:

```sh
# Local trial without questions.
emobase --yes create infra --local
emobase --yes create portal --local
emobase --yes create species --species Tcas --local
emobase bootstrap

# Server deployment: answer only the URL questions.
emobase create infra \
  --keycloak-public-url https://auth.example.org/keycloak

emobase create portal --public-server https://portal.example.org
emobase create species --species Tcas \
  --public-server https://tcas.example.org
emobase bootstrap
```

`create portal` and `create species` create or update their own Keycloak realm
when infra is already running. A species can join the portal realm, join
another species realm, or create a new independent realm. If infra is not
running, `emobase up` or `emobase bootstrap` creates the realm later.

Command-level details:

- [Deployment runbook](https://github.com/EMOBase/emobase-docs/blob/main/DEPLOYMENT.md)
- [Infra init reference](https://github.com/EMOBase/emobase-infra/blob/main/INIT.md)
- [Portal init reference](https://github.com/EMOBase/emobase-portal-deploy/blob/main/INIT.md)
- [Species init reference](https://github.com/EMOBase/emobase-deploy/blob/dev/INIT.md)
- [Realm model](https://github.com/EMOBase/emobase-docs/blob/main/REALMS.md)

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

## Ownership ledger

The CLI records which external resources it created in:

```text
<workspace>/.emobase/state.json
```

This is not a copy of `.env`. It records ownership: which database, realm and
Elasticsearch index prefix belong to which checkout. It contains no secrets and
is specific to one workspace/server, so do not commit it.

- `create` records the stack with its resource names but `created: false`.
- `species setup` and `bootstrap` mark resources created after success.
- `species remove --purge` stops the stack, drops its database, deletes its
  `<prefix>-*` Elasticsearch indices, removes the checkout directory and uses
  the ledger. If a resource was never created, it does not require MySQL,
  Keycloak or Elasticsearch. If cleanup cannot run, it marks the entry
  `pending_cleanup`. Add `--keep-dir` to keep the directory.
- `emobase prune` removes databases, dedicated realms and index prefixes
  recorded in the ledger for removed checkouts. It never guesses from names and
  never touches resources that are not recorded.
- Shared realms and shared index prefixes are skipped while any active checkout
  still uses them.

Example:

```sh
emobase sync                 # create missing databases/realms for active stacks
emobase sync --dry-run
emobase prune                # show the cleanup and confirm before deleting
emobase prune --yes          # delete without prompting
```

`sync` does not start containers. `bootstrap` syncs and starts, but never
prunes.

## Commands

| Command | What it does |
| --- | --- |
| `emobase` | Workspace dashboard: stacks, shared URLs, findings |
| `emobase doctor [--fix] [--yes] [--live] [--json]` | Static (and optionally runtime) cross-repo checks; `--fix` applies safe repairs |
| `emobase status` | Live containers, Docker network, MySQL databases, realms and HTTP status |
| `emobase credential show [stack]` | Seed users per realm plus the Keycloak admin; passwords only when a stack is named |
| `emobase domain [plan\|apply\|status]` | Review and update public URLs, the Keycloak issuer and realm origins |
| `emobase species list` | List discovered species checkouts |
| `emobase species add ...` | Same as `emobase create species` |
| `emobase species setup <ref>` | Create DB, register origin, initialise Directus, start |
| `emobase species remove <ref> [--purge --yes] [--keep-dir]` | Show (or execute) teardown commands and remove the checkout |
| `emobase create infra\|portal\|species` | Ask for values, then run the repository's `init.sh`; flags and `--yes` support automation |
| `emobase sync [--dry-run]` | Create missing databases and realm origins for active stacks |
| `emobase prune [--yes]` | Remove state-owned databases, realms and index prefixes left by deleted stacks |
| `emobase up [--only infra\|portal\|species] [--pull]` | Start in dependency order |
| `emobase down [--only ...]` | Stop dependants first |
| `emobase update [--only ...] [--no-git] [--skip-migrations]` | Pull, migrate, restart |
| `emobase bootstrap [--only ...] [--dry-run]` | Full ordered bring-up from existing `.env` files |

Use `--dry-run` on `create`, `sync`, `up`, `down`, `update` and `bootstrap` to
print every command instead of running it. `--no-color`, `NO_COLOR` and `--json` are
global.

## What `doctor` checks

- Every stack has a `.env` and every required key is non-empty.
- Shared keys match the authoritative `emobase-infra` values
  (`MYSQL_ROOT_PASSWORD`, `KEYCLOAK_PUBLIC_URL`, `KEYCLOAK_INTERNAL_URL` and
  `EMOBASE_INFRA_NETWORK`).
- Species `PORTAL_SERVER` values point at the local portal.
- `HTTP_PORT`, `PUBLIC_SERVER`, Directus database names and Compose project
  names are unique across stacks.
- URLs have no trailing slash.
- `INTERNAL_KEYCLOAK_ISSUER`/`DIRECTUS_KEYCLOAK_ISSUER_URL` end with
  `/realms/<realm>`.
- Container-facing URLs (`*_INTERNAL_URL`, `INTERNAL_*`) do not use `localhost`.
- A localhost Keycloak URL uses the published `KEYCLOAK_HOST_PORT`.
- Every checkout using the same realm uses the same client ID and secret.
- `COMPOSE_PROJECT_NAME` is set and unique.
- The ownership ledger matches active stacks and their database/realm names.
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

`--json` works with the dashboard, `doctor`, `status` and `species list`.
Examples:

```sh
emobase doctor --json | jq '.findings[] | select(.severity == "error")'
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
species list and runtime status, including empty, warning, conflict and
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
