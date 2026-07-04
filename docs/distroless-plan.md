# Fleet-wide distroless migration & CI validation plan

**Goal:** every image published from this repo gets a `distroless/<app>/` (Wolfi + apko) equivalent — or a written exemption — and **every version bump flows through CI that builds, boots, and scans the image before anything is published**, so a Renovate update that breaks an app is caught as a red PR, not a broken `:rolling` tag.

Status: plan. Authored 2026-07-04. Wolfi package availability below was audited against the live `x86_64` APKINDEX on that date — re-verify before starting each wave (`curl -sL https://packages.wolfi.dev/os/x86_64/APKINDEX.tar.gz | tar xzO APKINDEX | grep '^P:<name>'`).

---

## 1. What already exists (don't rebuild it)

The distroless pipeline (`.github/workflows/distroless-build.yaml`) is already generic and convention-driven. Adding an image = adding a directory; **no workflow edits needed**:

| Convention | Effect |
|---|---|
| `distroless/<app>/apko.yaml` | Image discovered by build matrix, daily 02:00 CVE-rebuild cron, and `vulnerability-scan.yaml` |
| `distroless/<app>/melange.yaml` (optional) | melange build + `melange test` run before apko; local apk repo appended |
| `distroless/<app>/container_test.go` | Boot smoke via `testhelpers` runs against the freshly built per-arch image (`TEST_IMAGE` override) |

Per-PR gate already in place, per image × {x86_64, aarch64}: apko/melange build → structure assertions (non-root, **no shell binary**, entrypoint set) → boot smoke → Grype + Trivy fail-on-HIGH+-fixable → `:sandbox` multi-arch manifest + provenance attestation + size-diff sticky comment. On merge/cron: release tag set derived from the SBOM's `versionInfo`.

The daily cron is itself test-gated: the boot smoke and scanners run before the per-arch push, and the manifest job only publishes if all build jobs pass. A Wolfi package bump that breaks an app fails the scheduled run instead of shipping.

Renovate already has a custom regex manager for trailing `# renovate: datasource=... depName=...` annotations in `melange.yaml` / `apko.yaml` (`.renovaterc.json5`). That's the hook this plan builds on.

---

## 2. The update-validation model (the core of this plan)

Three source types, three pinning mechanisms. All three end at the same place: **a PR whose CI runs the full gate, with automerge-on-green**.

### 2a. Wolfi ships the app → floor pin in `apko.yaml`

```yaml
packages:
  - wolfi-baselayout
  - ca-certificates-bundle
  - tzdata
  - cloudflared>=2026.6.2 # renovate: datasource=github-releases depName=cloudflare/cloudflared
```

- Renovate bumps the floor when upstream releases → PR → full gate → automerge on green.
- The apk resolver still floats *upward* within the floor, so the daily cron keeps delivering Wolfi's `-rN` CVE rebuilds without a PR (test-gated, as today).
- If Wolfi hasn't packaged the new upstream version yet, apk resolution fails → the Renovate PR sits red until Wolfi catches up, then goes green on the next CI run. Lag is *visible* instead of silent.
- The exact shipped version continues to come from the SBOM at tag time — floor pins don't affect the tag scheme.

Requires one new `matchStrings` entry (the existing one only matches `key: value` lines, not list items) — see §5, Wave 0.

### 2b. Melange builds from source (Go/C/C++ apps) → `git-checkout` + `expected-commit`

```yaml
package:
  name: kopia
  version: "0.23.0" # renovate: datasource=github-releases depName=kopia/kopia extractVersion=^v(?<version>.*)$
pipeline:
  - uses: git-checkout
    with:
      repository: https://github.com/kopia/kopia
      tag: v${{package.version}}
      expected-commit: 0123abc... # anchor for the digest-capture manager
```

- The `version:` line already matches the existing regex manager.
- `expected-commit` is updated by Renovate too: add a matchString capturing `currentValue` (tag) + `currentDigest` (commit) with `datasource=github-tags`, which resolves tag→commit digests natively. Version and commit bump atomically in one PR; no hash-fixing bot needed for source builds.
- Every `melange.yaml` must carry a `test:` block (runs in the melange sandbox where busybox is allowed) — cheap functional check before the image is even assembled.

