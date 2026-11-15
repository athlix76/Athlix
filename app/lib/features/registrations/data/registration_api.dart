import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'registration_models.dart';

class RegistrationApi {
  const RegistrationApi(this._api);

  final ApiClient _api;

  Future<Registration> register(int eventId, {String? teamName, List<TeamMember>? teamMembers}) =>
      _api.post(
        '/events/$eventId/registrations',
        (d) => Registration.fromJson(asMap(d)),
        body: {
          if (teamName != null) 'teamName': teamName,
          if (teamMembers != null && teamMembers.isNotEmpty)
            'teamMembers': teamMembers.map((m) => m.toJson()).toList(),
        },
      );

  Future<void> withdraw(int eventId) => _api.delete('/events/$eventId/registrations/me');

  Future<Paged<Registration>> mine({String? status, String? cursor}) => _api.page(
        '/me/registrations',
        Registration.fromJson,
        query: {'status': status, 'cursor': cursor},
      );

  Future<Paged<Registration>> forEvent(int eventId, {String? status, bool flagged = false, String? cursor}) =>
      _api.page(
        '/events/$eventId/registrations',
        Registration.fromJson,
        query: {'status': status, 'flagged': flagged ? true : null, 'cursor': cursor},
      );

  Future<Registration> update(
    int eventId,
    int regId, {
    String? paymentStatus,
    String? paymentNote,
    String? status,
  }) =>
      _api.patch(
        '/events/$eventId/registrations/$regId',
        (d) => Registration.fromJson(asMap(d)),
        body: {
          if (paymentStatus != null) 'paymentStatus': paymentStatus,
          if (paymentNote != null) 'paymentNote': paymentNote,
          if (status != null) 'status': status,
        },
      );
}

final registrationApiProvider =
    Provider<RegistrationApi>((ref) => RegistrationApi(ref.watch(apiClientProvider)));
