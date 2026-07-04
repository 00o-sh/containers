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
| `distroless/<app>/container_test.go` | Boot smoke via the `tests` helper package runs against the freshly built per-arch image (`TEST_IMAGE` override) |

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
- If Wolfi hasn't packaged the new upstream version yet, apk resolution fails → the Renovate PR sits red until Wolfi catches up. Lag is *visible* instead of silent. **Caveat learned live (PRs #126/#127):** Wolfi's advisory feed and recipe repo reference the new version *before* the apk lands in the APKINDEX — "Wolfi has it" per advisories ≠ installable. And since the apk landing generates no event on the PR branch, `retry-renovate-distroless.yaml` re-runs the failed builds daily (post-Wolfi-rebuild) so the PR self-greens and automerges when the package is actually pullable.
- The exact shipped version continues to come from the SBOM at tag time — floor pins don't affect the tag scheme.

Requires one new `matchStrings` entry (the existing one only matches `key: value` lines, not list items) — see §5, Wave 0.

### 2b. Melange builds from source (Go/C/C++ apps) → `git-checkout` with tag-follows-version

```yaml
package:
  name: kopia
  version: "0.23.0" # renovate: datasource=github-releases depName=kopia/kopia extractVersion=^v(?<version>.*)$
pipeline:
  - uses: git-checkout
    with:
      repository: https://github.com/kopia/kopia
      tag: v${{package.version}}
```

- The `version:` line matches the existing regex manager (extended with `extractVersion` support for v-prefixed upstream tags); the checkout tag follows the version template automatically, so a Renovate bump is a one-line diff that flows green end-to-end.
- **Deliberately no `expected-commit`** (decision made during Wave 2 implementation): a commit pin goes stale on every version bump and nothing can refresh it — Mend-cloud Renovate can't run postUpgradeTasks (admin-only `allowedCommands`), Renovate's `currentDigest` capture resolves annotated tags to the tag *object* hash rather than the peeled commit melange verifies against (and interacts badly with `extractVersion`), and a fixup-bot workflow fails structurally: `GITHUB_TOKEN` pushes don't re-trigger CI (the fixed commit would have no checks, wedging automerge) and foreign commits stop Renovate from managing its branch. Checkout-by-tag over HTTPS matches the existing `apps/` supply-chain posture, where Dockerfiles curl upstream tarballs by version with no checksum. Revisit commit pinning only if a bot-app token lands on this fork.
- Every `melange.yaml` must carry a `test:` block (runs in the melange sandbox where busybox is allowed) — cheap functional check before the image is even assembled.

### 2c. Prebuilt artifacts (.NET servarr tarballs, qbittorrent-nox, JARs) → open problem, solve before Wave 3

melange `fetch` *requires* `expected-sha256`, and per §2b nothing in the current setup can refresh a checksum on a Renovate bump. Options, in preference order:

