# User Stories

Temporary document. Exploring how different users interact with
c4 across the full range of features — local, mesh, sync,
identity. These stories validate the design against real
workflows.

---

## Media & Entertainment

### DIT on set (Maya)

Maya is a Digital Imaging Technician on a film shoot in New
Mexico. The set has no internet. She has a c4d node on her
workstation and shuttle drives for the lab.

**Daily workflow:**
```
# Camera cards come in. Maya ingests to her workstation.
c4 cp /mnt/card-A/ today.c4m:A-cam/
c4 cp /mnt/card-B/ today.c4m:B-cam/

# She can see the full day's shoot described in one file
c4 ls -p today.c4m:
# A-cam/
#   A001_C001_0301.R3D    234.7 GB
#   A001_C002_0301.R3D    189.2 GB
# B-cam/
#   B001_C001_0301.R3D    201.4 GB

# Materialize to shuttle drive for the lab
c4 cp today.c4m: /mnt/shuttle-1/

# Print the c4m for the drive label
c4 ls today.c4m: > /mnt/shuttle-1/MANIFEST.txt
```

Maya hands the shuttle drive to a PA who drives it to the lab.
The c4m on the drive is the chain of custody document. The lab
can verify every frame against its C4 ID.

**Next day — incremental:**
```
# New day, same project
c4 cp /mnt/card-A/ day2.c4m:A-cam/

# Materialize to the SAME shuttle drive (additive)
c4 cp day2.c4m: /mnt/shuttle-1/
```

Day 1 files are already on the drive. Day 2 files are added
alongside. If a card was accidentally re-inserted, the files
are identical — harmless overwrite.

**With flow links (if connectivity exists):**
```
# Maya's project c4m has flow declarations on the entries:
#   -w-------  ...  card-ingest/  -> lab:incoming/
#   -rw-r--r-- ...  today.c4m     -> backup:dailies/
#
# card-ingest/ is write-only + outbound = drop slot.
# Camera cards go in, flow to the lab, don't accumulate locally.
# today.c4m mirrors to backup automatically.
#
# If satellite uplink is available, c4d fulfills the flows.
# If not, content accumulates locally — shuttle drives are
# the backup transport. When connectivity returns, c4d
# pushes the delta. No gap in coverage either way.
```

**Problem she'd have without c4:**
Copying raw files to a shuttle drive with no manifest. No way
to verify the delivery is complete. No way to know if a frame
was corrupted in transit. She writes filenames on sticky notes.

---

### Post supervisor (Carlos)

Carlos oversees post-production on a feature film. Editorial
is in LA, VFX is in Vancouver, color is in London. Each site
has its own c4d cluster.

**Cross-site dailies:**
```
# Vancouver VFX delivers shots
c4 cp vfx-delivery.c4m: production:vfx/v42/

# Carlos in LA can immediately see what was delivered
c4 ls production:vfx/v42/
# He sees the c4m — structure, sizes, identities
# Content materializes on the LA node when someone opens a file

# He forwards to color in London
c4 cp production:vfx/v42/ color:incoming/v42/
```

**With flow links (automated pipeline):**
```
# The production c4m has flow declarations:
#   production:vfx/  <- vancouver:deliveries/   (inbound from VFX)
#   production:vfx/  -> color:incoming/          (outbound to color)
#
# Vancouver pushes a delivery. It flows to LA automatically.
# LA's outbound flow forwards it to London automatically.
# Carlos sees it arrive in c4 status:
c4 status
#   <- vancouver:deliveries/  synced    (3 new shots)
#   -> color:incoming/        pending   (forwarding 3 shots)
#
# No manual cp needed. The pipeline is declared, not scripted.
```

The c4m (description) propagates instantly. The actual EXR files
(10s of GB per shot) materialize on demand. When the colorist
in London opens a shot, it pulls from the nearest node that
has it.

**Version comparison:**
```
# Compare what VFX delivered in v41 vs v42
c4 diff production:vfx/v41/ production:vfx/v42/

# See exactly which shots changed, which are new, which were removed
# All without downloading either version in full
```

**Problem he'd have without c4:**
FTP uploads, email chains tracking what was sent, no way to
diff two deliveries without downloading both. "Did you send
the updated comp for shot 1420? I can't tell from the
filename."

---

### Freelance colorist (Priya)

Priya works with three studios simultaneously, each on a
different project. She has a home workstation and a portable
setup she takes to client facilities.

