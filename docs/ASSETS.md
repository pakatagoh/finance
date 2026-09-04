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

## CSS build inputs

The daisyUI module files under `assets/css/` are committed build inputs. Their
upstream release and checksum must be reviewed whenever they are replaced.

Rebuild CSS from the repository root with:

```sh
tailwindcss -i assets/css/input.css -o static/css/app.css --minify
```

The input stylesheet uses Tailwind v4's standalone CLI flow and loads daisyUI
through the local plugin module. Node.js is not required for this build.

## CSP

The application uses per-response CSP nonces together with the `hx-csp`
extension. Do not add `unsafe-eval`, unrestricted `unsafe-inline`, wildcard
CORS, or a client-side state framework. See `docs/web-shell-security.md` for
the middleware and shell boundaries.
