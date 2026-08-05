# Releasing telegram-cli

The **git tag is the version**. There is no separate version bump in the
source tree: `telegram-cli version` is stamped at build time from the tag via
`-ldflags "-X telegram-cli/internal/cli.version=<tag>"`, and the npm package
version is derived from the same tag in the publish workflow.

Pushing a `v*` tag triggers two permanent GitHub Actions pipelines that
chain deterministically:

1. **`.github/workflows/release.yml`** (on `push: tags v*`)
   Builds the binary matrix via `scripts/dist.sh`, attaches the assets to a
   GitHub Release named after the tag (auto-generated notes), then
   explicitly dispatches the npm pipeline as its final step — no reliance
   on the flaky `release: published` event for bot-created releases.
2. **`.github/workflows/npm-publish.yml`** (workflow_dispatch only)
   Publishes **@qmahyar/telegram-cli** at the same version, so
   `npm i -g @qmahyar/telegram-cli` resolves the binary for the user's
   platform from the matching GitHub Release. Without the `NPM_TOKEN`
   secret it reports a skip with the manual fallback instead of failing.

Asset naming is the contract between the two pipelines and the npm
postinstall (`scripts/install.js`): `telegram-cli-<os>-<arch>[.exe]` for
`linux|darwin|windows` × `amd64|arm64`, plus `SHA256SUMS`. The CI workflow's
`dist-check` job asserts this matrix on every push, so a drift fails before
any release.

## One-command release (maintainer path)

```bash
# 0. Prereqs (once):
#    - GitHub: gh auth login (done; token needs contents:write on this repo)
#    - npm:    add the NPM_TOKEN repo secret. The token MUST be a classic
#              Automation token: npm > Access Tokens > Generate New Token
#              > type **Automation** (NOT granular — granular tokens with
#              read/write scope still demand a 2FA one-time password on
#              publish and fail in CI with EOTP). Until the secret exists,
#              npm-publish.yml reports a skip and the manual fallback
#              below applies.

# 1. Make sure main is green (CI runs build/vet/test/lint on every push).
git checkout main && git pull --ff-only

# 2. Tag and push — that's the whole release.
git tag v0.1.0
git push origin v0.1.0
```

Watch it: `gh run watch` (first run) or `gh run list --workflow=release.yml`.

## Verification (always do after a release)

```bash
# GitHub release exists with 6 binaries + SHA256SUMS
gh release view v0.1.0

# npm package exists and installs cleanly on this platform
npm view @qmahyar/telegram-cli version
cd "$(mktemp -d)" && npm i -g @qmahyar/telegram-cli && telegram-cli version

# the baked version matches the tag
curl -sL https://github.com/QMahyar/Telegram-Cli/releases/download/v0.1.0/telegram-cli-linux-amd64 -o /tmp/tc && chmod +x /tmp/tc && /tmp/tc version
```

## Manual fallbacks

- **npm publish by hand** (when NPM_TOKEN is absent or npm-publish.yml failed):

  ```bash
  git checkout v0.1.0
  npm version 0.1.0 --no-git-tag-version --allow-same-version
  npm pack                 # inspect the tarball contents first
  npm publish --access public
  ```

- **Re-publish after adding NPM_TOKEN**:

  ```bash
  gh workflow run npm-publish.yml -f tag=v0.1.0
  ```

- **Local pre-flight** (no network side effects): `make dist` builds the
  matrix; `npm pack --dry-run` lists what npm would ship. To test the npm
  download end-to-end before a release, point the installer at any HTTP
  server that serves the asset layout:

  ```bash
  TELEGRAM_CLI_DIST_BASE_URL=https://github.com/QMahyar/Telegram-Cli/releases/download \
    npm rebuild @qmahyar/telegram-cli
  ```

## Rollback

- **npm**: `npm unpublish @qmahyar/telegram-cli@<version>` (allowed within 72h
  of publish; beyond that, publish a fixed version — never unpublish a
  long-live release others depend on).
- **GitHub**: `gh release delete v0.1.0 --yes` and `git push origin :v0.1.0`.
  The release workflow re-runs cleanly if the same tag is pushed again.

## Versioning policy

- Semver on tags: `v1.2.3`, prereleases `v1.2.3-rc.1`.
- Anything tagged with `-` (prerelease) is marked as a prerelease on GitHub
  and published to npm with `--tag next` (adjust npm-publish.yml if you want
  that wired; today it publishes the tag verbatim to `latest`).

## Agents: remember this pipeline

`recall "how to release telegram-cli"` returns this document and the teach
rows that point at the workflow files. The invariant to re-derive if you
forget everything else:

> **Tag = version. `git push origin vX.Y.Z` is the release. Two permanent
> workflows do the rest; verify with `gh release view` + `npm view`.**
