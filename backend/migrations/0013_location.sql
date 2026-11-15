-- 0013: location identity review note.
--   user_location_identities.review_note: the optional note an admin writes when approving
--   (POST /admin/location-identities/:id/approve {note}). Shown to admins only, never to the
--   user (rejection notes stay in rejection_reason). No PII in it, at most 255 characters.
ALTER TABLE user_location_identities
  ADD COLUMN review_note VARCHAR(255) NULL;
