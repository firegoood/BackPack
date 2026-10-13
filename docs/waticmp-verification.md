# Native Wat ICMP and peer Auto: verification and operator report

Date: 2026-10-13. Status: **LOCAL ACCEPTANCE PASSED WITH DOCUMENTED LIMITS**. This is an
experimental opt-in implementation, not approval to deploy on a live VPS.

## 1. Source, attribution and preservation

Target `E:/P_OX/back`; module `github.com/backpack/backpack`; implementation baseline HEAD
`132467bfdfb2698d74ada2992413e75bfccbfbd3`; branch
`fullpack/waticmp-coordination`. Existing uncommitted FullPack naming, managed
HTTPS and spoofing web changes were preserved. The implementation audit involved
no upstream fetch, dependency upgrade, commit, push, tag, signed release or
live-server change. The operator subsequently authorized a separate source
commit/push to `firegoood/BackPack`; this does not authorize a binary release
or deployment. The baseline SHA and measurements below remain audit evidence.

The read-only reference is `11/WatTunnel`, WatTunnel 2.4.8, MIT copyright
2026 Amirreza. It has no independent Git checkout. Its 79 original-byte file
hashes and tree fingerprint
`b02cf1a38a42649f8066ca14573d5a98028e94ccf7e676460979808b5b7cf7a2`
matched the starting inventory after implementation. Sorting uses Windows
Python Path ordering; the UTF-8 relative path, NUL, hexadecimal file SHA256
and LF are concatenated. This is a snapshot fingerprint, not a Git SHA.

The adapted engine's `LICENSE` and `ORIGIN.json` list exact copied portions,
original source hashes and repairs. Project LICENSE, NOTICE, TRADEMARK,
historical CHANGELOG, go.mod and go.sum remain unchanged. Native source has
no local module replacement and no dependency on the reference directory.

Evidence is retained at `E:/P_OX/fullpack-waticmp-audit`. Original C-drive
audit files were preserved. `before.patch`, `before-status.txt`,
`wat-reference.json` and the failed logs are retained alongside successful
reruns. Docker stale runtime directories were renamed and preserved during
recovery; images, volumes and user projects were not purged.

## 2. Implemented change and compatibility boundaries

The [design/source audit](waticmp-dual-server-design.md) maps registries,
factories, lifecycle and the original Wat APIs. The [change inventory](waticmp-changes.md)
distinguishes this task from pre-existing FullPack work.

| Surface | Implemented behavior / boundary |
|---|---|
| Native Wat | Actual attributed authenticated Session/Stream, selective ACK, retransmission, pacing and stream priorities; normal and reverse IPv4 Echo direction |
| xDi | Remains a separate unchanged KCP-based carrier; no relabeling or replacement |
| Data plane | Existing native reverse forwarding plus Auto-owned stable Iran TCP/UDP listeners; explicit enrolled backend mapping indices |
| Coordination | Version 1 typed mutual authentication, bounded frames, fresh incarnation/transaction binding, sole Iran authority, durable generation intent/commit, explicit uncertain-commit fencing |
| Bootstrap | Pre-enrolled permitted passive endpoints; reverse ICMP needs both explicit IPs and matching enrollment, without a Direct management channel |
| Auto candidates | Wat, Noise/stealth, QUIC, WSS, WSSMUX only; unverified/plaintext adapters are rejected even with a plaintext flag |
| Policy | Default-off Auto and severe-degradation switching; observe-only, manual lock/disable/test, pressure skips, debounce, quarantine, jitter, cooldown/dwell and persisted switch budget |
| UI | Terminal setup/edit/status/actions; web native/Auto forms, private enrollment import, secret-preserving edits and authenticated scoped API actions |
| CLI option 0 | Actual native normal/reverse TCP/UDP tests when both explicit IPs are available; missing prerequisites are SKIPPED |
| Direct/L3 | Native Wat and coordinated Auto are unsupported and explicitly refused |
| Old peers | Legacy manual carriers/links/configs remain supported; old peers cannot negotiate Auto or consume version-3 native enrollment |
| Native wire | FullPack native protocol v1 is not the standalone wattun wire; both peers must run this implementation |
| Installer/assets | Source installer and standalone legal archives; no wattun helper or reference tree in artifacts |

The application columns below mean Iran user requests and bidirectional
request/return data with the enrolled Kharej backend. ICMP request direction
is independent of that application role; it is not a second set of user ports.