### 2c. Melange repackages prebuilt artifacts (.NET servarr tarballs, qbittorrent-nox, JARs) → `fetch` + sha256 fixup workflow

`fetch` requires `expected-sha256`, which Renovate cannot compute. Add a small workflow (`distroless-digest-fixup.yaml`):

- Trigger: `pull_request` touching `distroless/**/melange.yaml`, filtered to branches matching Renovate's prefix.
- Step: substitute `${{package.version}}` into the fetch URL, download, compare sha256; on mismatch, rewrite `expected-sha256` and push a fixup commit to the PR branch (same-repo branch, `contents: write`, `github.token` — no bot secrets, consistent with the fork's no-`BOT_*` policy).
- The build workflow re-runs on the fixup push and validates for real.

This is the only new moving part; everything else reuses existing machinery.

### 2d. Renovate packageRules for the distroless track

Mirror the apps/ rules in `.renovaterc.json5`:

```json5
{
  description: ["Release rules for distroless updates"],
  matchFileNames: ["distroless/**/apko.yaml", "distroless/**/melange.yaml"],
  addLabels: ["distroless/{{parentDir}}"],
  additionalBranchPrefix: "{{parentDir}}-",
  semanticCommitType: "release",
  semanticCommitScope: "{{parentDir}}-distroless",
},
{
  description: ["Auto-merge distroless updates when CI is green"],
  matchFileNames: ["distroless/**/apko.yaml", "distroless/**/melange.yaml"],
  automerge: true,
  automergeType: "pr",
  ignoreTests: false, // green build+smoke+scan required — this IS the safety gate
},
```

The existing servarr custom datasources (`servarr-develop`, `sonarr-develop`, `qbittorrent`) carry over unchanged for the wave-3/5 melange pins.

### End-to-end flow after this plan lands

```
upstream release
  └─ Renovate PR (bumps apko floor pin / melange version+commit)
       └─ distroless-build.yaml PR gate:
            melange build + melange test (if recipe)
            apko build → structure assertions (non-root, no shell, entrypoint)
            go test (boot/HTTP smoke, both arches)
            Grype + Trivy fail on HIGH+ fixable
            :sandbox manifest + attestation + size diff comment
       └─ green → automerge → main build → semver tags + :rolling
       └─ red   → PR stays open; nothing published; failure is the signal

Wolfi -rN CVE rebuild (daily cron, no PR)
  └─ same build+test+scan gate → publish only if green
  └─ NEW: scheduled-run failure files/updates a pinned issue (see Wave 0)
```

---

## 3. Fleet audit — all 36 `apps/` images

Wolfi availability verified 2026-07-04. Runtimes confirmed present in Wolfi: `python-3.13/3.14`, `nodejs-24/26`, `aspnet-8/9/10-runtime`, `openjdk-17`, `ffmpeg`, `postgresql-17/18-client`, `smartmontools`, `icu`, `sqlite-libs`, `tini`. Confirmed **absent**: `libtorrent-rasterbar`, `mono`, and every media-stack app itself.

### Wave 1 — Wolfi ships the app: apko-only (like cloudflared/kubectl)

| App | Wolfi pkg | Test | Notes |
|---|---|---|---|
| cni-plugins | `cni-plugins` ✓ | `TestFileExists` on plugin binaries | Image's job is file distribution into `/opt/cni/bin`; needs a copy entrypoint. Write a ~30-line static Go `install-cni` shim in the melange recipe *or* set entrypoint to one plugin binary and document the initContainer copy pattern. Decide at implementation. |
| k8s-sidecar | `k8s-sidecar` ✓ | boot smoke (env-configured; needs kube API — command smoke only) | Python app but Wolfi packages it whole. |
| opentofu-runner | `opentofu-1.12` ✓ (per-minor, like kubectl) | `tofu version` command smoke | Current image = flux-iac `tf-runner` base + tofu-as-terraform. Distroless needs the tf-runner binary too → melange `git-checkout` of `flux-iac/tofu-controller`. Straddles wave 1/2; per-minor pin handled like kubectl (manual name bump, documented in comment). |

### Wave 2 — single binary, melange build from source (kopia precedent, PR #21)

| App | Source build | Runtime pkgs | Test |
|---|---|---|---|
| kopia | Go, `kopia/kopia` | ca-certs, tzdata | `kopia --version` — **revive PR #21 as-is, first melange image** |
| tqm | Go, `autobrr/tqm` | ca-certs | `tqm --version` |
| webhook | Go, `adnanh/webhook` | ca-certs | HTTP smoke on `:9000` with a static hooks file baked into the test |
| smartctl-exporter | Go, `prometheus-community/smartctl_exporter` | `smartmontools` | HTTP smoke on `:9633` (metrics endpoint serves without devices) |
| stash | Go, `stashapp/stash` (has JS asset build — needs nodejs in melange env) | `ffmpeg`, optionally `python-3.14` for scrapers | HTTP smoke on `:9999` |
| nzbget | C++, `nzbgetcom/nzbget` (autotools) | libxml2, openssl, `7zip`/`par2` extras — audit which Wolfi ships | HTTP smoke on `:6789` |
| transmission | C, `transmission/transmission` (cmake) | openssl, curl libs | HTTP smoke on `:9091` |
| postgres-init | small **Go rewrite of `entrypoint.sh`** (creates roles/DBs via `database/sql`) — removes both bash and the psql dependency | ca-certs | `TestCommandSucceeds --help`; full test needs a postgres testcontainer — add a helper or keep to flag-parse smoke |
| irqbalance | C, `irqbalance/irqbalance` | glib | ⚠️ must run as root to write `/proc/irq/*` → **conflicts with the non-root structure assertion**. Either add a per-image assertion override (explicit allowlist in the workflow step) or exempt. Decide before building; dormant app, low priority. |

### Wave 3 — .NET / JVM, melange `fetch` of upstream builds (needs §2c fixup workflow)

All servarr apps: runtime pkgs `aspnet-8-runtime` (verify per-app target framework), `icu`, `sqlite-libs`; entrypoint = app binary with `-nobrowser -data=/config`; the bash `entrypoint.sh` logic (env→flags) moves into apko `cmd`/`environment`.

| App | Fetch source | Test (HTTP port) |
|---|---|---|
| sonarr | services.sonarr.tv tarball (existing `sonarr-develop` datasource) | 8989 |
| radarr | servarr develop channel (existing datasource) | 7878 |
| lidarr | servarr develop channel | 8686 |
| prowlarr | servarr develop channel | 9696 |
| whisparr | servarr develop channel | 6969 |
| jackett | `Jackett/Jackett` GitHub release tarball | 9117 |
| nzbhydra2 | `theotherp/nzbhydra2` release zip; runtime `openjdk-17` JRE; entrypoint `java -jar` (its Python wrapper is optional — bypass it) | 5076 |

### Wave 4 — Python / Node / static-site, melange recipes

Pattern for Python apps: melange builds a venv under `/opt/<app>` from the upstream tag (pip install in the melange env), runtime image gets `python-3.14` + the venv; entrypoint `/opt/<app>/bin/python -m <app>`. No shell, no pip at runtime.

| App | Runtime | Test | Notes |
|---|---|---|---|
| tautulli | python-3.14 | HTTP 8181 | straightforward |
| bazarr | python-3.14 + ffmpeg | HTTP 6767 | large dep tree; expect the longest melange build |
| sabnzbd | python-3.14 | HTTP 8080 | `unrar` is non-free and not in Wolfi; ship with `par2` only and document degraded rar support, or build unrar in melange (license review first) |
| pyload-ng | python-3.14 | HTTP 8000 | |
| octoprint | python-3.14 | HTTP 5000 | |
| jbops | python-3.14 (scripts only) | `TestFileExists` on script paths | upstream pins `master` — pin by commit (`git-checkout` + `expected-commit`, digest-only Renovate updates) |
| tududi | nodejs-26; melange runs the npm build | HTTP smoke on app default port (confirm at impl) | |
| theme-park | `nginx` pkg + assets generated at melange time (replicates `themes.py` step) | HTTP 8080 | nginx official entrypoint is a shell script — use nginx binary directly with a baked config, like Chainguard's nginx image |

### Wave 5 — hard cases (each needs a spike before committing)

| App | Blocker | Approach |
|---|---|---|
| qbittorrent | `libtorrent-rasterbar` not in Wolfi | Repackage the static `userdocs/qbittorrent-nox-static` binary via melange `fetch` (custom Renovate datasource already exists). Avoids the C++ toolchain entirely. HTTP 8080. |
| qbittorrent-libtorrentv1 | same | same, v1 flavor of the static build |
| deluge | libtorrent **python bindings** — must build from source in melange | Heaviest recipe in the plan. Dormant app; do last or exempt. |
| home-assistant | Enormous Python dep tree, native wheels, ffmpeg, bluetooth libs | Feasible in principle (venv pattern scales) but budget a dedicated spike. HTTP 8123. |

### Exempt — distroless is not applicable (document in each app's README, revisit yearly)

| App | Reason |
|---|---|
| busybox | The product *is* a shell — permanently fails the no-shell assertion by definition. |
| actions-runner | Workflow jobs execute arbitrary shell steps; a shell-less runner is useless. |
| esphome | Compiles firmware at runtime — needs gcc/platformio toolchains in the image. |
| plex, plex-next | Proprietary blob, bundled FHS/library tree, spawns transcoders; no source to melange-build. |
| emby | Same class as plex. |

Tally: 2 shipped + 3 + 9 + 7 + 8 + 4 hard + 6 exempt = 39 rows ≈ 36 apps + cloudflared/kubectl (already done) + opentofu-runner counted once.

---

## 4. Testing standard (applies to every new image)

1. **`container_test.go` is mandatory.** Flip the existing "Warn if no boot smoke" step in `distroless-build.yaml` to a hard failure (both current images already have tests, so this can land immediately in Wave 0).
2. **Long-running services get `TestHTTPEndpoint`**, not just `--version` — a Renovate bump that breaks runtime config/migrations only shows up when the service actually serves. CLI tools (`kopia`, `tqm`, `kubectl`, `cloudflared`) keep `TestCommandSucceeds`.
3. **melange recipes get `test:` blocks** — functional check in the sandbox (where busybox is allowed) before image assembly.
4. **Structure assertions stay load-bearing** (non-root, no shell, entrypoint). Any exception (irqbalance's root requirement) is an explicit per-image allowlist in the workflow step with an inline rationale comment — never a global soften.
5. **Writable state:** apps needing `/config` get it created via apko `paths:` with `uid/gid 65532` ownership; the HTTP smoke implicitly verifies the app can write there on first boot.
6. **CVE gate stays strict** on distroless (HIGH+ fixable, both scanners). Big runtimes (python, aspnet) will occasionally trip on fresh CVEs — the daily Wolfi rebuild is the fix path; use time-bounded `.grype.yaml`/`.trivyignore.yaml` entries (thread-#8 pattern) only for genuinely unpatchable findings. Do not soften this gate; per STATE.md it is the fork's security floor.

---

## 5. Rollout

One image per PR (matches the changed-dir discovery, keeps sticky comments and size diffs per-image, and keeps reverts trivial). Each PR: recipe(s) + `container_test.go` + Renovate annotation(s), self-contained.

**Wave 0 — CI groundwork (before any new image):**
1. `.renovaterc.json5`: add the apko list-item floor-pin matchString, e.g. `"-\\s+[a-z0-9._-]+>=(?<currentValue>[^\\s#]+)\\s+#\\s*renovate:\\s+datasource=(?<datasource>\\S+)\\s+depName=(?<depName>\\S+)(\\s+versioning=(?<versioning>\\S+))?"`; add the `expected-commit` digest-capture matchString (github-tags, `currentDigest`); add the distroless packageRules (§2d).
2. Retro-fit floor pins onto `distroless/cloudflared` and `distroless/kubectl` (kubectl keeps its per-minor manual-bump comment). This is the pilot for §2a — verify Renovate opens a PR and the gate runs before scaling.
3. `distroless-digest-fixup.yaml` workflow (§2c) — can land with Wave 3 instead if preferred; it has no consumers before then.
4. Flip boot-smoke warning → failure.
5. Scheduled-run failure visibility: add a final job to `distroless-build.yaml` (`if: failure() && github.event_name == 'schedule'`) that creates/updates a pinned "nightly distroless build failed" issue. Today a 2 AM cron break is silent unless someone reads the Actions tab.
6. `distroless/README.md`: recipe + test templates (copy-paste skeletons of §2a/2b + `container_test.go`), conventions (uid 65532, `/config` via `paths:`, annotation requirements), and the exemption list from §3.

**Waves 1→5:** in table order. Wave 1 (~1 PR each, days), Wave 2 (kopia first — revive PR #21 — then one recipe at a time), Wave 3 (build sonarr first, the other five servarr apps are copy-adapt), Wave 4 (tautulli first as the venv-pattern pilot), Wave 5 (spike, then decide build-vs-exempt per app).

**Per-image acceptance criteria (definition of done):**
- [ ] PR gate green on both arches: build, structure assertions, melange test (if recipe), boot/HTTP smoke, Grype+Trivy.
- [ ] Renovate annotation present and *proven*: confirm the dep appears on Renovate's Dependency Dashboard after merge (wrong regexes fail silently — this check is the difference between "pinned" and "validated on update").
- [ ] Published as `<app>-distroless` with the standard tag set; size-diff comment shows a win vs the apps/ flavor.
- [ ] Daily cron includes it (automatic via discovery) and passes.

**Retiring the `apps/` counterpart** is out of scope per-image: apps/ flavors stay published in parallel until the operator switches consumption (kopia precedent). Only `cloudflared-distroless` is consumed downstream today, so there is no forcing deadline — dual-track is cheap because apps/ is dormant and its CVE gate is already report-only.

---

## 6. Risks & open questions

| Risk | Mitigation |
|---|---|
| Wolfi lags upstream releases → red Renovate PRs on floor pins | Working as designed (visible lag). If a specific app's PRs sit red for weeks, switch that app to a melange source build. |
| Cron matrix grows to ~30 images × 2 arches (~60 jobs nightly) | Public-repo runners (incl. `ubuntu-24.04-arm`) are free; add `max-parallel` if queueing bothers. Changed-dir discovery keeps PR builds at 2 jobs. |
| Digest-fixup workflow pushes to Renovate branches (zizmor/security review) | Trigger is `pull_request` (not `_target`), same-repo branches only, writes limited to `distroless/**/melange.yaml`; zizmor runs in CI already. |
| melange builds of big apps (bazarr, stash, home-assistant) are slow (>30 min) | Per-image PRs isolate the cost; melange build cache via `actions/cache` keyed on recipe hash if it becomes painful. |
| sabnzbd/unrar licensing | Ship without unrar initially; document. Separate decision, not a blocker. |
| Strict HIGH+ gate blocks a wave-3/4 image on an unpatchable upstream CVE | Time-bounded ignore entries (thread-#8 pattern), never a threshold change. |
| Renovate regex managers silently not matching | Per-image acceptance criterion: verify on the Dependency Dashboard, plus `renovate-config-validator` in a one-off check when touching `.renovaterc.json5`. |

Open questions to settle during implementation (none block Wave 0): cni-plugins entrypoint shape (Go shim vs. documented copy pattern), irqbalance root exemption vs. skip, aspnet runtime major per servarr app, tududi default port, whether home-assistant is worth the spike while dormant.