**Receiving work:**
```
# Studio A sends to priya@email.com via Avalanche.io
# Content lands on her cloud node (auto-established by c4 login)
c4 ls cloud:incoming/
# studio-a/color-package-v3.c4m    2026-03-07

c4 cp cloud:incoming/studio-a/color-package-v3.c4m color-a.c4m:

# Studio B has their own c4d — they issued her a cert
# and she added their peer to her c4d config.
# studio-b auto-resolves from the peer entry.
c4 cp studio-b:priya-inbox/grade-ref.c4m grade-b.c4m:

# Studio C drops a shuttle drive at her office
c4 cp /mnt/studio-c-drive/ grade-c.c4m:
```

Three studios, three delivery methods (Avalanche relay,
direct mTLS, sneakernet). Priya doesn't care — the result is
the same: a c4m file she can work with.

**Delivering work:**
```
# Finished grades back to Studio A (via Avalanche.io relay)
c4 cp graded-a.c4m: studio-a:

# Back to Studio B via their c4d
c4 cp graded-b.c4m: studio-b:priya-delivery/

# Studio C gets a shuttle drive back
c4 cp graded-c.c4m: /mnt/return-drive/
```

**Syncing between her machines:**
```
# Home workstation syncs with her portable setup
c4 ln <> portable: :

# Everything she does at home propagates
# When she takes the portable to a client facility,
# she has everything she needs
```

**Problem she'd have without c4:**
Different studios use different delivery platforms (Aspera,
Frame.io, Dropbox, FTP). She has five different apps for
receiving files. No unified view of what she has vs what she
needs. "Which version of the color package is on my portable?"

---

### Studio IT admin (Derek)

Derek manages infrastructure for a mid-size VFX studio.
150 artists, on-prem render farm, cloud archive.

**Setting up the mesh:**
```
# Studio CA issues certs to all nodes
# Derek configures the main storage nodes

# Primary artist storage
c4d serve --config /etc/c4d/primary.yaml
# config: locations: [render-farm, archive]

# Render farm (aggressive retention)
c4d serve --config /etc/c4d/render.yaml
# config: retention: max_store_bytes: 50TB

# Cloud archive (keep everything)
c4d serve --config /etc/c4d/archive.yaml
# config: s3: bucket: studio-archive
```

**Policy-based content flow:**
```
# Finished renders flow to archive automatically via flow links.
# The render c4m declares: -> archive:renders/
# c4d fulfills this — no cron job, no scripts.

# Artists' workstations are peers of primary storage
# Content resolution cascades:
#   artist workstation → primary → render-farm → archive
# First access pulls from wherever the content lives
# Second access is local (cached)
```

**Onboarding a new artist:**
```
# Issue a cert from the studio CA
c4d pki issue --cn "new-artist@studio" --out ~/new-artist/

# Artist installs cert, adds primary as a peer in c4d config.
# Location auto-resolves — no c4 mk needed.
# They can now push/pull from the studio mesh.
```

**Vendor collaboration:**
```
# Vendor needs to deliver VFX shots
# Derek sets up a shared relay that trusts both CAs
c4d serve --config /etc/c4d/vendor-relay.yaml
# config: tls: ca: [studio-ca.pem, vendor-ca.pem]

# Vendor pushes via the relay (auto-resolves from config)
c4 cp shots.c4m: vendor-relay:delivery/

# Studio pulls from relay
c4 cp vendor-relay:delivery/shots.c4m incoming.c4m:
```

**Problem he'd have without c4:**
SAN storage with NFS mounts, Aspera for external transfers,
custom scripts for archive sync. Artists can't access the
render farm directly. Moving content between tiers requires
manual intervention. Vendor file exchanges go through email.

---

### On-set VFX supervisor (Tomoko)

Tomoko reviews VFX shots on set during a shoot in Morocco.
Internet is satellite (slow, intermittent). She has a ruggedized
workstation with c4d.

**Before traveling:**
```
# Sync the latest VFX review package to her portable node
c4 cp production:review-package.c4m local-review.c4m:

# Materialize offline reference material to SSD
c4 cp reference.c4m: /mnt/portable-ssd/
```

**On set (offline):**
```
# Review VFX against live plates
c4 diff local-review.c4m:shot-1420/ today-plates.c4m:shot-1420/

# Annotate which shots are approved (add to a c4m)
c4 cp approved-shots/ approvals.c4m:day-3/
```

