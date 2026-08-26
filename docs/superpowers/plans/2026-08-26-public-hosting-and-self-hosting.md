# Public Hosting and Self-Hosting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a public default collaboration origin, a hosted-compatible load runner, and a reproducible single-replica self-hosting bundle suitable for `mapping.a13.info`.

**Architecture:** StrongDMM keeps an editable hosted origin defaulted to Aphelion's public service. The load runner uses ordinary hosted invitations and OIDC session credentials instead of privileged endpoints. A vendor-neutral Compose base runs the hardened service and PostgreSQL, while optional edge overlays support a dedicated Cloudflare Tunnel or a loopback reverse proxy.

**Tech Stack:** Go 1.25.13, Docker Compose, PostgreSQL, Cloudflare Tunnel, OIDC Authorization Code with PKCE, YAML, JSON Schema, PowerShell verification.

**Spec:** `docs/superpowers/specs/2026-08-26-public-hosting-and-self-hosting-design.md`

> **Status reconciliation (2026-08-26):** Tasks 1 through 4 are implemented and locally verified. The hosted load runner uses ordinary invitations with per-editor OIDC credentials; both Compose overlays validate; and the loopback production stack passed health, backup, restore, rollback, and clean-stop entry points. Repository tests, Aphelion race tests, and the Windows build pass. Task 5 is blocked only on interactive server access plus human-owned production OIDC values; no live Cloudflare resource has been mutated. Task 6's automated gates pass, while its final public evidence reconciliation remains pending Task 5.

## Global Constraints

- Content Tools is excluded.
- `mapping.a13.info` is public and has no Cloudflare Access policy.
- Any OIDC-authenticated user may create a session; joins remain invitation-only.
- Exactly one hosted-service replica is supported.
- Hosted credentials and invitation secrets never enter configuration, logs, command arguments, or result JSON.
- The existing shared `bark` tunnel is not modified.
- Changes remain uncommitted and unpushed unless the user separately authorizes Git operations.

---

### Task 1: Public Default Origin

**Files:**
- Create: `internal/aphelion/collab/ui/defaults.go`
- Create: `internal/aphelion/collab/ui/defaults_test.go`
- Modify: `internal/app/action_user.go`

**Interfaces:**
- Produces: `const DefaultHostedOrigin = "https://mapping.a13.info"`
- Consumes: the existing editable hosted sign-in dialog and `BeginHostedSignIn` origin validation.

- [x] **Step 1: Write the failing default-origin test**

```go
func TestDefaultHostedOriginIsPublicAphelionService(t *testing.T) {
	if DefaultHostedOrigin != "https://mapping.a13.info" {
		t.Fatalf("DefaultHostedOrigin = %q", DefaultHostedOrigin)
	}
	if _, err := normalizeHostedBaseURL(DefaultHostedOrigin); err != nil {
		t.Fatalf("default hosted origin is invalid: %v", err)
	}
}
```

- [x] **Step 2: Run the test and confirm it fails because the constant is absent**

Run: `go test ./internal/aphelion/collab/ui -run TestDefaultHostedOriginIsPublicAphelionService -count=1`

- [x] **Step 3: Add the constant and use it as the editable dialog value**

```go
const DefaultHostedOrigin = "https://mapping.a13.info"
```

Replace only the Aphelion-owned `hostedURL := "https://"` line with `hostedURL := collabui.DefaultHostedOrigin`. Do not persist credentials or disable custom entry.

- [x] **Step 4: Run focused UI tests**

Run: `go test ./internal/aphelion/collab/ui ./internal/app -count=1`

### Task 2: Hosted Load Credentials

**Files:**
- Modify: `internal/aphelion/collab/load/runner.go`
- Modify: `internal/aphelion/collab/load/runner_test.go`
- Modify: `internal/aphelion/collab/load/pilot_integration_test.go`
- Modify: `cmd/apheliondmm-loadtest/main.go`
- Modify: `cmd/apheliondmm-loadtest/main_test.go`
- Modify: `docs/verification/phase-7-load-tooling-2026-08-25.md`

**Interfaces:**
- Produces: `RunConfig.EditorTokens []string` and `LoadEditorTokens(path string) ([]string, error)`.
- Consumes: `POST /v1/sessions/{session_id}/hosted-invitations` and `/hosted-invitations/redeem`.

- [x] **Step 1: Add a failing hosted runner test**

Construct a hosted test service with three authenticated identities. Pass the owner credential plus two editor credentials in `RunConfig.EditorTokens`. Assert that the run completes and that no request reaches `/join-tokens`.

