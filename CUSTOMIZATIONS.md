# FullPack customization ledger

## Approved preferences

| Date | Preference | Scope |
| --- | --- | --- |
| 2026-10-12 | Project name: **FullPack** (two `l` characters) | CLI, web panel/PWA, Telegram messages, current documentation, installation and release packaging |
| 2026-10-12 | Preserve customizations through upstream updates | Repository instructions, versioned reapply manifest/tool and verification |
| 2026-10-12 | Add CLI tunnel methods missing from the web panel | Reverse Naive/HTTP2, XHTTP/TLS, REALITY/Vision; direct IP and SNI spoofing |
| 2026-10-13 | Execute audited Wat ICMP / peer coordination / intelligent Auto prompt | Opt-in native `waticmp`, preservation of xDi/manual/fallback defaults, immutable local WatTunnel reference, licensed source attribution and evidence gates |

## Approved Wat ICMP and peer Auto implementation

- Preserve `docs/waticmp-dual-server-design.md`, `docs/waticmp-verification.md`,
  the native engine's MIT LICENSE/ORIGIN.json and its security/resource repairs
  through upstream source merges. Do not rename xDi or replace its wire.
- `11/WatTunnel` is the authoritative read-only reference, excluded from Git,
  formatting, clean builds and releases. Native source is adapted into
  `internal/tunnel/waticmp`, without a local module replacement or helper process.
- Auto is disabled by default. Preserve typed peer authentication, explicit
  enrollment, stable Iran port ownership, fresh cryptographic incarnations,
  persisted generation fencing and exclusive per-tunnel controller ownership.
- This ledger records authorized scope; the verification ledger records which
  parts actually pass. Do not turn pending acceptance into a production claim.
- Native normal/reverse stream forwarding uses the attributed Wat engine's
  actual Session/Stream, not xDi/KCP, smux or a helper executable. Preserve fresh
  bilateral X25519/PSK keys, directional nonces, bounded packet/stream/window
  resources, ACK arithmetic, stream deadlines, FIN drain and joined shutdown.
  Preserve authenticated DF/PMTU checks and the delayed-path repairs: initial
  RTT timeout, bounded retired-packet timing, and full paced-header accounting.
- Auto owns stable Iran TCP/UDP ports and is the sole switching authority for
  that tunnel. Preserve the strict typed mutually authenticated protocol,
  explicit candidate/backend enrollment, fresh isolated probe sessions, durable
  fsync/rename generations, exclusive no-follow ownership locks, uncertain-
  commit fencing and reverse-only pre-enrolled recovery. Peer failures remain
  independent; a slow/offline application backend is not a carrier failure.
- Strong Auto adapters are Wat ICMP, Noise/stealth, QUIC, WSS and WSSMUX. Reject
  unverified/plaintext adapters even if a plaintext opt-in field is posted.
  Direct/L3 Wat and Auto remain explicitly unsupported in this version.
- Preserve default-off Auto and default-off severe-degradation switching,
  observe-only, manual lock/disable/test actions, pressure SKIPPED results,
  global/per-pair probe budgets, jitter, debounce, quarantine, cooldown, dwell
  and persisted hourly switch limits. Never change a healthy path for a small
  score advantage. Old TCP may drain or be interrupted; no migration claim.
- Preserve terminal and web setup/edit/status/action parity, secret-preserving
  edits, version-3 credential links and trusted Kharej enrollment import.
  Native forms hide incompatible legacy tuning; switching back restores the
  managed HTTPS forms. An imported client is reported as imported/waiting,
  not as an already-connected peer requiring another enrollment handoff.
- Connection Test option 0 includes actual normal/reverse native data tests
  when both explicit IPs are supplied; report missing prerequisites as SKIPPED.
  Its existing TCP/UDP test coordinator is distinct from Auto's reverse-only
  bootstrap and from TCP route Link Test diagnostics.
- Preserve `tools/package-release.sh`, embedded `fullpack licenses` attribution,
  Linux raw/network-namespace fixtures and the final evidence in
  `docs/waticmp-verification.md` / `docs/waticmp-benchmark.md`. Naming reapply
  cannot recreate these features. Merge source and pass feature guards/tests.
  Preserve actual authenticated QUIC/WSS/WSSMUX Auto setup, independent probe
  and user-stream handover/drain regression tests alongside native coverage.