| Carrier / mode | Iran→Kharej application and return | Reverse stream forwarding | Native/Auto Direct GRE/L3 | Acceptance status |
|---|---|---|---|---|
| Wat normal Echo | TCP/UDP bidirectional integrity | IMPLEMENTED / TESTED | UNSUPPORTED | Actual two-node namespaces, MTU 576/640/700 |
| Wat reverse Echo | TCP/UDP bidirectional integrity | IMPLEMENTED / TESTED | UNSUPPORTED | TESTED with Iran Echo requests and cross-node TCP/UDP blocked |
| Auto Wat / Noise | Stable Iran TCP/UDP listeners | IMPLEMENTED / TESTED | UNSUPPORTED | Actual two-node bootstrap, outage/degradation switch and recovery |
| Auto QUIC | TCP/UDP bidirectional integrity | IMPLEMENTED / TESTED | UNSUPPORTED | Actual authenticated loopback bootstrap, isolated probe and coordinated Noise handover/drain |
| Auto WSS / WSSMUX | TCP/UDP bidirectional integrity | IMPLEMENTED / TESTED | UNSUPPORTED | Actual authenticated loopback bootstrap, isolated probe and coordinated Noise handover/drain |
| Existing manual carriers, including xDi / managed HTTPS | Existing forwarding preserved | IMPLEMENTED / TESTED | Existing Direct/L3 preserved; new two-host acceptance NOT VERIFIED | Local old/current binary tests and actual pinned-helper traffic; independent of Auto |
| Auto plaintext TCP/UDP/WS/Mux, xDi/PCK or managed HTTPS | No approved adapter | UNSUPPORTED / refused | UNSUPPORTED | No automatic security downgrade; manual modes remain available |
| Any native/Auto carrier on real Iran/Kharej WAN | IMPLEMENTED locally | WAN behavior NOT VERIFIED | Native/Auto UNSUPPORTED | No real VPS access or throughput measurement |

Connection Test and route Link Test remain distinct. The existing interactive
Connection Test coordinator uses TCP/UDP and may be filtered; that limitation
does not apply to Auto's pre-enrolled reverse-only bootstrap.

## 3. Six review passes and defects found

These are separate review passes with different subjects, not claims of six
independent reviewers.

1. **Baseline and provenance:** an existing enginectl fixture treated socket
   creation as readiness and raced cleanup. It now waits for a real PING and
   joins shutdown; 100 race repetitions passed. Missing documentation stamps
   were found by the app suite and repaired. Reference/legal/module boundaries
   were checked; no original reference file was edited or executed.
2. **Native packet/stream safety:** fresh bilateral X25519/PSK key derivation,
   direction-separated packet nonces, source/type/tunnel/lane demux, replay,
   parser arithmetic and resource bounds were examined. Receive-window
   retirement double accounting, immediate-close write loss and unsigned ACK
   delay underflow were reproduced and repaired in the adapted engine.
   Real half-close/deadline tests and 100 repetitions of 100 streams passed.
3. **Distributed state/lifecycle:** duplicate PREPARE ordering was corrected.
   Tests cover actual lost COMMIT_ACK delivery, durable intent, stale/foreign
   transactions, control replay, clone conflicts, exclusive state ownership,
   symlink/FIFO/public-file refusal and restart fencing. Two independent peers
   retain separate backends/listeners; one peer failure does not reset another.
4. **Probe/policy/resources:** production traffic is isolated from new
   cryptographic probe sessions. Slow/offline backend tests keep the carrier
   healthy. Global/per-pair budgets and pressure skips are tested. A real
   high-RTT probe exposed a 120 ms unsampled PTO loop; a 1 s initial allowance
   now permits the first RTT sample. A second failure exposed inability to
   learn an RTT increase after packet retirement; bounded 4,096-entry timing
   metadata permits one authenticated late sample. Low caller pacing ceilings
   now bound initial/floor rates, and packet accounting includes the envelope.
   The reachable severe-degradation two-node case passed after these repairs.
5. **CLI/web/credentials:** real panel templates/JS were checked in headless
   Chrome with a local mock API. Empty legacy wizard sections were removed
   for native/Auto forms, and the Kharej result now reports imported enrollment
   rather than creating a false next-peer handoff. Desktop/mobile, native and
   Auto payloads, edit/test actions, HTTPS restoration and JS errors were
   checked. Actual enrollment POST was asserted. All 43 panel JS tests passed.
