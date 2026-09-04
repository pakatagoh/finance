# Finance application

A server-rendered finance application for reviewing and enriching imported
transactions.

- Go, templ, Tailwind CSS, daisyUI, HTMX, and PostgreSQL
- Server-rendered UI with no client-side state framework
- Authenticated ingestion API for importing source transactions
- Category and notes editing through the browser UI

## Development

See the documentation under `docs/` for the local database, browser security,
and asset build workflows.

### Styling

The UI uses Tailwind CSS v4 with daisyUI components. Add utility classes and
daisyUI component classes directly to the server-rendered templates; do not
edit `static/css/app.css` by hand.

The source stylesheet is `assets/css/input.css`. It imports Tailwind and loads
the vendored daisyUI plugins from `assets/css/daisyui.mjs` and
`assets/css/daisyui-theme.mjs`. Generate the minified browser stylesheet from
the repository root with:

```sh
mise run css
```

This writes the generated output to `static/css/app.css`. The same build step
runs in CI and in the Docker image build. See [`docs/ASSETS.md`](docs/ASSETS.md)
for the asset provenance and rebuild details.

## Status

Under active development. See `PACKAGE_BOUNDARIES.md` for the module map and
architecture rules.
