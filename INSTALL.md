# Installing emobase

`emobase` is a standalone binary with no runtime dependencies. It is available
for:

| OS | Architectures |
| --- | --- |
| Linux | `amd64` (`x86_64`), `arm64` (`aarch64`) |
| macOS | `amd64` (Intel), `arm64` (Apple Silicon) |

A machine that runs deployments also needs Docker with Compose, `git`, and
`python3`.

## Release channels

| Channel | Tag | Use |
| --- | --- | --- |
| Stable | `vX.Y.Z` | Recommended for production; reproducible and pinned |
| Rolling main | `latest` | Newest `main` build; pre-release, changes with every commit |
| Source | `main` | Development, offline hosts, or unsupported platforms |

All published assets and checksums are on the
[releases page](https://github.com/EMOBase/emobase-cli/releases).

## Stable release (recommended)

This resolves the newest `v*` release, verifies its checksum, and installs
`emobase` system-wide in `/usr/local/bin`. The download happens in a temporary
directory that is removed automatically.

```sh
(
  set -e
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  cd "$tmp"

  os="$(uname -s)"
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "unsupported OS: $os" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac

  tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    https://github.com/EMOBase/emobase-cli/releases/latest)"
  ver="${tag##*/v}"
  base="https://github.com/EMOBase/emobase-cli/releases/download/v${ver}"
  file="emobase_${ver}_${os}_${arch}.tar.gz"

  curl -fsSLO "$base/$file"
  curl -fsSLO "$base/checksums.txt"
  grep " $file\$" checksums.txt > checksum.txt
  if [ "$os" = linux ]; then
    sha256sum -c checksum.txt
  else
    shasum -a 256 -c checksum.txt
  fi

  tar -xzf "$file" emobase
  sudo install -m 0755 emobase /usr/local/bin/emobase
) && emobase --version
```

### Per-user install (no sudo)

Use the same block, but replace the final `sudo install ...` line with one of
these:

```sh
# Linux
install -D -m 0755 emobase "$HOME/.local/bin/emobase"

# macOS
mkdir -p "$HOME/.local/bin"
install -m 0755 emobase "$HOME/.local/bin/emobase"
```

Then make sure `~/.local/bin` is on `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Add that line to `~/.zshrc` or `~/.bashrc` to keep it.

### Pin a specific version

To install a version other than the current stable release, replace the two
tag-resolution lines in the script:

```sh
tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
  https://github.com/EMOBase/emobase-cli/releases/latest)"
ver="${tag##*/v}"
```

with the version from the releases page, without the leading `v`:

```sh
ver=0.2.0
```

The rest of the script is unchanged.

## Rolling `main` build (pre-release)

Every push to `main` publishes a `latest` release with stable asset names such
as `emobase_linux_amd64.tar.gz`. It follows the newest commit and is not
reproducible, so use a tagged release for production.

`emobase --version` reports `main-<commit>` for these builds.

```sh
(
  set -e
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  cd "$tmp"

  os="$(uname -s)"
  case "$os" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) echo "unsupported OS: $os" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
    x86_64|amd64) arch=amd64 ;;
    arm64|aarch64) arch=arm64 ;;
    *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
  esac

  base="https://github.com/EMOBase/emobase-cli/releases/download/latest"
  file="emobase_${os}_${arch}.tar.gz"

  curl -fsSLO "$base/$file"
  curl -fsSLO "$base/checksums.txt"
  grep " $file\$" checksums.txt > checksum.txt
  if [ "$os" = linux ]; then
    sha256sum -c checksum.txt
  else
    shasum -a 256 -c checksum.txt
  fi

  tar -xzf "$file" emobase
  sudo install -m 0755 emobase /usr/local/bin/emobase
) && emobase --version
```

For a per-user install, use the replacement commands in
[Per-user install](#per-user-install-no-sudo) above.

## Build from source

Requirements: Go 1.25 or newer, `git`, and `make`.

```sh
git clone https://github.com/EMOBase/emobase-cli.git
cd emobase-cli
make build            # writes ./dist/emobase
./dist/emobase --version
```

Install the binary from the source checkout:

```sh
make install-user                        # ~/.local/bin/emobase, no sudo
make install                             # /usr/local/bin/emobase, may need sudo
PREFIX=/opt/homebrew make install        # Homebrew prefix on Apple Silicon
```

If you use `make install-user`, add `~/.local/bin` to `PATH` as described
above.

`make build` injects `git describe` as the version. A checkout exactly on a
release tag reports that tag; a checkout ahead of it includes the commit
distance, for example `v0.2.0-5-gabc1234`.

## Build and copy to a server (offline or unreleased)

Cross-compile on your machine:

```sh
make build-linux      # dist/emobase-linux-amd64 and dist/emobase-linux-arm64
```

Choose the architecture matching the server:

- `uname -m` reports `x86_64` → use `emobase-linux-amd64`
- `uname -m` reports `aarch64` or `arm64` → use `emobase-linux-arm64`

Copy and install system-wide:

```sh
scp dist/emobase-linux-amd64 you@server:/tmp/emobase
ssh you@server \
  'sudo install -m 0755 /tmp/emobase /usr/local/bin/emobase && emobase --version'
```

Or install for one user without sudo:

```sh
scp dist/emobase-linux-amd64 you@server:/tmp/emobase
ssh you@server 'install -D -m 0755 /tmp/emobase ~/.local/bin/emobase'
```

## Manual download and verification

If you prefer not to pipe a script, download the archive and checksum file
directly. This example uses the stable `v0.2.0` Linux `amd64` build:

```sh
ver=0.2.0
os=linux
arch=amd64
base="https://github.com/EMOBase/emobase-cli/releases/download/v${ver}"
file="emobase_${ver}_${os}_${arch}.tar.gz"

curl -fsSLO "$base/$file"
curl -fsSLO "$base/checksums.txt"
grep " $file\$" checksums.txt > checksum.txt
sha256sum -c checksum.txt       # macOS: shasum -a 256 -c checksum.txt

tar -xzf "$file" emobase
sudo install -m 0755 emobase /usr/local/bin/emobase
emobase --version
```

## Verify the installation

```sh
emobase --version
emobase --help
```

Expected version formats:

| Install source | Example |
| --- | --- |
| Tagged release | `emobase version 0.2.0` |
| Rolling `main` build | `emobase version main-8baee04a814b` |
| Source build | `emobase version v0.2.0` or `v0.2.0-5-gabc1234` |

## Upgrade

Rerun the stable install script to replace the binary with the current stable
release. For a pinned version, change `ver=` and rerun the same block.

`emobase update` updates the deployment checkouts and containers; it does not
replace the CLI binary itself.

## Uninstall

```sh
# System-wide
sudo rm -f /usr/local/bin/emobase

# Per-user
rm -f "$HOME/.local/bin/emobase"
```

A workspace's `.emobase/state.json` and the deployment checkouts are separate
from the binary and are not removed by uninstalling `emobase`.

## Troubleshooting

- **Unsupported OS or architecture** — prebuilt binaries cover Linux and macOS
  on `amd64` and `arm64`. Use [Build from source](#build-from-source) or
  cross-compile for another target.
- **`emobase: command not found`** — `/usr/local/bin` or `~/.local/bin` is not
  on `PATH`. Add the directory and reopen the shell.
- **Checksum mismatch** — make sure the archive and `checksums.txt` come from
  the same release and re-download both. Do not install an archive that fails
  verification.
- **Docker errors** — installing the CLI does not install Docker. Deployment
  commands require Docker with Compose, `git`, and `python3` on the host.
