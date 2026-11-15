# features/organiser

Organiser verification: organisation details plus server-defined steps.

## Screens

| Screen | Route | Notes |
|---|---|---|
| `OrganiserVerificationScreen` | `/organiser/verification` | status banner, org name + type, one input per step (`text`, `document`, `checkbox`), save draft, submit |

## Endpoints

- `GET /organiser/verification`
- `PUT /organiser/verification` `{orgName?, orgType?, steps: [{stepId, valueText?, documentUploadId?, checked?}]}`
- `POST /organiser/verification/submit`
- Media: `POST /media/uploads` + confirm (purpose `organiser_document`, via core/media)

## Rules

- Steps come from the server (`organiser_verification_steps`); never hard-code them.
- Editable only in `not_started | draft | rejected`; otherwise read-only.
- Documents are referenced by upload id; the app only sees a signed URL to view them.
- Submit saves the draft first, so the server validates what the user sees.

## Changing it safely

New `inputType`: add a constant to `StepInputType`, then a branch in `_stepInputs`, `_filled`
and `_stepInput`. Unknown types are shown with an "update the app" note.
