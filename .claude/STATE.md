# Session state

Live working log. Future Claude: **read this first**, then update it as you go (mark items resolved, add new threads, prune stale ones). If this file contradicts the repo, trust the repo and fix the file.

**Last updated:** 2026-05-19 (session: bypass CVE-2026-40962 ffmpeg in apps/ CVE gate for emby PR #19 unblock)

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

### 8. Bundled-vendor CVE policy: `.grype.yaml` + `.trivyignore.yaml` (this PR)

**Motivation:** PR #19 (Renovate-authored emby version bump 4.9.3.0 → 4.9.5.0) failed the apps/ CVE gate even after #13's softening to CRITICAL-only. Investigation showed 34 instances on emby-ubuntu all stem from a single CVE — `CVE-2026-40962` (ffmpeg) — counted across emby's bundled ffmpeg binary + 7 shared libs (libavcodec, libavfilter, libavformat, libavutil, libavdevice, libswresample, libswscale). Same CVE also fires daily on stash, pyload-ng, octoprint via the Alpine 3.23 `ffmpeg` apk.

**Why we can't patch:**

- emby is closed-source. The Dockerfile extracts emby's upstream `.deb` (`dpkg-deb -xv`) which bundles ffmpeg statically; only MediaBrowser can re-bundle.
- Alpine apk ffmpeg is awaiting an upstream Alpine 3.23.x update. Renovate is constrained to 3.23.x by the `Allowed Alpine Version` packageRule, so we get the fix when Alpine ships it — not before.

**Fix shape:**

- `.grype.yaml` at repo root with one `ignore:` entry for `CVE-2026-40962`. Wired into all five Grype call sites via the `config:` input (one each in app-builder.yaml and distroless-build.yaml; three in vulnerability-scan.yaml).
- `.trivyignore.yaml` with the equivalent entry, including `expired_at: 2026-08-19` (Trivy honors this natively). Wired via `trivyignores:` input on both Trivy call sites.
- Both files carry a clear rationale block and a re-evaluation date.

**Re-evaluation triggers (~90 days from 2026-05-19):**

- Trivy will automatically re-fail the gate on `expired_at: 2026-08-19`.
- Grype doesn't enforce expiry, so the file's documented `Re-evaluate by` date is a manual reminder.
- The right hooks to re-check before then: (1) check emby release notes for a re-bundle, (2) check Alpine 3.23.x changelog for an ffmpeg patch, (3) any image migrating to `distroless/` drops out of this concern entirely.

**What this does NOT do:**

- Doesn't blanket-suppress other CVEs. New CVEs still gate normally.
- Doesn't suppress on `distroless/` images either (they shouldn't carry ffmpeg-bundling apps anyway, but the same ignore applies for consistency).

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
