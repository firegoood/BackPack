# Measured native Wat ICMP and xDi comparison

Date: 2026-10-13. This is a reproducible local fixture, not a WAN capacity claim.

The [32 raw samples](waticmp-benchmark.jsonl) are preserved byte-for-byte from
`current-benchmark-report/results.jsonl` (SHA256 `e1ba7ea1bb03f4c104615a1a37637c1d8e8f32ab524503de8828103a64160ab4`).

Both peers share a disposable container with a two-CPU quota, separate Linux
network namespaces, MTU 576, symmetric 10 ms ±2 ms delay, 0/1/5/10% loss,
0.1% duplication, 1% reorder (50% correlation), an 8 Mbps wire ceiling and
2,000 ICMP packets/s ceiling with burst 200 per direction. Cross-node TCP/UDP
is blocked; native uses reverse requests. Each row below contains **two runs**.
Small load: eight connections ×512 bytes each direction. Medium: 100
connections ×32 KiB each direction. All rows also exercise UDP 1/64/1400/8000.

**Confounders:** native uses fresh authenticated AEAD sessions; legacy xDi
uses its unchanged AES/CRC/token/KCP path. Security, framing and pacing are
different. These are equal path/workload limits, not equal-security or
unlimited-capacity measurements. RTT/jitter comes from 30 interactive
transactions **before** bulk; loaded interactive latency is NOT VERIFIED.
CPU is summed engine process time divided by sample duration and two CPUs;
kernel/client workload CPU is excluded. RSS/FD/goroutine peaks are sampled
every 250 ms through private IPC/process statistics, so brief peaks can escape.
Kernel ICMP counters include host echo/policing, not only useful carrier frames.

| Load | Carrier | Loss % | TCP passed/attempted | Useful Mbps each direction | Idle RTT p50/p95 ms | Jitter ms | CPU % of 2 | Aggregate peak RSS MiB | FD / goroutines peak | Host ICMP out pps | Cumulative retransmissions at end (Wat frames / KCP segments) |
|---|---|---:|---|---|---|---|---|---|---|---|---|
| small | waticmp | 0 | 8/8 / 8/8 | 0.647 / 0.645 | 21.6,24.1 / 21.6,24.4 | 1.88 / 1.83 | 7.4 / 6.0 | 45.9 / 46.6 | 22,61 / 23,66 | 271.4 / 254.9 | 0 / 0 |
| small | waticmp | 1 | 8/8 / 8/8 | 0.591 / 0.638 | 22.2,24.9 / 21.5,24.3 | 18.20 / 9.73 | 9.5 / 7.9 | 44.4 / 46.0 | 22,61 / 30,75 | 211.0 / 264.2 | 2 / 5 |
| small | waticmp | 5 | 8/8 / 8/8 | 0.424 / 0.335 | 21.9,142.8 / 22.6,144.2 | 26.34 / 58.10 | 6.6 / 9.0 | 44.2 / 45.6 | 29,70 / 25,76 | 188.4 / 158.1 | 18 / 33 |
| small | waticmp | 10 | 8/8 / 8/8 | 0.425 / 0.347 | 22.6,366.7 / 22.3,142.8 | 56.24 / 34.44 | 7.5 / 6.0 | 44.6 / 45.9 | 22,61 / 33,91 | 122.3 / 161.6 | 24 / 41 |
| small | xdi | 0 | 8/8 / 8/8 | 0.296 / 0.410 | 23.0,25.6 / 22.5,24.3 | 2.14 / 1.71 | 9.9 / 6.4 | 45.4 / 45.9 | 34,135 / 26,113 | 188.1 / 238.7 | 0 / 0 |
| small | xdi | 1 | 8/8 / 8/8 | 0.397 / 0.430 | 22.9,24.8 / 22.7,24.9 | 5.97 / 8.29 | 4.7 / 5.9 | 45.9 / 46.4 | 25,108 / 26,113 | 195.5 / 196.1 | 3 / 6 |
| small | xdi | 5 | 8/8 / 8/8 | 0.271 / 0.413 | 22.5,49.9 / 22.5,124.0 | 9.20 / 22.44 | 4.8 / 4.0 | 45.6 / 46.5 | 25,108 / 36,143 | 195.3 / 150.1 | 13 / 28 |
| small | xdi | 10 | 8/8 / 8/8 | 0.095 / 0.049 | 22.9,321.9 / 22.7,181.8 | 73.33 / 68.32 | 3.0 / 3.1 | 45.5 / 46.4 | 31,126 / 35,141 | 81.4 / 85.0 | 56 / 84 |
| medium | waticmp | 0 | 100/100 / 100/100 | 2.864 / 2.919 | 22.0,23.9 / 21.9,24.3 | 2.06 / 1.86 | 17.6 / 18.4 | 70.9 / 82.2 | 222,665 / 223,670 | 1635.9 / 1649.1 | 0 / 0 |
| medium | waticmp | 1 | 100/100 / 100/100 | 2.796 / 2.786 | 22.3,23.9 / 21.8,24.5 | 10.05 / 10.11 | 17.4 / 18.1 | 72.6 / 84.2 | 222,665 / 223,670 | 1647.4 / 1653.9 | 168 / 312 |
| medium | waticmp | 5 | 100/100 / 100/100 | 2.880 / 2.956 | 22.3,143.5 / 21.6,141.4 | 18.23 / 26.26 | 18.7 / 18.0 | 69.1 / 83.2 | 222,665 / 223,670 | 1639.3 / 1651.9 | 788 / 1564 |
| medium | waticmp | 10 | 100/100 / 100/100 | 2.921 / 2.957 | 22.3,142.7 / 22.9,144.8 | 50.17 / 51.09 | 17.8 / 17.8 | 69.9 / 84.2 | 222,665 / 223,670 | 1603.1 / 1626.6 | 1682 / 3337 |
| medium | xdi | 0 | 75/100 / 86/100 | 2.668 / 2.856 | 22.5,24.3 / 22.0,25.1 | 1.71 / 1.28 | 11.6 / 12.1 | 68.2 / 77.6 | 172,468 / 184,561 | 1536.2 / 1672.7 | 63 / 222 |
| medium | xdi | 1 | 66/100 / 71/100 | 2.114 / 2.014 | 21.6,23.9 / 22.7,25.0 | 1.46 / 8.54 | 11.4 / 10.0 | 67.8 / 77.5 | 161,399 / 183,524 | 1299.5 / 1239.6 | 94 / 215 |
| medium | xdi | 5 | 52/100 / 92/100 | 0.948 / 0.940 | 23.1,122.0 / 22.4,121.1 | 36.36 / 15.56 | 6.7 / 7.6 | 66.5 / 81.6 | 162,387 / 200,646 | 657.8 / 704.8 | 508 / 1375 |
| medium | xdi | 10 | 100/100 / 66/100 | 0.626 / 0.521 | 22.3,121.7 / 22.2,124.6 | 29.16 / 39.18 | 6.3 / 8.2 | 83.4 / 87.6 | 219,704 / 152,482 | 525.2 / 422.7 | 2076 / 3473 |

