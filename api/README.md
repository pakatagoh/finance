# API contract

`openapi.yaml` is the frozen MVP contract for the authenticated transaction
ingestion endpoint, `POST /api/v1/transactions` (OpenAPI 3.1).

## Freeze rules

1. **Field names are frozen** and must match the persistence schema exactly.
   Renames and case changes require a coordinated contract change.
2. **Enums are frozen.** Values for `kind`, `direction`, and
   `timestamp_source` may not be added or removed without a schema migration
   and contract review.
3. **Money uses integer minor units** (`amount_minor`). Never use floating
   point values in the payload or persistence layer.
4. **Idempotency identity** is the unique pair
   `(source_mailbox, gmail_message_id)`. A new record returns `201`; an
   identical retry returns `200`; the same identity with changed source-owned
   values returns `409`.
5. The ingestion MVP has no update endpoint, pending/settled lifecycle, source
   mutation endpoint, or CORS support. Browser transaction editing is a
   separate UI concern and must not be confused with ingestion semantics.
6. Nullable and optional fields are limited to those explicitly marked in
   `openapi.yaml`. All other fields are required and non-nullable.

## Changes

Any change to this file or `openapi.yaml` is a contract change. Coordinate with
all ingestion clients and update `info.version` before merging.
