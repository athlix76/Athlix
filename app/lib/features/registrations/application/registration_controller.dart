import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../events/application/event_providers.dart';
import '../data/registration_api.dart';
import '../data/registration_models.dart';

/// Player and organiser writes. Each refreshes the cached event (slots,
/// `myRegistration`).
class RegistrationController {
  RegistrationController(this._ref);

  final Ref _ref;

  RegistrationApi get _api => _ref.read(registrationApiProvider);

  Future<Registration> register(int eventId, {String? teamName, List<TeamMember>? members}) async {
    final r = await _api.register(eventId, teamName: teamName, teamMembers: members);
    _ref.invalidate(eventProvider(eventId));
    return r;
  }

  Future<void> withdraw(int eventId) async {
    await _api.withdraw(eventId);
    _ref.invalidate(eventProvider(eventId));
  }

  Future<Registration> markPayment(int eventId, int regId, String paymentStatus, {String? note}) =>
      _api.update(eventId, regId, paymentStatus: paymentStatus, paymentNote: note);

  Future<Registration> reject(int eventId, int regId) async {
    final r = await _api.update(eventId, regId, status: 'rejected');
    _ref.invalidate(eventProvider(eventId));
    return r;
  }
}

final registrationControllerProvider =
    Provider<RegistrationController>(RegistrationController.new);
