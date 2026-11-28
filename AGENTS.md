# Repository guidance

## Init scripts

`Runner.Init` in `internal/scaffold/scaffold.go` runs each checkout's `init.sh`
with `EMOBASE_CLI=1`; the scripts use it to suppress standalone hints.

- Keep `EMOBASE_CLI=1` there. Do not replace it with `--quiet` or a per-repo
  flag.
- The convention lives in `emobase-infra`, `emobase-portal-deploy` and
  `emobase-deploy`. Every new init script or checkout must follow it.
- A change to what `init.sh` prints usually needs the same change in all three
  repositories. Grep for `EMOBASE_CLI` and `Next:` before finishing.

## Realms and species

- Same-host species: `create species` generates the realm/client/secret/`coco`
  values and `bootstrap` creates the realm once the portal Keycloak is up.
- Remote species: `keycloak realm create` runs on the portal host first, and
  `create species` consumes the printed values. It must not generate or create
  a realm without a local portal checkout.
- `keycloak realm create|add-origin` and `scripts/create-realm.sh` share flags.
  Update both repositories in the same change.

## Verification

- `gofmt -l .`, `go vet ./...` and `go test ./...` must pass before committing.
- When a change affects `init.sh` output, verify both modes in the sibling
  checkouts: `EMOBASE_CLI=1 ./init.sh --yes` and `./init.sh --yes`.