xDi reset some connections under the 0%-loss medium load (75/100 and 86/100).
Other measured xDi failures remain in the raw data; successful-byte throughput
must be read together with passed/attempted connections. The first strict
`benchmark.log` stopped on such a failure; the report run records failures
rather than treating them as successful traffic. All 16 native samples
passed their TCP and UDP integrity workload.

Overload fixture: 1,000 **attempts**, not 1,000 simultaneously admitted users.
The admitted-flow ceiling is 192. Native: 194 passed/806 refused in 1.013 s,
83.500 MiB aggregate sampled RSS. xDi: 193 passed/807 refused in 0.815 s,
83.125 MiB RSS; completions over the measurement window may exceed the
simultaneous cap. A subsequent TCP/UDP health transaction passed on both.

Native default paced-packet limits and admission ceilings remain conservative.
Do not extrapolate these results to a 1,000-user VPS, claim native is universally
faster, or use reference WatTunnel throughput as a FullPack measurement.

Reproduce only in a disposable privileged Linux host/container:

```sh
BENCH_REPORT_FAILURES=1 BENCH_OUT=/tmp/fullpack-bench WATICMP_NETNS=1 bash internal/e2e/testdata/icmp-benchmark.sh
```

Read the script for output/workload knobs; the fixture changes only its own
namespaces and deletes them on exit. Final source acceptance is tracked in
[the verification ledger](waticmp-verification.md).

## خلاصهٔ فارسی

<div dir="rtl">

این اعداد از آزمایش محلی با سهمیهٔ دو CPU و مسیر محدودشده به‌دست آمده‌اند.
سرعت شبکهٔ واقعی ایران و خارج تأیید نشده است. تعداد تلاش‌های اتصال با تعداد
کاربر هم‌زمان یکی نیست؛ خطاهای xDi و تفاوت امنیت دو موتور در داده‌ها حفظ شده‌اند.

</div>

*Last verified against FullPack v1.9.0 (measured fixture, 2026-10-13).*