- [x] **Step 2: Run the focused test and confirm the current runner receives HTTP 403 from join-token minting**

Run: `go test ./internal/aphelion/collab/load -run TestRunnerUsesHostedInvitations -count=1`

- [x] **Step 3: Implement hosted invitation provisioning**

When `EditorTokens` is non-empty, require exactly one credential for every scenario actor. For each credential, create an editor invitation with the owner credential, decode its token and expiry, then redeem it with that editor credential. Connect using the editor credential. Retain `mintEditorToken` only when `EditorTokens` is empty.

- [x] **Step 4: Add negative tests**

Cover credential-count mismatch, an empty credential, invitation creation failure, redemption failure, and ensure error strings contain neither owner nor editor tokens.

- [x] **Step 5: Add a failing credential-file test**

Use a temporary `0600` JSON file containing `{"tokens":["first","second"]}`. Assert ordered loading, unknown-field rejection, a 1 MiB size limit, empty-token rejection, symlink rejection, and group/other-permission rejection on non-Windows systems.

- [x] **Step 6: Implement bounded secret-file loading and CLI selection**

Read the path from `APHELIONDMM_LOAD_EDITOR_TOKENS_FILE`. Reject simultaneous hosted-token-file and embedded-only usage errors. Never print the file contents or tokens.

- [x] **Step 7: Run load tests**

Run: `go test ./internal/aphelion/collab/load ./cmd/apheliondmm-loadtest -count=1`

### Task 3: Portable Hosted Configuration Contract

**Files:**
- Create: `docs/hosting/hosted-config.schema.json`
- Modify: `deploy/config/apheliondmm-collab.example.yaml`
- Modify: `internal/aphelion/collab/server/hosted_config_test.go`
- Modify: `internal/aphelion/collab/container/pilot_config_test.go`
- Create: `deploy/production/README.md`

**Interfaces:**
- Produces: a strict JSON Schema mirroring `server.HostedConfig` and a file-secret example.
- Consumes: `server.LoadHostedConfig` and `SecretSource`.

- [x] **Step 1: Add failing example-contract tests**

Read the repository example, require `public_origin`, `database.dsn.file`, `oidc.client_secret.file`, all limit fields, and telemetry. Load it with `server.LoadHostedConfig` and assert that unknown YAML keys remain rejected.

- [x] **Step 2: Run the focused test and confirm the environment-backed example fails the file-secret assertions**

Run: `go test ./internal/aphelion/collab/server ./internal/aphelion/collab/container -run 'HostedConfig|ProductionConfig' -count=1`

- [x] **Step 3: Publish the schema and file-secret example**

The schema uses draft 2020-12, `additionalProperties: false` at every object, HTTPS URI patterns for public/OIDC/telemetry origins, positive integer limits, and a `oneOf` requiring exactly one secret source.

- [x] **Step 4: Document every field**

Explain bind address, public origin, proxy trust, DSN, OIDC callback, limits, OTLP endpoint, secret-file rules, and the single-replica constraint. Include complete Cloudflare, Caddy, nginx, and Traefik edge expectations without making any one vendor mandatory.

- [x] **Step 5: Re-run focused contract tests**

Run: `go test ./internal/aphelion/collab/server ./internal/aphelion/collab/container -count=1`

### Task 4: Production Compose Bundle

**Files:**
- Create: `deploy/production/compose.yaml`
- Create: `deploy/production/compose.cloudflare.yaml`
- Create: `deploy/production/compose.loopback.yaml`
- Create: `deploy/production/config.yaml.example`
- Create: `deploy/production/.env.example`
- Create: `deploy/production/README.md`
- Create: `deploy/production/operations.ps1`
- Create: `internal/aphelion/collab/container/production_config_test.go`

**Interfaces:**
- Produces: one portable Compose project with mutually selectable edge overlays.
- Consumes: `deploy/container/Dockerfile`, file-backed `SecretSource`, `/v1/health/live`, and `/v1/health/ready`.

- [x] **Step 1: Add failing static deployment tests**

Parse the Compose YAML and assert: one hosted replica; no PostgreSQL published port; read-only service root; `no-new-privileges`; dropped capabilities; bounded memory/CPU; health checks; named PostgreSQL volume; file-backed database and OIDC secrets; no literal secret values; and no Cloudflare Access configuration.

- [x] **Step 2: Run the test and confirm the production bundle is absent**

Run: `go test ./internal/aphelion/collab/container -run TestProductionCompose -count=1`

