-- 0007: seed data. Every value here is a DEFAULT, changeable from the admin panel.
-- The meaning of every app_config key is documented in docs/config-system.md.

INSERT INTO app_config (config_key, value, value_type, category, description, is_public) VALUES
-- auth
('auth.otp.length',                    '6',     'int',  'auth', 'Number of OTP digits', 1),
('auth.otp.ttl_seconds',               '300',   'int',  'auth', 'OTP validity in seconds', 1),
('auth.otp.max_attempts',              '5',     'int',  'auth', 'Wrong OTP attempts before the request is burnt', 0),
('auth.otp.resend_cooldown_seconds',   '30',    'int',  'auth', 'Seconds before an OTP can be resent', 1),
('auth.otp.max_per_hour',              '5',     'int',  'auth', 'OTP requests per mobile per hour', 0),
('auth.access_token_ttl_minutes',      '30',    'int',  'auth', 'Access token lifetime', 0),
('auth.refresh_token_ttl_days',        '60',    'int',  'auth', 'Refresh token lifetime', 0),
('auth.sms_provider',                  '"stub"','string','auth', 'stub | msg91', 0),
('auth.min_age_years',                 '10',    'int',  'auth', 'Minimum age to use Athlix (checked against verified DOB)', 1),
-- verification
('verification.provider',              '"mock"','string','verification', 'digilocker_direct | aggregator | mock (mock is refused when ENV=prod)', 0),
('verification.max_attempts_per_day',  '3',     'int',  'verification', 'DigiLocker attempts per user per day', 0),
('verification.state_ttl_seconds',     '600',   'int',  'verification', 'How long a DigiLocker consent link stays valid', 0),
('verification.name_match_required',   'false', 'bool', 'verification', 'Require display name to resemble the DigiLocker name', 0),
-- minors and guardians
('guardian.minor_age_years',           '18',    'int',  'guardian', 'Below this age a user is a minor', 1),
('guardian.consent_otp_ttl_seconds',   '600',   'int',  'guardian', 'Parent consent OTP validity', 0),
('minor.dm_policy',                    '"mutual_follow"', 'string', 'guardian', 'Who can DM a minor: mutual_follow | none | anyone', 1),
('minor.profile_default_visibility',   '"followers"', 'string', 'guardian', 'Default profile visibility for minors', 1),
('minor.searchable',                   'true',  'bool', 'guardian', 'Minors appear in people search (limited fields)', 0),
('minor.can_join_public_communities',  'true',  'bool', 'guardian', 'Minors may join public communities', 1),
-- events
('event.require_admin_approval',       'true',  'bool', 'event', 'Paid listings wait for admin approval before publishing', 1),
('event.show_contact',                 '"after_registration"', 'string', 'event', 'When organiser contact is shown: always | after_registration', 1),
('event.max_active_per_organiser',     '20',    'int',  'event', 'Max draft+pending+published events per organiser', 1),
('event.min_hours_before_start',       '24',    'int',  'event', 'Event must start at least this many hours after publishing', 1),
('event.auto_create_group',            'true',  'bool', 'event', 'Create a group chat for every published event', 1),
('event.allow_free_events',            'true',  'bool', 'event', 'Allow events with entry fee 0', 1),
('event.max_slots_limit',              '1024',  'int',  'event', 'Upper bound for max_slots', 1),
('event.contact_reveals_per_day',      '50',    'int',  'event', 'Organiser contact reveals one user may do per day (anti-scraping)', 0),
('event.featured_list_size',           '10',    'int',  'event', 'Max featured events returned by GET /events/featured', 0),
-- listing payment
('listing.payment_provider',           '"mock"','string','listing', 'razorpay | mock (mock refused when ENV=prod)', 0),
('listing.order_ttl_minutes',          '30',    'int',  'listing', 'Unpaid listing order expiry', 1),
('listing.refund.before_first_registration_percent', '100', 'int', 'listing', 'Refund % if organiser cancels before anyone registers', 1),
('listing.refund.after_registrations_percent',       '0',   'int', 'listing', 'Refund % if organiser cancels after registrations exist', 1),
('listing.refund.on_admin_rejection_percent',        '100', 'int', 'listing', 'Refund % when admin rejects the event', 1),
-- registration
('registration.withdraw_until_hours',  '24',    'int',  'registration', 'Players can withdraw until this many hours before start', 1),
('registration.waitlist_enabled',      'true',  'bool', 'registration', 'Accept waitlist registrations when slots are full', 1),
-- chat / communities
('chat.message_max_length',            '2000',  'int',  'chat', 'Max characters in a message', 1),
('chat.group_max_members',             '500',   'int',  'chat', 'Max members in a community or event group', 1),
('chat.max_communities_created_per_user', '10', 'int',  'chat', 'Communities one user can own', 1),
('chat.messages_per_minute',           '30',    'int',  'chat', 'Per-user send rate limit', 0),
('chat.media_enabled',                 'true',  'bool', 'chat', 'Allow image messages', 1),
('chat.dm_requires_follow',            'false', 'bool', 'chat', 'Adults: only DM people who follow you', 1),
('chat.ws_ticket_ttl_seconds',         '60',    'int',  'chat', 'WebSocket ticket lifetime', 0),
-- feed
('feed.post_max_length',               '2000',  'int',  'feed', 'Max characters in a post', 1),
('feed.post_max_media',                '4',     'int',  'feed', 'Max images per post', 1),
('feed.page_size',                     '20',    'int',  'feed', 'Feed page size', 1),
-- profile
('profile.viewers_window_days',        '30',    'int',  'profile', 'Profile viewers shown to premium users for this many days', 1),
('profile.max_achievements',           '50',    'int',  'profile', 'Max achievements per user', 1),
-- media
('media.image_max_mb',                 '5',     'int',  'media', 'Max image size', 1),
('media.document_max_mb',              '10',    'int',  'media', 'Max document (certificate, organiser proof) size', 1),
('media.allowed_image_types',          '["image/jpeg","image/png","image/webp"]', 'string_list', 'media', 'Allowed image MIME types', 1),
('media.allowed_document_types',       '["application/pdf","image/jpeg","image/png"]', 'string_list', 'media', 'Allowed document MIME types', 1),
('media.upload_url_ttl_seconds',       '300',   'int',  'media', 'Presigned upload URL lifetime', 0),
('media.view_url_ttl_seconds',         '900',   'int',  'media', 'Signed view URL lifetime', 0),
-- moderation
('moderation.auto_hide_report_threshold', '5',  'int',  'moderation', 'Distinct reports that auto-hide content pending review', 0),
-- search
('search.page_size',                   '20',    'int',  'search', 'Search page size', 1),
-- ads
('ads.enabled',                        'false', 'bool', 'ads', 'Show sponsored placements', 1),
('ads.show_to_minors',                 'false', 'bool', 'ads', 'Show sponsored placements to minors', 0),
('ads.feed_every_n_posts',             '8',     'int',  'ads', 'Insert one ad after every N feed posts', 1),
-- premium / consultation
('premium.enabled',                    'false', 'bool', 'premium', 'Show premium plans in the app', 1),
('consultation.max_open_requests_per_player', '5', 'int', 'premium', 'Pending consultation requests a player may have', 1),
-- notifications
('notification.event_reminder_hours',  '24',    'int',  'notification', 'Remind registered players this many hours before start', 0),
-- admin panel
('admin.max_failed_logins',            '5',     'int',  'admin', 'Wrong passwords/TOTP before the admin account is locked', 0),
('admin.lockout_minutes',              '30',    'int',  'admin', 'Admin lockout duration', 0),
('admin.access_token_ttl_minutes',     '15',    'int',  'admin', 'Admin access token lifetime (role changes apply on next refresh)', 0),
('admin.refresh_token_ttl_hours',      '12',    'int',  'admin', 'Admin session length', 0),
-- app
('app.min_version_android',            '"1.0.0"', 'string', 'app', 'Force update below this version', 1),
('app.min_version_ios',                '"1.0.0"', 'string', 'app', 'Force update below this version', 1),
('app.maintenance_mode',               'false', 'bool', 'app', 'Show maintenance screen', 1),
('app.support_email',                  '"support@example.com"', 'string', 'app', 'Support contact shown in app', 1),
('app.grievance_officer',              '{"name":"To be appointed","email":"grievance@example.com"}', 'json', 'app', 'IT Rules grievance officer shown in app', 1);

