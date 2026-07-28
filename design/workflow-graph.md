# The C4 workflow graph — evidence, economics, and the language

Status: design note (2026-07-28). Supersedes the `@c4 graph 1` draft. Derived from `design/listings.md`, `design/derivation-records.md`, `design/receipt-record.md` (all three currently exist only on `master`; the working tree is `release/1.0.x`), plus `design/substrate-map.md`, `design/testimony-record.md`, `design/store-records-architecture.md`, `c4m/SPECIFICATION.md`.

Nothing here is built. Nothing here is scheduled. Section 7 states what must be settled before it could be.

---

# 1. What actually happened

## 1.1 The project

A 60-second window of a multi-camera interview (Eric Thomas, five feeds, jam-synced) had frame drops scattered across every camera. The job: recover a continuous, deliverable 60 s of picture by cutting between cameras where possible and generatively filling the residual gaps — then decide whether the method scales to a ~90-minute runtime at 4K ProRes 422 HQ delivery.

2026-06-18 through 07-04. Eleven merged session records, ten distinctly named (`main-a`, `main-b`, `w-a119`, `w-dbe3`, `w-d595`, `w-5a7d`, `w-9ee9`, `w-9a6f`, `w-c76c`, `a-8d6b`). Nine had 06-19 activity; the timestamps support at most four simultaneous. Rented GPU: RunPod A100-PCIe/SXM and H100. ~38.1 pod-hours and ~$81 through 06-25; spend after 06-25 (three days of H100 at $3.29/hr) was never tabulated.

It shipped two files — `eric_60s_edit.mp4` and `eric_60s_NOPOP.mp4`, an 11-shot hard-cut assembly at 1080p/h264 crf-18. It also produced, on 06-28, a *different* locked recipe (sparse 2-endpoint anchoring, composite over the byte-real 4K plate, lossless ProRes at 24000/1001) which **nothing has ever been built to**. Both facts are true; any sentence about "the final pipeline" has to say which one it means.

## 1.2 Narrative

**Ingest and the false index (06-18 → 06-19 03:00).** Materials arrived as camera cards readable over ranged HTTP. The `moov` tables were fetched and cached as `work/CAM*_frames.json`. An early hypothesis — that the drops were *corrupt* frames rather than *missing* ones — cost an hour and one montage before being correctly retired. A better challenge came from Joshua at 02:46: might the index be under-reporting frames that were physically written? A raw `mdat` `icpf` scan over the six longest gaps found the control run at 8/8 predicted offsets and zero hidden frames, removing "faulty index" from the risk list permanently.

Before that, audio cross-correlation with a ±10 s search window had produced a phantom 6.05 s all-camera gap at t=3619.9 s. Every coverage number computed against it was wrong and had to be recomputed after the jam-sync solve at 02:04.

**The question that saved three days (06-19 16:42).** Joshua asked, "do the materials include a cut?" It uncovered a never-opened folder — *Premiere File with Proxies & Synced Audio* — containing the editor's `.prproj` and both 2 GB per-speaker ISO WAVs. The project file yielded the authoritative sync constant, **+350.60 s**. Up to that point everything had been reverse-engineered from camera room mics.

**The audio reset (06-19 16:11 → 17:46).** Half an hour earlier Joshua had watched the day's deliverables and said the single highest-value sentence in the record: *"The audio clip for each of the videos has the man talking, but in none of the videos he is talking (even for the normal motion segments)."* Three faults compounded — CAM2's embedded room mic instead of the per-speaker ISO, a wrong speaker assignment, and a 171 ms chunk-boundary offset. No automated check had flagged any of it. Every held-shot deliverable was scrapped rather than patched, including a 10-minute LatentSync run that had lip-synced Eric to Mel's words.

**Model search (06-19 18:38 → 20:47).** A research pass recommended lip-only relipping (~95% real pixels by construction). Joshua rejected it: lips on a frozen body reads as dead eyes, no blink, no breath. That redirected the search from mouth-region models to full-frame audio-driven ones and is the reason the project did not end at LatentSync. 27 GB of LTX-Video 0.9.5 was downloaded and deleted the same evening — wrong model family, no audio conditioning — although another session had already recorded the correct model identity and could not be seen.

**The watchdog (06-19 22:32 → 06-20 06:52).** Eight hours. Nine start-poll-stop retry cycles; a stuck pod billing $1.55/hr for 25 minutes; a fresh A100 in Romania rented and terminated at 16 minutes for "not booting"; the user sent to hunt for a nonexistent RunPod setting; 2.5 hours spent building a retry loop for a scarcity problem that did not exist. The actual cause was a cost-saving tool reaping pods that were reachable and healthy (`idle 21m >= 20m: stop`, `reachable=True`) because the CPU-only structure phase looked idle. Direct GPU cost was small — ~$2.50 — *precisely because the pods kept being stopped*. The loss was the eight hours and the two false diagnoses. Joshua ended it with two corrections: *"you've been creating the failure mode! … Don't time out. Give a pod the chance to book."* and *"how are you going to know when it's actually running?"* — which forced a real SSH + `nvidia-smi` check and immediately surfaced that the pods had `volumeInGb: 0` and were losing 74 GB of staged environment on every stop.

**Seam forensics (06-20 21:32 → 06-21 01:31).** Joshua specified an ffmpeg rate-of-change metric to localize "abrupt pose changes and image quality changes"; the resulting YDIF series nailed all 10 cuts at exactly the predicted frame indices. He then pointed at a specific pair: *"Consider the difference between 151 and 152, both supposedly original frames."* MAE 0.07 vs 3.46 located the real→gen seam by frame number, unified every reported artifact into one root cause, and exposed an off-by-one in the diagnostic overlay. Then: *"It literally looks like slow motion"* — rejecting the QuickTime-decoder hypothesis and forcing measurement of actual frame timestamps, which found `int(23.976) = 23`. And: *"frames bounce back and forth like they are backwards in pairs"* — a B-frame display-order scramble diagnosed from a verbal description alone.

The 23 fps fix was applied to the assembly on 06-21. Its source — `ltx_pipelines.utils.media_io.encode_video` — was found seven days later, on 06-28 16:31. In between, every clip through the LTX-2 path played 4% slow with audio drifting, and a week of lip-sync and playback verdicts were formed on defective containers.

**The anchor sweep and its reversal (06-23 → 06-28).** A KFSTRIDE sweep, a variant scorer, four comparison clips, an S3 review page, and Joshua's eye all converged on k3 (31 anchors): *"face expression is significantly better on k3… if we can get the image quality of k6 and the face quality of k3."* Five days later a splice-vs-real motion benchmark showed dense anchoring **is** the pose-to-pose dissolve, and production switched to sparse 2-endpoint. The sweep was not wasted — it produced the harness and the landmark scorer — but the deliverable it selected was reversed, and the intervening sharpen investigation was built entirely on the losing branch.

**Evaluation medium rejected (06-25 20:20).** *"resolution is necessarily reduced in a quad contact sheet… the issue looks less like sharpness and more like compression blocking."* Joshua rejected the evidence medium rather than the result. That led directly to the crf-19 / 2.8 Mbps discovery and the lossless ProRes pipeline, and established a standing rule: no grids, full-res 1:1 crops only.

**Ground truth (06-28 18:00).** Joshua originated the fitness function: *"we have a ground truth source for the background so if we use SAM3 to select all of Erik out what remains is only the foreground mic and desk and the background… you should be able to drive generation and other features (up rez, denoise, etc) to achieve the closest quality match."* Quality stopped being opinion. Three levers — unsharp+grain, 4K-native generation, super-resolution — were killed the same day on `score_bg_quality` numbers.