**When satellite link is up (slow):**
```
# Push just the c4m (tiny) — the description of what she approved
c4 cp approvals.c4m: production:tomoko-approvals/

# The c4m travels over satellite instantly
# Actual content can follow over shuttle or better connection
```

**Problem she'd have without c4:**
Screenshots emailed over satellite. "Can you re-send the comp
for 1420? I think the one I have is from last week." No way
to verify which version of a shot she's looking at without
downloading it again.

---

### Archive manager (Lin)

Lin manages the long-term archive for a film library. 500+
titles, petabytes of content, compliance requirements.

**Verifying archive integrity:**
```
# Each title is a c4m file describing the complete archive package
c4 ls archive:titles/the-matrix.c4m:
# Reels, DCP, sound mixes, subtitles, artwork, documentation
# Every file has a C4 ID — verifiable forever

# Periodic integrity check
c4 diff archive:titles/the-matrix.c4m: cold-storage:titles/the-matrix.c4m:
# Empty diff = archives match. Done.
```

**Delivery to distribution:**
```
# Distributor needs the DCP + sound mix for theatrical
c4 cp archive:titles/the-matrix.c4m:dcp/ delivery.c4m:
c4 cp archive:titles/the-matrix.c4m:sound/ delivery.c4m:

# Materialize to delivery drive
c4 cp delivery.c4m: /mnt/delivery-drive/

# The c4m is the delivery manifest
# Distributor can verify: "did I get everything?"
```

**Disaster recovery:**
```
# Primary archive is on-prem (fast, expensive)
# Cold archive is S3 Glacier (slow, cheap)
# c4m files describe both — same content, same IDs
# Recovery: re-pull from cold storage using the c4m as the guide
c4 cp cold-archive:titles/ primary-archive:titles/
```

**Problem she'd have without c4:**
MD5 checksum lists maintained in spreadsheets. Manual comparison
of file listings across storage tiers. "Does the Glacier copy
match the on-prem copy? Let me check this spreadsheet from 2019."

---

## Developer / OSS

### Solo developer (Alex)

Alex has a laptop, a home server (NAS), and a cloud VM. They
want their project files backed up without thinking about it.

**Setup (once):**
```
# NAS auto-discovered via mDNS (c4d advertises _c4d._tcp)
c4 mesh
# nas    connected   0 channels

# Set up bidirectional sync for the project directory
c4 ln <> nas: :
```

**Daily workflow:**
```
# Flow link keeps everything in sync automatically
echo "new feature" >> README.md
c4 cp README.md :

# NAS now has the updated state
# If laptop dies, everything is on the NAS
```

**From the cloud VM:**
```
# Alex's cloud c4d is a peer of the NAS
# Pull the project
c4 cp nas:project/ local-project.c4m:

# Content resolves through the mesh:
# cloud VM → NAS → has it → serves it
```

**Problem they'd have without c4:**
rsync scripts, cron jobs, hoping they remembered to run the
backup. "Is my NAS copy up to date? When did I last sync?"

---

### Open source maintainer (Jordan)

Jordan maintains a project with large binary assets (fonts,
test fixtures, trained models). Git handles the code, c4 handles
the assets.

**Releasing:**
```
# Tag the release assets
c4 cp assets/ release-v2.3.c4m:
c4 cp models/ release-v2.3.c4m:trained-models/

# Push to the project's public c4d
c4 cp release-v2.3.c4m: releases:v2.3/

# The c4m goes in the git release notes
# Anyone can pull the exact assets for this version
```

**Contributors downloading:**
```
# Clone the repo (small, just code)
git clone github.com/jordan/project

# Pull the assets described by the c4m
c4 cp releases:v2.3/release-v2.3.c4m assets.c4m:
c4 cp assets.c4m: ./assets/
```

**CI/CD integration:**
```
# CI pipeline captures build artifacts
c4 cp build-output/ build-artifacts.c4m:

# Push to artifact storage
c4 cp build-artifacts.c4m: ci-store:builds/$BUILD_ID/

# Compare against last known good
c4 diff ci-store:builds/$LAST_GOOD/ ci-store:builds/$BUILD_ID/
```

**Problem they'd have without c4:**
Git LFS (vendor lock-in, bandwidth limits, no self-hosting
option). Or hosting binaries on a CDN with manual checksums.
"Download assets.tar.gz and verify the SHA256 matches..."

---

### Data scientist (Noor)

