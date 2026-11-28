# Command reference

Every `emobase` command, its flags and the rules behind them. For install and
the first run, see [README.md](README.md).

## Commands

| Command | What it does |
| --- | --- |
| `emobase` | Workspace dashboard: stacks, shared URLs, findings |
| `emobase doctor [--fix] [--yes] [--live] [--json]` | Static (and optionally runtime) cross-repo checks; `--fix` applies safe repairs |
| `emobase status` | Live containers, Docker network, MySQL databases, realms and HTTP status |
| `emobase credential show [stack\|keycloak]` | Seed users per realm plus the Keycloak admin (`keycloak`); passwords only when a stack is named |
| `emobase domain [plan\|apply\|status]` | Review and update public URLs, the Keycloak issuer and realm origins |
| `emobase image [list\|set\|reset]` | View or change the portal and species container images |
| `emobase species list` | List discovered species checkouts |
| `emobase species add [code]` | Same as `emobase create species` |
| `emobase species setup <ref>` | Create DB, initialise Directus, start |
| `emobase species remove <ref> [--purge --yes] [--keep-dir]` | Show (or execute) teardown commands and remove the checkout |
| `emobase create infra\|portal\|species [code]` | Ask for values, then run the repository's `init.sh`; flags and `--yes` support automation |
| `emobase keycloak realm create\|add-origin` | Create a realm/client or allow another origin (portal host only) |
| `emobase sync [--dry-run]` | Create missing databases and update the portal realm origin |
| `emobase prune [--yes]` | Remove state-owned databases and index prefixes left by deleted stacks |
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
after scaffolding. For a same-host species, `create species` also generates a
dedicated realm/client/secret and `coco` password; `bootstrap` creates them in
Keycloak.

**`infra/.env`** — `ELASTICSEARCH_HOST_PORT`, `ELASTICSEARCH_JAVA_OPTS`,
`ELASTICSEARCH_MEM_LIMIT`, the shared Docker network name and the generated
MySQL root password. Keycloak is not part of infra.

**`portal/.env`** — `PUBLIC_SERVER` and `HTTP_PORT` are the browser-facing
values; `KEYCLOAK_PUBLIC_URL`, `KEYCLOAK_INTERNAL_URL` and the Keycloak admin
credentials configure the portal-owned Keycloak; `EMOBASE_PORTAL_WEB_IMAGE` and
`EMOBASE_GENOMICS_IMAGE` select the images.

**`<species>/.env`**

| Variable | Meaning | Value |
| --- | --- | --- |
| `PUBLIC_SERVER` | The URL users open for this species | e.g. `https://tcas.example.org` |
| `PUBLIC_HOST` | Hostname part of `PUBLIC_SERVER`; the species nginx serves only this name | e.g. `tcas.example.org` |
| `PORTAL_SERVER` | Public portal URL used by browser links | the portal's `PUBLIC_SERVER` |
| `PORTAL_INTERNAL_URL` | Portal URL inside containers | `http://host.docker.internal:<portal port>` for a local portal; unset for a remote portal |
| `KEYCLOAK_PUBLIC_URL` | The portal Keycloak URL | from `portal/.env` |
| `KEYCLOAK_INTERNAL_URL` | Keycloak URL inside containers | `http://host.docker.internal:<portal port>/keycloak` for a local portal; unset for a remote portal |
| `KEYCLOAK_REALM` | Realm; generated for a same-host species, existing for a remote one | e.g. `emobase-portal`, `emobase-tcas` |
| `DIRECTUS_DB_DATABASE` | This species' database | e.g. `directus_tcas` |
| `EMOBASE_ES_INDEX_PREFIX` | This species' slice of the shared Elasticsearch | e.g. `tcas`; must be unique and not `genomics` |
| `COMPOSE_FILE` | Compose files for this stack | `compose.yml`; Tcas adds `:compose.tcas.yml` |

