# Canvas resize allocation audit and repair

Goal: remove the CPU zero-pixel upload immediately overwritten by canvas Clear,
while preserving texture format/filtering, complete framebuffer pixels and the
existing renderer. Work inline in the current dirty checkout, uncommitted.

Source: `internal/app/ui/cpwsarea/wsmap/pmap/canvas/canvas.go` allocates
`4 * width * height` bytes as float32 storage for an RGB/UNSIGNED_BYTE upload on
every changed size. `Canvas.Process` immediately clears that framebuffer before
drawing. OpenGL permits nil texture data, with undefined contents until written;
the existing full clear must therefore remain before rendering/sampling.
The shipped ImGui backend restores its scissor state; source has no pixel unpack
buffer binding. Preserve these existing GL-state assumptions explicitly.

- [x] Add real hidden-context framebuffer tests for first use, odd sizes,
  grow/shrink/revisit, clear and colored primitive output; record pixel hashes.
- [x] Measure changed-size CPU allocations through actual Canvas.Process. Build
  and preserve a control benchmark binary before changing production code.
- [x] Replace only the upload data pointer with nil, retaining the exact inherited
  line in an Aphelion provenance marker. No wrapper can remove an allocation made
  inside the inherited private texture-allocation method.
- [x] Run matching pixel/GL-error and allocation tests. Preserve a candidate
  binary; run five alternating control/candidate trials at 640x480 and 1920x1080
  (alternate each size with width+1/height+1 so every iteration resizes).
- [x] Keep GPU completion fencing, sizes, clear color and iterations identical.
  Report CPU allocation and synchronized blank-canvas resize latency separately
  from representative map/UI/GPU performance. Record GL renderer/version.
- [x] Run affected race, real application paste/workspace tests, repository gate
  and desktop/parser smoke. Update performance/feature plans and verification.

Results: [canvas resize and application routing](../../verification/2026-09-06-canvas-resize-and-application-routing.md).

New tests live beside the canvas and application code. Protected infrastructure,
protocol and assets are unchanged. Minimized/invalid dimensions, realistic sprite
loads, startup/reload, render resource lifetime and general frame pacing remain
separate entries in the repo-wide performance plan.