Noor works with large datasets (10-100 GB) that need to be
shared across a team and tracked across experiments.

**Dataset management:**
```
# Capture the training dataset
c4 cp training-data/ dataset-v4.c4m:

# The c4m describes the exact dataset — every file, every byte
# C4 ID of the c4m IS the dataset version

# Share with team
c4 cp dataset-v4.c4m: team-store:datasets/

# Colleague pulls the same exact dataset
c4 cp team-store:datasets/dataset-v4.c4m local-data.c4m:
c4 cp local-data.c4m: ./training-data/
```

**Experiment reproducibility:**
```
# Record which dataset produced which model
echo "dataset: $(c4 -i dataset-v4.c4m)" >> experiment.log
echo "model: $(c4 -i trained-model.bin)" >> experiment.log

# Months later, reproduce exactly:
# The C4 IDs in the experiment log are universal identifiers
# Pull the exact dataset and model from the mesh
```

**Dataset diff:**
```
# What changed between v4 and v5?
c4 diff dataset-v4.c4m: dataset-v5.c4m:
# Output is a c4m patch: new entries (additions),
# clobbered entries (modifications), re-emitted entries (removals)
# All without downloading either dataset in full
```

**Problem they'd have without c4:**
"Which version of the dataset did this model train on?"
Filenames with dates, shared drives with no versioning,
DVC (complex git integration, vendor-specific).

---

### DevOps engineer (Sam)

Sam manages deployments across staging and production
environments.

**Configuration management:**
```
# Capture deployment config
c4 cp /etc/app/ deploy-config.c4m:

# Compare staging vs production
c4 diff staging:config/ production:config/
# Exact structural diff — file by file, byte by byte

# Roll out: push staging config to production
c4 cp staging:config/ production:config/
```

**Artifact distribution:**
```
# Build produces artifacts
c4 cp build-output/ release.c4m:

# Push to all deployment targets simultaneously
for target in edge-1 edge-2 edge-3; do
  c4 cp release.c4m: $target:releases/v2.3/ &
done
wait

# Each edge node gets the c4m instantly
# Artifacts materialize from nearest peer (content resolution)
```

**Disaster recovery:**
```
# Nightly: push state snapshot to offsite
c4 cp : offsite:backups/$(date +%Y%m%d)/

# Recovery:
c4 cp offsite:backups/20260307/ recovery.c4m:
c4 cp recovery.c4m: ./restore/
# Every file verified against its C4 ID on materialization
```

**Problem they'd have without c4:**
Ansible playbooks copying files, rsync scripts, artifact
repositories (Nexus, Artifactory) that cost money and don't
provide content verification. "Is production running the same
config as staging? Let me diff these two servers..."

---

### Photographer (Elena)

Elena shoots weddings and events. Thousands of raw files per
event, delivered to clients, archived long-term.

**Event workflow:**
```
# Import from camera cards
c4 cp /mnt/card1/ johnson-wedding.c4m:raw/

# After editing, add selects
c4 cp selects/ johnson-wedding.c4m:selects/
c4 cp finals/ johnson-wedding.c4m:delivery/

# The c4m has everything: raw, selects, finals
c4 ls -p johnson-wedding.c4m:
# raw/        12,847 files   382.4 GB
# selects/       312 files    11.2 GB
# delivery/      312 files     4.8 GB
```

**Client delivery:**
```
# Client gets a download link (via Avalanche.io)
c4 cp johnson-wedding.c4m:delivery/ client@email.com:

# Or a USB drive
c4 cp johnson-wedding.c4m:delivery/ /mnt/client-usb/
```

**Archive:**
```
# Archive to NAS via flow link
c4 ln -> nas: :
# Everything auto-archives

# Years later, client needs a reprint
c4 ls nas:johnson-wedding.c4m:delivery/
c4 cp nas:johnson-wedding.c4m:delivery/IMG_4521.jpg ./reprint/
```

**Problem she'd have without c4:**
Dropbox for client delivery (monthly fees, upload limits).
External hard drives labeled with Sharpie for archive. "I know
the Johnson wedding is on one of these drives..."

---

### Research team (University lab)

A research lab shares large datasets and computational results
across institutions.

**Data sharing:**
```
# Lab A captures experimental data
c4 cp /instrument/output/ experiment-2026-03.c4m:

# Share with Lab B at another university
# Both labs use Avalanche.io for identity
c4 cp experiment-2026-03.c4m: lab-b-pi@university.edu:

# Lab B receives on their cloud node
c4 cp cloud:incoming/experiment-2026-03.c4m local.c4m:
```

