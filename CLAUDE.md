# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

> **Read `.claude/STATE.md` first.** That's the live working log — open threads, current branch context, recently closed work. This file is the static architecture map; STATE.md is the volatile session pointer. If STATE.md is stale (check its `Last updated`), update it as you go per the protocol at the bottom of that file.

## Repo at a glance

Fork of `home-operations/containers`, published to `ghcr.io/00o-sh/*`. Two parallel image families coexist:

- **`apps/<app>/`** — Alpine/Ubuntu-based, `Dockerfile` + `docker-bake.hcl`. Published as `<app>-<flavor>` (e.g. `home-assistant-alpine`, `actions-runner-ubuntu`). The historical home-operations style.
- **`distroless/<image>/`** — Wolfi-based via apko, optionally with a `melange.yaml`. Published as `<image>-distroless` (e.g. `cloudflared-distroless`). This is the in-progress distroless track.

Of the published images, only `cloudflared-distroless` is currently consumed downstream by the operator's cluster — all others are dormant. Factor that into PR-risk framing.

## Common commands

Tooling is pinned in `.mise.toml` (apko, melange, grype, cosign, just, yq, gh, go). `mise install` to provision; `lefthook install` to wire pre-commit hooks (gofmt, just --fmt, yamlfmt).

```sh
# Build + test a single app locally (uses docker buildx bake → loads image → runs Go test)
just local-build <app>

# Trigger the remote release workflow for an app (gh CLI)
just remote-build <app> [release=true|false]

# Run a single app's container tests against a pre-built image
TEST_IMAGE=<image-ref> go test -v ./apps/<app>/...

# Build a distroless image locally (Wolfi sandbox required)
melange build distroless/<image>/melange.yaml --arch x86_64   # only if melange.yaml exists
apko build distroless/<image>/apko.yaml <image>-distroless:local output.tar
```

## Build pipelines

### `apps/` — multi-arch Docker bake

`.github/workflows/release.yaml` (push to `main` under `apps/**`) and `pull-request.yaml` both fan out into `app-builder.yaml`, which:

1. Reads `docker-bake.hcl` via `.github/actions/app-options` to extract `VERSION`, `SOURCE`, `FLAVOR`, and the derived `image_name`.
2. Builds per-arch with `docker/bake-action`, pushes by digest to GHCR, then merges into a multi-arch manifest.
3. Attests provenance via `actions/attest` (verifiable with `gh attestation verify`).
4. Runs CVE gates (Grype + Trivy, both `severity-cutoff: high`, `--only-fixed`). Build fails if either reports HIGH+ fixable; SARIF posted to the Security tab and summarised as a sticky PR comment.
5. On PRs only: runs the Go testcontainers suite against the freshly-built `:sandbox` image and posts a size diff vs `:rolling`.
6. On release: tags `:<semver>`, `:<major>.<minor>`, `:<major>`, `:<raw>`, `:rolling`.

### `distroless/` — apko (+ optional melange)

`.github/workflows/distroless-build.yaml` discovers any directory containing `apko.yaml`. For each image × `{x86_64, aarch64}`:

1. **Only build a melange package when Wolfi doesn't already ship the app.** If `melange.yaml` is absent (the common case), apko consumes the Wolfi-provided apk directly. This is enforced policy — write a melange recipe only to fill a Wolfi gap, not to duplicate work.
2. Resolves the published version by reading the package's `versionInfo` out of the apko-emitted SBOM (single source of truth for "what's actually in the image").
3. Asserts image structure: non-root user, no shell binary (`sh|bash|ash|busybox|dash`), entrypoint set.
4. Per-image boot smoke (currently only `cloudflared --version`; add a `case` arm when introducing a new image).
5. Grype + Trivy gates as above, uploaded with distinct SARIF categories.
6. Manifest fan-in, provenance attestation, sticky PR comment with sandbox digest + size diff vs `:rolling`.

Daily `cron: "0 2 * * *"` rebuild pulls fresh Wolfi CVE patches even without code changes.

## Image naming (`FLAVOR`)

`.github/actions/app-options` derives the published image name from a `FLAVOR` variable in `docker-bake.hcl`:

- Unset → defaults to `"alpine"` → published as `<app>-alpine`. Most apps rely on this default.
- `"ubuntu"`, `"debian"`, etc. → `<app>-<flavor>`.
- Sentinel `"none"` → bare `<app>` (used for single-flavor apps like `busybox`).

The legacy bare-name images under `apps/` are queued for rename to `-alpine`/`-ubuntu`; the `-distroless` suffix landed first.

## Tag scheme & immutability

Mission stance (from README): tags are **not** immutable — consumers pin by `sha256` digest. `docker/metadata-action` emits, per release:

```
:<version>           # e.g. 2026.5.2
:<major>.<minor>     # 2026.5
:<major>             # 2026
:<raw>               # upstream tag verbatim, only if semver-valid
:rolling             # always
:sandbox             # PR builds only — never released
```

Distroless tag selection additionally requires the upstream version to match `^[0-9]+\.[0-9]+\.[0-9]+([.-].*)?$`; otherwise falls back to `type=raw`. This handles upstreams like `busybox 1.37.0` and `jackett 0.24.1870` whose versions don't parse strictly.

## Tests

Go module `github.com/home-operations/containers` (Go 1.26). The `testhelpers` package wraps `testcontainers-go` with three primitives:

- `TestHTTPEndpoint(t, ctx, image, HTTPTestConfig{Port, Path, StatusCode}, *ContainerConfig)` — start container, wait for listening port + 200.
- `TestFileExists(t, ctx, image, path, *ContainerConfig)` — exec `test -f`.
- `TestCommandSucceeds(t, ctx, image, *ContainerConfig, entrypoint, args...)` — exec, assert exit 0.

Each app's `container_test.go` is a thin caller. The `TEST_IMAGE` env var overrides the hard-coded default — set it when running locally so you're testing your local build, not `:rolling`.

## CI safety details worth knowing

- **Bot-app fallback**: `app-builder.yaml`'s test/announce paths check for `BOT_APP_ID`; forks (this one included) lack it and fall back to `github.token`. Don't add a hard dependency on `BOT_APP_*` secrets.
- **Vulnerability scan tolerates missing images**: `vulnerability-scan.yaml` uses `continue-on-error` + `steps.scan.outcome == 'success'` to guard the SARIF upload, so newly renamed or never-published images don't break the daily run.
- **Renovate-only versioning**: never edit `VERSION` defaults in `docker-bake.hcl` or pinned action SHAs by hand without a Renovate-shaped comment (`// renovate: datasource=...`). The `// renovate: datasource=docker depName=ghcr.io/wolfi-dev/sdk` style is what keeps the pipeline current.
- **Image structure assertions are load-bearing**: the distroless workflow re-tags loaded images from `<img>-amd64`/`<img>-arm64` back to the clean name; don't simplify that step.

## Conventions

- Conventional Commit titles required on PRs (`feat:`, `fix:`, `ci:`, `chore:`, `!` for breaking). Individual commit messages aren't enforced.
- One process per container, log to stdout, no s6/gosu/catatonit alternatives (catatonit specifically is OK — it's used as init in some images, see `home-assistant`).
- Default container UID/GID is `65534:65534` (apps) or `65532:65532` (distroless, the Wolfi `nonroot` user).
- `/config` is the hard-coded data volume path; don't parameterise it.
