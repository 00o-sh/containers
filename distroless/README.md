# Distroless images

Wolfi-based, apko-built, shell-free images published as `ghcr.io/00o-sh/<app>-distroless`. The fleet-wide migration plan (which apps, in what order, and why) lives in [`docs/distroless-plan.md`](../docs/distroless-plan.md).

## Adding an image

Create `distroless/<app>/` with the files below. **No workflow edits needed** — `distroless-build.yaml` and `vulnerability-scan.yaml` discover directories containing an `apko.yaml`, and the boot smoke picks up `container_test.go` automatically (and fails the build if it's missing — tests are mandatory).

| File | Required | Purpose |
|---|---|---|
| `apko.yaml` | yes | Image definition (packages, entrypoint, user) |
| `container_test.go` | yes | Boot smoke, runs against the freshly-built image on both arches |
| `melange.yaml` | only if Wolfi doesn't ship the app | Builds the apk locally; **never** duplicate a package Wolfi already provides |

## Conventions (enforced by CI where possible)

- **Feature parity is mandatory.** The distroless variant must not drop functionality the `apps/` flavor had — no "CLI-only" builds, no "degraded mode" shortcuts (e.g. kopia keeps its embedded web UI and defaults to server mode, exactly like `apps/kopia`). If an `entrypoint.sh` provided an env-switched default, reproduce the *default* branch as the image's `cmd:` — overriding args gets the other behavior (README §Passing Arguments). If parity is genuinely impossible without a shell, that app is an exemption, not a reduced image.
- **Non-root**: uid/gid `65532:65532` (`nonroot`), asserted at build time.
- **No shell**: `sh|bash|ash|busybox|dash` in the image fails the build. Entrypoint logic that used to live in an `entrypoint.sh` moves to apko `entrypoint`/`cmd`/`environment` — the orchestrator owns the command line.
- **Entrypoint must be set**, asserted at build time.
- **`/config`** is the data volume (repo-wide convention). Create it via apko `paths:` with `65532` ownership if the app writes state.
- **One process, log to stdout** — same mission rules as `apps/`.
- **CVE gate is strict**: Grype + Trivy, fail on HIGH+ fixable, both arches. This is the fork's security floor — don't soften it per-image.

## Version pinning & Renovate

Every image carries exactly one Renovate-managed version anchor:

- **Wolfi ships the app** → floor pin in `apko.yaml`:

  ```yaml
  - cloudflared>=2026.5.2 # renovate: datasource=github-releases depName=cloudflare/cloudflared
  ```

  The apk resolver floats upward within the floor, so the daily 02:00 rebuild keeps delivering Wolfi `-rN` CVE patches without a PR. Renovate bumps the floor on upstream releases; if Wolfi hasn't packaged that version yet, the PR fails apk resolution and sits red until it has — lag is visible, never silent. (Exception: per-minor-versioned packages like `kubectl-1.36` are bumped manually, see the comment in that recipe.)

- **We build the app** → annotation on `package.version` in `melange.yaml`:

  ```yaml
  version: "0.23.0" # renovate: datasource=github-releases depName=kopia/kopia extractVersion=^v(?<version>.*)$
  ```

  with `tag: v${{package.version}}` in the `git-checkout` step so the checkout follows the version automatically.

Renovate PRs for `distroless/**` auto-merge when the full gate is green (`.renovaterc.json5` — `ignoreTests: false` means red gate blocks the merge). **After adding an image, verify the dependency shows up on Renovate's Dependency Dashboard** — a mis-matched annotation fails silently, and "pinned but unwatched" defeats the whole design.

### Why no `expected-commit` / `expected-sha256`?

Deliberate v1 posture: a commit/checksum pin would go stale on every Renovate version bump, and Renovate can't re-resolve them (no postUpgradeTasks on Mend cloud; a fixup-bot workflow can't work either — `GITHUB_TOKEN` pushes don't re-trigger CI, and foreign commits stop Renovate from managing its branch). Checkout-by-tag over HTTPS matches the existing `apps/` posture (Dockerfiles curl upstream tarballs by version, unchecksummed). Revisit if a bot-app token ever lands on this fork.

Corollary: melange `fetch` pipelines (which *require* `expected-sha256`) don't fit the automation — **prefer building from source with `git-checkout`**. If an app only ships usable prebuilt artifacts (see the wave-3 .NET apps in the plan), solve the checksum-refresh problem first.

## melange recipe gotchas

Each of these burned a CI cycle to discover (PR #21). Wolfi's own CI provides them implicitly for in-tree recipes; ours must spell them out:

1. **Build env needs repos/keyring** — `environment.contents:` must list `repositories:` + `keyring:`, or: `failed to initialize apk repositories: must provide at least one repository`.
2. **Test env needs the same** — `test.environment.contents:` is a separate sandbox with its own `repositories:` + `keyring:`.
3. **Test env needs `packages:` too** — at minimum `busybox` (provides `/bin/sh` for `runs:` scripts; the *test* sandbox may have it — the final apko image must not) and `ca-certificates-bundle`. Otherwise: `bwrap: execvp /bin/sh: No such file or directory`.
4. **The workflow passes `--repository-append` to `melange test`** so the sandbox can resolve the just-built apk. Already wired in `distroless-build.yaml`; listed here so nobody "simplifies" it away.

## Boot smoke templates

CLI apps — prove the binary runs:

```go
package main

import (
	"testing"

	"github.com/home-operations/containers/testhelpers"
)

func Test(t *testing.T) {
	image := testhelpers.GetTestImage("ghcr.io/00o-sh/<app>-distroless:rolling")
	testhelpers.TestCommandSucceeds(t, image, nil, "/usr/bin/<app>", "--version")
}
```

Services — prove it actually serves (catches config/migration breaks that `--version` can't):

```go
testhelpers.TestHTTPEndpoint(t, image, testhelpers.HTTPTestConfig{Port: "<port>"}, nil)
```

`TEST_IMAGE` overrides the default ref — CI sets it to the just-built `:verify-<arch>` tag; set it locally to test your own build.

## Exemptions

These `apps/` images will **not** get distroless equivalents (rationale in `docs/distroless-plan.md` §3): `busybox` (is a shell), `actions-runner` (jobs need a shell), `esphome` (compiles firmware at runtime), `plex`, `plex-next`, `emby` (proprietary blobs). Revisit yearly.