- Uninstall must first stop/delete tunnel controllers safely, preserve files
  when that fails, and remove the default installer-owned helper tree after
  services stop. Never recursively follow a symlinked/public helper root or
  remove operator-supplied executable paths outside that dedicated namespace.

## Native/Auto verification recorded on 2026-10-13

- Whole-project Linux race/coverage, formatting, vet and staticcheck passed
  with Go 1.27.2; final total coverage was 57.3% and all fourteen existing
  package floors passed without being lowered. All 43 panel JavaScript tests
  and the eight customization-tool tests passed.
- Actual isolated two-node normal/reverse native traffic, restricted MTUs,
  cross-carrier recovery, reachable severe degradation, replay/identity
  isolation and legacy local-HEAD interoperability passed. Existing CI soak
  and bounded parser fuzz runs passed; measured xDi benchmark failures remain
  documented rather than removed.
  QUIC/WSS/WSSMUX Auto also passed actual sole-candidate bootstrap, isolated
  probes and coordinated handover/drain to Noise on loopback with race.
- Disposable Linux source installation, systemd traffic, rename, upgrade,
  compatible earlier-local-build rollback, deletion and uninstall passed.
  Installer-owned helper cleanup preserves external operator tools.
- Seven standalone Linux architecture archives include legal attribution and
  exclude the unchanged read-only WatTunnel tree. Two same-source builds were
  byte-identical. No signing key, publication or live deployment was performed.
- Evidence and exact limitations are in `docs/waticmp-verification.md`.
  Actual filtered two-VPS WAN, NAT/address changes, a published earlier-release
  binary, loaded interactive latency and a 24-hour soak remain NOT VERIFIED.
  Local acceptance must not become a production guarantee during an update.

## Source publication authorization on 2026-10-13

- After local acceptance, the operator requested committing and pushing the
  customized FullPack source to the existing `firegoood/BackPack` fork.
- This authorization covers this source publication. It does not authorize
  release tags, unsigned binary publication, live deployment or automatic
  publication of later work. The read-only reference and local audit artifacts
  remain excluded from Git.

## Web tunnel methods

- The reverse options API reads `cliTransportGroups()`; the direct options API
  exposes the complete `DirectCarriers()` list. The panel no longer maintains a
  separate filtered list of supported methods.
- HTTPS setup and edit offer the installed helper path, loopback reverse port,
  credentials, certificates/trust and protocol-specific fields. Server identities
  can be generated, while clients take matching values from a setup link.
  Naive needs sing-box on the server and naive on the client; XHTTP/REALITY need
  Xray. Blank helper paths use the existing CLI defaults, and missing helpers
  produce a validation error. The form does not download executables.
- Managed HTTPS uses the existing reverse TCP engine on loopback. The panel
  displays and edits the public HTTPS endpoint separately, refuses UDP and
  incompatible route/proxy/failover settings, and runs CLI/engine validation.
  REALITY server setup checks the selected cover with the existing bounded,
  authenticated TLS/HTTP2 and helper probe before saving.
- Dashboard transport names, public ports and peer/latency probes describe the
  HTTPS helper. Engine health still uses the underlying reverse TCP session;
  the helper's loopback connection is not reported as a remote peer.
- Edit preset menus filter the options API's `kcpOnly` flag and refresh when
  transport changes. Switching KCP to HTTPS retires a Throughput label while
  preserving manual tuning, or applies a newly selected compatible profile.
  Explicitly posting Throughput for HTTPS is refused. Switching between Naive
  and Xray clears the incompatible executable so the new helper's CLI default
  applies; XHTTP/REALITY retain their shared custom Xray executable.
- Settings responses omit passwords and REALITY private keys. Omitted edit
  fields keep their old values; empty passwords/private keys keep their secrets.
  Peer links contain the matching identity, public certificate or REALITY public
  key, never the server TLS/REALITY private key. Naive links infer the dial
  hostname from the certificate when no address was explicitly provided.
  Explicit Naive hostnames and link addresses must match that certificate.
  Clearing an optional HTTPS host or generated identity field still triggers
  the peer setup-link handoff; blank redacted secrets keep their existing value.
- The managed-node mirror carries HTTPS credentials, both ports and public CA
  trust in its JSON apply request. The destination validates and stores the
  public PEM locally and retires incompatible proxy/routing/simple-auth settings.
  Rebuilding the peer preserves its local helper path and loopback port when
  the helper is compatible (XHTTP/REALITY share Xray); peer credentials/trust
  and the public endpoint are replaced with the server's current values.
