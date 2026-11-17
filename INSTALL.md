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

The install script detects Linux/macOS and `amd64`/`arm64`, resolves the newest
stable `v*` release, verifies its checksum, and installs `emobase` in
`/usr/local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh | sh
```

It uses `sudo` only when the target directory is not writable. The script is
short and self-contained; you can review it at
[install.sh](install.sh) before running it.

### Installer options

| Option | Effect |
| --- | --- |
| `--version VERSION` | Install a specific release, with or without a leading `v` |
| `--channel latest` | Install the rolling `main` build instead of a stable tag |
| `--user` | Install into `~/.local/bin` instead of `/usr/local/bin` |
| `--prefix DIR` | Install into `DIR/bin` |
| `-h`, `--help` | Show the options |

When piping the script to `sh`, pass options after `sh -s --`:

```sh
# Pin a version
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh \
  | sh -s -- --version 0.2.0

# Per-user install, no sudo
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh \
  | sh -s -- --user

# Custom prefix
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh \
  | sh -s -- --prefix /opt/emobase
```

If you use `--user`, add `~/.local/bin` to `PATH`:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Add that line to `~/.zshrc` or `~/.bashrc` to keep it.

## Rolling `main` build (pre-release)

Every push to `main` publishes a `latest` release with stable asset names such
as `emobase_linux_amd64.tar.gz`. It follows the newest commit and is not
reproducible, so use a tagged release for production.

```sh
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh \
  | sh -s -- --channel latest
```

`emobase --version` reports `main-<commit>` for these builds.

## Manual download and verification

If you prefer not to run the installer, download the archive and checksum file
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

Rerun the stable install one-liner to replace the binary with the current
stable release:

```sh
curl -fsSL https://raw.githubusercontent.com/EMOBase/emobase-cli/main/install.sh | sh
```

For a pinned version, change `--version`; for a per-user install, add `--user`.

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
- **Permission denied** — rerun with `--user`, or make sure `sudo` is available
  for a system-wide install.
- **Docker errors** — installing the CLI does not install Docker. Deployment
  commands require Docker with Compose, `git`, and `python3` on the host.