**Publication reproducibility:**
```
# Paper includes the C4 ID of the dataset
# "Data available at c4id: c4abc..."
# Anyone with mesh access can pull the exact data
# The ID is the citation — immutable, verifiable
```

**Multi-site computation:**
```
# Lab A has the raw data
# Lab B has the compute cluster
# Lab C has specialized analysis tools

# Data flows between them via mesh
c4 cp raw-data.c4m: compute-cluster:input/
# Compute cluster processes, produces results
c4 cp compute-cluster:output/ results.c4m:
# Results go to analysis
c4 cp results.c4m: analysis-lab:incoming/
```

**Problem they'd have without c4:**
Globus for data transfer (complex, institutional). SCP between
servers. "Can you re-upload the dataset? I'm not sure the one
I have matches yours." No way to verify data integrity across
institutions without manual checksum exchange.

---

### Hobbyist / home user (Pat)

Pat has a laptop, a Raspberry Pi NAS, and wants simple backups.

**Setup:**
```
# Install c4 and c4d on both machines
# Generate self-signed key pair (c4d init handles this)
c4d init
# → created ~/.c4d/config.yaml
# → generated key pair

# On laptop, Pi auto-discovers via mDNS
c4 mesh
# pi    connected   0 channels
```

**Backup photos:**
```
c4 cp ~/Photos/ photos.c4m:
c4 cp photos.c4m: pi:

# Done. Photos are on the Pi.
# Next month:
c4 cp ~/Photos/ photos.c4m:
c4 cp photos.c4m: pi:
# Only new/changed photos transfer (CAS dedup)
```

**Problem they'd have without c4:**
rsync (works but no verification, no manifest).
Cloud backup (monthly fees). "Is my Pi backup current?"

---

## Edge Cases and Stress Tests

### The "everything is offline" scenario

A film crew in a remote location. No internet. No LAN between
departments. All content moves via shuttle drives.

```
# Camera → DIT workstation (SD card reader)
c4 cp /mnt/card/ day5.c4m:

# DIT → Editorial (shuttle drive)
c4 cp day5.c4m: /mnt/shuttle/

# Editorial → Color (another shuttle drive)
c4 cp editorial-cut.c4m: /mnt/shuttle-2/

# Each handoff has a c4m manifest
# Each delivery can be verified against its c4m
# Chain of custody is the sequence of c4m files
```

### The "huge sync" scenario

Two data centers, 500 TB each, mostly overlapping.

```
# Compare namespace entries (c4m IDs, not blob content)
c4 diff dc-east:projects/ dc-west:projects/

# Only different c4m files need deeper inspection
# Each different c4m is diffed (MB, not TB)
# Only missing blobs transfer
# Petabyte sync reduces to gigabyte transfer
```

### The "untrusted network" scenario

Content travels through untrusted intermediaries (hotel WiFi,
public relays, third-party CDN).

```
# Content is fetched from wherever
# But every blob is verified against its C4 ID on receipt
# Corruption or tampering is detected immediately
# The c4m describes exactly what should arrive
# No trust needed in the transport
```

### The "cert expired mid-transfer" scenario

```
# Transfer is interrupted when a cert expires
# Resume: re-authenticate, re-run the same command
# CAS means completed blobs don't re-transfer
# Pick up exactly where you left off
```

### The "project template with flow channels" scenario

A studio creates a project template c4m with flow declarations
baked in. Every workstation that receives it automatically gets
the right backup and delivery channels.

```
# Studio admin creates the project template
# The c4m has flow declarations on key directories:
#
#   -rw-r--r-- ...  work/         -> backup:projects/$PROJECT/
#   --w-------  ...  card-ingest/  -> ingest:incoming/
#   -r-xr-xr-x ...  reference/    <- library:shared/luts/
#   -rw-rw-r-- ...  shared/       <> collaboration:projects/$PROJECT/
#
# Deploy to 10 artist workstations:
for ws in artist-{01..10}; do
  c4 cp project-template.c4m: $ws:projects/feature-film/
done

# Each workstation now exposes 4 channels:
#   -> backup:     (work directory mirrors to backup server)
#   -> ingest:     (card reader drains to media pipeline)
#   <- library:    (reference LUTs arrive from shared library)
#   <> collaboration: (shared files sync bidirectionally)
#
# The admin configures c4d once per location name.
# The artists don't configure anything.
# A new workstation gets the template and immediately has
# all channels active.

# Admin sees the mesh:
c4 mesh
# artist-01  connected  4 channels (all synced)
# artist-02  connected  4 channels (3 synced, 1 pending)
# ...
# artist-10  connected  4 channels (all synced)
# backup     connected  10 inbound channels
# library    connected  10 outbound channels
```

