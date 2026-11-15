import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/models/sport.dart';
import '../../../core/session/session_controller.dart';
import '../../follow/application/follow_controller.dart';
import '../data/profile_api.dart';
import '../data/profile_models.dart';

/// One profile. Reloads after any follow / block change.
final profileProvider = FutureProvider.autoDispose.family<Profile, int>((ref, userId) {
  ref.watch(followChangesProvider);
  return ref.watch(profileApiProvider).get(userId);
});

/// The signed-in user's own profile (Me tab, edit screen). Null when signed out.
final myProfileProvider = FutureProvider.autoDispose<Profile?>((ref) {
  final id = ref.watch(currentUserIdProvider);
  if (id == null) return Future.value(null);
  return ref.watch(profileProvider(id).future);
});

final statsProvider = FutureProvider.autoDispose.family<StatsResult, int>(
  (ref, userId) => ref.watch(profileApiProvider).stats(userId),
);

final achievementsProvider = FutureProvider.autoDispose.family<List<Achievement>, int>(
  (ref, userId) => ref.watch(profileApiProvider).achievements(userId),
);

/// A stat template together with its sport, looked up in the bootstrap catalogue.
typedef SportTemplate = ({Sport sport, StatTemplate template});

/// Finds a template by id in the sports config. Null when the catalogue no longer has it.
SportTemplate? findTemplate(List<Sport> sports, int sportId, int templateId) {
  for (final s in sports) {
    if (s.id != sportId) continue;
    for (final t in s.statTemplates) {
      if (t.id == templateId) return (sport: s, template: t);
    }
  }
  return null;
}

/// Templates that fit the given profile sports: the sport matches, and the template
/// is either for all positions or for the profile's position in that sport.
List<SportTemplate> templatesForProfile(List<Sport> sports, Profile profile) {
  final result = <SportTemplate>[];
  for (final ps in profile.sports) {
    for (final s in sports) {
      if (s.id != ps.sport.id) continue;
      for (final t in s.statTemplates) {
        if (t.positionId == null || t.positionId == ps.position?.id) result.add((sport: s, template: t));
      }
    }
  }
  return result;
}
