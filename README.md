# octoconv-worker-html

HTML worker for OctoConv. One process serves one approved manifest over Worker Protocol generation 2.

Assignment `parameters` are the print options `page_size`, `margin_mm`, `landscape`, and `print_background`. An empty object leaves the page at A4, does not set a margin, and suppresses backgrounds. `page_size` is one of a4, letter, legal, a3, and a5. `margin_mm` is an integer from 0 through 50. An explicit 0 sets the margin to 0mm.

The process reads `HTML_PROTOCOL_CORE_URL`, `HTML_PROTOCOL_CLIENT_CERT`, `HTML_PROTOCOL_CLIENT_KEY`, `HTML_PROTOCOL_CLIENT_CA`, `HTML_PROTOCOL_WORKER_ID`, and `HTML_PROTOCOL_MANIFEST_PATH`. It refuses to start when `DATABASE_URL` or `REDIS_ADDR` is set. `HTML_WORKER_CONCURRENCY` must be unset or `1`. Hello and pull advertise one slot, and one conversion runs at a time.

`schema/manifest-html-convert.json` is capability `octoconv.convert.html`. It names html to pdf.

`chromium-headless-shell` must be on `PATH`. This process does not apply Landlock. The in-repo html worker does. Do not move html jobs onto this process until that confinement is measured.