---

### The "naming conventions" scenario

Two facilities collaborate on a film. Facility A uses RED cameras
and organizes by camera magazine (A001/, A002/). Facility B uses
ARRI and organizes by shooting day (Day01/, Day02/). Each facility
has its own internal naming conventions, directory structures, and
pipeline expectations. Neither will change for the other.

```
# Facility A (RED pipeline) — DIT labels entries with flow channels
# The c4m has the camera manufacturer's directory structure:
#
#   A001/A001C001_220315.R3D   c4abc...  -> raw:
#   A001/A001C001_220315.mov   c4def...  -> proxies:
#   A001/A001C001_220315.wav   c4ghi...  -> audio:
#   A002/A002C001_220315.R3D   c4jkl...  -> raw:
#   ...
#
# The DIT doesn't restructure. She labels WHAT things are,
# not WHERE they go. The flow channel names (raw:, proxies:,
# audio:) are the semantic interface.

# Facility B (ARRI pipeline) — receives the c4m
# B's c4d is configured:
#   raw:     -> /ingest/camera-raw/     (their raw storage path)
#   proxies: -> /editorial/media/       (their editorial path)
#   audio:   -> /audio/production/      (their sound dept path)
#
# The channel names resolve to B's internal structure.
# B never sees A's directory layout. A never learns B's.
# raw: is the contract between them.

# What Facility B sees:
c4 status
#   <- raw:      synced    (42 files received)
#   <- proxies:  synced    (42 files received)
#   <- audio:    synced    (42 files received)

# Content arrived in B's expected paths:
ls /ingest/camera-raw/
# A001C001_220315.R3D  A002C001_220315.R3D  ...
#
# The files are there. The RED naming is preserved in filenames
# but the directory structure is B's own. No one had to rename
# anything or write a mapping script.
```

**Why this matters:**

Every cross-facility delivery today requires either:
1. The sender restructures to match the receiver's naming
   convention (labor-intensive, error-prone)
2. The receiver runs an ingest script that maps the sender's
   layout to their own (fragile, per-vendor)
3. Both sides use an agreed-upon exchange format that neither
   actually uses internally (friction, overhead)

Flow channel names eliminate all three. The sender labels entries
by WHAT they are (raw, proxies, audio, vfx, grades). The receiver
maps channel names to their own paths. The channel vocabulary
becomes the interoperability layer.

This scales to any number of facilities. A VFX vendor in Mumbai,
a color house in London, and a sound mix stage in LA all receive
the same c4m with the same channel labels. Each maps the channels
to their own internal paths. No one needs to know anyone else's
directory conventions.

```
# Third facility joins the project
# Just configure what the channel names mean locally:
#   raw:     -> /SAN/projects/feature/camera/
#   proxies: -> /SAN/projects/feature/editorial/avid-media/
#   audio:   -> /SAN/projects/feature/sound/production-audio/
#
# Same c4m. Same channels. Different local paths.
# Zero coordination with the other facilities.
```

---

### The "cross-vendor VFX handoff" scenario

A feature film has VFX split across three vendors. Vendor A does
environment work (matte paintings, CG sets). Vendor B does creature
work. Vendor C does final compositing. Vendor B needs Vendor A's
environments as inputs. Vendor C needs both A and B's outputs as
inputs. The production coordinates through a VFX supervisor at
the studio.

Each vendor has completely different pipeline conventions:

- **Vendor A** (environments): organizes by sequence/shot, uses
  EXR multi-layer with their own layer naming, delivers in
  `build/` directories with version folders.
- **Vendor B** (creatures): organizes by asset/variant, uses
  Alembic caches + texture atlases, delivers in `publish/`
  directories with `_v###` filename suffixes.
- **Vendor C** (comp): expects inputs organized by shot with
  `fg/`, `bg/`, `ref/` subdirectories, flat EXR sequences named
  by shot code.

None of them will restructure for the others. They can't —
their render farms, review tools, and asset management systems
depend on their internal conventions.

