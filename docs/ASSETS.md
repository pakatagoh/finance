# Asset provenance

This document records vendored browser assets, their upstream sources, and
checksums. Re-download assets with a tool that fails on HTTP errors and verify
the checksum before committing.

## Vendored JavaScript

| File | Version | Upstream URL | SHA-256 |
|---|---|---|---|
| `static/js/htmx.min.js` | HTMX 4.0.0 | `https://unpkg.com/htmx.org@4.0.0/dist/htmx.min.js` | recorded in the repository history |
| `static/js/hx-live.min.js` | HTMX 4.0.0 core extension | `https://unpkg.com/htmx.org@4.0.0/dist/ext/hx-live.min.js` | recorded in the repository history |
| `static/js/hx-csp.min.js` | HTMX 4.0.0 core extension | `https://unpkg.com/htmx.org@4.0.0/dist/ext/hx-csp.min.js` | recorded in the repository history |

`hx-live` and `hx-csp` are extensions shipped with the matching HTMX package;
they are loaded separately by the application.

## Tailwind standalone CLI

The Tailwind CLI is a build dependency and is not committed. Use the version
and checksum policy defined by the build pipeline. Prefer a pinned release over
a moving `latest` URL.

The repository uses Tailwind CSS v4.3.3's standalone CLI. The CLI scans the
application templates for the utility and component classes used by the UI;
styling changes should therefore be made in the templates, not in the
generated output under `static/css/`.

## CSS build inputs

The CSS build has three inputs:

- `assets/css/input.css` — the project-owned Tailwind entry stylesheet and
  Finance theme configuration.
- `assets/css/daisyui.mjs` — the vendored daisyUI component plugin bundle.
- `assets/css/daisyui-theme.mjs` — the vendored daisyUI theme plugin bundle.

The two `.mjs` files are JavaScript modules consumed by Tailwind's
`@plugin` directive; they are not loaded by the browser. They are committed
build inputs sourced from daisyUI releases. Their upstream release and
checksum must be reviewed whenever they are replaced.

`static/css/app.css` is the generated, minified browser stylesheet. It is
checked into the repository as a usable build artifact, but the Docker and CI
builds regenerate it from the inputs above.

Rebuild CSS from the repository root with:

```sh
tailwindcss -i assets/css/input.css -o static/css/app.css --minify
```

Or use the pinned local toolchain:

```sh
mise run css
```

The input stylesheet uses Tailwind v4's standalone CLI flow and loads daisyUI
through local plugin modules with `@plugin`. Node.js is not required for this
build.

## CSP

The application uses per-response CSP nonces together with the `hx-csp`
extension. Do not add `unsafe-eval`, unrestricted `unsafe-inline`, wildcard
CORS, or a client-side state framework. See `docs/web-shell-security.md` for
the middleware and shell boundaries.
