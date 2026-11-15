# features/policies

Read policy documents and accept pending versions.

## Endpoints

- `GET /policies/:docType`
- `POST /me/policies/accept`

## Rules

- Policy text comes from the server; the app never embeds legal text.

## Changing it safely

New document types only need their `docType` string.