1. **Build from source instead** (`git-checkout`, §2b pattern) — works for most servarr apps, which are plain .NET builds.
2. Fetch inside a `runs:` block with checksum verification against an upstream-published checksums file (many releases ship `*_checksums.txt` — verifiable without hardcoding a hash in the recipe).
3. A fixup-bot workflow — only viable with a bot-app token (see §2b for why `GITHUB_TOKEN` can't do it). Blocked on the fork's no-`BOT_*` policy.

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

### Wave 1 — Wolfi ships the app: apko-only (like kubectl)

> **cloudflared moved off this track (2026-07-04).** Wolfi's cloudflared packaging lags upstream chronically (2026.6.1 with the CVE-2026-41178 fix was a month old with no apk published; their index also shows a 2025.10→2026.3 gap), so the §6 escape hatch was invoked: cloudflared is now a **melange source build** tracking upstream releases directly. Also noted for the record: Cloudflare's *official* image is itself distroless (`gcr.io/distroless/base-debian13:nonroot`) and satisfies the README's prefer-official criteria — keeping ours is an operator choice for the daily Wolfi base-CVE rebuilds, the PR gate on version bumps, and our provenance attestation; switching the cluster to `docker.io/cloudflare/cloudflared` remains a valid simplification at any time.

| App | Wolfi pkg | Test | Notes |
|---|---|---|---|
| cni-plugins | `cni-plugins` ✓ | `TestFileExists` on plugin binaries | **Deferred** (same call as PR #21): the image's job is an `rsync`-glob copy into `/host/opt/cni/bin`, and Wolfi installs the plugins into `/usr/bin` mixed with other binaries — shell-free copy semantics need a dedicated Go shim. Revisit with a melange-built `install-cni` helper. |
| k8s-sidecar | `k8s-sidecar` ✓ | import smoke through the venv interpreter (app fatal-exits without a kube API; Wolfi's own recipe smoke greps CRITICAL for the same reason) | **Done** (Wave 1 pilot). Python app but Wolfi packages it whole — venv at `/usr/share/k8s-sidecar`. Parity caveat flagged in the recipe: the optional SCRIPT change-hook can't exec *shell* scripts in a shell-free image (non-shell executables still work; core collection function unaffected). Operator sign-off on the caveat = keep; veto = exempt. |
| opentofu-runner | `opentofu-1.12` ✓ (per-minor, like kubectl) | `tofu version` command smoke | Current image = flux-iac `tf-runner` base + tofu-as-terraform. Distroless needs the tf-runner binary too → melange `git-checkout` of `flux-iac/tofu-controller`. Straddles wave 1/2; per-minor pin handled like kubectl (manual name bump, documented in comment). |

### Wave 2 — single binary, melange build from source (kopia precedent, PR #21)

| App | Source build | Runtime pkgs | Test |
|---|---|---|---|
| kopia | Go, `kopia/kopia` | ca-certs, tzdata | ✅ **Done** (PR #21 revived). Web UI embedded via the `htmluibuild` Go module (no node toolchain; the `nohtmlui` shortcut from #21 violated the no-functionality-loss rule); default cmd is server mode on 51515 for parity with apps/kopia. Tests: `--version` + HTTP smoke on the UI. |
| tqm | Go, `autobrr/tqm` | ca-certs | ✅ **Done.** `tqm version`; ldflags mirror upstream goreleaser; recipe carries x/crypto+x/net security bumps. Note: the 2026-07-04 upstream sync *removed* `apps/tqm` — the distroless image is now this repo's only tqm. |
| ~~webhook~~ | — | — | **Dropped**: `apps/webhook` was removed by the 2026-07-04 upstream sync; nothing left to migrate. |
| smartctl-exporter | Go, `prometheus-community/smartctl_exporter` | `smartmontools` | ✅ **Done.** HTTP smoke on `:9633` `/metrics` (serves 200 without device access — verified). Recipe carries x/crypto+x/net+x/oauth2 security bumps. |
| stash | Go, `stashapp/stash` (has JS asset build — needs nodejs in melange env) | `ffmpeg` + `python-3.14` (scrapers are stock functionality — no-functionality-loss rule) | HTTP smoke on `:9999` |
| nzbget | C++, `nzbgetcom/nzbget` (autotools) | libxml2, openssl, `7zip`/`par2` extras — audit which Wolfi ships | HTTP smoke on `:6789` |
| transmission | C, `transmission/transmission` (cmake) | openssl, curl libs | HTTP smoke on `:9091` |
| postgres-init | ✅ **Done.** Repo-local Go shim (`src/`) that execs the same pg_isready/psql/createuser/createdb binaries as the bash entrypoint — exact flag/behavior parity incl. `INIT_POSTGRES_USER_FLAGS` and `/initdb/<dbname>.sql` seeding; command failures now fatal (deliberate improvement over the script's missing `set -e`). Runtime dep `postgresql-18-client` (per-major manual bump, kubectl-style). Tests: `--version` smoke + full two-container integration (real postgres, asserts role+db exist). Workflow gained a `--source-dir` convention for `src/` dirs. |
| irqbalance | C, `irqbalance/irqbalance` | glib | ⚠️ must run as root to write `/proc/irq/*` → **conflicts with the non-root structure assertion**. Either add a per-image assertion override (explicit allowlist in the workflow step) or exempt. Decide before building; dormant app, low priority. |

### Wave 3 — .NET / JVM (blocked on the §2c checksum decision — prefer source builds)

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
| sabnzbd | python-3.14 | HTTP 8080 | `unrar` is non-free and not in Wolfi — build it from source in melange (freeware license permits redistribution; linuxserver precedent). Shipping without rar support would violate the no-functionality-loss rule. |
| pyload-ng | python-3.14 | HTTP 8000 | |
| octoprint | python-3.14 | HTTP 5000 | |
| jbops | python-3.14 (scripts only) | `TestFileExists` on script paths | upstream pins `master` — pin a git branch/commit manually in the recipe (no releases to track; exclude from Renovate) |
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

0. **Feature parity is mandatory** (operator directive, 2026-07-04): the distroless variant must not lose functionality the `apps/` flavor had — no CLI-only builds, no degraded modes. Env-switched `entrypoint.sh` defaults become the image's `cmd:`; args override for the other branch. Where parity is impossible, exempt the app instead of shipping a reduced image.
1. **`container_test.go` is mandatory.** Flip the existing "Warn if no boot smoke" step in `distroless-build.yaml` to a hard failure (both current images already have tests, so this can land immediately in Wave 0).
2. **Long-running services get `RequireHTTPEndpoint`**, not just `--version` — a Renovate bump that breaks runtime config/migrations only shows up when the service actually serves. CLI tools (`kopia`, `tqm`, `kubectl`, `cloudflared`) keep `RequireCommandSucceeds`.
3. **melange recipes get `test:` blocks** — functional check in the sandbox (where busybox is allowed) before image assembly.
4. **Structure assertions stay load-bearing** (non-root, no shell, entrypoint). Any exception (irqbalance's root requirement) is an explicit per-image allowlist in the workflow step with an inline rationale comment — never a global soften.
5. **Writable state:** apps needing `/config` get it created via apko `paths:` with `uid/gid 65532` ownership; the HTTP smoke implicitly verifies the app can write there on first boot.
6. **CVE gate stays strict** on distroless (HIGH+ fixable, both scanners). Big runtimes (python, aspnet) will occasionally trip on fresh CVEs — the daily Wolfi rebuild is the fix path; use time-bounded `.grype.yaml`/`.trivyignore.yaml` entries (thread-#8 pattern) only for genuinely unpatchable findings. Do not soften this gate; per STATE.md it is the fork's security floor.

---

## 5. Rollout

One image per PR (matches the changed-dir discovery, keeps sticky comments and size diffs per-image, and keeps reverts trivial). Each PR: recipe(s) + `container_test.go` + Renovate annotation(s), self-contained.

**Wave 0 — CI groundwork (before any new image):** ✅ **done**
1. ✅ `.renovaterc.json5`: apko list-item floor-pin matchString + `extractVersion` support on the existing melange/apko matchString; distroless packageRules (§2d).
2. ✅ Floor pin retro-fitted onto `distroless/cloudflared` (§2a pilot; anchors verified live on the Dependency Dashboard 2026-07-04, incl. Renovate PRs #126/#127). Superseded for cloudflared by the source-build switch (see Wave 1 note); k8s-sidecar remains the §2a reference. `distroless/kubectl` stays per-minor/manual per its in-file comment.
3. ~~digest-fixup workflow~~ — dropped; see §2b for why it can't work with `GITHUB_TOKEN`. §2c is the Wave-3 replacement decision.
4. ✅ Boot-smoke warning → hard failure.
5. ✅ Scheduled-run failure job files/updates an issue (was: silent 2 AM breaks).
6. ✅ `distroless/README.md`: conventions, templates, pinning rules, melange gotchas (PR #21's four lessons), exemption list.

**Waves 1→5:** in table order. Wave 1: k8s-sidecar ✅, cni-plugins deferred, opentofu-runner pending. Wave 2: kopia ✅ + tqm ✅ (PR #21 revived; recipe lessons baked into README) + smartctl-exporter ✅; webhook dropped (removed upstream); remaining Go/C apps one PR each. Wave 3: build sonarr first after settling §2c (the other five servarr apps are copy-adapt). Wave 4: tautulli first as the venv-pattern pilot. Wave 5: spike, then decide build-vs-exempt per app.

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
| No commit/checksum pinning on melange source checkouts (§2b decision) | Matches existing `apps/` posture (unchecksummed tarball curls). Provenance attestation + SBOM still record what was built. Revisit with a bot-app token. |
| melange builds of big apps (bazarr, stash, home-assistant) are slow (>30 min) | Per-image PRs isolate the cost; melange build cache via `actions/cache` keyed on recipe hash if it becomes painful. |
| sabnzbd/unrar licensing | Ship without unrar initially; document. Separate decision, not a blocker. |
| Strict HIGH+ gate blocks a wave-3/4 image on an unpatchable upstream CVE | Time-bounded ignore entries (thread-#8 pattern), never a threshold change. |
| Renovate regex managers silently not matching | Per-image acceptance criterion: verify on the Dependency Dashboard, plus `renovate-config-validator` in a one-off check when touching `.renovaterc.json5`. |

Open questions to settle during implementation: cni-plugins Go copy-shim design (deferred, see §3), irqbalance root exemption vs. skip, aspnet runtime major per servarr app, tududi default port, whether home-assistant is worth the spike while dormant.
