# Session state

Live working log. Future Claude: **read this first**, then update it as you go (mark items resolved, add new threads, prune stale ones). If this file contradicts the repo, trust the repo and fix the file.

**Last updated:** 2026-05-19 (session: extended bundled-vendor CVE bypass to 24 CRITICAL clusters)

---

## Current branch

`main`. PR #12 (bot-app trigger bypass) and #13 (apps CVE gate softened to CRITICAL) both merged. PR #14 (`Pull From origin`) being landed now via local merge resolution + push to fork's main.

## Open threads

### 1. CVE gate softened to CRITICAL on apps/ (resolved in #13)

`app-builder.yaml`: apps/ track gated on `CRITICAL + --only-fixed`. Distroless track unchanged (still `HIGH+` + `--only-fixed`). Inline comments on the Grype and Trivy steps document the rationale.

**Tradeoff still in effect:** HIGH findings no longer land in apps/ SARIF, so they drop off the Security tab and the sticky PR comment for these images. Distroless still surfaces everything HIGH+. If you want HIGH surfaced for apps/ as report-only (no fail-build), add a second non-failing Grype invocation.

**Long-term fix:** restore Renovate (thread #2) so base-image bumps land organically; migrate dormant `apps/` images to `distroless/`.

### 2. Bot-app workflows: auto-triggers stripped (resolved in #12; secret renamed in #14)

**Status:** auto-triggers disabled in #12. Workflows still callable via `workflow_dispatch`:

- `stale.yaml` — `schedule:` stripped.
- `retry-release.yaml` — `schedule:` stripped.
- `renovate.yaml` — `push:` stripped. The workflow dispatched Renovate via `home-operations/.github`; with that path closed, Renovate Cloud (Mend-hosted GitHub App) is the active runner. See thread #7 for the `forkProcessing: "enabled"` opt-in that makes Cloud actually process this fork.
- `labeler.yaml`, `label-sync.yaml` — already neutered upstream pre-#12.
- `deprecate-app.yaml` — `workflow_dispatch` only; left as-is.

**Important rename absorbed via #14:** upstream renamed the GitHub-App secret from `BOT_APP_ID` (numeric app-id) to `BOT_CLIENT_ID` (the client-id GitHub now prefers — `app-id` is deprecated). The `BOT_APP_PRIVATE_KEY` secret name is unchanged. **Re-enable instructions:** provision `BOT_CLIENT_ID` + `BOT_APP_PRIVATE_KEY` and restore the `schedule:` / `push:` blocks (search for "Temporarily disabled in this fork" in the three workflows above).

**Subtle upstream bug to know about:** `app-builder.yaml`'s `Detect bot-app secret` step still keys off `BOT_APP_ID`, even though the consuming `Generate Token` step uses `client-id: BOT_CLIENT_ID`. On a fork with only `BOT_CLIENT_ID` provisioned, the detection always says "not configured" and the `github.token` fallback always fires. Harmless on this fork (no bot app anyway), but the detection step needs to be updated to check `BOT_CLIENT_ID` if you ever want the bot-app code path to engage. Upstream issue worth filing.

### 3. Tooling migration: `.justfile` removed; tasks in `mise`

Absorbed via #14. The `.justfile` is gone; equivalent tasks live in `.mise.toml` under `[tasks.local-build]`, `[tasks.remote-build]`, `[tasks.generate-label-config]`. New invocation: `mise run local-build <app>` (was `just local-build <app>`).

CLAUDE.md updated to match. `just` is dropped from `.mise.toml`'s `[tools]` (no longer needed).

### 4. License: MIT → Apache 2.0

Absorbed via #14. Devin Buhl committed `chore: update LICENSE` upstream on 2026-05-18; the repo is now Apache 2.0. Functionally equivalent permissive license to MIT plus an explicit patent grant + attribution-preservation requirement. **Not Bitnami**, no commercial restrictions. No downstream impact for this fork's distribution model.

### 5. Stale codeql-action SHAs in non-conflicting workflows

After #14 merge, two upload-sarif call sites in `vulnerability-scan.yaml` were bumped to `v4.35.5` (one resolved via conflict, one bumped for in-file consistency). The remaining sites — `app-builder.yaml` (2x) and `distroless-build.yaml` (2x) — are still pinned to `v4.35.4`. Renovate would normally catch these but is paused for the fork (thread #2). Bump them in a small follow-up PR or wait until Renovate is restored.

### 7. Renovate Cloud enablement on the fork

**Status:** `forkProcessing: "enabled"` added to `.renovaterc.json5` (this PR). The Mend-hosted Renovate App is already installed but had been silently skipping the repo because forks are excluded by default.

**Expected behavior after merge:**

- Renovate creates the "Dependency Dashboard" issue on its first successful run (visible signal that it's working — its absence is what diagnosed the problem).
- A wave of 20–40 PRs lands as the backlog clears: base-image bumps (`Alpine 3.23.X`), action SHA bumps (codeql, etc.), tool version bumps in `.mise.toml` and `docker-bake.hcl` annotations.
- `docker-bake.hcl` updates **auto-merge** per the existing `packageRules` (PR-time CVE gate at CRITICAL still gates each merge — anything with CRITICAL+fixable is blocked).
- Tool / workflow / preset bumps in non-bake files require manual review.

**If it still doesn't work after this merges:**

- Check Mend dashboard at app.mend.io/renovate for the run log; look for "this is a fork, skipping" (means `forkProcessing` didn't take effect) or preset resolution errors (the `github>home-operations/renovate-config` preset may have been moved or made private).
- Force-run from the Mend UI to skip the scheduled-cycle wait.
- The `renovate.yaml` workflow in this repo is still disabled (thread #2) and is *not* the runner — don't confuse the two paths.

### 8. Bundled-vendor CVE policy: `.grype.yaml` + `.trivyignore.yaml` (this PR, expanded scope)

**Motivation:** Started from PR #19 (emby bump blocked by ffmpeg `CVE-2026-40962`). User then asked to extend bypasses to cover the rest of the Security tab's CRITICAL+fixable advisories in one pass. Survey identified **24 unique CRITICAL CVE clusters** across the apps/ fleet, all upstream-vendored or upstream-only-controlled.

**Categorization (all upstream-vendored / upstream-only-controlled):**

| Cluster | Example CVEs | Affected apps | Fix path |
|---|---|---|---|
| Bundled ffmpeg | `CVE-2026-40962` | emby + 6 Alpine | emby re-bundle / Alpine 3.23.x patch |
| Alpine python | `CVE-2026-6100`, `CVE-2026-7210` | 15+ Alpine apps | Alpine 3.23.x patch |
| Go stdlib in upstream binaries | `CVE-2025-22871`, `CVE-2025-68121`, `CVE-2026-27143` | webhook, smartctl-exporter, cni-plugins, tqm, postgres-init, home-assistant, stash, actions-runner | Upstream re-release with newer Go |
| theme-park nginx/php stack | `CVE-2025-48174` (libavif), `CVE-2025-49794`/`-49796` (libxml2), `CVE-2026-31789` (openssl), `CVE-2026-42945` (nginx) | theme-park | Upstream theme-park rebuild |
| Debian 13 base | `CVE-2026-5450` (libc), `CVE-2026-33845`/`-42010` (libgnutls) | esphome-debian | Debian security update |
| nzbhydra2 Java fat JAR | `GHSA-83qj-6fr2-vhqg`, `GHSA-95jq-rwvf-vjx4` (tomcat), `GHSA-c4q5-6c82-3qpw`, `GHSA-mf92-479x-3373` (spring-security-web) | nzbhydra2 | Upstream JAR rebuild |
| pyload-ng + bazarr Python | `GHSA-3f7w-p8vr-4v5f`, `GHSA-8w3f-4r8f-pf53` (pyload-ng), `GHSA-9298-4cf8-g4wj` (waitress), `GHSA-vqfr-h8mv-ghfj` (h11) | pyload-ng, bazarr | Upstream pip-pin update |
| Go embedded deps | `GHSA-v778-237x-gjrc` (golang.org/x/crypto), `GHSA-p77j-4mvh-x3m3` (grpc) | home-assistant, actions-runner, opentofu-runner | Upstream re-release |

**Fix shape:**

- `.grype.yaml` and `.trivyignore.yaml` at repo root, with 24 ignore entries each, organized in section blocks by root cause. All entries time-bounded to **2026-08-19** (~90 days from 2026-05-19).
- Wired into all 5 Grype call sites via `config:` input and both Trivy call sites via `trivyignores:` input. PR-time + daily scan both honor the ignores.
- Trivy enforces `expired_at` natively — after the date, the gate re-fails and forces re-evaluation. Grype doesn't enforce expiry, but the rationale comments document re-eval dates manually.

**Operational impact:**

- **Apps/ PR-time gate now clean** for all known CRITICAL+fixable CVEs. Future Renovate PRs should pass CI as long as they don't introduce NEW critical CVEs.
- **Security tab noise drops** by 24 clusters worth of alerts after the next daily-scan run picks up the config change.
- **No suppression of HIGH/MEDIUM/LOW** — those remain as informational signal in the Security tab. They never blocked builds (gate is critical-only post-#13).

**What this does NOT do:**

- Doesn't ignore future NEW CVEs. As Renovate bumps land and introduce different CVEs, the gate fires normally.
- Doesn't change gate thresholds (apps/ still CRITICAL+fixable; distroless/ still HIGH+).
- Doesn't suppress on `distroless/` images — the file is loaded there too for symmetry, but distroless shouldn't carry most of these vendored CVEs anyway.

**Re-evaluation hooks (by 2026-08-19):**

1. Trivy auto-fails on expiry — that's the forcing function. Run the gate intentionally; review what's still blocking.
2. Check upstreams for re-bundle/re-release:
   - Alpine 3.23.x for python + ffmpeg patches
   - Debian 13 for libc + libgnutls patches
   - emby, theme-park, nzbhydra2, pyload-ng, bazarr, webhook, smartctl-exporter, cni-plugins, tqm, home-assistant, opentofu-runner, actions-runner releases
3. Any image migrating from `apps/` to `distroless/` drops out of its row entirely (the kopia precedent).

### 6. Local branch hygiene

11 merged feature branches sit locally (all squash-merged on origin):
`chore/mise-dev-tools`, `feat/apps-cve-scan`, `feat/apps-flavor-suffix`, `feat/distroless-cloudflared-pilot`, `feat/distroless-cve-visibility`, `feat/distroless-image-suffix-and-cosign-digest`, `feat/distroless-sandbox-and-attest`, `feat/vuln-scan-distroless`, `fix/distroless-version-from-sbom`, `fix/vuln-scan-tolerate-missing`, `fix/vuln-scan-outcome-guard`, plus `fix/bot-app-trigger-bypass` and `fix/apps-cve-gate-critical` (this session's PRs, now merged).

Confirm before deleting any (the operator may use them as historical reference points).

## Recently closed (last 7 days)

- **#14** — upstream sync absorbed (this session). License → Apache 2.0; `.justfile` → `mise` tasks; `BOT_APP_ID` → `BOT_CLIENT_ID`; codeql-action bumped to v4.35.5 in vulnerability-scan; structural deletions (CODE_OF_CONDUCT.md, CONTRIBUTING.md, SECURITY.md). Two conflicts resolved: `.mise.toml` (union of toolchains) and `.github/workflows/vulnerability-scan.yaml` (kept #11's `if:` guard + upstream's SHA bump).
- **#13** — apps/ CVE gate softened to CRITICAL.
- **#12** — bot-app workflow auto-triggers stripped on the fork.
- **#1–#11** — full distroless pilot landed (see PR history).

## Don't forget

- **Only `cloudflared-distroless` is consumed downstream.** Frame PR risk on `apps/` accordingly — those images are dormant.
- **Renovate-style version pins**: never edit `VERSION` defaults in `docker-bake.hcl` without the `// renovate: datasource=...` comment.
- **`BOT_CLIENT_ID` won't be added to this fork (for now)** — design fixes around `github.token` fallback, not around acquiring the secret. The bot-app code paths in upstream workflows all gracefully degrade.
- **Tools**: prefer `mise run <task>` over assuming `just` is on PATH — `.justfile` is gone.

---

## Protocol for future Claude

1. Read this on session start; bring up open threads if relevant to the user's request.
2. When you finish a thread, move it to "Recently closed" with one-line outcome and PR/commit ref. Prune entries older than ~14 days from that section.
3. When you start non-trivial work, add a thread (rough scope + decision points), not just a task list.
4. Bump "Last updated" each time you write here.
5. If an entry contradicts what `git`, `gh`, or the workflow files actually show, the file is wrong — fix it. Memory is not a substitute for verification.
