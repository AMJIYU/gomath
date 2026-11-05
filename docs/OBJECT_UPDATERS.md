# Managed object updaters

`Scene.AddObjectUpdater` registers a bounded, absolute-time callback on an
object already owned by the scene. It returns an opaque `*UpdaterHandle`.
Unlike the legacy global `AddUpdater`/`AddRedraw` APIs, the registration starts
at the current authoring cursor and supports scheduled control:

```go
options := gomath.DefaultObjectUpdaterOptions()
handle, err := scene.AddObjectUpdater(dot, func(o *gomath.Object, frame *gomath.Frame, t float64) error {
    o.MoveTo(geom.V(math.Cos(t), math.Sin(t)))
    return nil
}, options)
if err != nil { return err }
if err := scene.Wait(1); err != nil { return err }
if err := scene.SuspendUpdater(handle); err != nil { return err }
if err := scene.Wait(1); err != nil { return err }
if err := scene.ResumeUpdater(handle); err != nil { return err }
if err := scene.Wait(1); err != nil { return err }
if err := scene.RemoveUpdater(handle); err != nil { return err }
```

Controls change activity from the exact current cursor onward. At one cursor,
the last control wins without consuming another event. Suspend/resume are
idempotent while live; removal is permanent and later controls on the handle
return errors. A removed registration is retained for historical sampling.
Unknown, zero, nil, copied and foreign-scene handles reject.

`ClearObjectUpdaters(object, recursive)` permanently removes managed callbacks
on a currently owned object, optionally including its descendants. The entire
batch is preflighted against event capacity before changing the schedule.
Legacy global callbacks are independent and are not removed by this method.

`AddObjectRedraw(object, RedrawFunc, options)` provides the same lifecycle for a
fixed-topology redraw. It validates generated geometry before copying it and
retains sampled object/child identities. It does not transfer generated object
IDs or share mutable path data. Dynamic display topology still uses the
separate legacy `AddDynamicRedraw` contract.

## Sampling and ownership

Callbacks run after animation tracks, camera sampling and replay integration,
in registration order shared with legacy frame callbacks. A later callback
reads the private frame modified by earlier callbacks. This is registration
order, not Manim's parent-before-descendant traversal order.

The callback may edit only its supplied private target family. `Frame` is for
reading other sampled objects/trackers. IDs, child pointers and child order must
remain fixed for that sample. Paths, valid styles and scalar values may change.
Callbacks must be pure with respect to captured state, deterministic, concurrent
safe and returning; external side effects cannot be rolled back. Errors and
panics (including panic(nil)) return a sample error without publishing a frame.
They cannot mutate authoring paths or other independent samples through the
provided target. Invalid ownership, cycles, excessive geometry and invalid
visual output reject before recursive copying/rendering.

A missing object is skipped. Replacement does not transfer the callback to its
new ID. If the same original object is later re-added, its still-live
registration can apply again. Earlier dynamic redraw callbacks may provide a
different private family; the managed updater binds the resulting root ID and
must preserve that sample's family.

No updater executes during authoring. `Play` captures authored snapshots, so
sampled updates do not become the next animation's initial state. This is a
material difference from a sequentially mutated Manim Scene. Use trackers and
explicit authored targets, or fixed-step `CompileReplay` for integrated state.

## Automatic animation suspension

`DefaultObjectUpdaterOptions()` enables `SuspendDuringAnimations`. The all-zero
options value disables this flag. Automatic suspension applies when the bound
object is itself animated or is inside an animated source family. An animation
on a child does not suspend an updater bound to its parent. A separately owned
Transform target is read-only and is not suspended merely because it supplies
a morph destination.

Each Play segment owns its animated families for its complete half-open time
interval, including explicit animation windows and flattened group delays.
At the exact endpoint normal updater evaluation resumes. Succession already
produces separate step segments, so suspension follows the current step.
Manual suspension/removal remains authoritative across animation endpoints.
This deliberately avoids trying to infer suspension by calling user rate
functions an additional time. General nested AnimationGroup begin/finish timing,
recursive manual suspension flags, automatic `dt` callbacks and full Manim
updater ordering are not implemented by this API.