**The leaky mask (06-29 13:20).** A u2net matte showed the MediaPipe selfie segmenter had been falsely tagging desk and papers as person, costing ~9 points of valid background and understating detail_ratio (0.538 vs 0.601). The qualitative verdicts probably survive; the magnitudes on which the 4K-vs-1080p delivery decision rests do not. The clean-matte re-baseline sweep was proposed at 13:20, costed at "a few minutes, local/CPU, no pod" — and **never run**. As of 07-04 the project's headline number is known-wrong and unfixed.

The one place provenance was attached deliberately — a `masker` field written into the score JSON at 13:17 — is the one place contamination was caught and bounded in minutes.

**State loss.** The main transcript reached 27 MB, killed its own session on a false-positive safety classifier, and had to be reconstructed by a 7.2-minute subagent pass over a 778 KB digest, after the on-disk notes proved a week stale. `RELIABLE_PROGRESS.md` is a hand-built, lossy substitute for a graph.

## 1.3 The experiment tree

Indentation is derivation depth. `[DEAD]` = work whose result was discarded. `[SUPERSEDED]` = correct at the time, replaced by a better input. `[NEVER RUN]` = specified and not executed.

```
ERIC THOMAS 60 s GAP FILL   2026-06-18 → 07-04   10 sessions   ~$81 / 38.1 pod-h through 06-25
│
├── INGEST
│   ├── camera cards (5 feeds, ranged HTTP)
│   │   ├── moov table scan ................ work/CAM*_frames.json   [reused 26 h later, cross-session, by luck]
│   │   ├── raw mdat icpf scan ............. control 8/8 at predicted offsets; 0 hidden frames in 6 longest gaps
│   │   └── coverage census
│   │       ├── from moov tables ........... free
│   │       └── from downloaded frames ..... 10 min, ~345 ranged reads, ~2 GB   [DEAD — moov already knew]
│   ├── corrupt-frames hypothesis .......... 1 h, 1 montage   [DEAD, correctly caveated]
│   ├── Chrome download .................... 7.3 GB   [DEAD — cancelled by hand, ⌘⇧J]
│   └── truncated zip ...................... 4.0 GB   [DEAD]
│
├── SYNC
│   ├── audio xcorr, ±10 s search .......... phantom 6.05 s all-camera gap @3619.9 s   [SUPERSEDED]
│   │   └── every coverage number computed against it   [recomputed 06-19 02:04]
│   └── Premiere project (opened 06-19 16:42, after the question "do the materials include a cut?")
│       ├── .prproj cut list ............... 10 cuts
│       └── ISO WAVs (2 GB × 2) ............ +350.60 s   [AUTHORITATIVE]
│           └── (before this: CAM2 room mic + wrong speaker + 171 ms chunk offset)
│               ├── FILM 1 s holdout ....... ~1 h A100   [DEAD]
│               ├── LatentSync arm G0a/G0b .. ~0.5 h A100   [DEAD]
│               ├── 3 held-shot deliverables  ~50 min A100, incl. Eric lip-synced to Mel's words   [DEAD]
│               └── reaction-cut audio bed .. [DEAD]
│
├── MODEL SEARCH
│   ├── lip-only relipping ................. recommended, rejected by Joshua 06-19 18:38   [DEAD]
│   ├── LTX-Video 0.9.5 .................... 27 GB + warmup gen   [DEAD — no audio conditioning]
│   ├── LongCat ............................ "reference as soft appearance condition"   [WRONG for 45 min]
│   └── LTX-2 22B distilled 1.1 ............ SELECTED
│       └── env stage: 46 GB weights + 7.6 GB LoRA + 1 GB upscaler + 23 GB Gemma = 74 GB
│           └── re-staged ≥4× (06-19, 06-20 after disk wipe, 06-23, 06-26 ×2, 06-27, 06-28 ×3)
│
├── INFRASTRUCTURE
│   ├── watchdog ........................... 8 h wall, 9 retry cycles, 2 misdiagnoses, ~$2.50 GPU
│   │   └── root cause: cost-saving reaper stopping healthy pods ("idle 21m >= 20m", reachable=True)
│   ├── volumeInGb: 0 ...................... ephemeral disk; 74 GB + all tool scripts lost per stop
│   ├── 3 secure-cloud pods hung 13–20 min at runtime:None; race script timed out without deleting
│   └── torch reinstalled 4× across 3 sessions (2.9.1→2.11.0/2.12.1 and back) ~10 GB wheel churn
│
├── GENERATION
│   ├── 95-keyframe Option B ............... aborted @105 s/step   [DEAD, ~10-15 min A100]
│   ├── gap 22 → 23 re-standardization ..... 8n+1 constraint found after the harness; all 4 methods re-run
│   ├── KFSTRIDE sweep k2/k3/k4/k6
│   │   ├── variant scorer + 4 clips + S3 review page
│   │   ├── human verdict 06-25 20:03 ...... k3   [REVERSED 06-28]
│   │   └── K4 sharpen sub-investigation .... built on the losing branch
│   ├── splice-vs-real motion benchmark .... dense anchoring IS the dissolve → sparse 2-endpoint
│   ├── 4K 145-frame ....................... OOM after ~70 min H100   [DEAD, ≈$3.84]
│   ├── lossless export .................... GEN_DONE secs=544, crashed on chunk generator, nothing saved
│   │   └── watcher polled only for success → pod idled ~50 min   [≈$2.75 idle + $0.50 discarded]
│   ├── TilingConfig() TypeError ........... no per-job try/except → whole batch died; 2 queued jobs lost
│   ├── bench_sparse_4k .................... pod deleted mid-scp   [DEAD, re-run 19:26]
│   ├── daVinci 540p ....................... OOM at SR stage   [DEAD]
│   └── daVinci 1080p ...................... base + SR completed, OOM at decode   [DEAD]
│
├── EVALUATION
│   ├── quad contact sheets ................ rejected as a medium 06-25 20:20 → crf-19/2.8 Mbps found
│   ├── evidence stills / filmstrips ....... 2 unreadable formats, 1 forced re-crop
│   ├── YDIF rate-of-change ................ all 10 cuts at predicted indices; off-by-one in overlay found
│   ├── frame 151 vs 152 ................... MAE 0.07 vs 3.46 → real/gen seam located
│   ├── interpolation eval ................. CAP=12 on broken flow (37m6s CPU), re-run CAP=10 after uint8 fix
│   └── score_bg_quality (detail_ratio)
│       ├── masker=mediapipe-selfie ........ 0.538   [LEAKY — desk/papers tagged as person]
│       │   └── killed: unsharp+grain, 4K-native gen, super-resolution   [MAGNITUDES INVALID]
│       ├── masker=u2net ................... 0.601
│       └── clean-matte re-baseline sweep ... "a few minutes, local/CPU"   [NEVER RUN]
│
├── CONTAINER
│   └── int(23.976) = 23
│       ├── found downstream 06-21 00:06 (NOPOP concat), fixed in the assembly only
│       └── found at source 06-28 16:31 (ltx_pipelines.utils.media_io.encode_video)
│           └── 7 days of clips 4% slow: v_k3_s10, v_k6_s10, gen_shot03, gen_ltx2_eric, sharpen sweep
│               └── a week of lip-sync and playback verdicts formed on defective containers
│
└── DELIVERY
    ├── eric_60s_edit.mp4 .................. 11 shots, hard cuts, 1080p, h264 crf-18   [SHIPPED]
    ├── eric_60s_NOPOP.mp4 ................. same shots, pop removed, no regeneration   [SHIPPED]
    ├── 06-28 locked recipe ................ sparse 2-endpoint, composite over 4K real plate, ProRes 24000/1001
    │   └── end-to-end execution ........... [NEVER RUN]
    ├── composite-over-real-plate demo ..... the locked recipe's central claim   [NEVER BUILT]
    ├── matte overlay clips ................ rendered 07-01   [NEVER ADJUDICATED]
    └── padded quote ....................... [NEVER PRODUCED]
```

