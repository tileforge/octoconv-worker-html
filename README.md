# octoconv-worker-html

HTML worker for OctoConv. One process serves one approved manifest over Worker Protocol v1.

The process reads `HTML_PROTOCOL_CORE_URL`, `HTML_PROTOCOL_CLIENT_CERT`, `HTML_PROTOCOL_CLIENT_KEY`, `HTML_PROTOCOL_CLIENT_CA`, `HTML_PROTOCOL_WORKER_ID`, and `HTML_PROTOCOL_MANIFEST_PATH`. It refuses to start when `DATABASE_URL` or `REDIS_ADDR` is set. `HTML_WORKER_CONCURRENCY` must be unset or `1`. Hello and pull advertise one slot, and one conversion runs at a time.

`schema/manifest-html-convert.json` is capability `octoconv.convert.html`. It names html to pdf.

`chromium-headless-shell` must be on `PATH`. This process does not apply Landlock. The in-repo html worker does. Do not move html jobs onto this process until that confinement is measured.
