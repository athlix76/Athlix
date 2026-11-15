import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/me.dart';
import '../../../core/providers.dart';

/// Roles a user can pick (`primaryRole`).
abstract final class UserRole {
  static const player = 'player';
  static const coach = 'coach';
  static const organiser = 'organiser';
  static const all = [player, coach, organiser];
}

/// `ProfileSetup` body of `POST /onboarding/profile`.
class ProfileSetup {
  const ProfileSetup({
    required this.displayName,
    required this.sportId,
    this.positionId,
    required this.city,
    this.district,
    required this.state,
    this.bio,
    this.academyClub,
    this.photoUploadId,
  });

  final String displayName;
  final int sportId;
  final int? positionId;
  final String city;
  final String? district;
  final String state;
  final String? bio;
  final String? academyClub;
  final int? photoUploadId;

  Map<String, dynamic> toJson() => {
        'displayName': displayName,
        'sportId': sportId,
        'positionId': positionId,
        'city': city,
        'district': district,
        'state': state,
        'bio': bio,
        'academyClub': academyClub,
        'photoUploadId': photoUploadId,
      };
}

class OnboardingApi {
  const OnboardingApi(this._api);

  final ApiClient _api;

  /// `POST /onboarding/role` → Me.
  Future<Me> setRole(String role, {List<String> additionalRoles = const []}) => _api.post(
        '/onboarding/role',
        (d) => Me.fromJson(asMap(d)),
        body: {'role': role, 'additionalRoles': additionalRoles},
      );

  /// `POST /onboarding/profile` → Me (step becomes `done`).
  Future<Me> completeProfile(ProfileSetup setup) =>
      _api.post('/onboarding/profile', (d) => Me.fromJson(asMap(d)), body: setup.toJson());
}

final onboardingApiProvider =
    Provider<OnboardingApi>((ref) => OnboardingApi(ref.watch(apiClientProvider)));