## 1.4 Unresolved in the record

Carried forward without repair, because a design that pretends the record is clean is worthless:

1. **Same-person camera correlation.** `w-5a7d` 04:34 and `main-a` 04:55: woman-union 48.1% / man-union 55.4% in the `[5468,5498]` window. `main-a` 20:46: CAM2+CAM4 union 42.9% vs an independent-union ideal of 42.2% over the whole file. Probably both right at their own scale. Never reconciled. The first number drove "generative is the only lever"; the second improved the economics.
2. **Spend disagrees.** 06-19 reported as $30.10 (06-19 21:17) and $35.05 (06-25). Post-06-25 never tabulated.
3. **Whether later H100 pods re-downloaded 74 GB.** 06-23 04:41 describes the pool booting in 15–30 s with "cached weights"; 04:48 describes a full `restage_ltx2.sh` climbing 46.2 → 61.2 → 79.2 GB.
4. **4K: better or worse?** 06-19 20:37 measures the 4K same-instant render gap at 2.43 against a normal frame step of 2.04 — the best result on the ladder. 06-28 19:26 measures 4K-native as worse on every masked-background metric (detail_ratio 0.375 vs 0.522). Different metrics on different quantities; both may hold; the delivery decision rests on the second, which was measured through the leaky mask. **Open.**
5. **Three sessions burned time diagnosing deliberate `runpodctl pod stop` calls** by the lead (`w-c76c` 02:06, `w-dbe3` 06-19 20:48, `a-8d6b` 06-19 16:50). An observability gap, not a technical failure, but structural at three recurrences.

---

# 2. The touch points

Every reusable operation the project performed. Counts are what the record supports; `~` means the record does not pin an exact number. Class is the determinism class the operation *actually* has, not what it was treated as.

| Operation | Inv. | Varied parameters | Class | Notes |
|---|---|---|---|---|
| `moov-scan` | 5 (once per feed) | `feed` | exact | output cached as `work/CAM*_frames.json`; survived one session boundary by accident |
| `coverage` | ~4 | `moov-set`, `window` | exact | computed once from frames (2 GB) that moov already answered free |
| `icpf-scan` | 1 (7 ranges) | `range` | exact | 8/8 control; removed a whole failure mode |
| `xcorr-offset` | 1 | `search=±10s` | validated | superseded by `prproj-parse`; produced a phantom gap |
| `prproj-parse` | 1 | — | exact | yielded the cut list and **+350.60 s** |
| `iso-slice` | ~30 | `wav`, `offset`, `t0`, `t1` | exact | the 171 ms chunk-boundary offset lived here |
| `seg-fetch` | ~5 | `feed`, `cam`, `t0`, `t1` | exact | ~6 GB per call; called twice on a mis-located window |
| `stage-env` | ≥8 | `repo`, `commit`, `weights` | exact | 74 GB each; the single largest byte cost in the project |
| `ltx2-gen` | ~40 | `plate`, `audio`, `anchors`/`kfstride`, `nframes` (8n+1), `res`, `seed` | validated | the project's only generative step |
| `latentsync` | ~3 | `clip`, `audio` | validated | abandoned after the full-frame ruling |
| `interpolate` (RAFT) | 3 | `cap`, `dtype` | exact | run twice on broken flow (uint8 bug); 37m6s CPU discarded |
| `encode` | very many | `fps`, `codec`, `crf` | exact | `fps` is where `int(23.976)` lived |
| `concat` | ≥4 | `shots`(ordered), `transitions`, `pop` | exact | SMOOTH and NOPOP built from existing `gen.mp4` with zero regeneration |
| `motion-score` / `rescore` | many | `clip` | exact | recomputed on already-decoded frames at zero network cost |
| `bg-score` (detail_ratio) | ~20 | `clip`, **`masker`** | exact given masker | `masker` is the one provenance field anyone wrote down |
| `matte` / segmentation | ~6 | `model` ∈ {mediapipe-selfie, u2net, SAM2, SAM3} | exact | the leak was here |
| `ydif` | 1 | `clip` | exact | located all 10 cuts |
| `contact-sheet` / `filmstrip` / `crop` | ~8 | `layout`, `scale` | exact | 2 unreadable formats; medium rejected by Joshua |
| `pod-provision` / `pod-delete` | ~25 | `gpu`, `region`, `volume` | volatile | never a graph problem; see §3 |
| `decide` (human, constitutional) | 4 | — | volatile + authored | outlived every rebuild |
| `judge` (human, perceptual) | 11 | — | volatile + authored | overturned automated results 5 times |
| `judge` (model, perceptual) | 4 | — | volatile + authored | correct twice, wrong twice, one contradiction never resolved |

**Observations that matter to the design.** Only two operations in the whole inventory are `validated` — generation and lip-sync. Everything else is either `exact` (and therefore cacheable, and therefore was recomputed for no reason) or `volatile` (and therefore uncacheable, and therefore the thing the project existed to obtain). The class column is nearly all `exact`, which is precisely why the waste in §3 is as large as it is.

## 2.1 Variant axes and what won

| Axis | Values tried | Winner | How decided | Standing |
|---|---|---|---|---|
| anchor density | kfstride 2 / 3 / 4 / 6; sparse 2-endpoint | sparse 2-endpoint | benchmark 06-28 reversed a human verdict from 06-25 | settled |
| resolution | 1080p, 4K-native | 1080p (provisionally) | masked-bg metrics 06-28 | **open** — measured through the leaky mask |
| sharpen | none, unsharp+grain | none | `score_bg_quality` 06-28 | **unverified** — same leak |
| super-resolution | none, SR pass | none | `score_bg_quality` 06-28 | **unverified** — same leak |
| masker | mediapipe-selfie, u2net | u2net | matte inspection 06-29 | settled |
| fps container | 23, 24000/1001 | 24000/1001 | frame-timestamp measurement | settled (bug) |
| codec / bitrate | h264 crf-18, crf-19 (2.8 Mbps), ProRes lossless | ProRes for evaluation | Joshua rejected the medium 06-25 | settled |
| audio source | CAM2 room mic, per-speaker ISO + 350.60 s | ISO | human perceptual catch 06-19 16:11 | settled |
| model family | LTX-Video 0.9.5, LatentSync, LongCat, LTX-2 22B | LTX-2 | ruled out by Joshua's full-frame requirement | settled |
| transitions | {cut, dissolve, freeze} | {cut} | Joshua 06-19 06:32 | constitutional |
| composite | real+gen, pure-gen, gen over 4K real plate | gen over 4K real plate | 06-28 recipe | **never built** |

Three of eleven axes are decided on numbers now known to be wrong or unreconciled. That is the shape of the problem: it is not that the project computed too slowly, it is that it could not enumerate what a bad input had touched.

---

# 3. What a workflow system would have saved

The honest way to do this is to split the loss by **who fixes it**, because most of the raw waste is not the graph's to claim. Substrate #2 (peer read-sources over the existing `MultiStore`) and the store itself already fix the biggest byte item; nothing in the substrate fixes the biggest wall-clock item.

