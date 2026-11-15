import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'organiser_models.dart';

class OrganiserApi {
  const OrganiserApi(this._api);

  final ApiClient _api;

  Future<OrganiserVerification> get() => _api.get(
        '/organiser/verification',
        (d) => OrganiserVerification.fromJson(asMap(d)),
      );

  /// Save draft. Only in `not_started | draft | rejected`.
  Future<OrganiserVerification> save({String? orgName, String? orgType, List<StepInput>? steps}) =>
      _api.put(
        '/organiser/verification',
        (d) => OrganiserVerification.fromJson(asMap(d)),
        body: {
          if (orgName != null) 'orgName': orgName,
          if (orgType != null) 'orgType': orgType,
          if (steps != null) 'steps': steps.map((s) => s.toJson()).toList(),
        },
      );

  Future<OrganiserVerification> submit() => _api.post(
        '/organiser/verification/submit',
        (d) => OrganiserVerification.fromJson(asMap(d)),
      );
}

final organiserApiProvider = Provider<OrganiserApi>((ref) => OrganiserApi(ref.watch(apiClientProvider)));
