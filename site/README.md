# Product site

Static Astro output for https://alexcatdad.github.io/pr-nanny/. The browser receives HTML, CSS and local images, plus a few lines of inline JavaScript for the Copy button. External requests are limited to Google Fonts (Bricolage Grotesque, headings only) and the sibling apps' icons on their own Pages sites. No tracking.

Use Node.js 24.19.0 and pnpm 11.25.0:

```sh
cd site
pnpm install --frozen-lockfile --ignore-scripts
ASTRO_TELEMETRY_DISABLED=1 pnpm build
pnpm check:links
```

For local preview use `ASTRO_TELEMETRY_DISABLED=1 pnpm dev`. The environment variable disables Astro CLI telemetry without changing global preferences. The Pages workflow sets it as well.

The link check resolves built page anchors and local assets under `/pr-nanny/`. The screenshots in `public/screenshots` are rendered from fixtures by `task screenshots` at the repository root; do not edit them by hand. Browser QA covers light/dark and mobile layouts. GitHub Actions builds and audits PRs; only pushes/manual runs on main deploy through the github-pages environment. Deployment success and live page validation are separate from a local build.
