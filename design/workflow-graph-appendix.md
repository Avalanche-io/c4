# Workflow graph — supporting material

Raw outputs from the 2026-07-28 mining of the 11 footage-reconstruction sessions.
The distilled document is `workflow-graph.md`; this is the evidence behind it.

---

# A. Merged experiment history

(cross-validation), but only *because* the worker's output was invisible to the main thread | A content-addressed graph makes the second run a cache hit *and* still gives the cross-validation guarantee for free (same node, same input hash, same output hash) |
| **Coverage of a 60 s window derived by downloading frames (06-19 20:53)** — 10 minutes, ~345 fragmented ranged reads, ~2 GB, to learn a fact (gap structure) that was already computable from the `moov` tables downloaded on 06-18 and stored as `work/CAM*_frames.json` | ~10 min wall + ~2 GB WAN; the window was then rejected anyway | **Pure cache miss.** `scan_coverage_from_moov` was written 2 minutes later and made the same query free. The inputs (moov tails) had existed for 26 hours in a different session's workspace |
| **`match_seg.py` fetched ~6 GB twice (06-19 04:16 and 04:21)** — the first fetch for window `[5438,5468]` was wasted because the switcher cut had been mis-located by 29 s | ~6 GB WAN + ~5 min | Partially cacheable: the *fetch* is keyed by (feed, t0, t1) so the second window is a genuine miss, but the crash-at-montage that left `seg_state.json` stale forced a third derivation — which is exactly what the hand-rolled `REUSE=1` flag was invented to avoid |
| **Full LTX-2 environment staged from scratch at least 4 times** — 06-19 19:14 (74 GB), 06-20 07:14 (74 GB, after the ephemeral-disk wipe), 06-23 04:48 (74 GB, fresh H100), plus fresh H100 pods on 06-26 (×2), 06-27, 06-28 (×3). Weights: `ltx-2.3-22b-distilled-1.1` 46 GB + LoRA 7.6 GB + upscaler 1 GB + Gemma 23 GB | **≥300 GB of redundant model download**, ~5 min each with `hf_transfer` (~20 min without), plus paid GPU time spent downloading. Ambiguity: `main-b` 06-23 04:41 describes the H100 pool as booting fast with "cached weights", so some later stages may have been partially warm — the record does not say definitively | The weights are *the* canonical content-addressed object: fixed repo, fixed commit `7809842`, fixed filenames. Every stage after the first is a pure cache miss caused by ephemeral disk (`volumeInGb: 0`) |
| **Tool scripts recreated from memory (06-20 07:14)** — `feeds.json`, `iso_fetch.py`, `slice_window.py` (the intricate moov-table ranged slicer) had to be rewritten because the pod disk was wiped | ~10 min, plus correctness risk | A content-addressed store makes this a hash lookup, not an act of recall |
| **daVinci weights re-downloaded after `hf download --include 'a' 'b' 'c'` silently dropped patterns** | 29 GB re-fetch | Deterministic; a content-addressed fetch verifies by hash rather than trusting a "complete" message |
| **torch reinstalled 4 times across 3 sessions** — `w-a119` 2.9.1→2.11.0 (torchvision side-effect, broke torchaudio); `w-dbe3` 2.9.1→2.12.1 (audio-separator side-effect) → back to 2.6.0+cu124; `w-d595` 2.9.1→2.12.1 (torchao side-effect) → back to 2.9.1+cu128 (~2.5 GB); `w-5a7d` 2.9.1→2.11.0 | ~10 GB of wheel churn, ~30–40 min cumulative | An environment lockfile keyed by content hash makes "which torch does this node require" a fact, not an archaeology exercise. Note the *same class of failure* (a transitive dep silently bumping torch) recurred in three independent sessions on the same day |
| **27 GB of LTX-Video 0.9.5 downloaded, then deleted (06-19 20:47)** — the wrong model family entirely; it has no audio conditioning | 27 GB + ~15 min + a garbage warmup generation | Not a cache miss but a *provenance* miss: `w-9a6f` had already recorded the correct model identity, and `w-d595` could not see it |

**Subtotal of avoidable bytes: roughly 340–380 GB of redundant model/weight/frame transfer, plus ~38 GB of outright wasted WAN downloads** (7.3 GB Chrome download, 4.0 GB truncated zip, 27 GB wrong model) — much of it over a Verizon 5G uplink.

## 6.2 Invalidation cascades — work redone because an upstream input turned out to be wrong

Ordered by magnitude.

