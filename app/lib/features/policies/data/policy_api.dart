import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/models/me.dart';
import '../../../core/models/policy.dart';
import '../../../core/providers.dart';

class PolicyApi {
  const PolicyApi(this._api);

  final ApiClient _api;

  /// `GET /policies/:docType` (public): the active version.
  Future<PolicyDocument> get(String docType) => _api.get(
        '/policies/${Uri.encodeComponent(docType)}',
        (d) => PolicyDocument.fromJson(asMap(d)),
        auth: false,
      );

  /// `POST /me/policies/accept` → Me.
  Future<Me> accept(List<int> policyDocumentIds) => _api.post(
        '/me/policies/accept',
        (d) => Me.fromJson(asMap(d)),
        body: {'policyDocumentIds': policyDocumentIds},
      );
}

final policyApiProvider = Provider<PolicyApi>((ref) => PolicyApi(ref.watch(apiClientProvider)));
