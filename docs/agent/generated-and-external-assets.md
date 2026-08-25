# Generated content and external assets

## Generated files

Generated files must identify their source, generator command, generator version, and verification command. A generated file is never the sole authority for a protocol or schema.

For collaboration contracts:

- `api/collaboration/openapi.yaml` and `api/collaboration/asyncapi.yaml` are reviewed source contracts.
- Generated Go clients, servers, or models live in an explicitly named generated package.
- Generated files contain a standard generated-code header and are not hand-edited.
- Contract generation must be deterministic and a clean regeneration must produce no diff.
- Wire compatibility fixtures are checked in independently of generated code.

Do not regenerate vendored Rust parser output with an unpinned toolchain and present it as reproducible.

## Human-authored creative material

Agents do not create, rename, select, or revise:

- branding and logos;
- art, icons, sprites, or screenshots;
- sound or music;
- lore, prose descriptions, or item names;
- attribution wording that requires creative or legal judgment.

Agents may implement loading, validation, packaging, accessibility, and attribution infrastructure around human-approved assets.

## External assets

Before adding an external asset, record:

- source URL;
- author or rights holder;
- exact license and version;
- required attribution;
- local file and transformation history;
- approving human and approval date.

Do not add an asset with unknown provenance or ambiguous redistribution rights. Do not replace existing StrongDMM assets during multiplayer work. Branding and asset replacement is a separate human-approved project.

## Repository images

Existing files such as `docs/sdmm-logo.png` and `docs/sdmm-example.png` remain inherited material. Their presence does not authorize reuse as new Aphelion branding. Preserve them unless the user explicitly approves a documented migration.