6. **Operator/update/release:** real source installation, private link export,
   actual CLI apply, systemd service traffic and generation-preserving rename
   were exercised in disposable Linux containers. An earlier local FullPack
   build rollback preserved configs/fencing and recovered at a fresh generation.
   Cross-version legacy traffic, reference exclusion, embedded licenses and
   unchanged signature verification were checked. Complete race/coverage,
   current network matrices and seven-architecture archives passed. Actual
   uninstall then found leftover installer-owned optional helpers; cleanup now
   refuses unsafe roots/redirected ancestors and preserves external operator
   paths. Ten race regression repetitions and actual helper uninstall passed.
   The final complete quality/release rerun after this cleanup repair passed.

The native delay regressions passed ten race repetitions. Original failures,
including the first regression fixture's incorrect retired-list-length check,
remain in the audit; the corrected assertion checks active entries, not the
retained slice backing storage. A shell fixture normalization error and an xDi
loss-parameter formatting error were also corrected in external audit scripts.
They are not reported as product passes.

## 4. Actual two-node evidence

All network changes were confined to disposable privileged Docker containers
and their Linux namespaces. No production firewall/sysctl/VPS was modified.

| Test | Actual result and limits |
|---|---|
| Native 2NS | Normal/reverse × MTU 576/640/700; 10 ms ±2 ms delay, 0.5% loss, 0.1% duplication, 1% reorder; 100 ×32 KiB TCP each direction plus UDP 1/64/1400/8000. All six latest engine cases passed (4.339–4.583 s) |
| Auto 2NS bootstrap | Same six cases using only pre-enrolled native ICMP; all cross-node TCP/UDP blocked, Iran requests blocked in reverse cases. Both peers agreed on generation and actual traffic passed in all six latest cases |
| Coexistence/negative | Ordinary ping + xDi + two independent native identities; wrong key/source, captured replay and corrupt authenticated frames. 12,000 negative packets; latest reverse native AuthDropped=4755 and ReplayDropped=2385; unrelated native did not allocate foreign streams. One-way 5% loss/0.1% corruption still passed actual users. Latest rerun passed |
| Recovery | Reverse-only Wat gen2; 2 s outage did not switch; observe-only refused a cross-carrier change; Noise gen3 after ICMP data/control loss; no automatic failback on ICMP return; Wat gen4 after Noise loss; both dead produced NO_ROUTE and refused users; restoration gen5 and follower restart gen7 converged. Latest rerun passed 100 TCP + UDP after each route |
| Reachable degradation | ICMP kept reachable with 2 Mbps and 180 ms±2 ms each direction, Noise unshaped. Three actual Wat probe transactions passed; sustained measured degradation selected Noise gen3 on both peers after 57 s policy wait. Reason explicitly says severe degradation, not hard failure. 100 TCP + UDP passed afterward |
| Probe with users/drain | Real held TCP exchange survived an isolated probe without generation change; manual native→Noise commit retained the user listener and allowed old-generation traffic to drain with a real Drained counter |
| Authenticated Auto adapters | QUIC, WSS and WSSMUX each passed with exactly one enrolled candidate, as well as a separate two-candidate handover case. Actual two-peer bootstrap, 32 KiB TCP + 8,000-byte UDP, independent integrity probe and manual handover to Noise passed. A held TCP stream survived probe/handover; its drain was counted. Final focused race suite: 105.002 s. These are loopback data-path tests, not three additional filtered-WAN outage matrices |
| Backend diagnosis | Closing the approved backend did not make a passing transport probe fail or change its generation |
| Native Connection Test | Separate normal/reverse actual two-namespace CLI-option-0 tests passed; this test's coordinator was permitted explicitly |
| Legacy binary compatibility | Built local pre-feature HEAD 132467b with Go 1.27.2, paired it with current source in both directions for all ten existing reverse carriers: 20 cases passed in 11.250 s. xDi old/new and new/old also passed actual 20 ×32 KiB TCP + UDP at MTU576 with cross-node TCP/UDP blocked |
| Actual source install | Both systemd containers installed from current source with Go1.27.2; private config/link mode0600; setup-link apply started the Kharej service |
| Installed traffic/admission | 32 simultaneous TCP + UDP passed. 100 attempts at the default 64 admission limit yielded exactly 64 accepted/completed and 36 rejected, rather than silently over-admitting |
| Rename | Actual Iran rename preserved state, advanced gen2→gen4, reconnected both peers and passed 32 TCP + UDP |
| Local earlier-build rollback | Both stopped, only their FullPack binary replaced by an earlier local native/Auto build, configs/state retained. Both recovered at gen6 and passed 32 TCP + UDP. This is not rollback compatibility with an upstream release lacking Auto |
| Final source upgrade | Both source installers preserved config hashes; both services recovered at gen8 and passed 32 TCP + UDP |
| Carrier disable/delete | Disabled Noise could not be selected; active Wat/gen8 remained intact. Actual Iran delete stopped listeners and removed config/state/unit/socket. Kharej remained running until separately uninstalled |
| Uninstall | Actual menu item9 removed each endpoint's units/configs/fencing/binary/install directory. After repairing helper retention, installer-owned helper directories were also removed while an external operator tool remained intact |
| Managed HTTPS | Pinned Naive/sing-box/Xray downloads were checksum-verified in the disposable container. Actual 13-carrier loopback Connection Test and authenticated REALITY cover/fallback/cancellation tests passed with race (137.448 s). Wat rows were honestly SKIPPED here without explicit IPs; their separate actual two-node tests passed |

