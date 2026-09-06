# Canvas resize performance and application paste routing

Baseline: `052e1acb02b790641d63466de40c91d56028383b`, with prior uncommitted
repairs preserved. This pass qualifies the application paste dispatcher and
repairs the canvas resize allocation identified in the repo-wide audit.

## Canvas finding and repair

`Canvas.createCanvasTexture` allocated a float32 array of `width * height`
elements and uploaded it as RGB/UNSIGNED_BYTE pixels on every size change.
`Canvas.Process` immediately clears the entire framebuffer before drawing or
exposing the texture. The CPU allocation/upload therefore supplied pixels that
the next clear overwrote. The source change replaces that upload pointer with
nil; format, size, filtering, attachment, clear and draw order remain unchanged.
The original inherited line is retained in its Aphelion provenance marker.

OpenGL 3.3 permits allocation with a null data pointer when no pixel unpack buffer
is bound. Contents are unspecified until written, making the subsequent full
clear essential. The editor source has no unpack-buffer binding, and its ImGui
backend restores scissor state. These existing pipeline assumptions are preserved.
See [OpenGL 3.3 Core specification, section 3.8.3, page 151](https://registry.khronos.org/OpenGL/specs/gl/glspec33.core.pdf#page=166).

The hidden framebuffer test exercises actual Canvas.Process, readback and brush
primitive drawing at first allocation, odd sizes, grow/shrink/revisit, and
unchanged dimensions. Every pixel is checked against expected opaque magenta or
a green rectangle, including clearing the rectangle on the next frame. All 16
pixel results match the preserved control, with complete framebuffers and no GL
errors. The allocation red test measured 1,232,921 B per resize around 640x480;
the candidate measured zero. All ten sampled iterations force a size change.

## Matched performance results

Artifacts: `.artifacts/canvas-resize-audit-2026-09-06/`.
Five alternating control/candidate trials per size, 50 measured resizes per
sample, 20 samples total. Width and height alternate between the named size and
size+1, ensuring every timed call resizes. Each Canvas.Process is followed by
`gl.Finish`, so times include GPU completion. Initialization, warmup and final
pixel readback/hash are outside the measured interval. Both variants use the
same blank canvas, clear color, format, dimensions and assertions. Every trial
also validates every final pixel, including the large 1921x1081 target.

| Base size | Control median ms (range) | Candidate median ms (range) | Median B/op, control -> candidate | Allocs/op |
| --- | --- | --- | --- | --- |
| 640x480 | 5.624 (5.577-6.013) | 2.005 (1.919-2.088) | 1,233,331 -> 0 | 2 -> 0 |
| 1920x1080 | 22.413 (21.634-23.504) | 4.699 (4.515-4.744) | 8,302,825 -> 0 | 2 -> 0 |

Every candidate sample is faster than every control sample for its size in this
run. Final pixel hashes match all ten samples per size:

- 641x481: `c78dc4ba7749752fd730d7c777d006e898f4026d3a7a7df910e50e6a5f435d8c`.
- 1921x1081: `5f23902ab8e70db8df22eb4c8a1a9dd33e5798fdb888db6a8a4ca4850d8873a2`.

Renderer: NVIDIA GeForce GTX 1650, OpenGL 3.3.0, NVIDIA 591.86. These are
synchronized blank-canvas resize results on one machine. They do not establish
general frame-rate, real-map input latency, startup performance, GPU-memory
reduction or service-capacity gains. GPU storage dimensions/format are unchanged.
No compilation or other agent-driven test workload ran during the comparisons;
normal desktop activity and power/driver scheduling were not controlled.

`control.exe` was built before production changes. Its source is preserved in
`control-canvas.go`. After adding final-pixel validation outside benchmark timing,
`control-bench.exe` was rebuilt through Go's relative-path overlay using that
preserved source; `candidate-bench.exe` uses current source and identical test
instrumentation. No checkout/reset was used. `run-trials.ps1`, raw logs,
`trials.csv` and `summary.json` reproduce and retain the comparison.

## Application paste qualification

`internal/app/paste_action_test.go` now exercises the real Menu shortcut registry,
`app.DoCopy`, `app.DoPaste`, active-workspace resolution and rendered WsArea focus
transitions. It verifies left/right Ctrl+V starts an uncommitted placement,
Enter creates exactly one revision, switching to a second workspace cancels the
first preview, subsequent paste targets the second map, and Escape leaves that
map at revision zero with no history. Premature Save is refused and the error
sink remains empty.

The fixture uses a real hidden OpenGL/ImGui context, real editor/model/clipboard
and local engine. Unrelated panels, configuration persistence and native window
callbacks use in-memory adapters. Keys enter ImGui directly; this is not physical
OS event injection or the complete Window.Process loop. The earlier tool-frame
click tests and human acceptance remain separate evidence. This closes the
previously unrun application shortcut-to-workspace dispatcher gate.

## Qualification and provenance

| Gate | Result |
| --- | --- |
| Application routing | Focused hidden run passes; full application package under race passes in 6.541 s. |
| Workspace regression | All 36 top-level workspace tests pass under race in 16.844 s, with no workspace test skipped. |
| Pane/editor/settings | Race suites pass: pane 5.470 s, editor 6.484 s, settings 5.425 s. Overlay/quick-edit/tile-menu packages have no tests. |
| Canvas | Hidden pixel/allocation test passes under race; final formatted source passes in 1.854 s. |
| Repository | `task verify` passes lint (zero issues), contracts, all Go tests, Rust test/fmt/clippy, parser release and desktop build. Rust currently has zero unit tests. Default Go runs skip explicitly gated GL tests; the separate hidden runs above provide that evidence. |
| Final instrumentation | Final benchmark pixel assertions are exercised in every trial; final linter reports zero issues. |
| Desktop and smoke | Final desktop rebuild passes; produced parser/loopback smoke passes open/save/reparse, two authenticated clients, equal hashes at revisions 1/2, leave and shutdown. Smoke does not exercise desktop UI. |

Only benchmark pixel validation and source line-ending normalization followed the
repository gate; affected canvas race, linter and final desktop checks cover
those final changes. The final production canvas source equals the preserved
benchmark candidate source after CR normalization. Desktop SHA-256 is unchanged
by that normalization. The inherited ImGui C++ `memset` warning remains.

SHA-256:

- Final desktop: `ba4c96e5260d95bf5d8a85f8ed8c484b3cafb9857af4048b1bdbb5f5e8bb0cc2`.
- Original control: `d3f8392f02a6a8497ef22c7458265f9e9434ff1e135b7441731df0e7e670cb62`.
- Instrumented control: `f5133fbedd0c49e13ef5147a8e8d507905ed8a76f4b0a44ca0e0021a9c29402a`.
- Instrumented candidate: `cfdae015894a043295fcc8269a728c492e894ba198320095119cbff0c8417532`.
- Smoke: `f6cded859990696720d07db096479377abcabd6d88bd3f56bf07d54f72b37dca`.

Toolchains remain Go 1.25.13 windows/amd64 and Rust 1.82.0 GNU. The manifest
records source, document, binary and evidence hashes with relative paths. Changes
are uncommitted; protected infrastructure, dependencies and assets are unchanged.

## Next work and reproduction

Restart the editor with rebuilt `dst/StrongDMM.exe` for human acceptance. Continue
real-map resize/frame timing, idle/minimized behavior, invalid/zero dimensions,
sprite/cache and repeated lifecycle resource measurements. Continue paste's
remaining all-level/fault/attachment interactions and floating-template
transforms, then configurable bindings/grid steps and other editor QoL tasks.
Keep the entire repo-wide performance matrix, real PostgreSQL, hosted CI,
Meridian integration and human acceptance open as separately measured gates.

From the repository root with the pinned toolchains, check each native exit code:

```powershell
$env:APHELIONDMM_GL_TEST = '1'
go test -race ./internal/app ./internal/app/ui/cpwsarea/wsmap ./internal/app/ui/cpwsarea/wsmap/pmap/... -count=1 -timeout 120s
& ./.artifacts/canvas-resize-audit-2026-09-06/run-trials.ps1
Remove-Item Env:APHELIONDMM_GL_TEST
$env:RUST_TARGET = '1.82.0-x86_64-pc-windows-gnu'
task verify
```
