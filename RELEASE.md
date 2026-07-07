# Release Process

Only maintainers with release rights may create `vMAJOR.MINOR.PATCH` tags.
Release tags are protected by a repository ruleset for `refs/tags/v*`; until a
dedicated release team exists, only organization admins may create, update, or
delete those tags.

All changes to `main` must go through a pull request. The `main` branch ruleset
blocks deletions and non-fast-forward updates, requires pull requests, and
requires the `Test`, `Govulncheck`, and `Snapshot Release` status checks to
pass before merge.

Tags must point to a commit that is reachable from `main`, and `ci.yml` must
already have completed successfully for that exact commit. The release workflow
also verifies both conditions before building artifacts.

Do not upload release artifacts from a workstation. The release workflow builds
all customer-facing artifacts in GitHub Actions, creates a draft release,
generates provenance, verifies the draft assets, and only then publishes the
release.

## Expected Artifacts

Each release must include:

- 6 OS/architecture archives: macOS, Linux, and Windows for `amd64` and `arm64`
- 6 archive SBOMs in SPDX JSON format
- 1 SHA-256 checksum file
- 1 Sigstore bundle for the checksum file
- 1 SLSA provenance file covering every archive and SBOM
- GitHub artifact attestations for every artifact listed in the checksum file

The release workflow verifies the checksum signature, artifact checksums,
GitHub artifact attestations, SLSA provenance, and a Linux archive smoke test
before publishing.

## Failed Verification

If verification fails, leave the release unpublished. Do not manually publish
or replace assets on the failed draft release.

Fix the issue in a new commit on `main`, wait for `ci.yml` to pass, then create
a new version tag. If the failed tag or draft release is visible and mutable,
delete it so the release list reflects only valid attempts.
