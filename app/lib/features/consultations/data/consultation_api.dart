import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/api/api_client.dart';
import '../../../core/api/envelope.dart';
import '../../../core/json/json_utils.dart';
import '../../../core/providers.dart';
import 'consultation_models.dart';

class ConsultationApi {
  const ConsultationApi(this._api);

  final ApiClient _api;

  /// 404 if the coach does not offer consultations (unless it is me).
  Future<ConsultationOffer> offer(int coachId) => _api.get(
        '/coaches/$coachId/consultation',
        (d) => ConsultationOffer.fromJson(asMap(d)),
      );

  /// `GET /me/consultation` (coaches only): my own offer, including when it is off.
  Future<ConsultationOffer> myOffer() => _api.get(
        '/me/consultation',
        (d) => ConsultationOffer.fromJson(asMap(d)),
      );

  Future<ConsultationOffer> updateMyOffer(ConsultationOfferInput input) => _api.put(
        '/me/consultation',
        (d) => ConsultationOffer.fromJson(asMap(d)),
        body: input.toJson(),
      );

  Future<ConsultationRequest> request(int coachId, {required String message, required String preferredMode}) =>
      _api.post(
        '/coaches/$coachId/consultation-requests',
        (d) => ConsultationRequest.fromJson(asMap(d)),
        body: {'message': message, 'preferredMode': preferredMode},
      );

  Future<Paged<ConsultationRequest>> mine({required String side, String? status, String? cursor}) =>
      _api.page(
        '/me/consultation-requests',
        ConsultationRequest.fromJson,
        query: {'as': side, 'status': status, 'cursor': cursor},
      );

  Future<ConsultationRequest> respond(int id, {required bool accept, String? note}) => _api.post(
        '/consultation-requests/$id/respond',
        (d) => ConsultationRequest.fromJson(asMap(d)),
        body: {'accept': accept, if (note != null && note.isNotEmpty) 'note': note},
      );

  Future<ConsultationRequest> cancel(int id) => _api.post(
        '/consultation-requests/$id/cancel',
        (d) => ConsultationRequest.fromJson(asMap(d)),
      );

  Future<ConsultationRequest> complete(int id) => _api.post(
        '/consultation-requests/$id/complete',
        (d) => ConsultationRequest.fromJson(asMap(d)),
      );
}

final consultationApiProvider =
    Provider<ConsultationApi>((ref) => ConsultationApi(ref.watch(apiClientProvider)));
