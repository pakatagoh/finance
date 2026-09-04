# Web shell and security

`internal/web/ui` contains the server-rendered templ shell. Render a full page
through the shared shell helper so the response receives the current CSP nonce
and the pinned HTMX assets in the required order.

Use HTMX 4 configuration explicitly in markup. For example:

```html
<div hx-ext="hx-live,hx-csp" hx-config='{"defaultSwapStyle":"outerHTML"}'>
  <input :text="q(#query).value" />
</div>
```

`hx-live` is a separately loaded core extension, not part of HTMX core.
`hx-csp` must remain enabled when strict CSP is in use. Do not add
`unsafe-inline`, `unsafe-eval`, wildcard CORS, or a client-side state
framework.

## Browser mutation requests

Configure the accepted browser origin through the application's environment
configuration. The browser state-changing middleware must reject missing or
mismatched `Origin` headers and reject an unsafe `Sec-Fetch-Site` value when
present.

Machine-to-machine ingestion routes must use the bearer-authentication
middleware instead. Keep browser-origin protection and bearer authentication
as separate boundaries; bearer credentials must never bypass browser CSRF
checks.

Never document real hostnames, account identifiers, tokens, secret values,
private filesystem paths, or deployment-specific infrastructure details in the
repository.
