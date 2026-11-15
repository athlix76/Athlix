-- 0008: location identities (native + residence) and tournament eligibility by location.
-- Design: docs/architecture.md#location-eligibility. Rules: docs/api.md#location-identity.
-- Proof documents are deleted location.proof_retention_days after review; only the result stays.
-- The full Aadhaar number is never stored (last 4 digits at most).

CREATE TABLE states (
  id          SMALLINT UNSIGNED NOT NULL AUTO_INCREMENT,
  code        CHAR(2)      NOT NULL,          -- ISO 3166-2:IN subdivision code without "IN-"
  name        VARCHAR(64)  NOT NULL,
  is_ut       TINYINT(1)   NOT NULL DEFAULT 0,
  lgd_code    INT UNSIGNED NULL,               -- Local Government Directory state code
  active      TINYINT(1)   NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  UNIQUE KEY uq_states_code (code),
  UNIQUE KEY uq_states_name (name)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Full district list: import the official LGD file with `make import-districts FILE=...`
-- (cmd/admincli import-districts). Admins can add districts later (new districts are created
-- every year). Seeded below: Tamil Nadu, as the reference example.
CREATE TABLE districts (
  id          INT UNSIGNED      NOT NULL AUTO_INCREMENT,
  state_id    SMALLINT UNSIGNED NOT NULL,
  name        VARCHAR(80)       NOT NULL,
  lgd_code    INT UNSIGNED      NULL,
  aliases     JSON              NULL,           -- other spellings, e.g. ["Tanjore"], used to match DigiLocker addresses
  active      TINYINT(1)        NOT NULL DEFAULT 1,
  PRIMARY KEY (id),
  UNIQUE KEY uq_districts_state_name (state_id, name),
  UNIQUE KEY uq_districts_lgd (lgd_code),
  KEY idx_districts_state (state_id, active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- One row per submission. History is kept (superseded / expired / rejected rows stay).
-- At most ONE verified and ONE pending row per (user, type): enforced by the generated slots.
CREATE TABLE user_location_identities (
  id                BIGINT UNSIGNED   NOT NULL AUTO_INCREMENT,
  user_id           BIGINT UNSIGNED   NOT NULL,
  identity_type     ENUM('native','residence') NOT NULL,
  district_id       INT UNSIGNED      NOT NULL,
  state_id          SMALLINT UNSIGNED NOT NULL,   -- copied from the district for state-level rules
  proof_type        ENUM('digilocker_aadhaar','masked_aadhaar','birth_certificate',
                         'rent_agreement','utility_bill','bank_statement','voter_id',
                         'employer_letter','institution_id') NOT NULL,
  proof_holder      ENUM('self','guardian') NOT NULL DEFAULT 'self',  -- guardian: minors' residence only
  proof_upload_id   BIGINT UNSIGNED   NULL,       -- media_uploads (purpose location_proof); NULL for DigiLocker or after purge
  aadhaar_last4     CHAR(4)           NULL,       -- only for Aadhaar-based proofs; never more digits
  notice_version    INT UNSIGNED      NULL,       -- location_identity_notice version accepted at submit
  status            ENUM('pending','verified','rejected','expired','superseded','revoked','withdrawn') NOT NULL DEFAULT 'pending',
  rejection_code    VARCHAR(32)       NULL,       -- one of location.rejection_reasons
  rejection_reason  VARCHAR(255)      NULL,       -- reviewer note shown to the user (no PII)
  submitted_at      DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  reviewed_at       DATETIME(3)       NULL,
  reviewed_by       BIGINT UNSIGNED   NULL,       -- admin_users.id; NULL when auto-verified (DigiLocker)
  verified_at       DATETIME(3)       NULL,
  expires_at        DATETIME(3)       NULL,       -- residence only: verified_at + location.residence_validity_months
  expiry_reminded_at DATETIME(3)      NULL,
  proof_purged_at   DATETIME(3)       NULL,
  updated_at        DATETIME(3)       NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
  verified_slot     VARCHAR(16) GENERATED ALWAYS AS (IF(status = 'verified', identity_type, NULL)) STORED,
  pending_slot      VARCHAR(16) GENERATED ALWAYS AS (IF(status = 'pending', identity_type, NULL)) STORED,
  PRIMARY KEY (id),
  UNIQUE KEY uq_loc_verified (user_id, verified_slot),
  UNIQUE KEY uq_loc_pending (user_id, pending_slot),
  KEY idx_loc_user (user_id, identity_type, status),
  KEY idx_loc_review (status, submitted_at),
  KEY idx_loc_expiry (status, expires_at),
  KEY idx_loc_purge (proof_purged_at, reviewed_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

-- Tournament eligibility (owned by the event module).
ALTER TABLE events
  ADD COLUMN category                ENUM('government','private') NOT NULL DEFAULT 'private' AFTER competition_level,
  ADD COLUMN eligibility_type        ENUM('open','native','residence','either') NOT NULL DEFAULT 'open' AFTER category,
  ADD COLUMN eligibility_scope       ENUM('district','state') NULL AFTER eligibility_type,
  ADD COLUMN eligibility_district_id INT UNSIGNED NULL AFTER eligibility_scope,
  ADD COLUMN eligibility_state_id    SMALLINT UNSIGNED NULL AFTER eligibility_district_id,
  ADD COLUMN require_verified        TINYINT(1) NOT NULL DEFAULT 1 AFTER eligibility_state_id,
  ADD KEY idx_events_eligibility (status, eligibility_type, eligibility_district_id, eligibility_state_id);

-- How each registrant qualified, and a flag when that identity changes later (owned by registration).
ALTER TABLE event_registrations
  ADD COLUMN qualified_via          ENUM('open','native','residence') NOT NULL DEFAULT 'open' AFTER payment_note,
  ADD COLUMN qualified_identity_id  BIGINT UNSIGNED NULL AFTER qualified_via,
  ADD COLUMN qualified_district_id  INT UNSIGNED NULL AFTER qualified_identity_id,
  ADD COLUMN eligibility_flag       ENUM('none','identity_expired','identity_rejected','identity_revoked','identity_changed') NOT NULL DEFAULT 'none' AFTER qualified_district_id,
  ADD COLUMN eligibility_flagged_at DATETIME(3) NULL AFTER eligibility_flag,
  ADD KEY idx_registrations_identity (qualified_identity_id);

ALTER TABLE media_uploads
  MODIFY COLUMN purpose ENUM('profile_photo','achievement_certificate','event_banner','chat_image','post_image','community_avatar','organiser_document','location_proof') NOT NULL;

ALTER TABLE policy_documents
  MODIFY COLUMN doc_type ENUM('terms','privacy','listing_refund','guardian_consent','community_guidelines','organiser_terms','entry_fee_disclaimer','location_identity_notice') NOT NULL;

INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
('location.enabled',                        'true', 'bool', 'location', 'Location identities and location-based eligibility are on', 1),
('location.residence_validity_months',      '12',   'int',  'location', 'A verified residence identity expires after this many months', 1),
('location.residence_change_cooldown_days', '180',  'int',  'location', 'Minimum days between residence changes to a different district', 1),
('location.native_change_allowed',          'true', 'bool', 'location', 'Allow a reviewed native change (e.g. birth certificate replacing the DigiLocker district)', 1),
('location.proof_retention_days',           '30',   'int',  'location', 'Days after review before the proof document is deleted (result is kept)', 1),
('location.expiry_reminder_days',           '30',   'int',  'location', 'Remind users this many days before residence expiry', 0),
('location.native_proof_types',             '["masked_aadhaar","birth_certificate"]', 'string_list', 'location', 'Upload proofs accepted for native identity (DigiLocker is automatic)', 1),
('location.residence_proof_types',          '["rent_agreement","utility_bill","bank_statement","voter_id","employer_letter","institution_id"]', 'string_list', 'location', 'Proofs accepted for residence identity', 1),
('location.utility_bill_max_age_days',      '90',   'int',  'location', 'Reviewer checklist: utility bill / bank statement must be this recent', 1),
('location.minor_guardian_proof_allowed',   'true', 'bool', 'location', 'Minors may submit a parent/guardian address proof for residence', 1),
('location.flag_registrations_on_change',   'true', 'bool', 'location', 'Flag (never cancel) registrations when the qualifying identity stops being valid', 0),
('location.rejection_reasons',              '["unclear_document","name_mismatch","address_mismatch","district_mismatch","document_too_old","unmasked_aadhaar","wrong_document_type","other"]', 'string_list', 'location', 'Reason codes a reviewer can pick; unmasked_aadhaar deletes the file at once', 0),
('location.proof_view_url_ttl_seconds',     '120',  'int',  'location', 'Lifetime of the signed URL a reviewer gets for a proof document', 0),
('event.max_search_radius_km',              '200',  'int',  'event', 'Largest radiusKm accepted by GET /events', 1),
('event.eligibility_default_government',    '"native"', 'string', 'event', 'Suggested eligibility type for government / official tournaments', 1),
('event.eligibility_default_private',       '"open"',   'string', 'event', 'Suggested eligibility type for private / open tournaments', 1);

INSERT INTO policy_documents (doc_type, version, title, content_md, requires_acceptance, active) VALUES
('location_identity_notice', 1, 'Why we ask for location proof',
 '# Location identity: why we ask and what we keep\n\nDRAFT - to be reviewed by a lawyer (DPDP Act 2023) before launch.\n\n**Purpose.** Some tournaments are only for players from a district or state (for example district association events). We verify where you are from (native) and where you live now (residence) only to check that eligibility.\n\n**What you give us.** A district, and one proof document. Native identity is taken from your DigiLocker Aadhaar address when you verify; we keep only the district, never the address or the Aadhaar number. If you upload an Aadhaar card it must be masked (only the last 4 digits visible).\n\n**What we keep.** The district, the type of proof, the result and dates, and at most the last 4 Aadhaar digits. The uploaded document is deleted 30 days after review. Residence proof must be renewed every 12 months.\n\n**Who sees it.** Only our verification reviewers see the document. Organisers see only whether you qualified as native or resident of the district.\n\n**Your choices.** Location identity is optional; without it you can still join open tournaments. You can ask us to delete it from Settings. For a child, a parent or guardian submits and consents.\n\n**Contact.** Grievance officer details are in the app under Settings > Help.',
 1, 1);

INSERT INTO admin_roles (name, description, permissions) VALUES
('location_reviewer', 'Reviews location identity proofs', '["dashboard.read","location.review"]');
UPDATE admin_roles SET permissions = JSON_ARRAY_APPEND(permissions, '$', 'location.review') WHERE name = 'operations';

INSERT INTO states (id, code, name, is_ut) VALUES
(1,'AP','Andhra Pradesh',0),(2,'AR','Arunachal Pradesh',0),(3,'AS','Assam',0),(4,'BR','Bihar',0),
(5,'CT','Chhattisgarh',0),(6,'GA','Goa',0),(7,'GJ','Gujarat',0),(8,'HR','Haryana',0),
(9,'HP','Himachal Pradesh',0),(10,'JH','Jharkhand',0),(11,'KA','Karnataka',0),(12,'KL','Kerala',0),
(13,'MP','Madhya Pradesh',0),(14,'MH','Maharashtra',0),(15,'MN','Manipur',0),(16,'ML','Meghalaya',0),
(17,'MZ','Mizoram',0),(18,'NL','Nagaland',0),(19,'OR','Odisha',0),(20,'PB','Punjab',0),
(21,'RJ','Rajasthan',0),(22,'SK','Sikkim',0),(23,'TN','Tamil Nadu',0),(24,'TG','Telangana',0),
(25,'TR','Tripura',0),(26,'UP','Uttar Pradesh',0),(27,'UT','Uttarakhand',0),(28,'WB','West Bengal',0),
(29,'AN','Andaman and Nicobar Islands',1),(30,'CH','Chandigarh',1),(31,'DH','Dadra and Nagar Haveli and Daman and Diu',1),
(32,'DL','Delhi',1),(33,'JK','Jammu and Kashmir',1),(34,'LA','Ladakh',1),(35,'LD','Lakshadweep',1),(36,'PY','Puducherry',1);

INSERT INTO districts (state_id, name, aliases) VALUES
(23,'Ariyalur',NULL),(23,'Chengalpattu',NULL),(23,'Chennai','["Madras"]'),(23,'Coimbatore','["Kovai"]'),
(23,'Cuddalore',NULL),(23,'Dharmapuri',NULL),(23,'Dindigul',NULL),(23,'Erode',NULL),
(23,'Kallakurichi',NULL),(23,'Kancheepuram','["Kanchipuram"]'),(23,'Kanniyakumari','["Kanyakumari"]'),(23,'Karur',NULL),
(23,'Krishnagiri',NULL),(23,'Madurai',NULL),(23,'Mayiladuthurai',NULL),(23,'Nagapattinam',NULL),
(23,'Namakkal',NULL),(23,'Nilgiris','["The Nilgiris"]'),(23,'Perambalur',NULL),(23,'Pudukkottai',NULL),
(23,'Ramanathapuram',NULL),(23,'Ranipet',NULL),(23,'Salem',NULL),(23,'Sivaganga','["Sivagangai"]'),
(23,'Tenkasi',NULL),(23,'Thanjavur','["Tanjore"]'),(23,'Theni',NULL),(23,'Thoothukudi','["Tuticorin"]'),
(23,'Tiruchirappalli','["Trichy","Tiruchirapalli"]'),(23,'Tirunelveli',NULL),(23,'Tirupathur','["Tirupattur"]'),(23,'Tiruppur','["Tirupur"]'),
(23,'Tiruvallur','["Thiruvallur"]'),(23,'Tiruvannamalai','["Thiruvannamalai"]'),(23,'Tiruvarur','["Thiruvarur"]'),(23,'Vellore',NULL),
(23,'Viluppuram','["Villupuram"]'),(23,'Virudhunagar',NULL);
