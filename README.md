# emobase-cli

`emobase` is the cross-repository control point for an EMOBase deployment. It
scaffolds each stack with that repository's own `init.sh`, validates the
configuration shared between the stacks, and runs them in dependency order.

It is **not** a fourth place to configure the stacks. Every checkout keeps its
own `.env` and its own secrets, and changing a setting means editing that
checkout's `.env`:

| Checkout | Role | Owns |
| --- | --- | --- |
| `infra` | shared infrastructure | MySQL root password, Elasticsearch settings, the shared Docker network |
| `portal` | multi-species portal | `PUBLIC_SERVER`, Keycloak admin credentials and public/internal URLs, portal realm/client/seed credentials, genomics database settings |
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

Create an empty workspace directory and run `emobase` from it. `create` clones
each repository when it is missing — it asks before cloning, and `--yes` accepts
the default. For an unattended run without `--yes`, pass `--clone`. Directory
names may differ; each checkout is identified by its `compose.yml`, not its
path. Keep all checkouts in the same parent so the repositories can read each
other's `.env`; `--dir` places a checkout elsewhere.

```sh
# 1. Scaffold. Each command asks for its values and shows the repository
#    default; press Enter to accept it. Missing checkouts are cloned on confirm.
emobase create infra
emobase create portal
emobase create species Tcas  # the species code is required

# Add another species. It gets its own checkout, port and Directus database.
emobase create species Dmel

# 2. Validate and repair the cross-repository contract.
emobase doctor

# 3. Bring everything up in the documented order.
emobase bootstrap
```

After scaffolding, the workspace looks like this:

```text
workspace/
├── infra/
├── portal/
├── Tcas/      # one emobase-deploy clone per species
└── Dmel/
```

The species code has no default: pass it as an argument, for example
`emobase create species Tcas`.

`create` clones missing checkouts and writes their `.env` files; it does not
start containers. The species setup prompt appears only when infra is running.
Use `emobase bootstrap` to start everything, or `emobase up --only infra` before
running setup.

`emobase` with no arguments prints a dashboard of the discovered workspace.
`--workspace DIR` points at another directory; when run from inside a checkout,
the parent directory is used automatically.

Without `--local`, URL questions default to the host's primary IPv4 address
(the address used for the default route). If the server is behind NAT, enter
its public IP or domain instead. `--local` uses `localhost` for a same-machine
trial.

Every other question can also be answered by a flag, which is useful for
automation. A value passed as a flag skips only that question. `--yes` accepts
every default without prompting.

Unattended local setup:

```sh
emobase --yes create infra --local
emobase --yes create portal --local
emobase --yes create species Tcas --local
emobase bootstrap
```

On a server, pass only the URLs and keep the rest of the defaults:

```sh
emobase create infra \
  --keycloak-public-url https://auth.example.org/keycloak
emobase create portal --public-server https://portal.example.org
emobase create species Tcas \
  --public-server https://tcas.example.org
emobase bootstrap
```

`create portal` and `create species` create or update their own Keycloak realm
when infra is already running. A species can join the portal realm, join
another species realm, or create a new independent realm. If infra is not
running, `emobase up` or `emobase bootstrap` creates the realm later.

## Commands

```sh
emobase create infra|portal|species   # scaffold .env; clones missing checkouts
emobase bootstrap                     # first start, in dependency order
emobase status                        # containers, databases, realms and URLs
emobase doctor [--live]               # check the cross-repo configuration
emobase domain [plan|apply|status]    # change public URLs and Keycloak issuer
emobase up                            # start in dependency order
emobase down                          # stop dependants first
emobase update                        # pull, migrate, restart
```

The full command reference — commands, flags, what `create` writes, the `doctor`
rules, URL changes and JSON output — is in [COMMANDS.md](COMMANDS.md).

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

## Related documentation

- [Deployment guide](https://github.com/EMOBase/emobase-docs/blob/main/DEPLOYMENT.md) — empty server → working site with `emobase`.
- [Operations](https://github.com/EMOBase/emobase-docs/blob/main/OPERATIONS.md) — add a species, update, change URLs, sync, remove, prune.
- [Realm model](https://github.com/EMOBase/emobase-docs/blob/main/REALMS.md) — realm ownership, sharing and credential checks.

## Development

```sh
make test     # go test ./...
make vet      # go vet ./...
make fmt      # gofmt -w .
make build
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the UI preview tool, the package
layout and the release process.
