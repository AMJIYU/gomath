# Exact-key bucket targeted counterexample review

2026-11-05. Completed. No reproducible production bug found; no failing regression remains. Severity: none. No production fix recommended on the evidence reviewed.

Only production sources read: matching.go, animation.go, scene.go, succession.go, replay.go, mathtex/matching.go. Related centralized tests and the existing reference generator were inspected. Production files were not edited; no commits, external runtimes, whole-repository suites or race suites were used. The existing 49/63 reference states were neither duplicated nor regenerated.

## Added coverage

Source: tests/_cases/root/matching_review_test.go.

| Test | Targeted counterexample / result |
| --- | --- |
| TestMatchingReviewParallelNestedBuckets | Two matching replacements at different tree depths plus an ordinary sibling track. Checks unique IDs across the whole frame, original source descendants remaining findable, target IDs absent until cleanup, actual target/sibling pointers and tree slots, serialized source preservation, independent sampling and stable replay topology. Passed. |
| TestMatchingReviewSuccessionReintroducesSource | Matching, FadeIn of the removed source, parallel edits through original source/target child handles, then a second matching of the reused source. Checks boundaries and the first segment's original captured geometry. Passed. |
| TestMatchingReviewManagedBucketReplay | Source root and child managed callbacks with automatic suspension both enabled and disabled, active parent callback, target callback after cleanup, source re-addition, frozen replay after callback removal and authoring edits. Passed. |
| TestMatchingReviewPaintOrder | Hand-derived RGB composition of overlapping matched repeated buckets, outgoing green and incoming white at three interior times. This tests layer order, unlike the existing reference's drawable multiset comparison. Passed. |
| TestMatchingReviewFailureAtomicity | Invalid window, non-finite pathArc axis, invalid endpoint rate and late Succession rate failure. Checks sibling/source/target geometry, topology and existing timeline are preserved. Passed. |
| TestMatchingReviewInteriorRateFailureIsIsolated | Endpoint-valid rate becomes NaN, greater than one or negative only in the interior, with a prior ordinary sibling track and curved matching trajectory. Scene and replay both return an error with no partial frame and retain valid earlier samples. Passed. |
| TestMatchingReviewRepeatDistribution | 2-to-3 repeated bucket, unlike pinned 1-to-N/N-to-1 cases: original IDs occupy positions zero and two; transparent copy occupies position one. Independently checks centers and opacity. Passed. |
| TestMatchingReviewRepeatedCurveBudget | 131 input nodes; 516 source curves repeated 128 times exceed the curve budget while node count remains small. Constructor accepts, Play rejects at the aggregate budget and preserves inputs/timeline. Complements the existing node-expansion test. Passed. |

Budget assessment combines this rejection test with inspection of repeatMatchingBucket: a repeated child's full curve/node/contour charge precedes that child's Copy loop. This is not a claim that the entire operation makes zero allocations before discovering a later aggregate error, nor a heap/latency benchmark.

## Reproduction

```sh
CGO_ENABLED=0 go run -tags noasm ./tests/cmd/run -pkg=root -pkg=mathtex -run '^Test(MatchingReview|MatchingParts|MatchingExplicitMathParts)' -count=1 -v
```

Go toolchain: go1.27.3 darwin/arm64. All eight new tests, the three existing non-reference root matching tests and the explicit mathtex matching test passed. Full output: focused-tests.txt. No additional suite is required by this review.

An initial candidate assertion that a zero pathArc axis must fail was discarded: zero is accepted, so the committed test uses a non-finite axis. This was a test-assumption correction, not a production finding.

## Reviewed source fingerprints

SHA-256 fingerprints were unchanged between initial review and the final focused run:

```text
44d56da4b2458c6f1457423fda7b339dffedaac80040f7f66ebb231311a55adf  matching.go
1279e97cc661ae69194737b91fd2c72d9dd334fa348f4dbdcbaf61b94867f1e4  animation.go
7acb836a2e4e4a66502149a7cf25fe8d77a20df80d3feae3a2d933031b1d80a0  scene.go
0300f0b2dd2138325827af3bf3312af52c22025f60f544c5708a7c64dd27354b  succession.go
ad750d79b479412b8c7483c19273db4669ddf0c1f6abb3c8e1804d9698f4df7d  replay.go
b8e5d8152f3de34da9db2d391783797e25a32e330e71f7bf9ace4e0dc5e146ff  mathtex/matching.go
```

This conclusion applies to these source fingerprints and the stated scenarios; it is not an all-input or whole-library correctness claim.