Latest recovery times: Noise 7 s, Wat after Noise loss 20 s, NO_ROUTE 30 s when
both carriers were blocked, same-candidate restoration 13 s, follower restart
19 s. These are observed fixture times, not guaranteed WAN deadlines. Three
flows were interrupted in that outage run. Existing TCP is not migrated
across carriers. Earlier runs and their different timings remain in raw logs.

The first 600-kbit ICMP bottleneck saturated/dropped its queue and failed;
that is retained as a failure, not silently counted as degradation success.
The later reachable case preserved authenticated data/control and both-peer
integrity confirmation. Neither case proves arbitrary ICMP bottlenecks safe.

## 5. Quality, benchmark and artifacts

Toolchain: **Go1.27.2 Linux**, Node22.23.3. Host Go1.27.1 was not used to
build or verify this code. Installer rejection of Go1.27.0/1.27.1 is retained.

- A complete Linux default suite passed: e2e 374.523 s, manage 445.418 s.
  The previous attempt's only failure was cleanup of a test directory on an
  NTFS bind. Repeating that exact network test ten times with Linux temporary
  storage passed; the full Linux-storage rerun then passed. No test was waived.
- A whole-project race/coverage run passed before the final delay repairs:
  57.3% total, above the unchanged 50% floor; all fourteen existing package
  floors passed. Native raw e2e contributed actual coverage.
- The complete post-delay-repair race/coverage passed: **57.4%** total; all
  fourteen unchanged floors passed, including server transport 87.9% (floor85)
  and client transport 86.9% (floor83). e2e=398.242 s; manage=452.328 s.
  The final complete rerun after the uninstall cleanup repair passed:
  **57.3%** total, all fourteen unchanged floors; server transport 88.0%,
  client transport 86.8%, e2e=395.694 s and manage=445.767 s.
  Final formatting, vet and staticcheck passed. These separate runs have
  different coverage percentages; none replaces a failed floor.
  A subsequent test-only extension covers the additional authenticated Auto
  adapters; its focused race run passed. Runtime implementation files did not
  change after the whole-project run or final release build.
- Staticcheck v0.8.2 passed. Govulncheck v1.9.0 with Go1.27.2 found zero reachable
  vulnerabilities and zero vulnerable imported packages, with two unused
  module-level advisories. The official online database returned 403; the scan
  used all 4,613 official OSV records from golang/vulndb snapshot
  `d5e0cfac5d260523df858cd4c0f849e248468fb6`, preserving affected ranges.
  The final post-uninstall-repair offline scan returned the same result
  (`uninstall-final-govulncheck.log`).
  This is a pinned offline scan, not proof against later advisories.
- Six original parser fuzz targets passed their 20/30 s budgets.
  The four remaining current targets passed 30 s each: share-link decoding
  306,499 executions; round trip 106,359; native ACK 16,269; peer frame 498,369.