INSERT INTO sports (id, code, name, is_team_sport, sort_order) VALUES
(1,'cricket','Cricket',1,1),(2,'football','Football',1,2),(3,'athletics','Athletics',0,3),
(4,'volleyball','Volleyball',1,4),(5,'basketball','Basketball',1,5),(6,'badminton','Badminton',0,6),
(7,'kabaddi','Kabaddi',1,7),(8,'hockey','Hockey',1,8),(9,'tennis','Tennis',0,9),(10,'table_tennis','Table Tennis',0,10);

INSERT INTO sport_positions (sport_id, code, name, applies_to, sort_order) VALUES
(1,'batter','Batter','player',1),(1,'bowler','Bowler','player',2),(1,'all_rounder','All-rounder','player',3),(1,'wicket_keeper','Wicket-keeper','player',4),(1,'coach','Coach','coach',5),
(2,'goalkeeper','Goalkeeper','player',1),(2,'defender','Defender','player',2),(2,'midfielder','Midfielder','player',3),(2,'forward','Forward','player',4),(2,'coach','Coach','coach',5),
(3,'sprinter','Sprinter','player',1),(3,'middle_distance','Middle distance','player',2),(3,'long_distance','Long distance','player',3),(3,'jumper','Jumps','player',4),(3,'thrower','Throws','player',5),(3,'coach','Coach','coach',6),
(4,'setter','Setter','player',1),(4,'attacker','Attacker','player',2),(4,'blocker','Blocker','player',3),(4,'libero','Libero','player',4),(4,'coach','Coach','coach',5),
(5,'guard','Guard','player',1),(5,'forward','Forward','player',2),(5,'center','Center','player',3),(5,'coach','Coach','coach',4),
(6,'singles','Singles','player',1),(6,'doubles','Doubles','player',2),(6,'coach','Coach','coach',3),
(7,'raider','Raider','player',1),(7,'defender','Defender','player',2),(7,'all_rounder','All-rounder','player',3),(7,'coach','Coach','coach',4),
(8,'goalkeeper','Goalkeeper','player',1),(8,'defender','Defender','player',2),(8,'midfielder','Midfielder','player',3),(8,'forward','Forward','player',4),(8,'coach','Coach','coach',5),
(9,'singles','Singles','player',1),(9,'doubles','Doubles','player',2),(9,'coach','Coach','coach',3),
(10,'singles','Singles','player',1),(10,'doubles','Doubles','player',2),(10,'coach','Coach','coach',3);

