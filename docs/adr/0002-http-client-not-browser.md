# HTTP client, not a browser

The Simulator replays the HTTP requests a browser would make — HTML, assets, API calls, cookies, browser-like headers — but does not render the DOM or execute JavaScript. Weave observes network flows, not the DOM; a browser farm cannot reach the target concurrency on a laptop, and it adds no edge that the request replay does not produce. The cost is that the Simulator cannot catch frontend-only regressions; a small Playwright smoke mode can cover those later.
