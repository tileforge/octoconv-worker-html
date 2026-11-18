# octoconv-worker-html

HTML worker for OctoConv. One process serves one approved manifest over Worker Protocol generation 2.

Assignment `parameters` are the print options `page_size`, `margin_mm`, `landscape`, and `print_background`. An empty object leaves the page at A4, does not set a margin, and suppresses backgrounds. `page_size` is one of a4, letter, legal, a3, and a5. `margin_mm` is an integer from 0 through 50. An explicit 0 sets the margin to 0mm.

The process reads `HTML_PROTOCOL_CORE_URL`, `HTML_PROTOCOL_CLIENT_CERT`, `HTML_PROTOCOL_CLIENT_KEY`, `HTML_PROTOCOL_CLIENT_CA`, `HTML_PROTOCOL_WORKER_ID`, and `HTML_PROTOCOL_MANIFEST_PATH`. It refuses to start when `DATABASE_URL` or `REDIS_ADDR` is set. `HTML_WORKER_CONCURRENCY` must be unset or `1`. Hello and pull advertise one slot, and one conversion runs at a time.

`schema/manifest-html-convert.json` is capability `octoconv.convert.html`. It names html to pdf.

The protocol certificate, key, CA, and manifest must not resolve under `/tmp`. That directory is writable by the engine and is swept before each conversion. Job directories are resolved before use. A `TMPDIR` symlink is replaced with the directory it names before the sweep, so later tasks still have a place to create their files.

`chromium-headless-shell` must be on `PATH`. The process applies the measured Landlock ruleset and refuses to start when Landlock is unavailable. `HTML_SANDBOX_DISABLED=true` is the only switch that runs the engine without that ruleset. Startup renders inside the job directory and then points the engine at a file outside every grant. A PDF of that file must omit the planted marker. Chromium 156 instead exits with net::ERR_ACCESS_DENIED and writes no PDF; a missing binary or a helper that never started the engine does not count. Approving the manifest is what sends jobs to this process.