```
# Vendor A delivers environments.
# Their c4m describes their internal layout:
#
#   build/seq010/shot0010/env_v003/
#     env_diffuse.####.exr   c4aaa...  -> environments:
#     env_reflect.####.exr   c4bbb...  -> environments:
#     env_depth.####.exr     c4ccc...  -> environments:
#   build/seq010/shot0020/env_v002/
#     ...                              -> environments:
#   build/seq020/shot0010/env_v001/
#     ...                              -> environments:
#
# Everything is labeled -> environments:
# A doesn't know or care who consumes this channel.

# Vendor B needs A's environments as background plates for
# creature lighting. B's c4d maps:
#   environments: -> /assets/external/bg-plates/
#
# B receives A's files in B's expected location.
# B's lighting artists open /assets/external/bg-plates/
# and find the EXRs. The directory structure under bg-plates/
# preserves A's layout — but B's pipeline doesn't care about
# the structure, it just reads the EXRs.

# Vendor B delivers creatures.
# Their c4m describes their internal layout:
#
#   publish/dragon/hero_v012/
#     dragon_hero.abc        c4ddd...  -> creatures:
#     dragon_hero_diff.tx    c4eee...  -> creatures:
#     dragon_hero_spec.tx    c4fff...  -> creatures:
#   publish/dragon/wing_v008/
#     ...                              -> creatures:
#   publish/horse/gallop_v003/
#     ...                              -> creatures:
#
# Everything labeled -> creatures:

# Vendor C (comp) needs both environments and creatures.
# C's c4d maps channels to their comp pipeline structure:
#   environments: -> /shots/$SHOT/bg/
#   creatures:    -> /shots/$SHOT/fg/
#
# C's compositors open a shot folder and find:
#   /shots/seq010_shot0010/bg/   (A's environments)
#   /shots/seq010_shot0010/fg/   (B's creatures)
#   /shots/seq010_shot0010/ref/  (editorial reference, separate channel)
#
# Each vendor's content arrived in C's expected structure.
# No renaming. No restructuring scripts. No FTP folders
# full of ambiguously named files.
```

**The VFX supervisor's view:**
```
# The supervisor at the studio sees all the channels:
c4 mesh
#   vendor-a    connected   1 channel
#     -> environments:      synced    (seq010: 3 shots, seq020: 1 shot)
#   vendor-b    connected   1 channel
#     -> creatures:         synced    (dragon: 2 variants, horse: 1)
#   vendor-c    connected   2 channels
#     <- environments:      synced
#     <- creatures:         3 pending  (vendor-b delivered 20 min ago)

# She can trace any piece of content:
c4 dig vendor-c:shots/seq010_shot0010/bg/env_diffuse.0001.exr
#   Source: vendor-a:build/seq010/shot0010/env_v003/env_diffuse.0001.exr
#   Flow:   vendor-a -> environments: -> vendor-c
#   Status: synced, Σ=0.0
#   Chain depth: 1 (direct, through studio relay)

# When Vendor A publishes a new environment version,
# it flows through environments: to both B and C automatically.
# The supervisor doesn't coordinate the handoff — it happens.
# She just watches c4 status and sees the updates arrive.
```

**What this replaces:**

Today this handoff involves: a coordinator emailing "v003 of
seq010/shot0010 environments is on the FTP, please download
and let me know when you have it." Vendor B downloads to a
staging area, an ingest script tries to parse the directory
names and map them to B's conventions, it fails on edge cases
(shot name format mismatch, version folder depth), a TD fixes
it manually, and 4 hours later the artist can start working.

With flow channels: Vendor A publishes. Both B and C have it
within minutes, in their own pipeline structure, verified by
C4 ID. Nobody emails. Nobody runs an ingest script. Nobody
waits.

---

### The "multi-band delivery" scenario

100 TB delivery: 90 TB goes on shuttle drives, 10 TB of
urgent material goes over the wire.

```
# Materialize bulk to shuttle drive
c4 cp delivery.c4m: /mnt/shuttle/

# Push urgent shots over the wire
c4 cp delivery.c4m:urgent/ client-relay:incoming/

# Client accumulates from all sources into one c4m
c4 cp client-relay:incoming/ received.c4m:    # wire (arrives first)
c4 cp /mnt/shuttle/ received.c4m:             # shuttle (arrives later)

# Check progress against the delivery manifest
c4 diff delivery.c4m: received.c4m:
# Shows exactly what's still missing across all bands
```