- IP spoofing uses the existing direct spoof drawer and peer-IP validation;
  SNI spoofing uses the existing domain field. The direct engine and wire
  protocol are unchanged. The legacy layer-4 engine is not a separate selectable
  CLI setup method in this baseline.
- Preserve `internal/manage/managedwebapi.go`, its tests, the shared
  `panel/js/ui/managed.js` helper and their integration in API/forms through
  updates. Required manifest snippets catch discarded integration points;
  the tests check both roles, validation, edit secrets/endpoints, generated
  identities, certificates, peer links and complete CLI/web choices.

## Naming and compatibility

- Display name: `FullPack`; executable: `fullpack`.
- Installation: `/root/FullPack`, `/etc/fullpack`, `/usr/local/bin/fullpack`.
- Services: `fullpack-*.service`; release assets: `fullpack_linux_<arch>.tar.gz`.
- Source/update repository: <https://github.com/firegoood/BackPack>. The GitHub
  repository has not been renamed. The updater must not download official
  BackPack binaries, because they do not contain this customization layer.
- Upstream source: <https://github.com/AminMGMT/BackPack>.
- The Go module path, tunnel protocol labels, `backpack://` setup links, backup
  metadata, Prometheus metric identifiers and existing environment variables
  remain compatible. These internal identifiers are not display branding.
- The original licensing/attribution documents and historical changelog remain
  unchanged. README attribution stays visible and unaltered.
- This is a fresh FullPack installation namespace. Migrating an already running
  BackPack installation requires a separate, reviewed migration of its config
  and services; a naming change does not perform that migration.

## Integrating a later upstream release

1. Start from a clean, saved FullPack checkout. Preserve the customization files
   and review upstream release notes and the source diff.
2. Fetch the desired upstream tag/commit and merge or rebase its **source** into
   FullPack. Resolve conflicts without discarding customizations or new upstream
   fixes. Do not copy an upstream binary over the FullPack executable.
3. Run `python tools/customize/reapply.py --apply`. The tool performs local,
   repeatable naming changes only; it does not fetch, reset, push or publish.
4. Run `python tools/customize/reapply.py --check` and inspect the diff. A failed
   check means the update is not ready; update the overlay and feature integration
   for structural changes instead of ignoring its diagnostics. The tool reapplies
   naming and verifies feature guards; source merges must preserve feature code.
5. Run formatting checks, build and relevant existing tests. For a release, run
   the repository's CI checks, including Linux, race and wire-compatibility tests.
6. Build and sign **FullPack** assets from the customized source. Configure the
   fork's signing public key in both the application and installer and store the
   private key only in the release secret. The inherited public key remains
   unchanged until a fork signing key is deliberately configured.

The reapply tool is idempotent. It handles the currently known naming surfaces;
upstream structural changes still require code review and test verification.

## Verification recorded on 2026-10-12

- FullPack naming check passed; repeating `--apply` changed zero files.
- Eight customization-tool tests passed, including repeatability, attribution,
  compatible cryptographic/protocol labels and fork release routing.
- Selected existing application naming/licensing/installer tests and terminal
  UI tests passed; all 35 JavaScript panel tests passed after the second review.
- `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath` succeeded.
- Linux short suites for `internal/manage` and `internal/webui` passed, including
  the web transport additions; these and `internal/node` also passed with the
  race detector. Windows CRLF checkout files were normalized only in an isolated
  Linux test copy for existing source-inspection tests.
- Browser checks used the real panel JS/templates with a local mock API: HTTPS
  setup, XHTTP edit, IP/SNI spoofing fields, method switching and the REALITY
  submitted payload were verified. Only applicable fields were sent.
  No production tunnel was created.
- Native Windows compilation remains unsupported by existing Linux socket
  code. The complete release CI, including whole-project race/coverage and
  security checks, remains required before publishing. No GitHub changes were
  published.

## Second review on 2026-10-12

- Fixed the missing peer handoff when an HTTPS host/identity field is cleared.
  Empty redacted passwords/private keys still preserve the old secret.
- Fixed node mirroring so compatible custom client helper executables and
  private loopback ports survive peer rebuilds; new peer identities still apply.
- Naive setup and exported links now reject certificate/address mismatches;
  node mirroring uses the certificate's concrete hostname when available.
