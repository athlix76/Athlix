/// Admin permissions from docs/security.md (Admin permissions). `*` grants all.
abstract final class Perm {
  static const all = '*';
  static const dashboardRead = 'dashboard.read';
  static const adminsManage = 'admins.manage';
  static const auditRead = 'audit.read';
  static const configRead = 'config.read';
  static const configWrite = 'config.write';
  static const policiesWrite = 'policies.write';
  static const sportsWrite = 'sports.write';
  static const feesWrite = 'fees.write';
  static const plansWrite = 'plans.write';
  static const usersRead = 'users.read';
  static const usersWrite = 'users.write';
  static const organisersReview = 'organisers.review';
  static const eventsReview = 'events.review';
  static const paymentsRead = 'payments.read';
  static const paymentsRefund = 'payments.refund';
  static const reportsRead = 'reports.read';
  static const reportsResolve = 'reports.resolve';
  static const moderationReadMessages = 'moderation.read_messages';
  static const communitiesModerate = 'communities.moderate';
  static const adsWrite = 'ads.write';
  static const notificationsBroadcast = 'notifications.broadcast';
  static const locationReview = 'location.review';
  static const geoWrite = 'geo.write';

  /// Every grantable permission, in the order the role editor shows them.
  static const List<String> values = [
    dashboardRead,
    adminsManage,
    auditRead,
    configRead,
    configWrite,
    policiesWrite,
    sportsWrite,
    feesWrite,
    plansWrite,
    usersRead,
    usersWrite,
    organisersReview,
    eventsReview,
    paymentsRead,
    paymentsRefund,
    reportsRead,
    reportsResolve,
    moderationReadMessages,
    communitiesModerate,
    adsWrite,
    notificationsBroadcast,
    locationReview,
    geoWrite,
  ];
}

/// True when [granted] includes [required] or `*`.
bool hasPermission(Iterable<String> granted, String required) {
  for (final g in granted) {
    if (g == Perm.all || g == required) return true;
  }
  return false;
}

/// True when [anyOf] is empty or at least one entry is granted.
bool hasAnyPermission(Iterable<String> granted, Iterable<String> anyOf) {
  if (anyOf.isEmpty) return true;
  return anyOf.any((p) => hasPermission(granted, p));
}