INSERT INTO stat_templates (sport_id, position_id, name, fields) VALUES
(1, NULL, 'Cricket season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"runs","label":"Runs","type":"int"},{"key":"batting_avg","label":"Batting average","type":"decimal"},{"key":"strike_rate","label":"Strike rate","type":"decimal"},{"key":"wickets","label":"Wickets","type":"int"},{"key":"economy","label":"Economy","type":"decimal"},{"key":"catches","label":"Catches","type":"int"}]'),
(2, NULL, 'Football season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"goals","label":"Goals","type":"int"},{"key":"assists","label":"Assists","type":"int"},{"key":"clean_sheets","label":"Clean sheets","type":"int"},{"key":"minutes","label":"Minutes played","type":"int"}]'),
(3, NULL, 'Athletics personal bests', '[{"key":"event","label":"Event","type":"text"},{"key":"personal_best","label":"Personal best","type":"text"},{"key":"competitions","label":"Competitions","type":"int"},{"key":"medals","label":"Medals","type":"int"}]'),
(4, NULL, 'Volleyball season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"points","label":"Points","type":"int"},{"key":"aces","label":"Aces","type":"int"},{"key":"blocks","label":"Blocks","type":"int"},{"key":"digs","label":"Digs","type":"int"}]'),
(5, NULL, 'Basketball season', '[{"key":"games","label":"Games","type":"int"},{"key":"points_per_game","label":"Points per game","type":"decimal"},{"key":"rebounds_per_game","label":"Rebounds per game","type":"decimal"},{"key":"assists_per_game","label":"Assists per game","type":"decimal"}]'),
(6, NULL, 'Badminton season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"wins","label":"Wins","type":"int"},{"key":"titles","label":"Titles","type":"int"},{"key":"best_ranking","label":"Best ranking","type":"int"}]'),
(7, NULL, 'Kabaddi season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"raid_points","label":"Raid points","type":"int"},{"key":"tackle_points","label":"Tackle points","type":"int"},{"key":"super_tens","label":"Super 10s","type":"int"}]'),
(8, NULL, 'Hockey season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"goals","label":"Goals","type":"int"},{"key":"assists","label":"Assists","type":"int"},{"key":"saves","label":"Saves","type":"int"}]'),
(9, NULL, 'Tennis season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"wins","label":"Wins","type":"int"},{"key":"titles","label":"Titles","type":"int"},{"key":"best_ranking","label":"Best ranking","type":"int"}]'),
(10, NULL, 'Table tennis season', '[{"key":"matches","label":"Matches","type":"int"},{"key":"wins","label":"Wins","type":"int"},{"key":"titles","label":"Titles","type":"int"},{"key":"best_ranking","label":"Best ranking","type":"int"}]');

-- Default listing fee: 5% of (entry fee x slots), min Rs 99, max Rs 4,999, Rs 99 for free events, + 18% GST.
INSERT INTO listing_fee_rules (name, sport_id, event_type, percent_bp, min_fee_paise, max_fee_paise, free_event_fee_paise, gst_bp, priority) VALUES
('Default', NULL, NULL, 500, 9900, 499900, 9900, 1800, 0);

