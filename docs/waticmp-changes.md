# Native Wat ICMP / peer Auto change inventory

Date: 2026-10-13. Target: `E:/P_OX/back`; local HEAD `132467bfdfb2698d74ada2992413e75bfccbfbd3`.

The starting work already contained FullPack naming, managed HTTPS and direct
spoofing changes. The lists below compare tracked diff blocks against the
saved `before.patch`, rather than presenting every uncommitted naming file
as a new Wat/Auto change. Existing untracked filenames are taken from
`before-status.txt`; their original contents were not Git diff objects.
The three persistent customization files are explicitly listed separately.

Native attribution and exact adapted/reference file hashes are in
`internal/tunnel/waticmp/engine/ORIGIN.json`. Reference `11/WatTunnel` is excluded,
read-only and absent from every list and release archive.

## Tracked files changed since the task baseline

- `.github/workflows/ci.yml`
- `.gitignore`
- `Makefile`
- `cmd/cmd.go`
- `cmd/defaults.go`
- `cmd/enginesocket.go`
- `config/config.go`
- `config/fallback.go`
- `docs/README.md`
- `docs/config-reference.md`
- `internal/cli/cli.go`
- `internal/client/client.go`
- `internal/client/transport/parity_test.go`
- `internal/enginectl/enginectl.go`
- `internal/enginectl/enginectl_test.go`
- `internal/manage/conntest.go`
- `internal/manage/conntest_live_test.go`
- `internal/manage/conntest_test.go`
- `internal/manage/conntestboard.go`
- `internal/manage/conntestmenu.go`
- `internal/manage/core/rename.go`
- `internal/manage/core/tunnel.go`
- `internal/manage/health/health.go`
- `internal/manage/health/watchdog.go`
- `internal/manage/linkapply.go`
- `internal/manage/menu.go`
- `internal/manage/peerform.go`
- `internal/manage/reversesetup.go`
- `internal/manage/setup.go`
- `internal/manage/sharelink.go`
- `internal/manage/sharelink_fuzz_test.go`
- `internal/manage/sharelinkv2.go`
- `internal/manage/spec/transport.go`
- `internal/manage/testdata/conntestlive.sh`
- `internal/manage/tunnelspec/load.go`
- `internal/manage/tunnelspec/spec.go`
- `internal/manage/webapi.go`
- `internal/manage/webapi_test.go`
- `internal/menu/system.go`
- `internal/metrics/metrics.go`
- `internal/server/server.go`
- `internal/utils/network/quic.go`
- `internal/utils/network/wssbind.go`
- `internal/webui/handlers_conntest.go`
- `internal/webui/panel/css/screens/add.css`
- `internal/webui/panel/css/screens/edit.css`
- `internal/webui/panel/js/api.js`
- `internal/webui/panel/js/views/add.js`
- `internal/webui/panel/js/views/conntest.js`
- `internal/webui/panel/js/views/edit.js`
- `internal/webui/paneltest/apicalls.mjs`
- `internal/webui/server.go`
- `tools/configref/main.go`

## New implementation, tests and documentation files

