"""Private disposable-fixture sampler; no production management listener."""
import json, os, pathlib, socket, sys, time

output = pathlib.Path(sys.argv[1])
pids = [int(p) for p in sys.argv[2:]]
hz = os.sysconf('SC_CLK_TCK')
def proc(pid):
    fields = pathlib.Path(f'/proc/{pid}/stat').read_text().split(') ', 1)[1].split()
    return (int(fields[11]) + int(fields[12])) / hz, int(fields[21]) * os.sysconf('SC_PAGE_SIZE'), len(os.listdir(f'/proc/{pid}/fd'))

start = time.monotonic()
cpu_start = sum(proc(p)[0] for p in pids)
peak_rss = peak_fd = peak_go = 0
last_diagnostics = {}
process_exited = False
while True:
    try:
        readings = [proc(p) for p in pids]
    except FileNotFoundError:
        process_exited = True
        break
    peak_rss = max(peak_rss, sum(r[1] for r in readings))
    peak_fd = max(peak_fd, sum(r[2] for r in readings))
    goroutines = 0
    for name in ('iran', 'kharej'):
        try:
            with socket.socket(socket.AF_UNIX) as s:
                s.settimeout(.2)
                s.connect('/run/fullpack/' + name + '.sock')
                s.sendall(b'{"op":"status"}\n')
                response = json.loads(s.makefile('rb').readline())
                diag = response.get('body', {}).get('diagnostics', {})
                last_diagnostics[name] = diag
                goroutines += diag.get('runtime', {}).get('goroutines', 0)
        except (OSError, ValueError):
            pass
    peak_go = max(peak_go, goroutines)
    if output.with_suffix(output.suffix + '.stop').exists():
        break
    time.sleep(.25)
elapsed = time.monotonic() - start
cpu = sum(r[0] for r in readings) - cpu_start
output.write_text(json.dumps({'sample_seconds': elapsed, 'engine_cpu_seconds': cpu,
    'engine_cpu_percent_of_two_cpus': cpu / elapsed / 2 * 100,
    'aggregate_rss_peak_mb': peak_rss / 1048576, 'aggregate_fd_peak': peak_fd,
    'aggregate_goroutine_peak': peak_go or None, 'process_exited': process_exited, 'last_diagnostics': last_diagnostics}))
