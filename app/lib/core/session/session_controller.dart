import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/api_exception.dart';
import '../api/api_signals.dart';
import '../auth/token_pair.dart';
import '../json/json_utils.dart';
import '../models/me.dart';
import '../providers.dart';

enum SessionStatus { unknown, signedOut, signedIn }

class SessionState {
  const SessionState({required this.status, this.me, this.loadError});

  final SessionStatus status;
  final Me? me;

  /// Set when `/me` could not be loaded at start (offline): splash offers retry.
  final ApiException? loadError;

  static const unknown = SessionState(status: SessionStatus.unknown);
  static const signedOut = SessionState(status: SessionStatus.signedOut);

  bool get isSignedIn => status == SessionStatus.signedIn && me != null;
}

/// Owns "who is signed in". Tokens go to secure storage only; `Me` is kept in
/// memory and re-fetched on start.
class SessionController extends Notifier<SessionState> {
  @override
  SessionState build() {
    final sub = ref.watch(apiSignalsProvider).stream.listen(_onSignal);
    ref.onDispose(sub.cancel);
    Future.microtask(restore);
    return SessionState.unknown;
  }

  void _onSignal(ApiSignal s) {
    switch (s.type) {
      case ApiSignalType.sessionExpired:
        unawaited(handleSessionExpired());
      case ApiSignalType.onboardingRequired:
        applyNextStep(s.nextStep);
      case ApiSignalType.suspended:
        final me = state.me;
        if (me != null) {
          state = SessionState(status: SessionStatus.signedIn, me: me.copyWith(status: 'suspended'));
        }
      case ApiSignalType.maintenance:
        break;
    }
  }

  /// Start-up: tokens in secure storage → load `/me`.
  Future<void> restore() async {
    state = SessionState.unknown;
    final tokens = await ref.read(tokenStoreProvider).read();
    if (tokens == null) {
      state = SessionState.signedOut;
      return;
    }
    try {
      final me = await _fetchMe();
      state = SessionState(status: SessionStatus.signedIn, me: me);
    } on ApiException catch (e) {
      if (e.code == ApiErrorCode.unauthenticated) {
        await ref.read(tokenStoreProvider).clear();
        state = SessionState.signedOut;
      } else {
        state = SessionState(status: SessionStatus.unknown, loadError: e);
      }
    }
  }

  Future<Me> _fetchMe() => ref.read(apiClientProvider).get('/me', (d) => Me.fromJson(asMap(d)));

  /// After a successful OTP verify.
  Future<void> signIn(TokenPair tokens, Me me) async {
    await ref.read(tokenStoreProvider).save(tokens);
    state = SessionState(status: SessionStatus.signedIn, me: me);
  }

  /// Any endpoint that returns `Me` (onboarding, policies, guardian) calls this.
  void setMe(Me me) {
    if (state.status == SessionStatus.signedIn) {
      state = SessionState(status: SessionStatus.signedIn, me: me);
    }
  }

  Future<Me?> refreshMe() async {
    if (state.status != SessionStatus.signedIn) return null;
    try {
      final me = await _fetchMe();
      setMe(me);
      return me;
    } on ApiException {
      return state.me;
    }
  }

  /// 403 `ONBOARDING_REQUIRED`: move the user to `nextStep` now (the router
  /// redirect reacts) and re-sync `Me` in the background.
  void applyNextStep(String? nextStep) {
    final me = state.me;
    if (me == null) return;
    if (nextStep != null && nextStep.isNotEmpty && nextStep != me.onboardingStep) {
      state = SessionState(status: SessionStatus.signedIn, me: me.copyWith(onboardingStep: nextStep));
    }
    unawaited(refreshMe());
  }

  Future<void> handleSessionExpired() async {
    await ref.read(tokenStoreProvider).clear();
    state = SessionState.signedOut;
  }

  /// Revokes this device's session family on the server, then forgets tokens.
  Future<void> logout() async {
    final store = ref.read(tokenStoreProvider);
    final tokens = await store.read();
    if (tokens != null) {
      try {
        await ref
            .read(apiClientProvider)
            .postVoid('/auth/logout', body: {'refreshToken': tokens.refreshToken});
      } on ApiException {
        // Signing out locally must work offline too.
      }
    }
    await store.clear();
    state = SessionState.signedOut;
  }

  /// `DELETE /me` (soft delete on the server), then sign out locally.
  Future<void> deleteAccount() async {
    await ref.read(apiClientProvider).delete('/me');
    await ref.read(tokenStoreProvider).clear();
    state = SessionState.signedOut;
  }
}

final sessionProvider = NotifierProvider<SessionController, SessionState>(SessionController.new);

/// The signed-in user, or null.
final meProvider = Provider<Me?>((ref) => ref.watch(sessionProvider).me);

/// User id for user-scoped providers: they rebuild when the user changes.
final currentUserIdProvider = Provider<int?>(
  (ref) => ref.watch(sessionProvider.select((s) => s.me?.id)),
);
