# StackRadar CLI

[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/stackradar/stackradar-cli/badge)](https://scorecard.dev/viewer/?uri=github.com/stackradar/stackradar-cli)

`stackradar` collects dependency evidence from CI workspaces and uploads it to
[StackRadar](https://stackradar.com/).

The CLI is intentionally provider-agnostic. It discovers dependency files,
creates a deterministic upload bundle, and uploads an existing bundle when given
an authentication token. CI-specific wrappers are responsible for provider
details such as requesting a GitHub Actions OIDC token.

For GitHub Actions, use [`stackradar/stackradar-action`](https://github.com/stackradar/stackradar-action).
The action downloads and verifies a released CLI binary, requests GitHub Actions
OIDC when uploading, and then delegates to this CLI.

## Install

Download the latest archive from
[GitHub Releases](https://github.com/stackradar/stackradar-cli/releases).

On macOS and Linux:

```sh
REPO=stackradar/stackradar-cli
TAG=v0.1.0
VERSION="${TAG#v}"
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  arm64|aarch64) ARCH=arm64 ;;
esac

gh release download "$TAG" \
  --repo "$REPO" \
  --pattern "stackradar_${VERSION}_${OS}_${ARCH}.tar.gz"

tar -xzf "stackradar_${VERSION}_${OS}_${ARCH}.tar.gz"
./stackradar version
```

Replace `TAG` with the release you want to install.

## Commands

```sh
stackradar --help
stackradar version
stackradar bundle --help
stackradar upload --help
```

Create an upload bundle:

```sh
stackradar bundle --path .
```

By default, `bundle` writes `stackradar.zip` in the current working directory.
Use `--output <file>` to choose another path:

```sh
stackradar bundle --path . --output /tmp/stackradar.zip
```

Upload an existing bundle:

```sh
STACKRADAR_TOKEN=<token> stackradar upload stackradar.zip
```

The token can also be passed with `--token`. Use `--dry-run` to validate the
bundle and print upload metadata without sending it:

```sh
stackradar upload stackradar.zip --dry-run
```

## Bundle Contents

Bundles contain:

- discovered dependency manifests and lockfiles
- `stackradar-manifest.json`, including CLI metadata, git commit/ref/branch
  context when available, clean/dirty git state when available, and per-file
  SHA-256 digests

Discovery honors repository ignore rules and skips dependency/vendor/build
directories that should not be uploaded as dependency evidence.

## Release Integrity

Every tagged release is built in GitHub Actions from a tag that points to a
commit on `main` with a successful `ci.yml` run for that exact commit.

The release workflow publishes:

- OS and architecture-specific archives
- a SHA-256 checksum file
- a keyless Sigstore signature bundle for the checksum file
- archive SBOMs in SPDX JSON format
- GitHub artifact attestations for every artifact listed in the checksum file
- SLSA provenance for every archive and SBOM listed in the checksum file

Releases are created as drafts while GoReleaser uploads archives, checksums,
signatures, and SBOMs. SLSA provenance is attached to the same draft release.
The draft release is verified before publication: checksum signature, archive
checksums, artifact attestations, SLSA provenance, and a Linux binary smoke
test must all pass.

SBOM files are not signed individually. They are covered by the signed checksum
manifest, GitHub artifact attestations, and SLSA provenance. This avoids
redundant per-file signatures while still proving the released SBOM bytes.

The workflows pin third-party Actions to commit SHAs with version comments for
Renovate, pin the installed cosign and Syft versions, and intentionally keep
the SLSA generator reusable workflow tag-pinned because the SLSA generator and
verifier require a trusted reusable workflow ref.

## Verify a Release

Install the GitHub CLI, `cosign`, and `slsa-verifier`, then run:

```sh
REPO=stackradar/stackradar-cli
TAG=v0.1.0
VERSION="${TAG#v}"
WORKDIR="stackradar-${VERSION}-verify"

mkdir -p "$WORKDIR"
gh release download "$TAG" --repo "$REPO" --dir "$WORKDIR"
cd "$WORKDIR"

cosign verify-blob "stackradar_${VERSION}_checksums.txt" \
  --bundle "stackradar_${VERSION}_checksums.txt.sigstore.json" \
  --certificate-identity "https://github.com/${REPO}/.github/workflows/release.yml@refs/tags/${TAG}" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"

shasum -a 256 -c "stackradar_${VERSION}_checksums.txt"

artifacts=()
while read -r _ artifact; do
  case "$artifact" in
    *.tar.gz|*.zip|*.sbom.spdx.json)
      gh attestation verify "$artifact" \
        --repo "$REPO" \
        --signer-workflow "${REPO}/.github/workflows/release.yml" \
        --source-ref "refs/tags/${TAG}"
      artifacts+=("$artifact")
      ;;
  esac
done < "stackradar_${VERSION}_checksums.txt"

slsa-verifier verify-artifact "${artifacts[@]}" \
  --provenance-path "stackradar_${VERSION}_multiple.intoto.jsonl" \
  --source-uri "github.com/${REPO}" \
  --source-tag "$TAG"
```

## Development

```sh
make fmt-check
make vet
make test
make lint
make release-check
```

Build a local binary:

```sh
make build
```

Create a local GoReleaser snapshot:

```sh
make snapshot
```

Release process details are in [RELEASE.md](./RELEASE.md). Security reporting
instructions are in [SECURITY.md](./SECURITY.md).
