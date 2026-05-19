# Session state

Live working log. Future Claude: **read this first**, then update it as you go (mark items resolved, add new threads, prune stale ones). If this file contradicts the repo, trust the repo and fix the file.

**Last updated:** 2026-05-18 (session: CVE gate softening shipped on top of bot-app trigger bypass)

---

## Current branch

`fix/apps-cve-gate-critical` — this PR. Stacked on `fix/bot-app-trigger-bypass` (PR #12). Merge order: #12 first, then this one.

## Open threads

### 1. Apps CVE gate raised to CRITICAL (resolved in this PR; visibility tradeoff to monitor)

**Shipped (2026-05-18):** apps/ track gated on `CRITICAL + --only-fixed`. Distroless track unchanged (still `HIGH+` + `--only-fixed`). Edit lives in `.github/workflows/app-builder.yaml` — see the inline comments on the Grype and Trivy steps for rationale.

**Why this is OK for the apps/ track:** only `cloudflared-distroless` is consumed by the cluster. The ~10 HIGH findings (Run `26003929109`: `cni-plugins`, `actions-runner`, `bazarr`, `deluge`, `emby`, `esphome`, `home-assistant`, `lidarr`, `nzbhydra2`, `opentofu-runner`, …) come from upstream Alpine patch cadence on a dormant fleet. A strict gate held releases hostage to upstream timing without protecting anything the operator actually runs.

**Tradeoff to know:** with `severity-cutoff: critical`, HIGH findings no longer land in SARIF for apps/, so they drop off the Security tab and PR sticky comment for these images. If you want HIGH still surfaced as visibility-only (no fail-build), add a second non-failing Grype invocation — not done now to keep the diff small. Distroless still surfaces everything HIGH+.

**Follow-ups (not in this PR):**

- Restore Renovate (see thread #2) so base-image bumps land organically — that's the actual path to clearing the CRITICAL+HIGH backlog over time.
- Migrate dormant `apps/` images to `distroless/` per the standing migration plan. Each migrated image then gates strict.

### 2. Bot-app workflows: auto-triggers stripped (this PR), re-enable when BOT_APP_ID is provisioned

**Status:** auto-triggers disabled in this PR. Affected files:

- `stale.yaml` — stripped `schedule: 30 1 * * *`. Still callable via `workflow_dispatch`.
- `retry-release.yaml` — stripped `schedule: 30 1 * * *`. Still callable via `workflow_dispatch`.
- `renovate.yaml` — stripped `push: branches: main` trigger. **Side effect: Renovate no longer fires for this fork**, so base-image / package bumps stop arriving organically. Relevant to thread #1 — Renovate is normally how Alpine patches land.
- `labeler.yaml`, `label-sync.yaml` — already neutered upstream of this PR (`workflow_dispatch` only with the same comment pattern). No changes.
- `deprecate-app.yaml` — `workflow_dispatch` only already. Will error if the operator manually invokes it; left as-is so the error surfaces at the time of intent.

**Re-enable when `BOT_APP_ID` + `BOT_APP_PRIVATE_KEY` are configured:** restore the `schedule:` / `push:` blocks in each file (search for "Temporarily disabled in this fork"). The same pattern is already used as a sentinel in `labeler.yaml`/`label-sync.yaml`, so the diff is mechanical.

`app-builder.yaml` is **not** affected — it already has the `Detect bot-app secret` fallback pattern and works on the fork with `github.token`. Don't touch it.

### 3. Local branch hygiene

10 merged feature branches sit locally (all squash-merged on origin):
`chore/mise-dev-tools`, `feat/apps-cve-scan`, `feat/apps-flavor-suffix`, `feat/distroless-cloudflared-pilot`, `feat/distroless-cve-visibility`, `feat/distroless-image-suffix-and-cosign-digest`, `feat/distroless-sandbox-and-attest`, `feat/vuln-scan-distroless`, `fix/distroless-version-from-sbom`, `fix/vuln-scan-tolerate-missing`.

Plus `fix/vuln-scan-outcome-guard` (the previous working branch, squashed-merged as #11). Confirm before deleting any (the operator may use them as historical reference points).

## Recently closed (last 7 days)

- #1–#11 — full distroless pilot landed: cloudflared via Wolfi+apko, naming convention `<app>-<flavor>` / `<image>-distroless`, sandbox PR tags, attestation, dual scanner CVE gate, sticky comments, daily Wolfi rebuild + daily vuln scan, two scan-resilience fixes (#10, #11).

## Don't forget

- **Only `cloudflared-distroless` is consumed downstream.** Frame PR risk on `apps/` accordingly — those images are dormant.
- **Renovate-style version pins**: never edit `VERSION` defaults in `docker-bake.hcl` without the `// renovate: datasource=...` comment.
- **`BOT_APP_ID` won't be added to this fork (for now)** — design fixes around `github.token` fallback, not around acquiring the secret.

---

## Protocol for future Claude

1. Read this on session start; bring up open threads if relevant to the user's request.
2. When you finish a thread, move it to "Recently closed" with one-line outcome and PR/commit ref. Prune entries older than ~14 days from that section.
3. When you start non-trivial work, add a thread (rough scope + decision points), not just a task list.
4. Bump "Last updated" each time you write here.
5. If an entry contradicts what `git`, `gh`, or the workflow files actually show, the file is wrong — fix it. Memory is not a substitute for verification.