- The existing 120-second CI soak passed: 1,460,739 round trips, zero failed;
  goroutines 62.3→66.6 and FDs 82.3→88.3 between the early/late windows,
  within the existing trend guards. This is not a 24-hour native WAN soak.
- All 43 JavaScript panel tests passed. Reapply apply/check changed zero files;
  all eight persistent customization tests passed. The final documentation
  checks and their raw outputs are retained in the audit directory.
- Latest native engine standalone Linux amd64/arm64/386/s390x/armv5/armv6/armv7
  builds and legal archives passed without `11`. Checksums/SBOM were generated,
  and two same-source builds were byte-identical. The final cleanup-repair
  release-artifact rerun also passed; final archives, SHA256SUMS and SBOM.txt
  are in `E:/P_OX/fullpack-waticmp-audit/final-release`. No signature or
  publication was performed. These archives include every runtime repair;
  the subsequent final documentation updates do not change the binary.
- The [benchmark](waticmp-benchmark.md) includes byte-exact raw samples, passing
  and failing transactions, two-run results and confounders. The current-engine
  rerun passed all 16 native samples; xDi failures remain recorded. The 1,000
  attempts test admitted/completed 194 native and 193 xDi transactions over its
  window; simultaneous cap192 was not removed. Both remained healthy afterward.
  No equal-security or universal-speed claim is justified.

Reproduce on a disposable Linux host with raw-socket/network-namespace rights;
use a patched compiler, Node and the existing pinned optional HTTPS helpers
for their dedicated acceptance gate:

```sh
sh tools/check-gofmt.sh
go vet ./...
WATICMP_LIVE=1 go test ./... -count=1 -timeout 20m
WATICMP_LIVE=1 go test ./... -race -coverpkg=./... -coverprofile=coverage.out -count=1 -timeout 20m
go tool cover -func=coverage.out
WATICMP_LIVE=1 go test ./internal/e2e -run '^TestPeerCoordinated(NativeSwitchKeepsUserListener|AuthenticatedAdapters)$' -race -count=1 -v -timeout 5m
WATICMP_NETNS=1 bash internal/e2e/testdata/waticmp-netns.sh
WATICMP_NETNS=1 WATICMP_PEER_TEST=1 bash internal/e2e/testdata/waticmp-netns.sh
WATICMP_NETNS=1 bash internal/e2e/testdata/icmp-coexist.sh
WATICMP_NETNS=1 bash internal/e2e/testdata/peerauto-netns.sh
WATICMP_NETNS=1 WATICMP_DEGRADATION=1 DEGRADATION_ENABLED=true bash internal/e2e/testdata/peerauto-netns.sh
BENCH_REPORT_FAILURES=1 BENCH_OUT=/tmp/fullpack-bench WATICMP_NETNS=1 bash internal/e2e/testdata/icmp-benchmark.sh
python tools/customize/reapply.py --apply
python tools/customize/reapply.py --check
```

Do not mount Go temporary/test storage on the Windows bind for these runs.
Read the actual CI per-package coverage floors; a global percentage is not a
substitute. Never lower floors or treat a skipped privileged test as passed.

## 6. Operator enable, disable, upgrade and rollback

1. Build FullPack from the customized source; retain its original legal files
   and MIT inventory. Native runtime needs Linux raw ICMP capability and
   explicitly enrolled IPv4 addresses. There is no wattun installation.
2. Upgrade both endpoints before enabling native protocol/Auto. Existing
   manual/fallback configs remain valid with Auto absent/disabled.
3. Use Setup Iran → Reverse → Native, or the panel Native family. Supply exact
   user/backend mappings, both identities and only approved candidate endpoints.
   Start in observe-only. Transfer the credential setup link through a trusted
   private channel; import it on Kharej, or use `fullpack link apply`.
4. Verify `fullpack auto status <name> --json` on both peers: HEALTHY, matching
   active/generation, ready candidates and actual user traffic. Test a candidate
   with `fullpack auto test-now <name> <candidate>`.
5. Runtime `observe`, `resume`, `lock`, `unlock`, `switch` and
   `disable-carrier` are local controls. An enrollment lock cannot be weakened
   by runtime unlock. Shared identity/secret/candidate edits require deliberate
   new enrollment on both endpoints; ordinary local edits preserve redacted
   secrets. Manual switch validates fresh carrier traffic and two-peer commit.
