# Command reference

Every `emobase` command, its flags and the rules behind them. For install and
the first run, see [README.md](README.md).

## Commands

| Command | What it does |
| --- | --- |
| `emobase` | Workspace dashboard: stacks, shared URLs, findings |
| `emobase doctor [--fix] [--yes] [--live] [--json]` | Static (and optionally runtime) cross-repo checks; `--fix` applies safe repairs |
| `emobase status` | Live containers, Docker network, MySQL databases, realms and HTTP status |
| `emobase credential show [stack]` | Seed users per realm plus the Keycloak admin; passwords only when a stack is named |
| `emobase domain [plan\|apply\|status]` | Review and update public URLs, the Keycloak issuer and realm origins |
| `emobase species list` | List discovered species checkouts |
| `emobase species add [code]` | Same as `emobase create species` |
| `emobase species setup <ref>` | Create DB, register origin, initialise Directus, start |
| `emobase species remove <ref> [--purge --yes] [--keep-dir]` | Show (or execute) teardown commands and remove the checkout |
| `emobase create infra\|portal\|species [code]` | Ask for values, then run the repository's `init.sh`; flags and `--yes` support automation |
| `emobase sync [--dry-run]` | Create missing databases and realm origins for active stacks |
| `emobase prune [--yes]` | Remove state-owned databases, realms and index prefixes left by deleted stacks |
| `emobase up [--only <role\|stack>] [--pull]` | Start in dependency order |
| `emobase down [--only ...]` | Stop dependants first |
| `emobase update [--only ...] [--no-git] [--skip-migrations]` | Pull, migrate, restart |
| `emobase bootstrap [--only ...] [--dry-run]` | Full ordered bring-up from existing `.env` files |

`create` passes its flags to each repository's `init.sh`; see the
[infra](https://github.com/EMOBase/emobase-infra/blob/main/INIT.md),
[portal](https://github.com/EMOBase/emobase-portal-deploy/blob/main/INIT.md) and
[species](https://github.com/EMOBase/emobase-deploy/blob/dev/INIT.md) init
references for the prompts and flags it forwards.

Without `--only`, a command acts on every stack. `--only` accepts a role
(`infra`, `portal`, `species`) or one stack: its species code, checkout name,
Compose project name, or `species:<checkout>`. For example,
`emobase up --only Tcas`.

Use `--dry-run` on `create`, `sync`, `up`, `down`, `update` and `bootstrap` to
print every command instead of running it. `--no-color`, `NO_COLOR` and `--json`
are global.

Commands that create or remove resources track them in the workspace ownership
ledger; see [Ownership ledger](README.md#ownership-ledger).

## What `create` writes

`create` only scaffolds `.env`; it never starts containers. Review these values
after scaffolding.

**`infra/.env`** — `KEYCLOAK_PUBLIC_URL`, `KEYCLOAK_HOST_PORT`, the fixed
`KEYCLOAK_INTERNAL_URL=http://keycloak:8080/keycloak`, `ELASTICSEARCH_HOST_PORT`,
`ELASTICSEARCH_JAVA_OPTS`, `ELASTICSEARCH_MEM_LIMIT`, the shared Docker network
name, and the generated MySQL root and Keycloak admin passwords.

**`portal/.env`** — `PUBLIC_SERVER` and `HTTP_PORT` are the browser-facing
values; `EMOBASE_PORTAL_WEB_IMAGE` and `EMOBASE_GENOMICS_IMAGE` select the
images.

**`<species>/.env`**

| Variable | Meaning | Value |
| --- | --- | --- |
| `PUBLIC_SERVER` | The URL users open for this species | e.g. `https://tcas.example.org` |
| `PUBLIC_HOST` | Hostname part of `PUBLIC_SERVER`; the species nginx serves only this name | e.g. `tcas.example.org` |
| `PORTAL_SERVER` | Public portal URL used by browser links | the portal's `PUBLIC_SERVER` |
| `PORTAL_INTERNAL_URL` | Portal URL inside containers | `http://host.docker.internal:<portal port>` for a local portal; unset for a remote portal |
| `KEYCLOAK_PUBLIC_URL` | This server's Keycloak URL | from `infra/.env` |
| `KEYCLOAK_REALM` | Joined realm or a dedicated species realm | e.g. `emobase-portal`, `emobase-tcas` |
| `DIRECTUS_DB_DATABASE` | This species' database | e.g. `directus_tcas` |
| `EMOBASE_ES_INDEX_PREFIX` | This species' slice of the shared Elasticsearch | e.g. `tcas`; must be unique and not `genomics` |
| `COMPOSE_FILE` | Compose files for this stack | `compose.yml`; Tcas adds `:compose.tcas.yml` |

The realm mode is written as `KEYCLOAK_REALM_MODE=joined|dedicated`. See the
[realm model](https://github.com/EMOBase/emobase-docs/blob/main/REALMS.md) for
ownership and sharing.

`doctor` validates the result; see [What `doctor` checks](#what-doctor-checks).

## What `doctor` checks

The values are described above in [What `create` writes](#what-create-writes).
This section lists the rules, not the values.

- Every stack has a `.env` and every required key is non-empty.
- Shared keys match the authoritative `emobase-infra` values
  (`MYSQL_ROOT_PASSWORD`, `KEYCLOAK_PUBLIC_URL`, `KEYCLOAK_INTERNAL_URL` and
  `EMOBASE_INFRA_NETWORK`).
- Species `PORTAL_SERVER` values point at the local portal, or at a remote
  portal when the workspace has no `emobase-portal-deploy` checkout.
- `HTTP_PORT`, `PUBLIC_SERVER`, Directus database names and Compose project
  names are unique across stacks.
- URLs have no trailing slash.
- `INTERNAL_KEYCLOAK_ISSUER`/`DIRECTUS_KEYCLOAK_ISSUER_URL` end with
  `/realms/<realm>`.
- Container-facing URLs (`*_INTERNAL_URL`, `INTERNAL_*`) do not use `localhost`.
- A localhost Keycloak URL uses the published `KEYCLOAK_HOST_PORT`.
- Every checkout using the same realm uses the same client ID and secret.
- `COMPOSE_PROJECT_NAME` is set and unique.
- A species overlay such as `compose.tcas.yml` is listed in `COMPOSE_FILE`.
- The ownership ledger matches active stacks and their database/realm names.
- No `change-me` placeholders remain.

`--live` additionally checks the Docker daemon, the shared network, container
health, MySQL databases, Keycloak realm reachability, registered redirect URIs,
portal/species HTTP endpoints, and that the app container can fetch
`INTERNAL_KEYCLOAK_ISSUER` with its own DNS and CA store (TLS, DNS and
connection errors are reported separately).

### Fix classes

Findings advertise what it takes to resolve them:

| Class | Behaviour |
| --- | --- |
| `safe` | Applied by `doctor --fix`. Fills missing shared values, normalises trailing slashes, keeps `PUBLIC_HOST` equal to the `PUBLIC_SERVER` hostname, adds a derived Compose project name and lists species Compose overlays. |
| `choose` | Applied only by `doctor --fix --yes`. A real conflict, for example a mismatched MySQL password. Accepts infra's value. |
| `destructive` | Never applied automatically. Changing a Keycloak issuer/origin after the realm was created needs a manual realm update or a reset. `doctor` only reports it. |

`.env` files are edited in place, preserving comments and ordering. A
`<file>.env.bak` copy of the previous contents is kept before each save.

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