## Bounds and frozen replay

A scene retains at most4096 managed registrations and65536 timestamped events;
removed registrations still count. Zero family limits use4096 objects,
262144 curves,16384 contours and depth64. `DynamicRedrawOptions` supplies the
same lowering/raising policy and hard maxima as dynamic redraw. Managed
execution also preflights the entire frame against65536 objects/contours,
1048576 curves and depth256, and bounds animation-family suspension search to
1048576 visited nodes. These bounds concern scene geometry/scheduling; bitmap
and immutable paint payloads retain their existing independent limits.

`CompileReplay` deep-copies activity histories and callback order. Later
same-cursor control edits or additional authoring cannot alter the frozen
schedule. Immutable callback closures remain shared and must obey the same
concurrency contract. Arbitrary repeated and out-of-order samples remain
independent.

## Evidence and limits

`tests/reference/updater_lifecycle.py` reconstructs13 cases independently using
Manim0.22.1 at commit16ea443e2a5f1d79db0a721161411ce916d6e733. It invokes real
`Animation.begin/interpolate/finish`, recursive object update, manual suspension
and removal.91 complete ordered cubic states include animation interiors,
parent/child interactions, two ordered callbacks, disabled automatic suspension
and a separately owned read-only morph target. The external absolute clock is
explicit; this is an object-update/suspension unit oracle, not a sequential
Manim Scene/dt lifecycle equivalence claim.

Central Go-only tests additionally cover cursor histories, same-cursor edits,
handle ownership, event/registration caps, atomic recursive clear, topology/ID
rejection, panic/error behavior, legacy ordering, fixed redraw ownership,
replacement/re-add, Succession, frozen replay and concurrent sampling.

## Performance (batch 9)

[summary.json](../tests/data/root/updater-lifecycle/performance/summary.json)
records all three samples, per-metric medians/ranges, formulas and SHA-256
hashes for the selected logs and source evidence. Only `before.txt` and
`after-final.txt` enter these statistics; `after.txt` remains historical.

`BenchmarkObjectUpdaterFrame` uses 1/32/128 square children in one Group
(N+1 tree objects), with one managed updater per child. One operation calls
`Scene.Sample(0.75)` once after `Wait(2)`; construction, registration and
authoring precede `ResetTimer`. Sampling/snapshots and updater checks are
included; rendering, I/O, encoding and replay are excluded. No image size or
FPS applies. The specified protocol is three repetitions at a 200ms target,
using an unweighted median per metric. Three rows per case are verified;
the historical command and 200ms flag are not recorded in the logs.

| Updater objects | ns/op median, before → after | Observed ratio | B/op median, before → after | allocs/op median, before → after |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 2262 → 1293 | 1.75× | 1488 → 2144 | 10 → 20 |
| 32 | 340718 → 31891 | 10.68× | 375410 → 51608 | 1419 → 241 |
| 128 | 5047866 → 131900 | 38.27× | 6089300 → 208857 | 8207 → 835 |

Both logs report Apple M2, darwin/arm64 and a `-8` benchmark suffix
(GOMAXPROCS, not parallel workers). Historical toolchain, OS version, RAM,
CGO/build flags and host load are **unknown**. Summary-time `go version`
reports `go1.27.3 darwin/arm64`; current `go.mod` declares `go 1.26.0`.
Neither establishes the compiler used for the historical runs.

This compares this batch's naive per-callback frame traversal with its batch
index/validation implementation; it is not a Manim performance comparison.
The shared-host phases were non-interleaved (task-provided context), so ratios
are descriptive and susceptible to load/thermal drift. Three repetitions
do not establish significance. The 1-object case regresses by **656 B/op
(+44.09%) and 10 allocs/op (+100%)**, despite lower median time. Current source
hashes identify the summary-time tree; the logs do not pin the exact historical
after source/toolchain. No render/export or universal speedup claim is implied.