**(1) The audio-source / speaker-assignment cascade (06-19 16:11–17:46).** One wrong constant — using CAM2's embedded room mic instead of the per-speaker ISO, plus a wrong speaker assignment and a 171 ms chunk-boundary offset — invalidated:
- the FILM 1 s holdout's lip-sync framing (`w-5a7d`, ~1 h A100),
- gen-pipe's entire LatentSync evaluation arm (`w-9ee9`, G0a/G0b, ~0.5 h A100),
- all three held-shot deliverables (`w-a119`, ~50 min A100 including a 10-minute LatentSync run that **lip-synced Eric to Mel's words**),
- the reaction-cut clip's audio bed.

Main-a's own words: *"All held-shot demos scrapped rather than patched."* Estimated **≥2 GPU-hours plus ~4 hours of worker wall-clock**, and — worse — a full day of *judgments* that had to be re-formed. The method survived; only the audio did not. **A content-addressed graph makes this the cheap case**: the `+350.60 s` constant is one node's output; re-running only the descendants that consumed it would have rebuilt the held shots in minutes instead of scrapping them.

**(2) The `int(23.976) = 23` cascade.** Discovered downstream on 06-21 00:06 (in the NOPOP concat) and *again upstream* on 06-28 16:31 (in `ltx_pipelines.utils.media_io.encode_video`). Between those two dates, every clip encoded through the LTX-2 path — `v_k3_s10`, `v_k6_s10`, `gen_shot03`, `gen_ltx2_eric`, and the sharpen variants — played 4% slow with audio drifting. Consequence: **a week of "lip-sync" and "playback" quality judgments were made on defective containers**, including the k3-vs-k6 human comparison on 06-25 and the sharpen sweep. The 06-21 fix was applied to the assembly only; the same defect was re-diagnosed seven days later at its source. **The same bug class was paid for twice.**

**(3) The leaky-mask cascade (06-29 13:20).** Three levers were killed on 06-28 using `score_bg_quality` with the MediaPipe selfie segmenter: unsharp+grain, 4K-native generation, and super-resolution. On 06-29 the u2net matte showed the selfie mask was falsely tagging desk/papers as person, costing ~9 points of valid background and understating detail_ratio (0.538 vs **0.601**). The qualitative verdicts probably survive; **the magnitudes on which the 4K-vs-1080p delivery decision rests do not.** The re-baseline sweep was proposed at 13:20 and **never run** — so as of 07-04 the project's headline number is known-wrong and unfixed. Cost if run: "a few minutes, local/CPU, no pod."

**(4) The anchor-density reversal (06-23 → 06-28).** The KFSTRIDE sweep, the variant scorer, the four comparison clips, the S3 review page, and the user's eye-check all converged on k3 (31 anchors). Five days later the splice-vs-real motion benchmark showed dense anchoring *is* the pose-to-pose dissolve, and production switched to sparse 2-endpoint. The sweep was not wasted (it produced the harness and the landmark scorer), but **the deliverable it selected was reversed**, and the intervening sharpen work (K4) was built entirely on the losing branch.

**(5) The corrupt-frames hypothesis (06-18 19:38 → 19:52).** One hour, one montage, one wrong mental model — cheap, and correctly caveated at the time.

**(6) The `±10 s` xcorr offsets.** Produced a phantom "6.05 s all-camera gap at 3619.9 s" that persisted in the record until 06-19 02:04. Every coverage number computed before the jam-sync solve had to be recomputed.

## 6.3 Aborted or destroyed GPU runs

| Event | Wasted compute | Cost |
|---|---|---|
| 4K 145-frame generation OOM after **~70 min** on an H100 (06-26 18:00) | 70 min H100 | **≈ $3.84** |
| Lossless export crashed on a chunk generator; the gen had **completed** (`GEN_DONE secs=544`) but nothing was saved; the watcher polled only for the success marker so the pod idled **~50 min** | 544 s of successful compute discarded + 50 min idle | **≈ $2.75 idle + ≈ $0.50 discarded** |
| `TilingConfig()` `TypeError` with no per-job try/except → **the whole batch died**; the Mel generalization and long-gap stress jobs never ran (06-27 00:11) | 2 queued jobs lost, pod idle (crash check required `proc==0`, never triggered) | unquantified, ~1 pod-hour class |
| 95-keyframe Option B run aborted at 105 s/step (06-19 21:40–21:50) | ~10–15 min A100 | ≈ $0.30 |
| `bench_sparse_4k` truncated — pod deleted mid-`scp` (06-28 17:08) | full 4K generation lost, re-run 06-28 19:26 | ≈ $0.50–1.00 H100 |
| daVinci 540p OOM (base stage completed, died at SR) + 1080p OOM (base **and** SR completed, died at decode) | 2 full runs, ~10 min A100 | ≈ $0.26 |
| Three secure-cloud A100-PCIe pods hung 13–20 min at `runtime:None`; the race script timed out **without deleting the losers** | 3 dead pods leaking | ≈ $0.90+ |
| Duplicate build chains raced on the same output dirs, deleting/recreating `final_9s` mid-write (06-19 22:08) | one clean rebuild | ~15 min |
| Interpolation eval run twice (CAP=12 on broken flow: 1m29.5 s wall / **37m6s CPU**; then CAP=10 after the uint8 fix) + the RAFT proxy regenerated (48.7 s wall / 8m55s CPU) | ~46 min CPU | negligible $ but real wall-clock |
| gap 22 → 23 re-standardization (`w-9ee9` 05:02): **all four prior methods re-run** (2 baselines + 2 LatentSync passes) because LTX's `8n+1` constraint was discovered after the harness was built | ~15 min GPU + full re-eval | ≈ $0.40 |
| Evidence stills / filmstrips: 2 unreadable formats, 1 re-crop with forced cache invalidation (`a-8d6b`) | ~10 min | negligible |

## 6.4 The single largest line item: the watchdog

**~8 hours of wall-clock (06-19 22:32 → 06-20 06:52)**, during which:
- 9 start-poll-stop retry cycles over ~2 h cost **~$1.57**,
- one stuck pod billed **$1.55/hr for ~25 min** while useless (≈ $0.65),
- a fresh A100-PCIe in Romania rented and was terminated at 16 min for "not booting",
- the user was sent to hunt for a nonexistent RunPod setting (Misdiagnosis 1),
- ~2.5 h were spent building and running a retry loop for a scarcity problem that did not exist (Misdiagnosis 2),
- and the actual cause was a cost-saving tool reaping pods that were **reachable and healthy** (`idle 21m >= 20m: stop`, `reachable=True`) because the CPU-only structure phase looked idle.

Direct GPU cost is small (~$2.50) precisely because the pods kept being stopped. **The loss is the 8 hours and the two false diagnoses.** A content-addressed system would not have prevented the watchdog, but the checkpointing it forced (`skip any shot whose gen.mp4 exists`) is exactly the mechanism such a system provides by default — and would have made each reap cost seconds rather than a restart.

## 6.5 Totals and the counterfactual

| Category | Quantified |
|---|---|
| Redundant model/weight staging | **≥300 GB**, ≥4 full re-stages |
| Outright wasted WAN transfer | **~38 GB** (7.3 + 4.0 + 27) |
| Redundant frame fetching | ~8 GB (6 GB mis-windowed + ~2 GB rejected 60 s slice) |
| Directly quantified wasted GPU spend | **≈ $9–11** of a ~$81 total through 06-25 (~12%) |
| Wall-clock lost to self-inflicted infrastructure | **~8 hours** (watchdog) + ~2 h (pod hangs/restarts on 06-23) + ~16 min (LatentSync env archaeology) |
| Judgments invalidated and re-formed | 3 major cascades (audio source, fps container, leaky mask) + 1 full reversal (anchor density) |

**The counterfactual worth stating plainly:** almost every expensive redo in this record traces to one of three things a content-addressed experiment graph directly addresses.
1. **Outputs were not addressable, so they were recomputed** — the all-camera census twice, coverage-from-frames when coverage-from-moov already existed, weights re-downloaded every pod.
2. **Provenance was not attached, so wrongness propagated invisibly** — the room-mic audio, the 23 fps container, and the selfie mask each contaminated every downstream artifact with no way to enumerate the affected set. The one place provenance *was* added deliberately (the `masker` field in the score JSON, 06-29 13:17) is the one place the contamination was caught and bounded in minutes.
3. **State lived only in a transcript** — which grew to 27 MB, killed its own session on a false-positive safety classifier, and had to be reconstructed by a 7.2-minute subagent pass over a 778 KB digest, after the on-disk notes proved a week stale. `RELIABLE_PROGRESS.md` is a hand-built, lossy substitute for the graph.

Counter-evidence, in fairness: caching *was* used ad hoc and it worked every time it was reached for. `rescore.py` recomputed motion on already-decoded frames at zero network cost; `REUSE=1` rebuilt editorial state without a 6 GB re-fetch; SMOOTH and NOPOP were both built from existing per-shot `gen.mp4` files with **no regeneration at all**; per-shot checkpointing turned a mid-run reap into a resume. Each was invented locally, in-session, under pressure, and none of them transferred between sessions.

---

# 7. THE HUMAN DECISION POINTS

These are the nodes with no reproducible input→output function. I have split them by *what kind* of authority the human exercised, because they load-bear differently in an authorship model.

## 7.1 Constitutional — constraints that outlived every rebuild

| When | Decision | Effect |
|---|---|---|
| 06-19 06:32 | **"assembled_audioled is horrible... There should NEVER be a dissolve. Not ever. There should never be a sudden change in body posture."** | Reduced the transition alphabet from {cut, dissolve, freeze} to {cut, freeze}, then effectively to {cut}. Every later build — reaction cut, 11-shot edit, NOPOP — is hard-cuts-only. Verified frame-by-frame at delivery on 06-20 |
| 06-19 18:38 | **Rejects lip-only relipping** after a research pass recommended it (~95% real pixels by construction): lips on a frozen body reads as dead eyes, no blink, no breath | Redirected the entire model search from mouth-region models to full-frame audio-driven models. Without this the project ends at LatentSync |
| 06-26 04:36 | **Scope lock** (via AskUserQuestion): ~90+ min runtime, "holds up to scrubbing", 4K ProRes 422 HQ delivery, build-from-scratch now with graded frames later | Made the 4K quality test a prerequisite for any quote; is why 4K was tested at all |
| 06-26 20:37 | **"Acknowledging error and accepting responsibility is of little use if it fails to lead to permanent vigilance."** | Six override-binding GPU cost guards written into `~/.claude/CLAUDE.md`, each tied to a specific real loss. Governed every subsequent pod operation |

## 7.2 Perceptual verdicts — a person looked at pixels and ruled

| When | Judgment | What it overturned or confirmed |
|---|---|---|
| 06-19 16:11 | *"The audio clip for each of the videos has the man talking, but in none of the videos he is talking (even for the normal motion segments)."* | **The highest-value observation in the record.** Caught a three-fault sync defect that no automated check had flagged; invalidated a full day of lip-sync judgments |
| 06-19 17:41 | *"proof_eric_sync is in sync"* | First unambiguously-correct artifact after the reset; unblocked everything built on top |
| 06-19 17:46 | *"Neither Mel clip is useful"* (CAM3 34% real, CAM1 6% real) | Forced a third attempt on CAM5 — which, being the *other* sync group, doubled as the cross-group validation of +350.60 s |
| 06-20 21:56 | *"Consider the difference between 151 and 152, both supposedly original frames, but there is a significant jump."* | Located the real→gen seam by frame number; the answer (MAE 0.07 vs 3.46) unified every reported artifact into one root cause and also exposed an off-by-one in the diagnostic overlay |
| 06-21 00:02 | *"It literally looks like slow motion"* | Rejected the QuickTime-decoder hypothesis and forced measurement of actual frame timestamps → the 23.000 fps defect |
| 06-21 01:31 | *"frames bounce back and forth like they are backwards in pairs"* | Diagnosed a B-frame display-order scramble from a verbal description alone |
| 06-25 20:03 | *"face expression is significantly better on k3... if we can get the image quality of k6 and the face quality of k3"* | Chose k3 over the metric-tied recommendation and set the entire sharpen sub-investigation in motion |
| 06-25 20:20 | *"resolution is necessarily reduced in a quad contact sheet... the issue looks less like sharpness and more like compression blocking"* | **Rejected the evaluation medium, not the result.** Led directly to the crf-19 / 2.8 Mbps discovery and the lossless ProRes pipeline. Also established a standing rule: no grids, full-res 1:1 crops only |
| 06-26 21:07 | *"strange grid like rippling structure... appearing suddenly midway"* | Described a temporal onset pattern precisely enough that source reading later matched it exactly to an 80−24 = 56-frame chunk stride |
| 06-27 00:00 | *"the gridding problem is starting to look systemic... we need to do some more research"* | Stopped generate-and-look and forced a source + literature investigation |
| 07-01 20:34 | *"Create a matte overlay of the segmentation so that I can visually check it"* | Refused to accept numbers and single-frame masks as sufficient evidence for edge behaviour under motion |

## 7.3 Method origination — the human supplied the idea, not just the verdict

| When | Contribution |
|---|---|
| 06-19 02:46 | **Challenged the index itself**: might the moov be under-reporting frames that were actually written? Produced the raw-`mdat` `icpf` scan — control run 8/8 at predicted offsets, zero hidden frames in the 6 longest gaps. Removed the "faulty index" failure mode from the risk list |
| 06-19 16:42 | **"do the materials include a cut?"** — a question, not an instruction. It uncovered the never-opened `Premiere File with Proxies & Synced Audio` folder containing the editor's `.prproj` and both 2 GB ISO WAVs. **This single question replaced three days of reverse-engineering sync from camera room mics and supplied the authoritative +350.60 s constant** |
| 06-20 21:32 | **Specified the ffmpeg rate-of-change metric** as the way to localize "abrupt pose changes and image quality changes I'm seeing". The resulting YDIF series nailed all 10 cuts at exactly the predicted frame indices |
| 06-28 18:00 | **Originated the ground-truth fitness function**: *"we have a ground truth source for the background so if we use SAM3 to select all of Erik out what remains is only the foreground mic and desk and the background... you should be able to drive generation and other features (up rez, denoise, etc) to achieve the closest quality match."* This converted quality from opinion into a measurable objective and immediately killed three levers |
| 06-29 13:14 | **"We need simple full body segmentation for this new goal"** — cut off a SAM2-vs-SAM3 investigation by correctly reframing the requirement |

## 7.4 Process and steering corrections

| When | Correction |
|---|---|
| 06-18 18:50 | *"I have no Dropbox account"* — collapsed the rclone branch in one line |
| 06-18 19:45 | Manually cancelled a runaway 94 GB Chrome download (⌘⇧J) that the agent could not stop |
| 06-19 19:19 (`w-d595`) | **Human-in-the-loop license grant** — clicked "Agree and access" on two gated HuggingFace repos (`google/t5gemma-9b-9b-ul2`, `stabilityai/stable-audio-open-1.0`). A hard blocker no amount of engineering could route around |
| 06-19 21:20 → 22:20 | After the 9.55 s fill failed, **reframed the deliverable** from a flat single-camera recovery to a multi-camera edit with fills only in residual gaps |
| 06-20 05:31 | **"you've been creating the failure mode! ... Don't time out ... Give a pod the chance to book."** Removed all boot timeouts |
| 06-20 07:00 | **"how are you going to know when it's actually running?"** — forced replacement of unreliable `pod list` JSON parsing with a real SSH + `nvidia-smi` check, which immediately surfaced the ephemeral-disk discovery |
| 06-23 04:14 | *"not much happening. I think you need to have more meaningful logging or progress metric."* → the 60 s heartbeat |
| 06-26 22:24 | **Autonomous mandate**: *"simply continue on all of this in an organized way until we have closed all unknowns, or you are unable to proceed without my intervention"* → the 8-task exploration board that structured three days |
| 06-29 12:50 | *"That sounds like old state... You should have a subagent read through the session log and provide point by point progress log that we can rely on."* Rejected a stale sitrep and specified the reconstruction method |

## 7.5 Model-perceptual judgments — semi-reproducible, and worth separating

A distinct class: an agent looked at an image and formed a judgment that was neither a metric nor a human call. These are reproducible in principle (same pixels, same model) but were treated as authoritative and were **wrong at least twice**:

- **Correct and decisive:** reading `sw_strip.jpg` directly to establish the switcher was on the woman for the entire window, after an automated brightness classifier had emitted a confidently wrong timeline (`a-8d6b` 16:36).
- **Correct and self-correcting:** the "hallucinated shirt text" claim, retracted when a hard crop of the 4K reference showed the text is genuinely printed on Eric's shirt (`w-dbe3` 19:28).
- **Wrong for 45 minutes:** "LongCat uses the reference as a soft appearance condition" — an inference from 480p output, overturned by source reading *and* a 720p empirical check (`w-dbe3` 19:09 → 19:54).
- **Wrong and expensive:** "same-person cameras drop together" (`w-5a7d` 04:34, window-local) versus "essentially independent, even slightly anti-correlated" (`main-a` 20:46, whole-file). **This contradiction was never resolved in the record.**

---

# 8. AMBIGUITIES AND UNRESOLVED ITEMS IN THE RECORD

Stated explicitly, as requested.

1. **Same-person camera correlation.** `w-5a7d` 04:34 and `main-a` 04:55 report woman-union 48.1% / man-union 55.4% in the `[5468,5498]` window; `main-a` 20:46 reports CAM2+CAM4 union 42.9% vs an independent-union ideal of 42.2% over the whole file. Both are probably right at their own scale (local bursts correlate; global rates do not), but the record never reconciles them, and the *first* number drove the "generative is the only lever" reframing while the *second* improved the economics. Unresolved.

2. **Spend figures disagree.** `main-a` 06-19 21:17 gives 06-19 = $30.10 and ~$60 total over 38.1 pod-hours; `main-b` 06-25 gives Jun 19 = $35.05. The difference is plausibly later 06-19 hours billed after the first report, but neither reconciles, and `runpodctl` is noted to report usage rather than deposits. Post-06-25 spend (06-26 through 06-28 H100 work at $3.29/hr) is never tabulated at all.

3. **Whether later H100 pods re-downloaded the full 74 GB.** `main-b` 06-23 04:41 describes the H100 pool as booting in 15–30 s with "cached weights", but 04:48 also describes a full `restage_ltx2.sh` run with weights climbing 46.2 → 61.2 → 79.2 GB. My ≥300 GB redundant-download figure assumes ≥4 full stages; the true figure could be higher or somewhat lower.

4. **4K: better or worse?** `main-a` 06-19 20:37 measures the 4K same-instant render gap at 2.43 vs a normal frame step of 2.04 — the best result on the resolution ladder. `main-b` 06-28 19:26 measures 4K-native generation as worse on every masked-background metric (detail_ratio 0.375 vs 0.522). These are different metrics on different quantities (full-frame MAE against the *same* real frame vs background-only Laplacian ratio), and both may hold — but the project's delivery decision rests on the second, which 06-29 then showed was measured through a leaky mask. **The 1080p-vs-4K question is open.**

5. **The five-concurrent-sessions claim.** Nine sessions have 06-19 activity; the maximum simultaneous count the timestamps support is four (19:06–20:01). See §0.

6. **`w-c76c`'s "pod outage" at 02:06.** Presented in that session as an infrastructure failure requiring 9 minutes of escalating diagnosis; revealed at 02:15 to have been a deliberate `runpodctl pod stop` by the lead. Similar pattern on 06-19 20:48 (`w-dbe3`) and 06-19 16:50 (`a-8d6b`). **Three separate workers burned time diagnosing intentional shutdowns** — an observability gap, not a technical failure, but it recurs often enough to be structural.

7. **Never completed:** the clean-matte re-baseline sweep (proposed 06-29 13:20, "a few minutes, local/CPU"); a human scrub verdict on the matte overlay clips (rendered 07-01, never adjudicated); the composite-over-real-plate demo (the locked recipe's central claim, never built); the client review before/after clip specified on 06-19 20:37 and superseded within minutes; the padded quote; and any execution of the 06-28 production recipe end to end.

8. **The delivered artifact and the locked recipe disagree.** `eric_60s_edit.mp4` / `eric_60s_NOPOP.mp4` were built with dense-ish multi-keyframe anchoring, a real+gen composite (later replaced by pure-gen), 1080p generation, and crf-18 h264. The 06-28 recipe specifies sparse 2-endpoint anchoring, compositing over the byte-real 4K plate, and lossless ProRes at 24000/1001. **Nothing has been built to the locked recipe.** Any statement of "the final pipeline" must say which of the two it means.

---

# B. Prior-art survey

# Prior Art: Workflow / Pipeline Definition Languages

## 1. Make / Ninja — the origin

**Right:** The whole idea. A rule is `output ← inputs + command`; the graph is *inferred* from that relation rather than authored. Ninja's contribution was proving the definition language should be a *compilation target*, not a human surface — it deliberately removed expressiveness so scheduling could get fast.

**Costs:** The rebuilder is timestamp-based (`make = topological modTimeRebuilder` in the *Build Systems à la Carte* taxonomy). No content addressing, therefore no early cutoff, no sharing between machines, no correctness under clock skew. Recursive make fragments the graph into non-composable subgraphs.

**Variant:** Badly. The only mechanisms are pattern rules, `VPATH`, and `$(MAKECMDGOALS)` — all of which encode the variant *in the output path*, or you set a variable and the entire tree silently rebuilds in place, clobbering the previous variant. Two variants cannot coexist without a hand-rolled `build/$(CONFIG)/` convention. This is exactly the mistake everyone downstream inherited.

## 2. Nextflow / Snakemake / CWL / WDL — the science tier

These are the only community that treated *fan-out and resumability* as first-class language problems, so they're the richest source.

**Nextflow.** Dataflow channels, not a declared DAG — the graph emerges from channel connections. Each task gets a 128-bit hash over input values, input file content, and the command string; `-resume` skips anything whose hash is unchanged. That's a genuine content-addressed rebuilder in a domain-specific language. **Cost:** the work directory is write-once garbage that grows without bound, and there's no way to invalidate a *specific* process — you get all-or-nothing resume ([nextflow#6149](https://github.com/nextflow-io/nextflow/issues/6149)). **Variant:** mechanically excellent (`combine` gives you a Cartesian sweep in one operator) but *identity-poor* — the result of a variant is a hash directory nobody can name, cite, or diff. You can produce a thousand variants and address none of them.

**Snakemake.** Wildcards are the sharpest single idea in this whole survey: the variant lives *in the output path pattern*, so the filename **is** the variant key, and the graph is resolved backwards by matching a requested output against rule patterns. That makes a parameter sweep a glob, and makes "give me just this one variant" a first-class request — which Nextflow structurally cannot do. **Cost:** invalidation is timestamp-based by default (richer `--rerun-triggers` on params/code/env are opt-in and newer), and encoding parameters in filenames doesn't scale past ~3 dimensions before the paths become unreadable and the escaping rules bite.

**CWL.** Declarative, JSON/YAML, engine-independent, with real scatter semantics: `dotproduct`, `flat_crossproduct`, `nested_crossproduct` — the correct three-way distinction between zip, sweep, and structure-preserving sweep. **Cost:** ruinous verbosity, and everything interesting is gated behind opt-in requirements (`ScatterFeatureRequirement`, `SubworkflowFeatureRequirement`), so "valid CWL" doesn't mean "runnable here." Caching isn't in the spec at all.

**WDL.** Deliberately less expressive than CWL, more readable; caching (Cromwell "call caching") is an *engine* feature, not a language feature — which is why WDL portability claims don't survive contact with a second engine. Nested scatter wasn't supported in draft-2 at all.

**Variant, for all four:** within a run, scatter/wildcards are solid. *Across* runs, all four fail identically — a variant is a new inputs JSON with no recorded relationship to the previous one. Nothing in the format says "this is the same graph with `--denoise 0.4` instead of `0.3`." The provenance of a variant lives in someone's lab notebook.

## 3. Bazel / Buck — action graphs and content addressing

**Right:** The strongest technical model in the survey. Actions are content-addressed (`bazel = restartingQ ctRebuilder` — *constructive traces*, which is what enables a shared remote cache and remote execution, not just local skip). Hermeticity is enforced rather than hoped for. Buck2's Starlark-defined rules pushed this further by removing built-in language knowledge from the core.

**And crucially — Bazel is the one system that models variants structurally.** The unit isn't a target, it's a **configured target**: the pair `(target, configuration)`. Change one flag and you get a *different node with a different cache key*, coexisting with the original, both addressable, both queryable via `cquery`. Configuration transitions let one build produce many variants of the same subgraph in a single invocation. This is the correct answer, and it is the thing almost nobody else has.

**Cost:** it's expensive in exactly the way that matters. Every configured target is a distinct in-memory object over the whole transitive closure, so cache reuse *drops* as variants multiply. Worse, transitions historically poisoned cache sharing outright — Bazel recorded an internal "affected by Starlark transition" marker that made configurations distinct even when the resulting flags were identical, requiring `--experimental_output_directory_naming_scheme=diff_against_baseline` to compute the real difference instead. **Lesson for a new design: variant identity must be derived from the *effective* parameters, never from the *history* of how they were set.** And the whole edifice demands total input declaration, which media tools (Nuke, Houdini, Resolve, anything with a global prefs file or a license daemon) will not give you.

## 4. Airflow / Dagster / Prefect — DAG orchestration

**Airflow.** Right: scheduling, retries, backfill windows, operational visibility — the boring production layer everything else underrates. Cost: it models *task execution*, so the answer to "did it work" is "the script exited 0," not "the data is correct." Data lineage is bolted on. Dynamic fan-out didn't exist until dynamic task mapping in 2.3. **Variant:** effectively a copy-paste of the DAG, or `params` smuggled through a trigger config with no record linking the two runs.

**Dagster.** The important reframe: the unit of definition is the **asset** (the table, model, file that should exist), not the task that makes it. You declare the desired end state; the system derives the execution. That flips the question from "did the job run" to "is this artifact current," which is the same move a content-addressed system makes and the reason Dagster is the most relevant orchestrator here. **Variant:** the best in the orchestration tier — `PartitionsDefinition` makes the variant an *axis of the asset itself*, and `MultiPartitionsDefinition` crosses two axes; a sweep is a backfill, and every cell is independently addressable and independently materializable. **Cost:** multi-partitions are capped at **two dimensions**, partition keys must be enumerable up front (bad for continuous or open-ended parameters), and the whole thing is Python-object-defined — the "format" is a running Python process, so there is no artifact you can diff, sign, or hand to another tool.

**Prefect.** Right: workflows are ordinary imperative Python; the graph is discovered at runtime, so genuinely dynamic control flow is trivial. Cost: no static graph means nothing can be reasoned about, costed, or verified *before* execution, and there's no definition artifact at all.

## 5. Nuke / Houdini HDA / ComfyUI — media-native node graphs

**Right:** These won the artist. Direct manipulation of a node graph, live preview at every node, immediate feedback. Any format that can't be *seen and dragged* loses to these regardless of technical merit.

**Houdini deserves separate credit** and is the most underrated prior art in this list. HDAs are namespaced, **versioned** assets (`namespace::name::version`) with a declared parameter interface — a real reusable component model. And PDG/TOPs has an explicit primitive for the exact problem being targeted: the **Wedge** TOP generates one work item per variation of specified attributes, work items carry those attributes as data, and nodes cook from disk cache when expected outputs already exist. That is a media-native parameter sweep with variant-level caching, shipping since 2018. Cost: it is 100% Houdini. The graph, the cache, the scheduler, and the identity scheme are all inside one commercial DCC.

**Format-lock, generally:** a `.nk` script is a serialization of Nuke's *node classes and knob names* — it is a memory dump with a text syntax, not an interchange format. ComfyUI's workflow JSON is worse: nodes carry `class_type` plus `widgets_values` **positionally**, so a custom node that inserts a widget silently reindexes every parameter, and the JSON references models and custom nodes it does not carry. These formats are locked because **they describe the tool's internal object graph rather than the work being done** — the identity of a node is "a thing my host app can instantiate," so the format cannot outlive the app version.

**Variant, all three:** File > Save As. The variant model of the entire media industry is a copied file with `_v002_final` in the name and no machine-readable relationship to its parent. This is the gap.

## 6. OpenTimelineIO — the one that got adopted

**Why it worked**, and each reason is a design instruction:

1. **Radical narrowness.** It describes editorial time — cuts, tracks, transitions, time transforms. It does not describe color, effects, or compute. It solved one problem completely instead of ten partially.
2. **It does not own the media.** A clip holds a `MediaReference` — a pointer. OTIO never becomes the custodian of your bytes, so adopting it costs nothing and risks nothing. (It also allows *multiple* references per clip, the closest thing in adopted media formats to a variant slot.)
3. **Plain JSON with real schema versioning.** Files from old versions are auto-upgraded on read, and new versions can *downgrade* on write for older consumers. Bidirectional compatibility is what let vendors ship adapters without fear.
4. **Adapters, not conversion mandates** — it sat *beside* AAF/EDL/XML rather than demanding replacement (the mistake OMF/AAF made).
5. **Neutral governance** (ASWF/Linux Foundation), so no vendor had to bet on a competitor's format.

**Variant:** none — there's no compute model, so nothing to vary. But note that OTIO's success came precisely from *not* attempting execution semantics.

## 7. MovieLabs OMC — the industry's own graph

**Right:** It correctly identifies that the domain nouns are shared and unnamed, and names them — Creative Work, Asset, Participant, Task, Context — with relationships as RDF predicates, published as Turtle (`omc.ttl`, `cw.ttl`) *and* a JSON Schema (`OMC-JSON-v2.8.schema.json`) in recognition that nobody will adopt RDF. It's the only serious attempt at a media production graph model with studio backing, and its vocabulary is worth stealing.

**Cost:** ontology-first. It's a *description* language with no execution semantics, no reference implementation people build workflows on, no identity/verification story for the bytes, and adoption explicitly left voluntary — MovieLabs states it obligates no market participant. Five years and eight point-releases in, it's a vocabulary that ships in slide decks. **Variant:** modeled ontologically (assets relate to other assets; you can *say* one is derived from another) but unverifiable and uncomputable — nothing recomputes the claim, so a variant relationship is an assertion, not a fact.

---

## Who actually does variants well

| System | Variant mechanism | Grade |
|---|---|---|
| **Bazel** | Configured target — `(target, config)` is the node identity; variants coexist, are queryable, cache independently | **A** — the reference answer |
| **Dagster** | Partitions as an axis of the asset; sweep = backfill; each cell addressable | **A−** — capped at 2 dimensions, keys must be enumerable |
| **Houdini PDG** | Wedge TOP: one work item per attribute variation, with per-item disk cache | **B+** — right model, totally host-locked |
| **Snakemake** | Wildcard in the output path; the filename *is* the variant key | **B** — elegant, doesn't scale past ~3 dims |
| **Nextflow** | Channel `combine` for the sweep | **C** — sweeps well, but variants get anonymous hash dirs |
| **CWL / WDL** | `flat_crossproduct` within a run | **C−** — no identity across runs |
| **Airflow / Prefect** | `params` or a copied DAG | **D** |
| **Make / Nuke / HDA file / ComfyUI / OTIO** | Copy the file | **F** |

The honest summary: **only Bazel and Dagster treat "same graph, one parameter changed" as a first-class entity with its own identity.** Both come from software/data, neither is usable in a media context, and neither produces a plain-text artifact you can diff or sign. Media's entire answer is `_v002`.

---

## The 5 design mistakes most commonly made here

1. **Modeling the run instead of the artifact.** Airflow, Make, CWL and every DCC all describe *executions*. The user's question is never "did the task run," it's "does this output exist, is it current, and what produced it." Dagster's asset reframe is the correction, and it's the single highest-leverage decision in the space.

2. **Variant-as-copy.** The default reflex is duplicate-the-graph-and-edit. That destroys the relationship between variants at the moment of creation, so lineage must later be reconstructed from filenames and memory. If a variant isn't an *addressable identity derived from (graph, parameters, input content)*, sweeps are expensive, comparison is manual, and provenance is fiction.

3. **Binding identity to the host application's object graph.** `.nk`, `.hip`, and ComfyUI JSON are serializations of in-memory tool state, which is why they're format-locked and version-brittle (ComfyUI's positional `widgets_values` is the purest case — insert one widget upstream and every parameter silently shifts). OTIO's inversion — describe the *work*, point at the bytes — is why it's the one that crossed vendors.

4. **Timestamp or path invalidation instead of content addressing.** Make and default Snakemake are wrong under clock skew, touch, restore-from-backup, and any shared cache. And without content-addressed traces you can't get **early cutoff** — the property that a recomputed-but-unchanged intermediate stops the cascade. In media, where a re-render often produces identical bytes, early cutoff is worth more than parallelism.

5. **Ontology-first / completeness before usefulness.** OMC and CWL both front-loaded modeling the whole domain. The result is a spec that's correct and unused, because nothing is useful until everything is specified. OTIO shipped a fifth of the model and won. Corollary mistake: making the definition language the human surface *and* the execution target at once (Ninja got this right by refusing to be authored; Bazel's Starlark and Buck2 got it right by separating rule definition from action graph).

## The 3 things a new entrant must get right

1. **A variant must be a name, not a copy.** Identity derived deterministically from the graph plus the effective parameters plus the content IDs of inputs — so a sweep is free, "give me only this cell" is answerable, two variants coexist without collision, and the diff between them is one line of plain text. Take Bazel's configured-target model, and heed its scar: derive the key from the *effective* parameter values, never from the *history* of how they were set, or transitions will fragment your cache the way Bazel's did.

2. **Content-addressed, verifiable, plain text, with early cutoff.** Every claim about what produced what must be recomputable rather than asserted (OMC's failure), invalidation must be content-based rather than timestamp-based (Make's failure), and the artifact must be diffable and mergeable line-locally so it lives in git beside the work. Constructive traces are what make the cache shareable across machines — build for that from the start, because retrofitting it into Nextflow-style opaque work dirs is not possible.

3. **Beside, not instead — never own the bytes or replace the tool.** OTIO's `MediaReference` is the model: reference immutable content by ID, let Nuke stay Nuke and Houdini stay Houdini, make adoption incremental and reversible. Related and non-negotiable: **partial knowledge must be a valid state.** Nine of ten variants materialized with the tenth referenced-but-absent has to be a first-class, useful condition — CWL's requirement-gating and Bazel's hermeticity demand both punish incompleteness, and neither would survive a real facility.

**Sources:** [Build Systems à la Carte](https://www.microsoft.com/en-us/research/wp-content/uploads/2018/03/build-systems.pdf) · [Bazel Configurations](https://bazel.build/extending/config) · [Bazel Glossary](https://bazel.build/reference/glossary) · [bazel#19917 (transitions and cache fragmentation)](https://github.com/bazelbuild/bazel/issues/19917) · [Nextflow cache & resume](https://training.nextflow.io/2.1/basic_training/cache_and_resume/) · [nextflow#6149](https://github.com/nextflow-io/nextflow/issues/6149) · [CWL Workflow spec — scatter methods](https://www.commonwl.org/v1.0/Workflow.html) · [Cromwell language support](https://cromwell.readthedocs.io/en/stable/LanguageSupport/) · [Dagster: What Are Software-Defined Assets?](https://dagster.io/blog/software-defined-assets) · [Dagster partitioning assets](https://docs.dagster.io/guides/build/partitions-and-backfills/partitioning-assets) · [Houdini Wedge TOP](https://www.sidefx.com/docs/houdini/nodes/top/wedge.html) · [Houdini PDG file tags](https://www.sidefx.com/docs/houdini/tops/filetags.html) · [OTIO schema versioning](https://opentimelineio.readthedocs.io/en/latest/tutorials/versioning-schemas.html) · [ComfyUI workflow JSON format](https://deepwiki.com/Comfy-Org/ComfyUI/7.3-workflow-json-format) · [MovieLabs OMC repo](https://github.com/MovieLabs/OMC) · [OMC v2.8 Overview](https://mc.movielabs.com/omc/Overview/ML_Ontology_Pt1_Overview_v2.8.pdf)

---

# C. Adversarial review of the first draft

## Sources verified

Three of the five cited foundations are **not on the checked-out branch** — `design/listings.md`, `design/derivation-records.md`, `design/receipt-record.md` exist only on `master` (commits `8130557`, `f52d474`, `ef45377`); the working tree is on `release/1.0.x`. Anyone reviewing this proposal in the repo as it stands cannot see what it claims to derive from. I recovered all three and read them, plus `/Users/joshua/ws/active/software/c4/oss/c4/design/substrate-map.md`, `design/testimony-record.md`, `design/store-records-architecture.md`, and `c4m/SPECIFICATION.md`.

---

# 1. Does the worked example cover the real history?

**No — and it dodges the hardest case in the record.**

**(a) The 11-shot fan-out is missing entirely.** `exact concat shots=c4S01…,…,c4S11…` consumes eleven IDs that **no edge in the document produces and no bind line names**. The delivered artifact — an 11-shot edit — is the project's only shipped deliverable, and its provenance is absent. Writing it honestly costs eleven near-identical `ltx2-gen` lines differing only in `t0`/`t1`/`cam`, and §4 has already banned the fold (`kfstride=[2-6]`) that would compress them. The example avoids the exact structure the history is built on.

**(b) `-` conflates "never attempted" with "attempted and failed."** The grammar gives an edge exactly one output and no failure state. The history contains ≥8 aborted runs: 4K 145-frame OOM at 70 min, the `TilingConfig()` `TypeError` that killed a whole batch, the truncated `bench_sparse_4k`, two daVinci OOMs. Each is *the same character* as the never-run locked recipe. The claim **"Read the last column and the whole state of the project falls out"** is therefore false: the most expensive facts in the record (§6.3) render identically to facts that never happened.

**(c) The 23 fps cascade is described but not enumerable.** The format records `note main-b … Every node downstream of c4Ec0… played 4% slow`. But the defect lives in the *process* (`int(23.976)` in `media_io.encode_video`), not in an input. Fixing it mints a new process ID; there is no `superseded`, no negative testimony, and no way to mark every edge citing the old process object suspect. Answering "what is contaminated" requires transitive closure over edges — which needs the reference index that `store-records-architecture.md` puts **app-layer, later, and never on a destructive path**. The counterfactual the history calls out as failure #2 ("provenance was not attached, so wrongness propagated invisibly") is *described* by this format and *not answered* by it.

**(d) The largest line item is out of scope by the design's own admission.** §6.4 is 8 hours of watchdog; §4 says "the watchdog was never a graph problem." Fine — but then note what's left. §6.5's ≥300 GB of redundant weight staging is solved by the **store plus the SSH peer read-source** (substrate #2, already committed), not by this format. Strip out what the store, journal, and receipt already cover and the graph's exclusive contribution shrinks to: the `+350.60 s` re-key (genuinely excellent), the mask cascade (partial), and the anchor reversal (a note).

**(e) The example violates its own central rule, twice.**
- `validated bench by=main-b at=2026-06-28 subject=… finding=dense-anchoring-is-the-dissolve …` — `finding=` is a **result**, but "every field is an input," so the conclusion is an input to the edge that produced it. Same for `reject=lip-only-relip`.
- That line is `validated`, so rule 3 applies: `key = H(process-ID, sorted resolved fields, minus by/test)`. **`at=` is not exempt.** A timestamp is in the cache key. This edge can never hit. The proposal's own showcase demonstrates the footgun.

**(f) The integrity boundary is breached in the example.** `exact stage-env repo=Lightricks/LTX-2 commit=7809842` — a git SHA and a repo slug are **assigned identities written as bare literals**, not `nom:` tags. `derivation-records.md` is unambiguous: *"silent assignment-as-content is the one lie the system must be structurally unable to tell."* Because `literal = 1*VCHAR` subsumes both `c4-id` and `nom-tag`, the grammar is structurally *unable to prevent* it, and the flagship example commits it on the most assigned line in the document.

**Verdict §2 (ET example): KILL as evidence.** It does not demonstrate the claim it is offered for. Rewrite it with the eleven shot edges written out, a failed 4K run represented, and every assigned identifier `nom:`-tagged — then re-judge whether the format still looks small.

---

# 2. Concept count — five duplications should die

| # | Concept | Existing primitive that already carries it | Verdict |
|---|---|---|---|
| 1 | `@c4 graph 1` kind | listings.md framing | **KEEP** (free) |
| 2 | `class` in column 1 | `class=volatile` field on the receipt edge line | **KILL** — two spellings of one fact |
| 3 | `process` in column 2 | `process=<B>` field (derivation-records Layer 1) | **KILL** |
| 4 | ID-**last** edge line | receipt's shape-A line is ID-**first**, and the `/receipt` render is **PINNED** | **KILL or unpin receipts** |
| 5 | every field is an input | nothing — generalizes `inputs=` | **KEEP — the best idea here** |
| 6 | `by=` | `authored-by` predicate (testimony-record) | **KILL** |
| 7 | `test=` | nothing — `validated` never named its check | **KEEP** |
| 8 | `bind` line | `resolves-to` predicate | **KILL** |
| 9 | `note` line | nothing — and it is a 7th predicate in all but name | **AMEND** |
| 10 | comma-list fan-in | nothing; ordered-vs-set unaddressed | **AMEND** |
| 11 | bytewise sort | c4m uses **natural sort** (`SPECIFICATION.md` §"Natural Sort Algorithm", "files before directories, then natural sort") | **AMEND** |
| 12 | `set` + `@node` scoping | c4m patch algebra | **AMEND / defer** |
| 13 | cycle rule + "unrunnable" | nothing — new validator obligation ×5 | **KEEP, but cost it** |
| 14 | memo-key serialization | `H(tool-object-ID, input-closure-ID)` | **AMEND** — byte encoding unpinned |

**The predicate-budget claim is true on a technicality and false in substance.** Zero predicates are consumed *because the word `produced-from` was deleted from the line*. `grep produced-from` finds receipt edges and misses every graph edge. The budget was preserved by moving the vocabulary into an **unbudgeted field-key namespace** where `by=` and `test=` already have reserved semantics — a second vocabulary with no budget and no home.

**The "Why it costs no new concepts" table is an argument against the format.** Every row names an existing mechanism that already carries the requirement. A table that complete is evidence the *format* is unnecessary, not that it is cheap.

**Verdict on the table: KILL.** Read literally it says: build nothing.

---

# 3. Is this Make/Nextflow/CWL with new syntax?

Mostly yes, and the doc never says so.

- **It is Nix's `.drv` in plain text.** A derivation is `(builder, inputs) → output`; the memo key is `H(process, inputs)`. Nix is discussed at length in `derivation-records.md` and appears **nowhere** in this proposal. A derivation-format proposal that omits Nix is not being straight.
- **It is Ninja's shape** (`build out: rule in…`) with content hashing — and the prior art's own lesson is that *Ninja got it right by refusing to be human-authored*. This proposal takes Ninja's shape and makes it the human surface, then calls the ergonomic form a courtesy.
- **The re-run rule is `topological ctRebuilder`** from *Build Systems à la Carte*, presented as if derived from C4 principles.

**Genuinely new, and worth defending:**
1. **Class in column 1 as a served label** — axis (c), which `derivation-records.md`'s dated scan found unclaimed. Real.
2. **Every field is an input** — collapses params and artifacts into one thing. Snakemake puts params in the path; Bazel splits configuration from inputs; Nextflow hashes both without unifying them. This kills the distinction that made `--rerun-triggers` an opt-in retrofit. **This is the one idea in the document no prior-art system has.**
3. **Plan and run in one document via the nominal rung.** Every system in the prior art separates them.
4. **`transitions=nom:et/transitions`** — a human constitutional rule as an input to a derivation key. Bazel cannot; Dagster cannot; OMC can only assert. *Whatever format wins, this must survive.*

**Verdict: AMEND.** Add a prior-art section that names Nix, Ninja, and *à la Carte* and states plainly which four things are new. Reinvention is fine; unacknowledged reinvention is what makes reviewers stop trusting the rest.

---

# 4. The variant claim, stress-tested

**20 variants:** fine — 20 `set` lines, better than any prior art. Credit given.

**Variant of a variant:** works, and works for the right reason. `set(set(base,k=6),k=3)` re-runs the outer `set` (different key) but produces the same canonical graph text, hence the same content ID, hence downstream hits. That is early cutoff, and it means **the Bazel scar is avoided** — identity comes from the effective graph, not the history of how it was set.

**But only if promotion is mandatory, and that guts the abstract-plan claim.** Rule 1: *"Any unresolved value ⇒ the edge is unrunnable."* A mostly-nominal graph therefore caches nothing and runs nothing. So the two headline modes — cheap abstract structural capture, and incremental re-run — are **mutually exclusive within one document**. "More true as nodes promote" is doing a lot of work to hide that a plan is a drawing until it is fully bound.

**Shape variants:** `set` "rebinds parameters." Adding, removing, or rewiring an edge is not a parameter rebind — it is a patch, which is Q8, unresolved. So the general case of variation is not covered by the mechanism the section is built on.

**The retention promise is one the format cannot keep.** §3 claims *"the k3 sweep's outputs stay addressable rather than becoming rubble."* `store-records-architecture.md`, accepted decision #4: **"links never pin bytes — retention is the retention record's job alone"** (federation-DoS). A graph edge naming an output does not keep it alive. Preserving the sweep requires keep lines the graph does not write.

**Verdict §3: AMEND.** Delete the addressability claim or pair every sweep with a retention warrant. Pin `set` before it is quoted as the variant answer.

---

# 5. Frozen-grammar risk — what will be regretted

Ranked by how badly five implementations will diverge:

1. **Ordered vs set-valued comma lists.** `shots=c4S01…,…,c4S11…` is order-critical; `weights=…` is a set. Canonical form sorts lines and fields — it never says whether it sorts *values*. Sort them and editorial order dies; don't and set-valued inputs fragment the cache. **Not in the open questions.** This alone will split implementations silently.
2. **Bytewise vs natural sort.** c4m already canonicalizes by natural sort with files before directories. Adding a bytewise rule for a sibling kind destroys listings.md's stated payoff ("one canonicalization rule, one identity rule") on the very first new kind.
3. **The ABNF is ambiguous by construction.** `node-ref = c4-id / nom-tag / literal` with `literal = 1*VCHAR` — literal subsumes both alternatives, and ABNF alternation is unordered. A 90-character literal is indistinguishable from a content ID. This is the mechanism by which the integrity boundary gets breached (finding 1f).
4. **The ABNF contradicts its own prose.** `literal = 1*VCHAR` excludes SP; the prose says spaces are c4m-escaped. Pick one.
5. **Whitespace and blanks are unpinned.** The example uses **two spaces** after `note joshua` and column-aligned padding — unparseable under `*( SP word )` without a normalization rule that is never stated. c4m says blank lines are *ignored*; this grammar makes `blank` a line form. Does a blank survive canonicalization? Different answers → different IDs → different everything.
6. **Key charset excludes `_` and `.`.** `key = 1*( ALPHA / DIGIT / "-" / "@" )`. Real tool parameters use `_` constantly. `@` is in the set only to serve `set`'s scoping — a frozen general grammar bent around one unpinned process.
7. **Literal node identity (Q2) is not a detail — it is the entire cache.** Two implementations disagreeing on `3` vs `3\n` share no cache and raise no error. Cross-machine cache sharing is the whole point of constructive traces. This must be pinned *before* any implementation.
8. **Scalar normalization does not exist.** `fps=23`, `fps=23.976`, `fps=24000/1001` are three nodes. That is exactly why the 23 fps bug would have been visible — the flattering half. The other half is that hit rate depends on humans spelling scalars identically forever.
9. **Class in two places (Q6).** "Narrow, never widen" requires a pinned lattice over `exact`/`validated`/`volatile`. Unpinned = two sources of truth for the value that gates cache serving.
10. **Bind chains.** `nom:et/anchors → nom:et/anchor-verdict-2` binds nominal to nominal. `derivation-records.md` sanctions `resolves-to` only as **promotion to a content ID**. Aliasing admits bind cycles; the cycle rule covers only the edge DAG.

**Of roughly a dozen conformance-critical behaviors, six are open questions and four more are unidentified. Conformance vectors cannot be written today** — which is the design's own Q10 arriving as a structural fact rather than a scheduling note.

**One more, unlisted: three-valued recall (R4) is violated.** A truncated stream is a valid partial graph, so incomplete and complete graphs are byte-indistinguishable. `store-records-architecture.md` makes a **completeness watermark mandatory** on the reference index for exactly this reason — "so silence is distinguishable from not-indexed — three-valued recall, R4." The graph has none. It can say present/absent; it cannot say *no record covers this*. In a nine-session project where `w-9a6f` knew the right model and `w-d595` could not see it, that is the failure mode.

---

# 6. The honest alternative: no new format

**It exists, it is smaller, and on the founder's stated standard it wins.**

```
# plan.c4m — nodes; frozen grammar, null IDs for unbuilt, patch algebra, folding
# plan.testimony — edges; shape-A lines, existing predicates

c4Gk3…  produced-from  joshua  process=c4Ltx2… inputs=c4En1…,c4Bf2…,c4Ae6…,nom:et/anchors \
                               nframes=185 res=1080p seed=1234 class=validated test=nom:et/bg-score
nom:et/offset   resolves-to    c4Ln5…
nom:et/transitions  authored-by  joshua  at=2026-06-19T06:32Z
c4Ec0…  produced-from  main-b  process=c4Note… class=volatile      # the note is a file; its ID is a node
```

Every capability survives:
- **Variants** — a c4m patch, which is *already* a tier-1 `exact` derivation with its own ID, already line-local diffable, already implemented. This is precisely what `set` reinvents.
- **Human decisions** — `authored-by`, already budgeted, already the ruled answer in "Creation is a first-class edge."
- **Binds** — `resolves-to`, already budgeted.
- **Re-run rule** — byte-identical; the fields are the same fields.
- **Fan-in / fan-out / partial knowledge** — unchanged.
- **Every field is an input** — adopt it here; it is a *reading* of `inputs=`, not a grammar.
- **`test=`** — one new field key on an existing line.

**What is lost:** `class` moves from column 1 to `class=`, so the query becomes `grep 'class=volatile'` — indistinguishable in value. The process moves from column 2 to `process=`. You get two files instead of one, which **is the sidecar doctrine** this design claims to honor.

**What is gained:** zero new grammar, zero new conformance vectors, five implementations untouched, the pinned `/receipt` render unbroken, and — decisively — **a receipt becomes literally a subgraph of a workflow graph**, because they are the same lines. Under the proposal they are two grammars for one relation and a receipt is only *analogous* to a graph.

**Yes, it is better.** listings.md's promise was that a new kind costs *a line grammar, not a format* — a saving only when the line grammar carries new information. Three of this proposal's five structural choices carry no new information; they re-spell `produced-from`.

---

# Verdicts

| Section | Verdict |
|---|---|
| One-sentence design ("a listing of edges") | **AMEND** — the lines are *not* the ones a receipt writes (ID-last vs pinned ID-first, no `produced-from`). Say so or make it true. |
| "Why it costs no new concepts" table | **KILL** — read literally it argues for building nothing. |
| §1 Grammar | **KILL as written** — ambiguous ABNF, prose/ABNF contradiction, unparseable example, key charset without `_`, `literal` subsuming `c4-id`. |
| Canonical form and identity | **AMEND** — reconcile bytewise vs c4m natural sort; pin blanks and whitespace; pin literal identity **before** implementation. |
| The re-run rule | **KEEP** — correct and complete. **AMEND** the exempt list: `at=` and any clock/actor/annotation field must be exempt or banned, per the proposal's own `bench` line. |
| §2 ET worked example | **KILL as evidence** — dodges the 11-shot fan-out, cannot represent 8 failed runs, breaches the integrity boundary, violates "every field is an input." |
| §3 variant example | **AMEND** — `set` unpinned; shape variants uncovered; the addressability promise contradicts "links never pin." |
| §4 Deliberately left out | **KEEP** — the strongest section in the document. Every exclusion is right, especially control flow and the fold. |
| §5 Open questions | **AMEND** — add: ordered-vs-set comma lists; bind-chain cycles and transitivity; blank/whitespace canonicalization; the class lattice; the R4 completeness watermark. |
| **The format as a whole** | **KILL the kind. KEEP four ideas.** |

## The recommendation

Do not ship `@c4 graph 1`. Ship **three field keys and one reading** on the line that already exists:

1. `test=<node>` on `validated` edges,
2. free-form `key=value` fields on `produced-from`, with **"every field is an input"** as the binding reading,
3. `nom:` values in `inputs=`/fields, already sanctioned,
4. the `authored-by` + `volatile` creation pattern, already ruled.

That buys the `+350.60 s` re-key, the constitutional-rule-as-input (the single best idea in the proposal), variant-as-patch, and the served determinism class — **at the cost of one paragraph in `testimony-record.md` and zero new conformance vectors.** Then let real use, on a real pipeline, prove whether a distinct kind earns its five reimplementations. Right now the evidence points the other way, and §5 Q10 already knows it: three release blockers are open and the v1.1 surface is unfrozen.