| Loss | Magnitude | Fixed by |
|---|---|---|
| Redundant LTX-2 env staging | ≥300 GB, ≥4 full re-stages | store + peer read-source — **not the graph** |
| Tool scripts rewritten from memory | ~10 min + correctness risk | store |
| daVinci weights re-fetched after `hf download` dropped patterns | 29 GB | store (verify by hash, not by "complete" message) |
| torch churn across 3 sessions | ~10 GB, 30–40 min | environment lockfile keyed by content hash |
| Chrome 7.3 GB + truncated zip 4.0 GB + wrong model 27 GB | ~38 GB | nothing — human/tooling errors |
| Watchdog | **8 h wall, 2 misdiagnoses** | nothing. But its own forced fix ("skip any shot whose `gen.mp4` exists") *is* what a graph gives by default; each reap would have cost seconds, not a restart |
| Pod hangs / boot failures 06-23 | ~2 h | nothing |
| 8 aborted GPU runs | ≈$5–6 | nothing. A graph records failure; it does not prevent OOM |
| `coverage` from frames | 10 min, ~2 GB | **graph** — pure cache hit; `scan_coverage_from_moov` was written 2 minutes later |
| `match_seg` third derivation after stale `seg_state.json` | ~6 GB, ~5 min | **graph** — the hand-rolled `REUSE=1` flag is the graph's default behavior |
| All-camera census computed twice in parallel sessions | 2 runs | **graph** — cache hit *and* free cross-validation (same node, same input hash, same output hash) |
| gap 22 → 23 re-standardization | ~15 min GPU | nothing — a genuine invalidation; all four descendants really did change |
| **Audio-source cascade** | ≥2 GPU-h, ~4 h worker wall, one day of judgments, 4 deliverables scrapped | **graph** — `+350.60 s` is one bound node; repointing it re-keys `iso-slice` and its descendants and nothing else. Rebuild in minutes instead of scrap |
| **23 fps cascade** | one week of defective containers; diagnosed twice, seven days apart | **graph** — the contaminated set is the transitive closure of edges citing the old process object. Not fast; possible |
| **Leaky-mask cascade** | 3 levers killed on invalid magnitudes; re-baseline never run | **graph** — `masker=` on the line makes the contaminated set a query. This is the one place the project did it by hand, and the one place it caught the problem in minutes |
| Anchor-density reversal | k3 sweep + the entire K4 sharpen branch | partial — the graph keeps the sweep's outputs *nameable*; keeping them *alive* needs a retention warrant (§6) |

**The totals, stated so they can be argued with.**

- Raw avoidable transfer: ~340–380 GB. Graph-attributable: **~8 GB**.
- Directly quantified wasted GPU spend: ≈$9–11 of ~$81 (~12%). Graph-attributable: **≈$5–8**, mostly the audio cascade's re-derivable GPU-hours.
- Wall-clock lost to self-inflicted infrastructure: ~10 h. Graph-attributable: **~0**.
- Invalidation cascades: 4 major. Graph-attributable: **3 of 4**, plus one reversal made cheaper.

**So the economic case is not the cache.** On dollars the graph recovers under 10% of a small number. The case is the third column of that last row: *three of four cascades*, all of them cases where a wrong input silently contaminated everything downstream and there was no way to enumerate the affected set. The project's own postmortem named this failure #2 — "provenance was not attached, so wrongness propagated invisibly" — and named the counterexample: the deliberate `masker` field, which bounded a contamination in minutes.

The single most valuable query over this history is not "is this cached." It is **"what is downstream of this."** Every prior-art system optimizes the first. That is worth knowing before designing for the second.

---

# 4. The language

## 4.1 What this is, plainly

A workflow graph is **a set of testimony lines with a canonical order and a completeness terminator**. The lines are the ones `receipt-record.md` already writes. The predicates are the ones `testimony-record.md` already budgets. The kind adds three things and nothing else: a total order (so the document has a stable ID), a terminator (so partial is distinguishable from complete), and a closed exempt-field list (so annotations cannot poison cache keys).

It is not a workflow language. No control flow, no interpreter, no execution semantics. It is testimony about IDs — substrate class (c) — and a runner may read it or ignore it.

**Prior art, named.** This is Nix's `.drv` written in plain text: a derivation is `(builder, inputs) → output` and the memo key is `H(process, inputs)`. It is Ninja's `build out: rule in…` shape with content hashing instead of timestamps. The re-run rule in §4.5 is `topological ctRebuilder` from *Build Systems à la Carte* — constructive traces, which is what makes a cache shareable across machines. ThruWire (2605.06365) has published the graph-of-identity-keyed-nodes-with-selective-recompute core. **None of this is new and the earlier draft was wrong not to say so.**

Four things are new, and only four:

1. **Determinism class as a *served* class** — `exact` / `validated` / `volatile` labeling non-determinism rather than forbidding it (Nix forbids it; ThruWire structures it away; durable-execution vendors wrap it in apology).
2. **Every non-exempt field is an input.** Artifact inputs, scalar parameters, and lists are the same thing, so a one-parameter change and a changed input file are the same event. Snakemake puts params in the path; Bazel splits configuration from inputs; Nextflow hashes both without unifying them. This is why `--rerun-triggers` had to be retrofitted into Snakemake and is not needed here.
3. **Plan and run in one document**, via the identity-basis axis (`null` → `nom:` → content). Every system in the prior art keeps them in separate artifacts.
4. **A human decision is an ordinary input to a derivation key.** `transitions=nom:et/transitions` makes "never a dissolve" participate in invalidation exactly as a byte does. Bazel cannot express it, Dagster cannot, OMC can only assert it.

## 4.2 What was cut, and why

The earlier draft is dead. Applying the review:

