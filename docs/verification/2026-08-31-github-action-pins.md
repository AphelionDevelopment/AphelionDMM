# GitHub Action pin review

Date: 2026-08-31

Each tag or maintained major-version ref was resolved through GitHub's Git data API. Annotated tags, if returned, were dereferenced to their commit object. The workflow retains the reviewed human-readable release beside the immutable commit SHA.

| Action | Reviewed release/ref | Commit SHA |
| --- | --- | --- |
| `actions/checkout` | `v6` | `d23441a48e516b6c34aea4fa41551a30e30af803` |
| `actions/setup-go` | `v6` | `924ae3a1cded613372ab5595356fb5720e22ba16` |
| `golangci/golangci-lint-action` | `v9` | `ba0d7d2ec06a0ea1cb5fa41b2e4a3ab91d21278a` |
| `arduino/setup-task` | `v2.0.0` | `b91d5d2c96a56797b48ac1e0e89220bf64044611` |
| `Swatinem/rust-cache` | `v2` | `6323deb102c322ba6fcbdcafc7e3dddab59af2b6` |
| `actions/upload-artifact` | `v6` | `b7c566a772e6b6bfb58ed0dc250532a479d7789f` |
| `aquasecurity/trivy-action` | `v0.36.0` | `ed142fd0673e97e23eac54620cfb913e5ce36c25` |
| `actions/download-artifact` | `v7` | `37930b1c2abaa49bbe596cd826c3c89aef350131` |
| `actions/attest` | `v4` | `1e69f48acb82d1966a394da916b4c1698aa569d6` |
| `softprops/action-gh-release` | `v2` | `3bb12739c298aeb8a4eeaf626c5b8d85266b0e65` |

`arduino/setup-task@v2` is a maintained branch rather than a tag. Its head matched the immutable `v2.0.0` tag commit, so the workflow comment records `v2.0.0` rather than the moving branch name.