- `cmd/waticmpconfig_test.go`
- `config/auto.go`
- `config/waticmp.go`
- `docs/waticmp-benchmark.jsonl`
- `docs/waticmp-benchmark.md`
- `docs/waticmp-changes.md`
- `docs/waticmp-dual-server-design.md`
- `docs/waticmp-verification.md`
- `internal/cli/licenses.go`
- `internal/client/transport/waticmp.go`
- `internal/e2e/peerauto_carriers_test.go`
- `internal/e2e/peerauto_isolation_test.go`
- `internal/e2e/peerauto_test.go`
- `internal/e2e/testdata/icmp-benchmark.sh`
- `internal/e2e/testdata/icmp-coexist.sh`
- `internal/e2e/testdata/icmp-negative.py`
- `internal/e2e/testdata/icmp-resources.py`
- `internal/e2e/testdata/peerauto-netns.sh`
- `internal/e2e/testdata/waticmp-netns.sh`
- `internal/e2e/testdata/waticmpfixture.go`
- `internal/e2e/waticmp_test.go`
- `internal/enginectl/ownership_linux.go`
- `internal/enginectl/ownership_other.go`
- `internal/enginectl/ownership_test.go`
- `internal/manage/conntest_waticmp.go`
- `internal/manage/health/peerauto_test.go`
- `internal/manage/installedhelpers.go`
- `internal/manage/installedhelpers_test.go`
- `internal/manage/peeredit.go`
- `internal/manage/peerenrollment.go`
- `internal/manage/peerfeatures_test.go`
- `internal/manage/peerlink.go`
- `internal/manage/peermenu.go`
- `internal/manage/peersetup.go`
- `internal/metrics/diagnostics.go`
- `internal/metrics/diagnostics_test.go`
- `internal/server/transport/waticmp.go`
- `internal/tunnel/peerauto/actions.go`
- `internal/tunnel/peerauto/adversarial_test.go`
- `internal/tunnel/peerauto/carrier.go`
- `internal/tunnel/peerauto/config.go`
- `internal/tunnel/peerauto/enrollment.go`
- `internal/tunnel/peerauto/lock_linux.go`
- `internal/tunnel/peerauto/lock_other.go`
- `internal/tunnel/peerauto/manager.go`
- `internal/tunnel/peerauto/outcomes_test.go`
- `internal/tunnel/peerauto/policy.go`
- `internal/tunnel/peerauto/probes.go`
- `internal/tunnel/peerauto/router.go`
- `internal/tunnel/peerauto/state_test.go`
- `internal/tunnel/peerauto/statefiles.go`
- `internal/tunnel/peerauto/statefiles_test.go`
- `internal/tunnel/peerauto/store.go`
- `internal/tunnel/peerauto/wire.go`
- `internal/tunnel/peerauto/wss.go`
- `internal/tunnel/probe/exchange.go`
- `internal/tunnel/probe/exchange_test.go`
- `internal/tunnel/waticmp/capability_linux.go`
- `internal/tunnel/waticmp/capability_other.go`
- `internal/tunnel/waticmp/config.go`
- `internal/tunnel/waticmp/conn.go`
- `internal/tunnel/waticmp/conn_test.go`
- `internal/tunnel/waticmp/engine/LICENSE`
- `internal/tunnel/waticmp/engine/ORIGIN.json`
- `internal/tunnel/waticmp/engine/backoff.go`
- `internal/tunnel/waticmp/engine/batch_linux.go`
- `internal/tunnel/waticmp/engine/cc.go`
- `internal/tunnel/waticmp/engine/datagram.go`
- `internal/tunnel/waticmp/engine/fec.go`
- `internal/tunnel/waticmp/engine/icmp_linux.go`
- `internal/tunnel/waticmp/engine/icmp_other.go`
- `internal/tunnel/waticmp/engine/io.go`
- `internal/tunnel/waticmp/engine/legal.go`
- `internal/tunnel/waticmp/engine/paths.go`
- `internal/tunnel/waticmp/engine/ranges.go`
- `internal/tunnel/waticmp/engine/safety_test.go`
- `internal/tunnel/waticmp/engine/sched.go`
- `internal/tunnel/waticmp/engine/session.go`
- `internal/tunnel/waticmp/engine/stream.go`
- `internal/tunnel/waticmp/engine/wire.go`
- `internal/tunnel/waticmp/listener.go`
- `internal/tunnel/waticmp/pmtu.go`
- `internal/webui/handlers_peerauto.go`
- `internal/webui/panel/js/ui/peerauto.js`
- `internal/webui/paneltest/peerauto.test.js`
- `internal/webui/peerenrollment_test.go`
- `tools/check-gofmt.sh`
- `tools/package-release.sh`

## Updated persistent rules already present at the baseline

- `AGENTS.md`
- `CUSTOMIZATIONS.md`
- `tools/customize/fullpack.json`

## Review and preservation boundaries

This includes baseline fixture readiness, source/API documentation, raw/Auto
CI gates, native configuration/factories, lifecycle/fencing, CLI/web enrollment
and controls, metrics, installer/uninstall safety, tests and legal packaging.
The existing reverse/direct carriers, licensing texts, Go dependencies and
upstream-compatible namespaces are retained. Exact algorithms and defects are
explained in the design and verification ledgers; a filename alone is not
evidence of passing a gate.

No generated release binary, private key, reference archive, runtime state
or audit fixture is added to this repository. Local test artifacts stay outside
it. User-authorized preferences and update checks remain in CUSTOMIZATIONS
and the manifest. During the implementation audit there was no commit/push/
release or upstream replacement. The operator authorized source publication
to the existing fork after that audit; these lists describe its development
baseline, not a claim that the published source has no Git commit.

## خلاصهٔ فارسی

<div dir="rtl">

این فهرست تغییرات این توسعه را از شخصی‌سازی‌های موجود FullPack جدا می‌کند.
پوشهٔ مرجع تغییر نکرده است. گزارش تست‌ها و محدودیت‌ها در سند تأیید قرار دارد؛
وجود یک فایل در این فهرست به معنی موفقیت تست یا تأیید انتشار نیست.

</div>

*Last verified against FullPack v1.9.0 (local source inventory, 2026-10-13).*