INSERT INTO premium_plans (id, code, audience, name, description, price_paise, billing_period, active, sort_order) VALUES
(1, 'player_plus_monthly', 'player', 'Player Plus', 'Stand out to coaches and scouts.', 9900, 'monthly', 1, 1),
(2, 'coach_pro_monthly', 'coach', 'Coach Pro', 'List consultations and discover talent faster.', 29900, 'monthly', 1, 2),
(3, 'organiser_pro_monthly', 'organiser', 'Organiser Pro', 'Promote your tournaments.', 49900, 'monthly', 1, 3);

INSERT INTO premium_plan_benefits (plan_id, benefit_key, value, label, sort_order) VALUES
(1, 'profile_boost', 'true', 'Boosted profile in coach searches', 1),
(1, 'advanced_stats', 'true', 'Advanced stats and history charts', 2),
(1, 'see_profile_viewers', 'true', 'See who viewed your profile', 3),
(2, 'consultation_listing', 'true', 'Offer paid consultations on your profile', 1),
(2, 'advanced_search_filters', 'true', 'Advanced player search filters', 2),
(2, 'extra_communities', '25', 'Create up to 25 communities', 3),
(3, 'featured_events', '2', '2 featured event placements per month', 1),
(3, 'extra_communities', '25', 'Create up to 25 communities', 2);

INSERT INTO policy_documents (doc_type, version, title, content_md, requires_acceptance, active) VALUES
('terms', 1, 'Terms of Use', '# Terms of Use\n\nDRAFT - to be replaced with lawyer-reviewed terms before launch.', 1, 1),
('privacy', 1, 'Privacy Policy', '# Privacy Policy\n\nDRAFT - to be replaced with a DPDP Act compliant policy before launch.\n\nWe never store your full Aadhaar number.', 1, 1),
('listing_refund', 1, 'Listing Fee Refund Policy', '# Listing Fee Refunds\n\n- Cancelled before anyone registers: full refund.\n- Cancelled after registrations: no refund.\n- Rejected by Athlix: full refund.\n\nRefund percentages are configured by Athlix and shown before payment.', 0, 1),
('guardian_consent', 1, 'Parent / Guardian Consent', '# Parent / Guardian Consent\n\nDRAFT - I confirm I am the parent or legal guardian and consent to my child using Athlix.', 1, 1),
('community_guidelines', 1, 'Community Guidelines', '# Community Guidelines\n\nBe respectful. No harassment, scams or fake profiles.', 1, 1),
('organiser_terms', 1, 'Organiser Terms', '# Organiser Terms\n\nDRAFT - Organisers collect entry fees directly from players. Athlix is not a party to that payment.', 1, 1),
('entry_fee_disclaimer', 1, 'Entry Fee Disclaimer', 'Entry fees are paid directly to the organiser. Athlix does not collect, hold or refund entry fees. Contact the organiser for payment questions and report problems from the event page.', 0, 1);

INSERT INTO verification_requirements (role, requirement, is_required, gates) VALUES
('player',    'digilocker',                 1, 'app_access'),
('coach',     'digilocker',                 1, 'app_access'),
('organiser', 'digilocker',                 1, 'app_access'),
('player',    'guardian_consent_if_minor',  1, 'app_access'),
('coach',     'guardian_consent_if_minor',  1, 'app_access'),
('organiser', 'guardian_consent_if_minor',  1, 'app_access'),
('organiser', 'organiser_approval',         1, 'create_event'),
('coach',     'coach_pro_subscription',     0, 'offer_consultation');

INSERT INTO organiser_verification_steps (step_code, title, description, input_type, is_required, sort_order) VALUES
('org_details',      'Organisation name and type', 'Your club, academy, school or your own name.', 'text', 1, 1),
('past_event_proof', 'Proof of a past event',      'A poster, photo or certificate from an event you organised (optional for first-time organisers).', 'document', 0, 2),
('affiliation_doc',  'Affiliation or registration', 'Association affiliation, society/company registration, or school letter if you have one.', 'document', 0, 3),
('accept_terms',     'Accept organiser terms',      'I will collect entry fees directly and honour the published event details.', 'checkbox', 1, 4);

INSERT INTO admin_roles (name, description, permissions) VALUES
('super_admin', 'Full access', '["*"]'),
('operations',  'Users, organisers, events', '["dashboard.read","users.read","users.write","organisers.review","events.review","communities.moderate","notifications.broadcast"]'),
('moderator',   'Reports and content', '["dashboard.read","users.read","reports.read","reports.resolve","moderation.read_messages","communities.moderate"]'),
('finance',     'Listing orders and refunds', '["dashboard.read","payments.read","payments.refund","plans.write"]'),
('config_editor','Runtime config', '["dashboard.read","config.read","config.write","policies.write","sports.write","fees.write","plans.write","ads.write"]');
