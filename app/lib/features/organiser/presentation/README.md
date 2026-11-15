# organiser/presentation

- `organiser_verification_screen.dart`: the form. It is keyed by status, so it rebuilds from
  the server's copy after submit or review.

Document extensions follow `media.allowed_document_types`; the server checks type and size again.
