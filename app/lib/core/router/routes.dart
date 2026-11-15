/// Every app location in one place. Screens navigate with these helpers,
/// never with string literals.
abstract final class Routes {
  // Start-up gates.
  static const splash = '/splash';
  static const forceUpdate = '/force-update';
  static const maintenance = '/maintenance';
  static const suspended = '/suspended';

  // Sign-in and onboarding.
  static const authPhone = '/auth/phone';
  static const authOtp = '/auth/otp';
  static const onboardingPolicies = '/onboarding/policies';
  static const onboardingRole = '/onboarding/role';
  static const onboardingProfile = '/onboarding/profile';
  static const verification = '/verification';
  static const verificationResult = '/verification/result';
  static const guardian = '/guardian';
  static const guardianOtp = '/guardian/otp';
  static String policy(String docType) => '/policy/$docType';

  /// Deep link `athlix://verification/result?...`: Flutter drops the host, so
  /// the router sees `/result?...`.
  static const deepLinkResult = '/result';

  // Bottom tabs.
  static const home = '/home';
  static const events = '/events';
  static const feed = '/feed';
  static const chat = '/chat';
  static const me = '/me';

  // Events, listing and registrations.
  static const eventNew = '/events/new';
  static String event(int id) => '/events/$id';
  static String eventEdit(int id) => '/events/$id/edit';
  static String eventCheckout(int id) => '/events/$id/checkout';
  static String eventRegistrations(int id) => '/events/$id/registrations';
  static String eventRegister(int id) => '/events/$id/register';
  static const myEvents = '/my-events';
  static const myRegistrations = '/registrations';

  // Search and people.
  static const search = '/search';
  static const searchPeople = '/search/people';
  static String profile(int userId) => '/profile/$userId';
  static String followers(int userId) => '/profile/$userId/followers';
  static String following(int userId) => '/profile/$userId/following';
  static String stats(int userId) => '/profile/$userId/stats';
  static String achievements(int userId) => '/profile/$userId/achievements';
  static String userPosts(int userId) => '/profile/$userId/posts';
  static const meEdit = '/me/edit';
  static const meViewers = '/me/viewers';

  // Chat and communities.
  static String conversation(int id) => '/chat/$id';
  static String conversationMembers(int id) => '/chat/$id/members';
  static const communities = '/communities';
  static const communityNew = '/communities/new';
  static String community(int id) => '/communities/$id';
  static String communityEdit(int id) => '/communities/$id/edit';
  static String communityManage(int id) => '/communities/$id/manage';

  // Feed.
  static const postNew = '/posts/new';
  static String post(int id) => '/posts/$id';

  // Everything else.
  static const notifications = '/notifications';
  static const premium = '/premium';
  static const consultations = '/consultations';
  static const consultationOffer = '/consultations/offer';
  static String coachConsultation(int userId) => '/coaches/$userId/consultation';
  static const organiserVerification = '/organiser/verification';
  static const settings = '/settings';
  static const blocked = '/settings/blocked';
  static const locationIdentity = '/settings/location';
  static const locationIdentitySubmitPath = '/settings/location/submit';
  static String locationIdentitySubmit(String identityType) =>
      '$locationIdentitySubmitPath?type=$identityType';
}