- [x] **Step 3: Create the vendor-neutral base and overlays**

The base contains PostgreSQL and the hosted service on an internal network. The Cloudflare overlay adds `cloudflared tunnel --no-autoupdate run --token-file /run/secrets/cloudflare_tunnel_token`. The loopback overlay publishes only `127.0.0.1:8080:8080` for an operator-managed reverse proxy.

- [x] **Step 4: Add the operator command wrapper**

`operations.ps1` exposes fixed `Validate`, `Build`, `StartCloudflare`, `StartLoopback`, `Status`, `Logs`, `Backup`, `Restore`, `Upgrade`, `Rollback`, and `Stop` actions. It invokes Docker directly without dynamic shell evaluation, validates the repository-relative deployment root, and refuses destructive restore unless the service is stopped and the exact backup file exists.

- [x] **Step 5: Document first-run and ongoing operation**

Include secret generation, OIDC registration, health verification, backup retention, restore rehearsal, image pinning, stop-then-start upgrades, rollback, log collection, and self-hosted DNS/TLS alternatives.

- [x] **Step 6: Validate Compose and tests**

Run: `docker compose -f deploy/production/compose.yaml -f deploy/production/compose.cloudflare.yaml config`

Run: `docker compose -f deploy/production/compose.yaml -f deploy/production/compose.loopback.yaml config`

Run: `go test ./internal/aphelion/collab/container -count=1`

### Task 5: Reference Deployment and Cloudflare Routing

**Files:**
- Modify: `docs/testing/multiplayer-online-pilot-guide.md`
- Create: `docs/verification/phase-7-public-hosting-2026-08-26.md`

**Interfaces:**
- Produces: live evidence for `https://mapping.a13.info`.
- Consumes: the production Compose bundle and dedicated remotely managed Cloudflare Tunnel.

- [ ] **Step 1: Inventory the Meridian server after Cloudflare Access login**

Record OS, Docker Engine and Compose versions, CPU, memory, free storage, existing container networks, listening ports, time synchronization, backup destination, and service manager. Do not record secrets.

- [ ] **Step 2: Prepare the server without public routing**

Install the Compose project in an isolated directory, create ACL-restricted secret files, configure `public_origin: https://mapping.a13.info`, register the exact OIDC callback, build the image, and require local readiness before creating DNS.

- [ ] **Step 3: Provision the dedicated tunnel and public hostname**

Create a new remotely managed tunnel, install its token only on the Meridian server, configure public hostname `mapping.a13.info` to `http://hosted:8080`, and create the proxied tunnel CNAME. Do not change `bark`.

- [ ] **Step 4: Exercise the public entry points**

Verify live/ready/version endpoints, browser OIDC sign-in, authenticated session creation, one-use invitation redemption, two StrongDMM clients, WSS reconnect, and hosted-service restart recovery.

- [ ] **Step 5: Run hosted load, fault, backup, restore, and rollback gates**

Run the 25-editor profile with hosted credentials. Record p50/p95/p99 acknowledgements, convergence, connection churn, PostgreSQL health, slow-consumer behavior, presence pressure, database interruption, backup restore, and rollback.

- [ ] **Step 6: Update the pilot guide and evidence ledger**

Record exact artifact hashes, image identifier, configuration hash with secrets excluded, timestamps, commands, outcomes, exceptions, and the final named-user pilot decision.

### Task 6: Repository Verification

**Files:**
- Modify: `docs/superpowers/plans/2026-08-26-final-multiplayer-progression-sheet.md`

**Interfaces:**
- Produces: final current-status accounting.
- Consumes: evidence from Tasks 1 through 5.

- [x] **Step 1: Run focused packages**

Run: `go test ./internal/aphelion/collab/ui ./internal/aphelion/collab/load ./internal/aphelion/collab/server ./internal/aphelion/collab/container ./cmd/apheliondmm-loadtest -count=1`

- [x] **Step 2: Run repository and race gates**

Run: `go test ./... -count=1`

Run: `go test -race ./internal/aphelion/... -count=1`

- [x] **Step 3: Build the shipped Windows entry point**

Run: `$env:RUST_TARGET = '1.82-x86_64-pc-windows-gnu'; task task_win:gen_syso; task build`

Record `$LASTEXITCODE`, the produced `dst/StrongDMM.exe` hash, and any inherited warnings.

- [x] **Step 4: Reconcile the progression sheet**

Mark only evidence-backed gates complete. Keep external OIDC certification, named-user testing, updater signing ownership, and multi-replica fanout visible when they remain outstanding.
