import '../json/json_utils.dart';

/// Typed reader over `Bootstrap.config` (every `app_config` row with
/// `is_public = 1`, see docs/config-system.md).
///
/// There are **no hard-coded business defaults** here: when a key is missing
/// the getter returns `null` and the UI does not enforce that limit locally.
/// The server always enforces it and answers with a typed error.
class AppConfig {
  const AppConfig(this.values);

  final Map<String, dynamic> values;

  static const empty = AppConfig(<String, dynamic>{});

  int? integer(String key) => asIntOrNull(values[key]);
  bool? boolean(String key) => asBoolOrNull(values[key]);
  String? string(String key) {
    final v = values[key];
    return v is String ? v : null;
  }

  List<String>? stringList(String key) {
    final v = values[key];
    return v is List ? asStringList(v) : null;
  }

  Map<String, dynamic>? json(String key) => asMapOrNull(values[key]);

  // auth
  int? get otpLength => integer('auth.otp.length');
  int? get otpTtlSeconds => integer('auth.otp.ttl_seconds');
  int? get otpResendCooldownSeconds => integer('auth.otp.resend_cooldown_seconds');
  int? get minAgeYears => integer('auth.min_age_years');

  // guardian / minors
  int? get minorAgeYears => integer('guardian.minor_age_years');
  String? get minorDmPolicy => string('minor.dm_policy');
  String? get minorProfileDefaultVisibility => string('minor.profile_default_visibility');
  bool? get minorCanJoinPublicCommunities => boolean('minor.can_join_public_communities');

  // events
  bool? get eventRequireAdminApproval => boolean('event.require_admin_approval');
  String? get eventShowContact => string('event.show_contact');
  int? get eventMaxActivePerOrganiser => integer('event.max_active_per_organiser');
  int? get eventMinHoursBeforeStart => integer('event.min_hours_before_start');
  bool? get eventAllowFreeEvents => boolean('event.allow_free_events');
  int? get eventMaxSlotsLimit => integer('event.max_slots_limit');
  int? get eventMaxSearchRadiusKm => integer('event.max_search_radius_km');
  String? get eventEligibilityDefaultGovernment => string('event.eligibility_default_government');
  String? get eventEligibilityDefaultPrivate => string('event.eligibility_default_private');

  // Location identity.
  /// Defaults to on when the key is missing; the server enforces the real switch.
  bool get locationEnabled => boolean('location.enabled') ?? true;

  // listing
  int? get listingOrderTtlMinutes => integer('listing.order_ttl_minutes');
  int? get refundBeforeFirstRegistrationPercent =>
      integer('listing.refund.before_first_registration_percent');
  int? get refundAfterRegistrationsPercent => integer('listing.refund.after_registrations_percent');
  int? get refundOnAdminRejectionPercent => integer('listing.refund.on_admin_rejection_percent');

  // registration
  int? get withdrawUntilHours => integer('registration.withdraw_until_hours');
  bool? get waitlistEnabled => boolean('registration.waitlist_enabled');

  // chat
  int? get chatMessageMaxLength => integer('chat.message_max_length');
  int? get chatGroupMaxMembers => integer('chat.group_max_members');
  int? get maxCommunitiesCreatedPerUser => integer('chat.max_communities_created_per_user');
  bool get chatMediaEnabled => boolean('chat.media_enabled') ?? false;

  // feed
  int? get postMaxLength => integer('feed.post_max_length');
  int? get postMaxMedia => integer('feed.post_max_media');
  int? get feedPageSize => integer('feed.page_size');

  // profile
  int? get profileViewersWindowDays => integer('profile.viewers_window_days');
  int? get maxAchievements => integer('profile.max_achievements');

  // media
  int? get imageMaxMb => integer('media.image_max_mb');
  int? get documentMaxMb => integer('media.document_max_mb');
  List<String>? get allowedImageTypes => stringList('media.allowed_image_types');
  List<String>? get allowedDocumentTypes => stringList('media.allowed_document_types');

  // search
  int? get searchPageSize => integer('search.page_size');

  // ads
  bool get adsEnabled => boolean('ads.enabled') ?? false;
  int? get adsFeedEveryNPosts => integer('ads.feed_every_n_posts');

  // premium / consultation
  bool get premiumEnabled => boolean('premium.enabled') ?? false;
  int? get maxOpenConsultationRequests => integer('consultation.max_open_requests_per_player');

  // app
  bool get maintenanceMode => boolean('app.maintenance_mode') ?? false;
  String? get supportEmail => string('app.support_email');
  String? get grievanceOfficerName => asStringOrNull(json('app.grievance_officer')?['name']);
  String? get grievanceOfficerEmail => asStringOrNull(json('app.grievance_officer')?['email']);
}