`doctor` validates the result; see [What `doctor` checks](#what-doctor-checks).

## What `doctor` checks

The values are described above in [What `create` writes](#what-create-writes).
This section lists the rules, not the values.

- Every stack has a `.env` and every required key is non-empty.
- Shared keys match the authoritative `emobase-infra` values
  (`MYSQL_ROOT_PASSWORD` and `EMOBASE_INFRA_NETWORK`).
- Species `PORTAL_SERVER` values point at the local portal, or at a remote
  portal when the workspace has no `emobase-portal-deploy` checkout.
- `HTTP_PORT`, `PUBLIC_SERVER`, Directus database names and Compose project
  names are unique across stacks.
- URLs have no trailing slash.
- `KEYCLOAK_INTERNAL_URL`, when set, is a base URL without `/realms/`; a
  loopback public Keycloak needs a container-reachable value. Directus exits
  when OIDC discovery fails, so a same-host species needs the host-gateway URL.
- Container-facing URLs (`*_INTERNAL_URL`, `INTERNAL_*`) do not use `localhost`.
- Every checkout using the same realm uses the same client ID and secret.
- `COMPOSE_PROJECT_NAME` is set and unique.
- A species overlay such as `compose.tcas.yml` is listed in `COMPOSE_FILE`.
- The ownership ledger matches active stacks and their database/realm names.
- No `change-me` placeholders remain.

`--live` additionally checks the Docker daemon, the shared network, container
health, MySQL databases, Keycloak realm reachability, registered redirect URIs,
portal/species HTTP endpoints, and that the app container can fetch the
server-side issuer (`KEYCLOAK_INTERNAL_URL`, falling back to
`KEYCLOAK_PUBLIC_URL`) with its own DNS and CA store (TLS, DNS and connection
errors are reported separately).

### Fix classes

Findings advertise what it takes to resolve them:

| Class | Behaviour |
| --- | --- |
| `safe` | Applied by `doctor --fix`. Fills missing shared values, normalises trailing slashes, keeps `PUBLIC_HOST` equal to the `PUBLIC_SERVER` hostname, adds a derived Compose project name and lists species Compose overlays. |
| `choose` | Applied only by `doctor --fix --yes`. A real conflict, for example a mismatched MySQL password. Accepts infra's value. |
| `destructive` | Never applied automatically. Changing a Keycloak issuer/origin after the realm was created needs a manual realm update or a reset. `doctor` only reports it. |

`.env` files are edited in place, preserving comments and ordering. A
`<file>.env.bak` copy of the previous contents is kept before each save.

## Keycloak realms

Keycloak runs in the portal stack. Use `realm create` when the species is on
another server, or when you want the realm explicitly; a same-host
`create species` generates the credentials and `bootstrap` creates the realm.

```bash
emobase keycloak realm create --species Tcas
emobase keycloak realm add-origin \
  --realm emobase-portal \
  --client-id emobase-portal-client \
  --origin https://tcas.example.org
```

Realm ownership, the same-host and remote flows, defaults, `--print-env`, and
`--rotate-secret` are covered in
[REALMS.md](https://github.com/EMOBase/emobase-docs/blob/main/REALMS.md).

## Changing public URLs

Use `emobase domain` after DNS and TLS work. It changes the browser-facing
URLs, the Keycloak issuer, the dependent `.env` files, and the realm origins in
one transaction.

```sh
emobase domain            # prompts for the portal and species URLs
emobase domain plan       # show the changes without applying them
emobase domain status     # show the configured URLs
```

The Keycloak URL is derived as `<portal>/keycloak`; it is not asked separately.
Use `--auth` only for an external Keycloak.

Non-interactive form:

```sh
emobase domain apply --yes \
  --portal https://portal.example.org \
  --species Tcas=https://tcas.example.org
```

Pass `--species REF=URL` once per changed species. Values not passed keep their
current setting. Changing the portal URL also changes the derived Keycloak
issuer, which invalidates existing login sessions.

The command does not configure DNS, TLS, or the reverse proxy, and it refuses
to apply changes while Keycloak is down. Old realm origins are left in place;
remove them in the Keycloak admin console if the cutover is permanent.

## Changing container images

The portal and every species stack select their application image from that
stack's `.env`:

| Target | Variable | Stack |
| --- | --- | --- |
| `genomics` | `EMOBASE_GENOMICS_IMAGE` | portal |
| `portal-web` | `EMOBASE_PORTAL_WEB_IMAGE` | portal |
| `species-web` | `EMOBASE_WEB_IMAGE` | every species, or one with `--only` |
| `ontologyservice` | `ONTOLOGYSERVICE_IMAGE` | every species, or one with `--only` |
| `imageservice` | `IMAGESERVICE_IMAGE` | every species, or one with `--only` |
| `phenotypeservice` | `PHENOTYPESERVICE_IMAGE` | every species, or one with `--only` |
| `goannotationservice` | `GOANNOTATIONSERVICE_IMAGE` | every species, or one with `--only` |
| `publicationservice` | `PUBLICATIONSERVICE_IMAGE` | every species, or one with `--only` |
| `oba` | `OBA_IMAGE` | Tcas-overlay species, or one with `--only` |

The full repository names (`emobase-genomics`, `emobase-portal-web`,
`emobase-web`) are accepted as target aliases, as is `web` for `species-web`.
`oba` is only available when the stack's `COMPOSE_FILE` includes
`compose.tcas.yml`.

By default `emobase image` lists the main app images plus any supporting service
image that has an explicit override. `--all` also lists the supporting services
that still use their defaults. The `default` column is `yes` when the effective
image equals the `${VAR:-default}` fallback declared in the Compose file, and
`no` when `.env` overrides it. It is `-` when Compose declares no fallback. An
empty value in `.env` is treated the same as unset, matching Compose.

```sh
emobase image                                      # same as emobase image list
emobase image --all                                # include unchanged services
emobase image set genomics ghcr.io/emobase/emobase-genomics:v0.6.0
emobase image set species-web emobase-web:local    # every species stack
emobase image set species-web emobase-web:local --only Tcas
emobase image set ontologyservice ghcr.io/emobase/ibb-ontologyservice:test
emobase image set oba azul/zulu-openjdk:7 --only Tcas
emobase image reset genomics                       # back to the Compose default
emobase image reset species-web --only Tcas
emobase image reset --all                          # every managed override
```

`set` updates the key in place, or appends it when absent. `reset` removes it;
`reset --all` removes every managed image override in the workspace and saves
each affected `.env` once. When the Compose file declares no fallback, a
targeted reset refuses to remove the image; `reset --all` leaves that image
unchanged and prints a warning. Both preserve comments and ordering and keep a
`.env.bak` copy. `reset --all` cannot be combined with a target or `--only`.
Neither recreates containers: run `emobase up --only portal` (or the affected
checkout or `--only species`) to apply the change.

Supporting service targets are the overridable images in
`emobase-deploy/compose.yml` plus the Tcas overlay. Other images, such as
`nginx`, `directus`, `querypipelineservice`, MySQL, Keycloak and Elasticsearch,
are fixed in their Compose files and are not managed by this command.

## JSON output

`--json` works with the dashboard, `doctor`, `status`, `image` and
`species list`. Examples:

```sh
emobase doctor --json | jq '.findings[] | select(.severity == "error")'
emobase image --json | jq -r '.[] | "\(.target)\t\(.image)\t\(.source)"'
emobase species list --json | jq -r '.[] | "\(.species)\t\(.url)"'
```

Exit codes: `0` success, `1` failed checks or command error.

Runtime checks degrade gracefully: if Docker is not running, `doctor --live`
reports a single warning instead of failing.

## How it decides what to run

No registry file is needed. Each immediate child of the workspace is classified
from its `compose.yml`:

- `mysql` service → infrastructure
- `genomicsservice` service → portal (it also owns `keycloak` now)
- `directus` service → species

Exactly one infra and one portal are allowed; species are unlimited. A checkout
is identified by its directory name, `MAIN_SPECIES`, or Compose project name, so
`emobase species setup tcas`, `... Tcas` and `... emobase-deploy-tcas` all work.