- Fixed HTTPS dashboard summaries and latency/peer lookups to use the public
  helper protocol/address/port. Loopback helper peers are excluded from remote
  location fallback. Engine health and wire identifiers remain unchanged.
- Corrected Go formatting in the renamed Telegram translation table.
- All default Linux Go suites passed without `-short`, with Node 22 available
  for the panel API contract tests. `go vet ./...`, formatting and Linux build
  also passed. After the dashboard fix, the affected manage/webui/node suites
  were rerun with `-race -short`, and web vet/build passed on the final source.
- The pinned Naive, sing-box and Xray archives were checksum-verified and
  installed only in an isolated container. Real loopback traffic passed all
  13 tested reverse carriers, including Naive, XHTTP and REALITY (8/8 echoes
  each); authenticated REALITY cover/fallback/cancellation tests also passed
  with the race detector. The source installer succeeded in that container.
- All 35 JavaScript tests and 8 customization-tool tests passed. Repeating the
  naming reapply changed zero files, and the updated feature guards passed.
- Production systemd deployment and an actual two-VPS filtered-network test
  were not performed; loopback results do not measure those environments.

## Final review on 2026-10-12

- Added regression tests that reproduced two transport-switching mistakes:
  retaining a KCP-only Throughput profile on HTTPS and reusing Xray as the
  Naive executable (or sing-box as the client helper). Both were corrected.
- All Linux Go packages passed `go test ./... -race -count=1 -timeout 20m`
  with Node 22 and the checksum-verified pinned HTTPS helpers installed.
  Full formatting, `go vet ./...` and the Linux build passed as well.
  After the final profile/helper fixes, affected manage/webui race suites,
  their validators/API contract tests and the embedded-panel build were rerun.
- All 40 JavaScript tests and 8 customization-tool tests passed. Required
  update guards include the new profile/helper switching behavior. Repeating
  the naming reapply still changed zero files; whitespace checks passed.
- Real panel templates/JS were checked in the browser with a local mock API:
  KCP offered Throughput, switching to HTTPS selected a compatible profile,
  the HTTPS menu excluded Throughput, and XHTTP-to-Naive cleared the Xray path.
  No form was submitted to a production server.
- Changes outside mechanical naming were checked against the naming transform
  and matched the recorded web additions, tests and formatting. LICENSE,
  NOTICE, TRADEMARK.md, historical changelog, Go module and dependency files
  remain unchanged.
- An actual two-VPS/systemd deployment is still outside this verification.
  Release coverage/security gates must run before publishing a release.

## Independent final recheck on 2026-10-12

- A complete fresh Linux race run, build, formatting and vet passed before
  the additional security check. Staticcheck v0.8.2 reported no findings.
- The security scan found 12 reachable standard-library vulnerabilities in
  the audit image's Go 1.27.1. The project's Go 1.26.9 baseline contains the
  fixes; on the newer branch they require Go 1.27.2. See the
  [official release history](https://go.dev/doc/devel/release).
- Regression tests reproduced the installer accepting Go 1.27.0/1.27.1 just
  because their minor version is higher. The installer now rejects these
  versions, uses an available patched compiler or requests its checksum-verified
  Go 1.26.9 download. Tests cover both rejection and actual compiler selection.
  Repository rules and required update guards preserve this correction.
- Govulncheck v1.9.0 on Go 1.27.2 reported zero reachable vulnerabilities and
  zero vulnerable imported packages. It noted two module-level advisories on
  code the application does not call. Module requirements remain unchanged.
- The public database endpoint returned HTTP 403. The scan instead used all
  4,613 official OSV records from `golang/vulndb` snapshot
  `d5e0cfac5d260523df858cd4c0f849e248468fb6`, with Git-derived timestamps and
  the documented local database API. Full affected ranges were retained;
  optional fixed-version shortcuts were omitted so records were not skipped.
- On the final source with Go 1.27.2 and Node 22, all Linux packages passed
  `go test ./... -race -count=1 -timeout 20m`; the installer regressions also
  passed separately with the race detector. Formatting, `go vet ./...` and
  a static Linux build passed. All 40 panel JavaScript tests and 8 customization
  tests passed; reapplying naming changed zero files and whitespace checks
  were clean. No additional CLI/web parity or HTTPS edit/mirror issue was found.
- Actual two-VPS/systemd deployment and release coverage remain untested.
  No release was published and no user server was changed.
