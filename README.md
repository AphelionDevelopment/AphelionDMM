# AphelionDMM

AphelionDMM is a downstream fork of StrongDMM focused on deterministic editing and multiplayer collaboration.

## Project identity

This fork does not claim to be an independently originated map editor, and it does not seek to replace StrongDMM's identity with a major identity of its own. The editor, its core workflows, and most of the foundation on which AphelionDMM is built exist because human authors and developers created, maintained, reviewed, documented, and supported StrongDMM and its dependencies.

Anyone interested in this project should first visit the upstream project, learn from its documentation, and support its authors and contributors. AphelionDMM should be understood as downstream engineering built on that work. Fork-specific bug reports and multiplayer work belong here; improvements that apply generally to StrongDMM should be considered for upstream contribution.

## Upstream project and support

The upstream, author, support, documentation, and inherited-project links are intentionally kept together here:

- **StrongDMM:** [source repository](https://github.com/SpaiR/StrongDMM), [documentation and usage guide](https://github.com/SpaiR/StrongDMM/blob/main/README.md), [releases and verified downloads](https://github.com/SpaiR/StrongDMM/releases), [issue tracker](https://github.com/SpaiR/StrongDMM/issues), [pull requests](https://github.com/SpaiR/StrongDMM/pulls), [build pipeline](https://github.com/SpaiR/StrongDMM/actions/workflows/ci.yml), and [contributors](https://github.com/SpaiR/StrongDMM/graphs/contributors).
- **Original author and direct support:** [SpaiR on GitHub](https://github.com/SpaiR), [support StrongDMM through Ko-fi](https://ko-fi.com/P5P5BF17Q), or use the [public contact address](mailto:despsolver@gmail.com) published by the upstream project.
- **Inherited parser work:** [SpacemanDMM](https://github.com/SpaceManiac/SpacemanDMM), created by [SpaceManiac](https://github.com/SpaceManiac).
- **Inherited application icon:** designed by [Clément "Topy"](https://github.com/clement-or).

For ordinary map-editor features, installation-free usage, CLI examples, keyboard and mouse controls, FAQ answers, platform prerequisites, and the original build explanation, use the StrongDMM documentation linked above. Those instructions remain the baseline unless this README or the fork documentation explicitly describes a divergence.

## How this fork differs

AphelionDMM preserves the inherited StrongDMM editor, DMM/TGM handling, Dear ImGui desktop interface, search and variable-editing tools, screenshots, layers, shortcuts, CLI opening, and vendored Rust parser. Its principal downstream changes are:

- **Deterministic map operations.** Local and collaborative edits pass through a shared operation/executor layer with stable revisions, map hashes, conflict behavior, actor-aware inverse operations, and atomic save paths.
- **Desktop collaboration.** The editor adds local and online session controls, owner/editor/viewer roles, invitations, display profiles, presence, conflicts, undo/redo integration, explicit reconnect, and local collaboration status.
- **Client-owned online sessions.** The current protocol-v2 design keeps durable map and operation state on participating clients. The session owner orders accepted work while a stateless WebSocket relay routes bounded, signed, end-to-end encrypted frames.
- **Local durability and recovery.** SQLite-backed client state, installation identities, replay/snapshot reconciliation, revision/hash checks, and owner-first relay recovery protect acknowledged work without requiring a server-side map database.
- **Self-hostable relay operations.** The public relay is configurable, and a native Windows service package installs and supervises a dedicated Cloudflare connector without requiring a container runtime or modifying another application's tunnel.
- **Versioned integration contracts.** Staged-map manifests and verification tooling provide narrow integration boundaries for Meridian-Rift and Meridian-MCP without making those projects part of the collaboration transport.
- **Expanded engineering gates.** The fork adds protocol compatibility fixtures, fuzzing, race and fault tests, load scenarios, privacy checks, hosted-operation evidence, security guidance, and human pilot procedures.

The older server-authoritative protocol-v1 implementation and its PostgreSQL/OIDC deployment material remain in the repository as a labeled compatibility path. They are not the target architecture for protocol-v2 collaboration.

## Current status

AphelionDMM multiplayer is under active development and testing. Local automated gates and same-machine collaboration exercises do not constitute public acceptance. The remaining acceptance path is recorded in the [multiplayer progression sheet](docs/superpowers/plans/2026-08-26-final-multiplayer-progression-sheet.md), and testers should use the [client-owned relay human test guide](docs/testing/client-owned-relay-human-test-guide.md).

Notable current boundaries:

- the default public relay is intended to be `https://mapping.a13.info`, but clients may configure another compatible HTTPS relay;
- collaboration pauses when the owner is unavailable;
- relay restarts recover from client state and require owner-first reconnection;
- automatic reconnect is not claimed for the first pilot;
- protocol support for owner transfer exists, but the complete desktop transfer workflow remains deferred from first-pilot acceptance;
- ordinary single-user editing remains supported and must not require a network service.

## Using the editor

AphelionDMM continues to build the desktop executable as `StrongDMM.exe` pending any separately approved branding change. The inherited editor remains installation-free. For general editing and CLI usage, follow the upstream documentation collected under [Upstream project and support](#upstream-project-and-support).

Fork-specific collaboration testing begins with:

- [Client-owned relay human test guide](docs/testing/client-owned-relay-human-test-guide.md)
- [Multiplayer human test guide](docs/testing/multiplayer-human-test-guide.md)
- [Windows relay operator guide](deploy/relay/README.md)

Invitations and collaboration logs may contain sensitive session information. Do not publish invitation links, keys, client databases, or unredacted logs.

## Building this fork

The upstream build documentation explains the inherited Go, Rust, CGO, and platform dependencies. This fork pins its authoritative versions in `go.mod`, the Rust toolchain invocations in `Taskfile.yml`, and CI.

Current Windows requirements include:

- Go 1.25.13;
- Rust 1.82.0 with the `x86_64-pc-windows-gnu` toolchain;
- MinGW-w64 for CGO;
- Task 3.x;
- PowerShell for the relay packaging and service operator path.

From the repository root:

```powershell
task build
task test-go
task test-rust
task verify-relay
```

`task build` writes the editor to `dst/StrongDMM.exe`. To create the self-contained Windows relay-server ZIP:

```powershell
task package-relay-windows
```

The package is written below `dst/relay-package/`. A server installing that ZIP does not need Go, Rust, Git, the source repository, or a container runtime.

## Fork documentation

- [Documentation index](docs/index.md)
- [Agent and contributor guidance](docs/agent/README.md)
- [Current architecture](docs/agent/architecture.md)
- [Security and networking rules](docs/agent/security-and-networking.md)
- [Verification requirements](docs/agent/verification.md)
- [Upstream reconciliation policy](docs/agent/upstream-drift.md)
- [Relay hosting handoff](docs/hosting/game-server-deployment-agent-handoff.md)

## License

AphelionDMM remains distributed under the inherited GPL-3.0 license. See [LICENSE](LICENSE) for the applicable rights and obligations. Copyright and attribution remain with their respective authors and contributors.