6. Per-tunnel Auto disabled means the existing manual/fallback engine owns its
   ports. Global `BACKPACK_DISABLE_AUTO=1` or
   `BACKPACK_DISABLE_WATICMP=1` rejects enabled configurations; it does not
   silently fall back to a weaker or unauthorized carrier.
7. On an update, merge upstream source with AGENTS/CUSTOMIZATIONS/manifest
   intact, reapply naming, verify feature guards, build and run affected tests.
   Preserve durable fencing state/lock permissions. Never restore an older
   state JSON over a live controller or clone an active enrolled identity.
8. Rollback stops both endpoints, saves private configs, replaces only compatible
   FullPack binaries and retains current fencing state. For a version without
   native/Auto support, restore a reviewed manual configuration on both peers
   while both are stopped; use fresh enrollment for any later Auto pairing.
   Do not run unsupported Auto against an old upstream binary or roll back
   generations. Deleting/recreating identity is a new pairing, not recovery.

Fork signed release publication still needs its own signing key/public-key
configuration; no private key was generated or added, and verification was
not disabled. The unsigned local artifacts are review/test outputs.

## 7. Practical limits and unverified environments

Real two-VPS filtered WAN, NAT/address change, loaded interactive benchmark
latency, sustained 24-hour soak and a published previous-release binary are
**NOT VERIFIED**. Local HEAD interoperability is labeled accordingly. Neither
a raw loopback pass nor a namespace matrix proves WAN throughput or availability.

This implementation assumes a permitted reachable enrolled path and running
peers. No reachable physical route means NO_ROUTE; no software can repair
fully blocked ICMP/TCP/UDP or a powered-off remote machine. Auto resource
pressure thresholds skip probes; they are not an OS-enforced total RSS cap.
Maximum simultaneous Auto users is 192, not 1,000. TCP may end during failure
or the bounded 30 s drain. Small ACK/control/handshake traffic is separately
bounded and is not a strict all-packet bandwidth guarantee.

Recommendation: use observe-only for the first real two-server evaluation,
keep a reviewed manual recovery configuration and collect both-peer status
and real TCP/UDP results before enabling automatic cross-carrier changes.
WAN testing/deployment needs the user's actual servers and authorization;
neither was inferred from a request to edit the local project.

The explicit filtered-network failure/recovery/degradation matrix uses Wat
and Noise. QUIC/WSS/WSSMUX were tested for real coordinated bootstrap, probe,
handover and drain on loopback; the corresponding filtered two-node outage
matrices are **NOT RUN**. Do not extend the Wat/Noise outage timings to them.

## خلاصهٔ فارسی

<div dir="rtl">

قابلیت بومی Wat و هماهنگی Auto در سورس FullPack پیاده‌سازی شده‌اند و xDi روش
مستقل قبلی باقی مانده است. تست‌های واقعی داخل محیط Linux شامل ترافیک TCP/UDP،
جهت عادی و معکوس، قطع مسیر، بازیابی، پیام‌های جعلی/بازپخش و افت شدیدِ مسیر
همچنان سالم انجام شده‌اند. ایراد تایمرهای RTT در نسخهٔ اقتباس‌شده اصلاح شد؛
پوشهٔ WatTunnel اصلی تغییر نکرد.

نتایج موفق، شکست‌های قبلی و محدودیت‌ها جدا ثبت شده‌اند. تست نهایی کل پروژه با
race detector و پوشش ۵۷٫۳٪، هر ۱۴ حداقل پوشش، ساخت هفت معماری و چرخهٔ واقعی
نصب، ارتقا، بازگشت، حذف تونل و حذف برنامه موفق بودند. پوشهٔ همیارهای نصب‌شده
نیز پس از حذف پاک می‌شود و ابزارهای بیرون از آن حفظ می‌شوند. هیچ VPS کاربر
یا انتشار GitHub در مرحلهٔ پیاده‌سازی تغییر نکرده است. پوش سورس به GitHub پس از
این بررسی با درخواست جداگانهٔ کاربر انجام می‌شود. پیشنهاد راه‌اندازی اولیه حالت
مشاهده است؛ نتیجهٔ آزمایشگاهی
به معنی تضمین عملکرد روی WAN یا مهاجرت بدون قطع اتصال نیست.

</div>

*Last verified against FullPack v1.9.0 (local acceptance with documented limits, 2026-10-13).*