| Cut | Replaced by | Why |
|---|---|---|
| positional 4-column edge line (`class process fields… node`) | shape-A line: `<subject> <predicate> <actor> <fields…>` | two grammars for one relation; `grep produced-from` missed every graph edge |
| `class` in column 1 | `class=` field | the receipt already spells it as a field; `grep 'class=volatile'` is worth the same |
| `process` in column 2 | `process=` field | same |
| ID **last** | ID **first** | the `/receipt` render is PINNED ID-first and is a launch metric. The graph is unbuilt; it moves, not the receipt. `listings.md` wants the opposite — see §7 Q1 |
| `by=<actor>` | the actor column shape A already has | duplicated `authored-by` and the receipt's own column |
| `bind` line form | `resolves-to` predicate | already budgeted |
| `note` line form | a note is a file; `<note-id> authored-by <actor> about=<node>` | a 7th predicate in all but name, and the budget is full. Making notes content also makes them immutable and attributable — "everything is content; testimonial content is a known parseable type" |
| `set` verb | a c4m patch on the graph document | patch is already a tier-1 `exact` derivation with its own ID and a shipped implementation. `set` reinvented it |
| `@` in the key charset | `_` and `.` added, `@` removed | `@` existed only to scope `set` |
| sequence folds (`kfstride=[2-6]`) | stays cut | a comprehension in frozen-grammar clothes |
| "read the last column and the whole state of the project falls out" | deleted | false — it could not represent 8 failed runs. See §4.6 |
| "the sweep's outputs stay addressable" | deleted | links never pin bytes (`store-records-architecture.md` decision #4). Naming an output does not keep it alive |
| the "why it costs no new concepts" table | deleted | read literally it argued for building nothing |
| `at=` in the derivation key | closed exempt list | the draft's own showcase line could never have cached |
| open question "is the ID of `3` the hash of `3` or `3\n`" | dissolved | a scalar is not a node and has no ID. See §4.4 |

## 4.3 Grammar

```abnf
graph      = magic LF *( line LF ) [ terminator LF ]
magic      = "@c4 graph 1"
terminator = "@c4 end"

line       = subject SP predicate SP actor *( SP field )
subject    = node
predicate  = "produced-from" / "resolves-to" / "authored-by"
actor      = 1*VCHAR                       ; the declaring party; never in a key
field      = key "=" value

key        = 1*( ALPHA / DIGIT / "-" / "_" / "." )
value      = item *( "," item )            ; ORDERED, never re-sorted
item       = c4-id / nom-tag / scalar
node       = c4-id / nom-tag / "-"

c4-id      = <90 chars, per the -m c conformance ID grammar>
nom-tag    = "nom:" authority "/" name
scalar     = 1*( VCHAR-except-SP ) ; c4m escaping: "\ " for space, "\\" for backslash
```

One line form. Three predicates, all pre-existing, none new.

- `produced-from` — a derivation edge. Fields follow.
- `resolves-to` — a binding. Takes exactly one node as its value, in field form: `to=<node>`.
- `authored-by` — attribution. `volatile` + `authored-by` = a creation point; unattributed volatility is flake.

**Lexical disjointness is the integrity boundary.** An item is a reference iff it is a 90-character C4 ID or begins with `nom:`. Everything else is a scalar. `c4` + 88 base58 and the `nom:` prefix are reserved shapes; no scalar may take either form. This is what makes it structurally impossible to write an assigned ID where a content ID is expected — the failure `derivation-records.md` calls "the one lie the system must be structurally unable to tell."

**The honest limit on that rule.** The grammar can guarantee the three forms are distinguishable. It cannot guarantee a human *tags* a reference: `commit=7809842` is a git SHA masquerading as a scalar, and the earlier draft committed exactly this on its most assigned line. The remedy is a lint — a `--assigned` check reporting scalars whose shape suggests an identifier (hex runs ≥7, path-like slugs, URLs) — not a grammar rule. Stated as a limit, not solved.

**A step is its output.** No step names. One edge, one output. A step producing many files produces one directory node, which is a c4m, which already has an ID. Fan-out is two edges sharing an input; fan-in is a comma list.

**Two edges may name the same output.** Competing routes — a render edge and a binary-diff edge to one frame — each with its own class. A `resolves-to` line adjudicates. The grammar never chooses.

**Cycles are invalid.** The DAG is the only ordering; `at=` is an ordinary exempt field, never a sequencer.

## 4.4 Canonical form, identity, and the key

**Canonical form.** Magic line first. All other lines sorted **bytewise ascending over the whole line**. Fields within a line sorted bytewise ascending by key. Exactly one space between fields. No leading or trailing space, no blank lines, no CR, no BOM. One LF per line. `@c4 end` last, if the document is complete.

On bytewise vs c4m's natural sort: `listings.md` says "**kind** fixes the line grammar *and the canonical order*." Natural sort is defined over filenames — files before directories, numeric runs compared as integers. A graph line has no filename and no file/directory distinction, and applying integer comparison to numeric runs inside a base58 ID is actively wrong. So the kind declares bytewise. This is consistent with the per-kind rule and in tension with the same document's "one canonicalization rule" bullet; see §7 Q2.

**Document identity** is the hash of the canonical text. One identity rule, already true for directories.

**Ergonomic form** — pipeline order, unsorted fields, aligned columns, blank lines — is accepted on input and is what humans write. Canonicalization is a deterministic transform, never a judgment.

**The derivation key.** For each **non-exempt** field, in bytewise key order, emit `key` `=` then the items joined by `,`, where each item is its **resolved content ID** if it is a reference and its literal text if it is a scalar; terminate each field with LF; hash the result.

The exempt set is closed and frozen: **`class`, `test`, `at`, `about`** — plus the subject, the predicate, and the actor column, which are positional and never keyed. Four field names. No registry, no process-declared exemptions.

Two consequences worth stating:

- Keying over **resolved** IDs, not over text, is what makes the `+350.60 s` story work. Repointing one `resolves-to` line changes every downstream key. Keying over text would change nothing.
- `fps=23`, `fps=23.976` and `fps=24000/1001` are three different keys. There is no scalar normalization. This is why the 23 fps bug would have been visible; it is also why cache hit rate depends on spelling scalars identically forever. Both halves are true.

**Field-order note.** Canonical form sorts fields, so document identity is order-independent. The derivation key is computed over sorted fields regardless of how they were written, so **cache behavior is unaffected by field order in either form**. The pinned `/receipt` render writes `inputs= process= class=`, which is not sorted — that is fine: a receipt is a different kind with its own canonical order (append). A receipt's *lines* are graph lines; a receipt file is not byte-identical to a canonical graph document. Converting costs a sort. Said plainly because the earlier draft implied a byte-level identity that does not hold.

## 4.5 The re-run rule

For each edge in topological order:

1. Resolve every reference item to a content ID. Any unresolved reference ⇒ the edge is **unrunnable**; report it, never guess.
2. `class=volatile` ⇒ no key. Run — unless the request carries `recorded` freshness intent, in which case serve the recorded output *with its edge* (disclosed recording, never silent staleness) and suppress side effects.
3. Otherwise compute the key per §4.4. If the store holds an output for that key: `exact` ⇒ serve; `validated` ⇒ serve only if `test` passes or the caller opted in. Else run, then bind the output.

A changed input changes that edge's key and every downstream key. **The re-run set is the transitive closure of changed keys; nothing else re-runs.**

**The limit this rule imposes, stated rather than hidden.** A mostly-nominal graph resolves nothing, so it caches nothing and runs nothing. Cheap abstract structure capture and incremental re-run are therefore **mutually exclusive within one document at one moment**. What a nominal graph buys is real but narrower than the draft implied: the shape of the work, the receipt, an enumeration of what is unbuilt, and cost estimation. It becomes runnable only as nodes promote.

## 4.6 Failure

The draft could not distinguish "never attempted" from "attempted and failed," which in this project's record is the difference between the locked recipe and a 70-minute H100 OOM. Both rendered as `-`.

**A failed run is an edge that produced its log.** The run happened; it produced bytes; those bytes are the log. The intended output is simply absent from the graph.

```
{oom-log} produced-from main-b process=nom:et/ltx2-gen inputs={env},{plate-4k} nframes=145 res=4k class=volatile at=2026-06-26T18:00Z
```

- attempted and failed = an edge exists whose output is a log
- attempted and succeeded = an edge exists whose output is the artifact
- planned, never attempted = an edge with a `-` output, or no edge at all

Failure is read from *which edges exist*, not from a column. The claim "read the last column and the whole state falls out" is withdrawn.

## 4.7 Completeness

`store-records-architecture.md` makes a completeness watermark mandatory on the reference index "so silence is distinguishable from not-indexed — three-valued recall, R4." A listing that may legally be truncated has no such distinction: an incomplete and a complete document are byte-indistinguishable, and in a nine-session project where one worker knew the right model and another could not see it, that is the failure mode.

`@c4 end` is the fix, and it costs one reserved line. A document that ends with it is complete; one that does not is a partial stream, still valid, still readable, honestly labeled. Because the terminator is inside the hashed text, a complete document and its own prefix have different IDs.

This is a **framing** construct, not a graph construct — `listings.md`'s "a stream that stops is a valid partial listing" has no way to say "and this one didn't stop," for any kind. It is proposed as an amendment to the listings format, not owned by the graph. See §7 Q3.

## 4.8 Still deliberately left out

- **All control flow.** No conditionals, loops, retries, comprehensions, scheduling. A DAG that branches on runtime values is a program and needs an interpreter; that is the workflow language `substrate-map.md` forbids.
- **Execution and resource fields** — pod type, GPU, region, cost, retries, timeouts, status. Observations about a run, not testimony about IDs. They belong in the receipt and the journal. The watchdog was never a graph problem, and §3 says so with a number.
- **Parameter types, schemas, and any process-name registry.** Process semantics are pinned by content-addressing the process object. The grammar knows *fields*; processes know what fields *mean*.
- **A cost model or fetch-vs-recompute policy.** The resolver's decision record, priced on measurement.
- **New predicates.** Budget untouched at ≤6.
- **Signatures and anchoring.** The document's ID is the signable subject, as everywhere else.

---

# 5. Worked example

## 5.1 Convention

`{name}` is a **placeholder** for a full 90-character C4 ID — not a truncation. A real document carries full IDs and never truncates them. Lines are shown in ergonomic form (pipeline order, unsorted fields); canonical form sorts them. Long lines are wrapped here with `↩` for the page; real lines never wrap.

## 5.2 The Eric Thomas pipeline

```
@c4 graph 1

# ---- sources -------------------------------------------------------------
nom:et/cards      resolves-to  joshua  to=-                         # remote feeds, never hashed
nom:et/prproj     resolves-to  joshua  to={prproj}
nom:et/iso-eric   resolves-to  joshua  to={iso-eric}
nom:et/iso-mel    resolves-to  joshua  to={iso-mel}
nom:et/feeds      resolves-to  joshua  to={feeds}

# ---- structure discovery -------------------------------------------------
{moov}      produced-from  main-a  process=nom:et/moov-scan src=nom:et/cards class=exact
{coverage}  produced-from  main-a  process=nom:et/coverage moov={moov} window=60s class=exact
{icpf}      produced-from  main-a  process=nom:et/icpf-scan src=nom:et/cards ranges=6 class=exact

# ---- sync: two competing routes to one fact ------------------------------
{xcorr}    produced-from  w-5a7d  process=nom:et/xcorr cards=nom:et/cards search=10s ↩
                                  class=validated test=nom:et/jam-check
{cutlist}  produced-from  main-a  process=nom:et/prproj-parse proj=nom:et/prproj class=exact
{offset}   produced-from  main-a  process=nom:et/prproj-parse proj=nom:et/prproj field=sync-offset class=exact

nom:et/offset  resolves-to  joshua  to={offset}
{note-sync}    authored-by  joshua  about=nom:et/offset at=2026-06-19T16:42Z
# {note-sync} is a file. Its text: "+350.60 s from the editor's project is authoritative.
#   The xcorr offsets produced a phantom 6.05 s all-camera gap at 3619.9 s and are superseded."

# ---- constitutional rules, as inputs -------------------------------------
nom:et/transitions   authored-by  joshua  at=2026-06-19T06:32Z
{note-cuts}          authored-by  joshua  about=nom:et/transitions at=2026-06-19T06:32Z
# {note-cuts}: "There should NEVER be a dissolve. Not ever. There should never be a
#   sudden change in body posture."
nom:et/model-class   authored-by  joshua  at=2026-06-19T18:38Z
nom:et/scope         authored-by  joshua  at=2026-06-26T04:36Z

# ---- environment ---------------------------------------------------------
{env}  produced-from  w-9ee9  process=nom:et/stage-env ↩
                              repo=nom:hf/Lightricks-LTX-2 commit=nom:git/7809842 ↩
                              weights=nom:hf/ltx2-22b-distilled-1.1,nom:hf/ltx2-lora,↩
                                      nom:hf/ltx2-upscaler,nom:hf/gemma-t5-9b ↩
                              class=exact
# every identifier that names a thing is nom:-tagged. No bare literal references.

# ---- per-shot inputs (11 shots; abbreviated to the bind lines) ------------
nom:et/plate-s01 resolves-to w-9ee9 to={plate-s01}   # ... through plate-s11
nom:et/aud-s01   resolves-to w-9ee9 to={aud-s01}     # ... through aud-s11
nom:et/anchors   resolves-to main-b to=nom:et/anchor-policy-sparse

# ---- generation: eleven edges, written out --------------------------------
{gen-s01} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s01 audio=nom:et/aud-s01 anchors=nom:et/anchors nframes=185 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s02} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s02 audio=nom:et/aud-s02 anchors=nom:et/anchors nframes=121 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s03} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s03 audio=nom:et/aud-s03 anchors=nom:et/anchors nframes=97  res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s04} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s04 audio=nom:et/aud-s04 anchors=nom:et/anchors nframes=153 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s05} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s05 audio=nom:et/aud-s05 anchors=nom:et/anchors nframes=89  res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s06} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s06 audio=nom:et/aud-s06 anchors=nom:et/anchors nframes=137 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s07} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s07 audio=nom:et/aud-s07 anchors=nom:et/anchors nframes=113 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s08} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s08 audio=nom:et/aud-s08 anchors=nom:et/anchors nframes=169 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s09} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s09 audio=nom:et/aud-s09 anchors=nom:et/anchors nframes=105 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s10} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s10 audio=nom:et/aud-s10 anchors=nom:et/anchors nframes=145 res=1080p seed=1234 class=validated test=nom:et/bg-score
{gen-s11} produced-from w-9ee9 process=nom:et/ltx2-gen env={env} plate=nom:et/plate-s11 audio=nom:et/aud-s11 anchors=nom:et/anchors nframes=193 res=1080p seed=1234 class=validated test=nom:et/bg-score

# ---- a run that failed ---------------------------------------------------
{oom-log} produced-from main-b process=nom:et/ltx2-gen env={env} plate=nom:et/plate-4k ↩
                               audio=nom:et/aud-s10 anchors=nom:et/anchors nframes=145 ↩
                               res=4k seed=1234 class=volatile at=2026-06-26T18:00Z
# the run produced a log and no clip. 70 min H100, OOM. Not the same as "never attempted".

# ---- encode: the bug, and its fix, as two edges --------------------------
{enc-bad}  produced-from main-b process=nom:et/encode-v1 clip={gen-s10} fps=23 codec=h264 crf=18 class=exact
{enc-good} produced-from main-b process=nom:et/encode-v2 clip={gen-s10} fps=24000/1001 codec=h264 crf=18 class=exact
{note-fps} authored-by main-b about=nom:et/encode-v1 at=2026-06-28T16:31Z
# {note-fps}: "int(23.976)=23 in ltx_pipelines.utils.media_io.encode_video. Every clip
#   encoded through this process object played 4% slow with audio drifting."
# The contaminated set is: every edge whose process= is nom:et/encode-v1, plus their
# descendants. That is a walk, not an index. See §6.

# ---- scoring: the masker leak -------------------------------------------
{bg-mp} produced-from main-b process=nom:et/bg-score clip={enc-good} masker=nom:et/mediapipe-selfie class=exact
{bg-u2} produced-from main-b process=nom:et/bg-score clip={enc-good} masker=nom:et/u2net          class=exact
{note-mask} authored-by main-b about={bg-mp} at=2026-06-29T13:20Z
# {note-mask}: "the selfie matte tagged desk and papers as person; detail_ratio 0.538 vs 0.601."
-       produced-from main-b process=nom:et/bg-score clip=nom:et/killed-levers ↩
                             masker=nom:et/u2net class=exact
# the clean-matte re-baseline. Planned, never attempted: no log, no output.

# ---- assembly ------------------------------------------------------------
{edit}  produced-from main-a process=nom:et/concat ↩
                     shots={gen-s01},{gen-s02},{gen-s03},{gen-s04},{gen-s05},{gen-s06},↩
                           {gen-s07},{gen-s08},{gen-s09},{gen-s10},{gen-s11} ↩
                     transitions=nom:et/transitions class=exact
{nopop} produced-from main-a process=nom:et/concat ↩
                     shots={gen-s01},{gen-s02},{gen-s03},{gen-s04},{gen-s05},{gen-s06},↩
                           {gen-s07},{gen-s08},{gen-s09},{gen-s10},{gen-s11} ↩
                     transitions=nom:et/transitions pop=none class=exact
nom:et/delivered  resolves-to  main-a  to={nopop}

# ---- the locked recipe, never built -------------------------------------
-  produced-from main-b process=nom:et/composite gen=nom:et/gen-sparse ↩
                        plate=nom:et/plate-4k-real anchors=2-endpoint ↩
                        codec=prores4444 fps=24000/1001 class=validated test=nom:et/bg-score
nom:et/locked-recipe  resolves-to  main-b  to=-
{note-fork} authored-by joshua about=nom:et/locked-recipe at=2026-07-04T00:00Z
# {note-fork}: "Two recipes exist. eric_60s_NOPOP.mp4 was built at 1080p/crf-18 with
#   dense anchoring and a real+gen composite. The 06-28 recipe specifies sparse
#   2-endpoint anchoring, compositing over the byte-real 4K plate, and lossless ProRes.
#   Nothing has been built to the locked recipe."

@c4 end
```

**Three things this is doing.**

*The `+350.60 s` cascade becomes cheap.* `nom:et/offset` is one `resolves-to` line. Repointing it changes the key of every `iso-slice` and everything downstream — and of nothing else. The held shots get rebuilt in minutes instead of scrapped.

*The eleven shot edges are written out, and that is the right cost.* Eleven near-identical lines differing in three fields. A fold would compress them and destroy exactly the property that saved the project: per-shot addressability, which is what made "skip any shot whose `gen.mp4` exists" possible when the watchdog reaped the pod mid-run. The lines are the checkpoint.

*Failure, planning, and success are three visibly different states.* `{oom-log}` is a 70-minute OOM. The re-baseline sweep and the locked recipe are `-` with no run edge — planned, never attempted. `{nopop}` shipped.

## 5.3 The four-variant sweep

`set` is gone. A variant is a c4m patch on the graph document — already a tier-1 `exact` derivation with its own ID and a shipped implementation.

```
@c4 graph 1

{p-k2}  authored-by  main-b  about={plan} at=2026-06-23T00:00Z    # patch: kfstride=2
{p-k3}  authored-by  main-b  about={plan} at=2026-06-23T00:00Z    # patch: kfstride=3
{p-k4}  authored-by  main-b  about={plan} at=2026-06-23T00:00Z    # patch: kfstride=4
{p-k6}  authored-by  main-b  about={plan} at=2026-06-23T00:00Z    # patch: kfstride=6

{g-k2}  produced-from main-b process=nom:c4/patch inputs={plan},{p-k2} class=exact
{g-k3}  produced-from main-b process=nom:c4/patch inputs={plan},{p-k3} class=exact
{g-k4}  produced-from main-b process=nom:c4/patch inputs={plan},{p-k4} class=exact
{g-k6}  produced-from main-b process=nom:c4/patch inputs={plan},{p-k6} class=exact

{verdict-1} authored-by joshua at=2026-06-25T20:03Z
# "face expression is significantly better on k3; if we can get the image quality of
#  k6 and the face quality of k3"
nom:et/anchors resolves-to joshua to={g-k3}

{bench}     produced-from main-b process=nom:et/motion-bench ↩
                          subject=nom:et/splice-vs-real inputs={g-k3},{g-k6} ↩
                          class=validated test=nom:et/motion-test at=2026-06-28T00:00Z
{verdict-2} authored-by main-b about={bench} at=2026-06-28T00:00Z
# "dense anchoring IS the pose-to-pose dissolve" — the finding is the report {bench},
#  not a field. A result is never an input.
nom:et/anchors resolves-to main-b to=nom:et/anchor-policy-sparse

@c4 end
```

**Cost, stated honestly.** `set` was four lines. This is eight lines plus four patch objects. That is the price of not inventing a verb, and it buys the c4m patch algebra unchanged, line-local diffs, and a variant that is an ordinary node.

**What works and why.** A variant of a variant — patch(patch(base, k=6), k=3) — re-runs the outer patch (different key) but produces the same canonical graph text, hence the same document ID, hence downstream cache hits. Identity comes from the *effective* graph, never from the history of how it was set. This is the Bazel scar (`--experimental_output_directory_naming_scheme=diff_against_baseline`) avoided by construction, and it is why the canonical order in §4.4 is load-bearing rather than cosmetic.

**What does not work.** A patch handles shape change — adding, removing, rewiring edges — where `set` could only rebind parameters. That is strictly better. But rebinding `nom:et/anchors` mid-document, as above, means two `resolves-to` lines for one name at two times; the later one wins only by convention. §7 Q6.

---

# 6. What survived attack, and what didn't

## 6.1 Didn't survive

- **The positional line.** Four columns, class-first, ID-last. It was a second grammar for a relation the receipt already spells, and it broke the predicate budget by moving vocabulary into an unbudgeted field-key namespace. `grep produced-from` would have found receipt edges and missed every graph edge.
- **`set`, `by=`, the `bind` line, the `note` line.** Each duplicated an existing primitive.
- **The "why it costs no new concepts" table.** Every row named an existing mechanism that already carried the requirement. Read literally, it argued for building nothing. Deleted rather than defended.
- **The original worked example.** It omitted the 11-shot fan-out that is the project's only shipped deliverable; it rendered eight failed runs identically to work never attempted; it used `finding=` as an input field, which contradicted its own central rule; it put `at=` in a cache key on a `validated` line, so its showcase edge could never have cached; and it wrote `commit=7809842` as a bare literal, which is the integrity-boundary breach the whole assigned-ID regime exists to prevent. All five are fixed in §5, and the example is longer for it.
- **"Read the last column and the whole state of the project falls out."** False, withdrawn, replaced by §4.6.
- **"The k3 sweep's outputs stay addressable rather than becoming rubble."** Links never pin bytes (`store-records-architecture.md` decision #4, permanent, federation-DoS). An edge naming an output does not keep it alive; preserving a sweep requires retention keep lines the graph does not write. Claim deleted.
- **Silence about prior art.** A derivation-format proposal that never said "Nix," "Ninja," or "constructive traces" was not being straight. §4.1 says all three.

## 6.2 Survived

- **The re-run rule.** Correct and complete, amended only by the closed exempt list.
- **Every non-exempt field is an input.** The single best idea in the proposal and the one no prior-art system has.
- **A human decision as an ordinary input to a derivation key.** `transitions=nom:et/transitions` in the concat edge. The requirement "record human decision points" and the requirement "know what to re-run" turn out to be one requirement.
- **Determinism class as a served label**, and the creation reading: `volatile` + `authored-by` = a creation point; the irreproducible edge is the one the workflow existed to obtain, never its weak link.
- **Plan and run in one document** via the nominal rung — with the limit in §4.5 stated rather than glossed.
- **§"Deliberately left out."** Every exclusion held. Control flow and the fold in particular.

## 6.3 Conceded, unfixed

- **Contamination enumeration is a walk, not an index.** "What did the buggy encoder touch" is `grep nom:et/encode-v1` plus a descendant walk. Over one project's graph — a few thousand lines — that is coreutils and instant. Over a store, it needs the reference index, which `store-records-architecture.md` puts app-layer, later, and never on a destructive path. The graph makes the query *possible*; it does not make it *fast at scale*.
- **Superseding a process needs no new predicate and gets no help.** Fix the bug, mint a new process object, and old edges keep citing the old one — which is *correct*, they really were produced by the buggy process. But nothing marks the old process suspect except a note, and nothing enumerates the affected set except the walk above.
- **The largest losses in §3 are not the graph's.** ≥300 GB of redundant staging is the store plus the peer read-source. Eight hours of watchdog is nobody's — though the checkpointing it forced is what the graph gives by default.
- **Reference-vs-scalar tagging is linted, not enforced.** §4.3.
- **Two `resolves-to` lines for one name** is how the anchor reversal is recorded, and "later wins" is convention, not grammar. §7 Q6.

## 6.4 The no-new-format alternative

The strongest counter-proposal was: ship no kind at all. Two ordinary files — `plan.c4m` for nodes, `plan.testimony` for edges — with shape-A lines, existing predicates, and three additions: a `test=` field key, free-form `key=value` fields on `produced-from`, and "every field is an input" as a binding *reading* rather than a grammar. Cost: one paragraph in `testimony-record.md` and zero new conformance vectors.

**It is nearly right, and most of it was taken.** Every positional construct in the draft is dead. The lines in §5 *are* shape-A testimony lines with existing predicates. The variant mechanism *is* c4m patch. `test=` is the one new field key. The reading is adopted verbatim.

**What was not taken, and why.** Three properties do not survive a bare testimony sidecar:

1. **Document identity.** A testimony file is append-ordered and unordered as a set; two files with the same edges in different order have different IDs. Then patch(patch(base, k=6), k=3) does *not* converge on the same document ID, and the variant-of-a-variant early cutoff — the property the review itself credited as avoiding the Bazel scar — is lost. A canonical order is what makes a graph a value rather than a log.
2. **Three-valued recall (R4).** A sidecar cannot say "no record covers this." `@c4 end` costs one line and buys it. In a project where one session knew the right model and another could not see it, this is the failure mode, not a nicety.
3. **The closed exempt list.** Without it, `at=` sits in cache keys and every edge carrying a timestamp is permanently uncacheable — the exact footgun the draft demonstrated on itself.

`listings.md` says a new kind should cost "a line grammar, not a format," and that it is worth it only when the line grammar carries new information. A total order, a completeness terminator, and a closed exempt list are new information. Three of the draft's five structural choices were not, and they are gone.

**The concession.** The alternative's best argument — that a receipt would then be *literally, byte-wise* a subgraph of a graph — is genuinely lost. Under the amended kind a receipt's lines are graph lines and a receipt file is not a canonical graph document; converting costs a sort (§4.4). That is a real cost, paid to get document identity. If Joshua weighs byte-level receipt-is-a-subgraph above variant convergence and R4, the alternative wins outright and this document reduces to four field keys. §7 Q8.

---

# 7. Open questions for the freeze

Each is Joshua's. Each is stated as an either/or with the cost of each side named.

**Q1 — ID position.** *ID first* (matches the PINNED `/receipt` render, which is a launch-sightings metric; `grep '^<id>'` answers "what produced X" and bisects a sorted file) **or** *ID last* (`listings.md`'s explicit ruling — the frozen c4m grammar's choice, and `awk '{print $NF}'` then works on every listing in the system, at the cost of unpinning the receipt render). Recommendation: **ID first for the graph, because the graph is unbuilt and the receipt is pinned.** Non-negotiable either way: receipt and graph must agree.

**Q2 — Canonical order.** *Bytewise over the whole line* (recommended; natural sort is defined over filenames and applying integer comparison inside base58 is wrong) **or** *no order at all, append only* (matches the journal and the receipt, at the cost of document identity and therefore variant convergence). Sub-question `listings.md` must answer regardless: does "one canonicalization rule" mean one comparator across kinds, or one rule *shape* with a per-kind comparator? The document currently says both.

**Q3 — `@c4 end`.** *A listings-level framing construct* (recommended; every kind gains partial-vs-complete distinguishability, and "a stream that stops is a valid partial listing" gains the sentence it is missing) **or** *graph-only* (smaller blast radius, but then R4 is solved once instead of generally) **or** *not at all* (accept two-valued recall at document level; cheapest, and the failure mode §1.4 item 5 documents).

**Q4 — The exempt-field list.** *Four frozen names* — `class`, `test`, `at`, `about` — (recommended; no registry, and it kills the `at=`-in-key footgun permanently) **or** *process-declared exemptions* (a process object states which of its fields are annotations; more flexible, reintroduces a per-process vocabulary the format was built to avoid).

**Q5 — Comma lists.** *Always ordered, never re-sorted* (recommended; order-critical cases like an 11-shot concat are unrecoverable if lost, whereas set-valued cases spelled in different orders lose only cache hits, and the fix is an emit-sorted convention) **or** *an explicit set marker* (`weights:={…}`) which recovers those hits at the cost of a second value syntax. Failing safe points one way: losing order costs correctness, losing hits costs money.

**Q6 — Rebinding a name.** The anchor reversal is two `resolves-to` lines for `nom:et/anchors` at two times, and "later wins" is convention. Options: *last-line-wins under canonical order* (fragile — canonical order is bytewise, not temporal), *`at=` breaks the tie* (makes an exempt field semantically load-bearing), or *a rebind is an error and the name must be versioned* (`nom:et/anchors-2`; honest, verbose, and makes reversal visible in every downstream line). No recommendation; all three are defensible and the third is the most C4-shaped.

**Q7 — Scalar spelling.** *No normalization* (recommended; `fps=23` ≠ `fps=23.976` ≠ `fps=24000/1001` is exactly why the 23 fps bug would have been visible) **or** *numeric canonicalization* (better hit rates, at the cost of silently equating two spellings that meant different things to two tools).

**Q8 — Whether the kind exists at all.** *A kind* — magic line, canonical order, terminator, exempt list, over shape-A lines (recommended, §6.4) — **or** *no kind*: `test=`, free-form fields, the "every field is an input" reading, and `nom:` values, all landing as one paragraph in `testimony-record.md` with zero new conformance vectors. The tiebreak is whether document identity and R4 are worth losing byte-level receipt-is-a-subgraph. Real evidence would settle it faster than argument: run one real pipeline both ways.

**Q9 — Filename convention.** `.c4g`, a bare name beside the c4m, or something else. Deliberately deferred, per the testimony-record precedent.

**Q10 — Order of work, and it is not negotiable.** `listings.md` §5 is explicit: fix #42 / #43 / #44 (pure subtraction, stops active data loss), freeze the v1.1 surface, *then* take the listing format through the crucible. This kind sits downstream of all three, and additionally downstream of the testimony freeze (which pins the predicate budget and the `-m c` conformance ID grammar that §4.3 cites). Of roughly a dozen conformance-critical behaviors in §4, seven are settled here and the rest are Q1–Q7. **Conformance vectors cannot be written until Q1, Q2, Q4 and Q5 are ruled**, and no implementation should begin before they are.

---

**Status, restated so no one mistakes this for a plan.** Designed, not built. Not scheduled. Blocked behind three release blockers and an unfrozen surface. The evidence in §1–§3 is real and the economics in §3 are modest: under 10% of the dollars, three of four correctness cascades. That second number is the case; the first one never was.