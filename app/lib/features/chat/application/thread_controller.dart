import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../core/media/media_upload.dart';
import '../../../core/session/session_controller.dart';
import '../../../core/widgets/error_view.dart';
import '../../../core/ws/chat_socket.dart';
import '../data/chat_api.dart';
import '../data/chat_models.dart';
import 'chat_socket_provider.dart';
import 'client_msg_id.dart';

/// Messages per page. Transport-level only: the contract has no config key
/// for it, so keep it small and change it here.
const int kMessagePageSize = 30;

/// Wait this long for a socket `ack` before re-sending over REST (safe: the
/// REST send is idempotent on `clientMsgId`).
const Duration kAckTimeout = Duration(seconds: 10);

/// Poll `GET .../messages` this often while the socket is down.
const Duration kPollInterval = Duration(seconds: 15);

/// A typing indicator disappears after this long without a new frame.
const Duration kTypingTtl = Duration(seconds: 5);

/// Send at most one `typing` frame per this interval.
const Duration kTypingThrottle = Duration(seconds: 3);

/// One-off message for the screen to show as a snack. A new instance each
/// time, so equal text still triggers `ref.listen`.
class ThreadNotice {
  ThreadNotice(this.text);
  final String text;
}

class ThreadState {
  const ThreadState({
    this.messages = const [],
    this.pending = const {},
    this.failed = const {},
    this.readUpTo = const {},
    this.typing = const {},
    this.hasMore = true,
    this.loading = true,
    this.loadingMore = false,
    this.error,
    this.notice,
  });

  /// Newest first. Unsent (optimistic) messages have `id == 0`.
  final List<Message> messages;

  /// `clientMsgId`s in flight / failed.
  final Set<String> pending;
  final Set<String> failed;

  /// userId -> highest message id they have read (from `read` frames this session).
  final Map<int, int> readUpTo;

  /// userIds typing right now (never includes me).
  final Set<int> typing;
  final bool hasMore;
  final bool loading;
  final bool loadingMore;

  /// Error of the first load only; later errors become [notice]s.
  final Object? error;
  final ThreadNotice? notice;

  ThreadState copyWith({
    List<Message>? messages,
    Set<String>? pending,
    Set<String>? failed,
    Map<int, int>? readUpTo,
    Set<int>? typing,
    bool? hasMore,
    bool? loading,
    bool? loadingMore,
    Object? error,
    bool clearError = false,
    ThreadNotice? notice,
  }) =>
      ThreadState(
        messages: messages ?? this.messages,
        pending: pending ?? this.pending,
        failed: failed ?? this.failed,
        readUpTo: readUpTo ?? this.readUpTo,
        typing: typing ?? this.typing,
        hasMore: hasMore ?? this.hasMore,
        loading: loading ?? this.loading,
        loadingMore: loadingMore ?? this.loadingMore,
        error: clearError ? null : (error ?? this.error),
        notice: notice ?? this.notice,
      );
}

/// What we need to (re)send one message. Kept only in memory.
class _Outgoing {
  _Outgoing({required this.kind, this.body, this.file, this.replyToId});

  final String kind;
  final String? body;
  final UploadFile? file;
  int? uploadId; // set after the first upload so a retry does not upload again
  final int? replyToId;
}

/// State of one open thread: history, optimistic sends, receipts, typing.
///
/// Transport: the socket when connected, REST otherwise; every path works
/// without the socket. Never log message bodies or tickets from here.
class ThreadController extends AutoDisposeFamilyNotifier<ThreadState, int> {
  final Map<String, _Outgoing> _outbox = {};
  final Map<String, Timer> _ackTimers = {};
  final Map<int, Timer> _typingTimers = {};
  StreamSubscription<ServerFrame>? _frames;
  StreamSubscription<SocketStatus>? _status;
  Timer? _poll;
  _ResumeObserver? _observer;
  DateTime? _lastTypingSent;
  int _lastReadSent = 0;
  bool _disposed = false;

  int get _id => arg;
  ChatApi get _api => ref.read(chatApiProvider);
  ChatSocket get _socket => ref.read(chatSocketProvider);
  int? get _me => ref.read(currentUserIdProvider);

  @override
  ThreadState build(int conversationId) {
    final socket = ref.watch(chatSocketProvider);
    _disposed = false;
    _frames = socket.frames.listen(_onFrame);
    _status = socket.statusChanges.listen(_onStatus);
    _observer = _ResumeObserver(_onResume);
    WidgetsBinding.instance.addObserver(_observer!);
    ref.onDispose(_teardown);
    _syncPoll(socket.status);
    Future.microtask(() => refreshLatest(initial: true));
    return const ThreadState();
  }

  void _teardown() {
    _disposed = true;
    _frames?.cancel();
    _status?.cancel();
    _poll?.cancel();
    for (final t in _ackTimers.values) {
      t.cancel();
    }
    for (final t in _typingTimers.values) {
      t.cancel();
    }
    _ackTimers.clear();
    _typingTimers.clear();
    final o = _observer;
    if (o != null) WidgetsBinding.instance.removeObserver(o);
    _observer = null;
  }

  void _set(ThreadState s) {
    if (!_disposed) state = s;
  }

  // ---- history -------------------------------------------------------------

  /// Fetch the newest page and merge it. Used on open, resume, reconnect and poll.
  Future<void> refreshLatest({bool initial = false}) async {
    try {
      final page = await _api.messages(_id, limit: kMessagePageSize);
      if (_disposed) return;
      final known = _newestSentId(state.messages);
      final gap = page.items.isNotEmpty && known > 0 && page.items.last.id > known && _hasMore(page.items.length, page.hasMore);
      final base = gap ? state.messages.where((m) => !m.isSent).toList() : state.messages;
      _set(state.copyWith(
        messages: _merge(base, page.items),
        hasMore: initial || gap ? _hasMore(page.items.length, page.hasMore) : state.hasMore,
        loading: false,
        clearError: true,
      ));
      _markReadLatest();
    } catch (e) {
      if (_disposed) return;
      if (state.loading) {
        _set(state.copyWith(loading: false, error: e));
      } else if (initial) {
        _set(state.copyWith(notice: ThreadNotice(errorMessage(e))));
      }
      // Background refreshes fail quietly; the next poll/resume retries.
    }
  }

  /// Retry after a failed first load.
  Future<void> reload() async {
    _set(state.copyWith(loading: true, clearError: true));
    await refreshLatest(initial: true);
  }

  /// Older messages (`?before=<oldest id>`).
  Future<void> loadMore() async {
    if (state.loadingMore || !state.hasMore || state.loading) return;
    final oldest = _oldestSentId(state.messages);
    if (oldest == 0) return;
    _set(state.copyWith(loadingMore: true));
    try {
      final page = await _api.messages(_id, before: oldest, limit: kMessagePageSize);
      _set(state.copyWith(
        messages: _merge(state.messages, page.items),
        hasMore: _hasMore(page.items.length, page.hasMore),
        loadingMore: false,
      ));
    } catch (e) {
      _set(state.copyWith(loadingMore: false, notice: ThreadNotice(errorMessage(e))));
    }
  }

  /// The server may or may not send `meta.nextCursor` for `before` paging;
  /// a full page is treated as "maybe more".
  bool _hasMore(int count, bool serverSaysMore) => serverSaysMore || count >= kMessagePageSize;

  // ---- sending -------------------------------------------------------------

  void sendText(String text, {Message? replyTo}) {
    final body = text.trim();
    if (body.isEmpty) return;
    _enqueue(_Outgoing(kind: MessageKind.text, body: body, replyToId: replyTo?.id), replyTo: replyTo);
  }

  /// Uploads [file] as `chat_image`, then sends a `kind: image` message.
  void sendImage(UploadFile file, {Message? replyTo}) {
    final problem = ref.read(mediaUploaderProvider).validate(MediaPurpose.chatImage, file);
    if (problem != null) {
      _set(state.copyWith(notice: ThreadNotice(problem)));
      return;
    }
    _enqueue(_Outgoing(kind: MessageKind.image, file: file, replyToId: replyTo?.id), replyTo: replyTo);
  }

  void _enqueue(_Outgoing out, {Message? replyTo}) {
    final clientMsgId = newClientMsgId();
    _outbox[clientMsgId] = out;
    final optimistic = Message(
      id: 0,
      conversationId: _id,
      kind: out.kind,
      body: out.body,
      replyTo: replyTo == null
          ? null
          : ReplyPreview(id: replyTo.id, senderName: replyTo.sender?.displayName, bodyPreview: replyTo.preview),
      clientMsgId: clientMsgId,
      status: MessageStatus.visible,
      createdAt: DateTime.now(),
    );
    _set(state.copyWith(
      messages: [optimistic, ...state.messages],
      pending: {...state.pending, clientMsgId},
    ));
    unawaited(_dispatch(clientMsgId));
  }

  /// Retry a failed message with the same `clientMsgId`.
  void retry(String clientMsgId) {
    if (!_outbox.containsKey(clientMsgId)) return;
    _set(state.copyWith(
      failed: {...state.failed}..remove(clientMsgId),
      pending: {...state.pending, clientMsgId},
    ));
    unawaited(_dispatch(clientMsgId));
  }

  /// Drop a failed message that was never sent.
  void discard(String clientMsgId) {
    _outbox.remove(clientMsgId);
    _set(state.copyWith(
      messages: state.messages.where((m) => m.isSent || m.clientMsgId != clientMsgId).toList(),
      failed: {...state.failed}..remove(clientMsgId),
      pending: {...state.pending}..remove(clientMsgId),
    ));
  }

  Future<void> _dispatch(String clientMsgId) async {
    final out = _outbox[clientMsgId];
    if (out == null) return;
    if (out.file != null && out.uploadId == null) {
      try {
        out.uploadId = await ref.read(mediaUploaderProvider).upload(MediaPurpose.chatImage, out.file!);
      } catch (e) {
        _fail(clientMsgId, errorMessage(e));
        return;
      }
      if (_disposed) return;
    }
    final viaSocket = _socket.sendMessage(
      conversationId: _id,
      clientMsgId: clientMsgId,
      kind: out.kind,
      body: out.body,
      mediaUploadId: out.uploadId,
      replyToId: out.replyToId,
    );
    if (viaSocket) {
      _ackTimers[clientMsgId]?.cancel();
      _ackTimers[clientMsgId] = Timer(kAckTimeout, () => unawaited(_sendRest(clientMsgId)));
    } else {
      await _sendRest(clientMsgId);
    }
  }

  Future<void> _sendRest(String clientMsgId) async {
    _ackTimers.remove(clientMsgId)?.cancel();
    final out = _outbox[clientMsgId];
    if (out == null || _disposed) return;
    try {
      final m = await _api.send(
        _id,
        clientMsgId: clientMsgId,
        kind: out.kind,
        body: out.body,
        mediaUploadId: out.uploadId,
        replyToId: out.replyToId,
      );
      _applyServerMessage(m);
    } catch (e) {
      _fail(clientMsgId, errorMessage(e));
    }
  }

  void _fail(String clientMsgId, String text) {
    _ackTimers.remove(clientMsgId)?.cancel();
    if (!_outbox.containsKey(clientMsgId)) return;
    _set(state.copyWith(
      pending: {...state.pending}..remove(clientMsgId),
      failed: {...state.failed, clientMsgId},
      notice: ThreadNotice(text),
    ));
  }

  /// Insert or replace a server message (matching an optimistic one by `clientMsgId`).
  void _applyServerMessage(Message m) {
    if (m.conversationId != _id) return;
    final cid = m.clientMsgId;
    if (cid != null) {
      _ackTimers.remove(cid)?.cancel();
      _outbox.remove(cid);
    }
    final typing = {...state.typing};
    final sid = m.sender?.id;
    if (sid != null && typing.remove(sid)) _typingTimers.remove(sid)?.cancel();
    _set(state.copyWith(
      messages: _merge(state.messages, [m]),
      pending: cid == null ? null : ({...state.pending}..remove(cid)),
      failed: cid == null ? null : ({...state.failed}..remove(cid)),
      typing: typing,
    ));
  }

  // ---- delete --------------------------------------------------------------

  Future<void> delete(Message m) async {
    await _api.deleteMessage(m.id);
    _applyDeleted(m.id);
  }

  void _applyDeleted(int messageId) {
    _set(state.copyWith(
      messages: [for (final m in state.messages) m.id == messageId ? m.markDeleted() : m],
    ));
  }

  // ---- receipts and typing --------------------------------------------------

  /// Mark the newest message from someone else as read (socket, else REST).
  void _markReadLatest() {
    final me = _me;
    final latest = state.messages.where((m) => m.isSent && m.sender != null && m.sender!.id != me);
    if (latest.isEmpty) return;
    final id = latest.first.id;
    if (id <= _lastReadSent) return;
    // Only while the user can actually see the thread.
    final life = WidgetsBinding.instance.lifecycleState;
    if (life != null && life != AppLifecycleState.resumed) return;
    _lastReadSent = id;
    if (_socket.markRead(_id, id)) {
      // The socket gives no ack for reads; refresh the tab badge shortly after.
      Timer(const Duration(seconds: 1), () {
        if (!_disposed) ref.invalidate(unreadChatProvider);
      });
    } else {
      unawaited(_api.markRead(_id, id).then((_) {
        if (!_disposed) ref.invalidate(unreadChatProvider);
      }).catchError((Object _) {
        _lastReadSent = 0; // try again on the next refresh
      }));
    }
  }

  /// Call on every composer change; throttled, socket only (typing is best effort).
  void userTyping() {
    final now = DateTime.now();
    final last = _lastTypingSent;
    if (last != null && now.difference(last) < kTypingThrottle) return;
    if (_socket.typing(_id)) _lastTypingSent = now;
  }

  // ---- socket frames ----------------------------------------------------------

  void _onFrame(ServerFrame f) {
    if (_disposed) return;
    final d = f.dataMap;
    switch (f.type) {
      case WsType.ack:
        final msg = d['message'];
        if (msg is Map) _applyServerMessage(Message.fromJson(Map<String, dynamic>.from(msg)));
      case WsType.messageNew:
        final m = Message.fromJson(d);
        if (m.conversationId != _id) return;
        _applyServerMessage(m);
        _markReadLatest();
      case WsType.messageDeleted:
        if (_asId(d['conversationId']) == _id) _applyDeleted(_asId(d['messageId']));
      case WsType.typing:
        final uid = _asId(d['userId']);
        if (_asId(d['conversationId']) != _id || uid == 0 || uid == _me) return;
        _typingTimers[uid]?.cancel();
        _typingTimers[uid] = Timer(kTypingTtl, () {
          _typingTimers.remove(uid);
          _set(state.copyWith(typing: {...state.typing}..remove(uid)));
        });
        if (!state.typing.contains(uid)) _set(state.copyWith(typing: {...state.typing, uid}));
      case WsType.read:
        final uid = _asId(d['userId']);
        final mid = _asId(d['messageId']);
        if (_asId(d['conversationId']) != _id || uid == 0) return;
        if ((state.readUpTo[uid] ?? 0) >= mid) return;
        _set(state.copyWith(readUpTo: {...state.readUpTo, uid: mid}));
      case WsType.error:
        final cid = d['clientMsgId'];
        if (cid is String && _outbox.containsKey(cid)) {
          _fail(cid, d['message'] is String ? d['message'] as String : 'Message not sent.');
        }
    }
  }

  static int _asId(Object? v) => v is num ? v.toInt() : int.tryParse('$v') ?? 0;

  void _onStatus(SocketStatus s) {
    if (_disposed) return;
    _syncPoll(s);
    if (s == SocketStatus.connected) {
      // Catch up on anything missed while disconnected.
      unawaited(refreshLatest());
    }
  }

  void _syncPoll(SocketStatus s) {
    if (s == SocketStatus.connected) {
      _poll?.cancel();
      _poll = null;
    } else {
      _poll ??= Timer.periodic(kPollInterval, (_) => unawaited(refreshLatest()));
    }
  }

  void _onResume() {
    if (_disposed) return;
    unawaited(_socket.ensureConnected());
    unawaited(refreshLatest());
  }

  // ---- merge helpers ---------------------------------------------------------

  static int _newestSentId(List<Message> list) {
    for (final m in list) {
      if (m.isSent) return m.id;
    }
    return 0;
  }

  static int _oldestSentId(List<Message> list) {
    for (final m in list.reversed) {
      if (m.isSent) return m.id;
    }
    return 0;
  }

  /// Sent messages by id (newest first), with unsent ones on top. A server
  /// message replaces the optimistic one with the same `clientMsgId`.
  static List<Message> _merge(List<Message> current, List<Message> incoming) {
    final byId = <int, Message>{};
    final unsent = <Message>[];
    final arrivedCids = {for (final m in incoming) if (m.clientMsgId != null) m.clientMsgId!};
    for (final m in current) {
      if (m.isSent) {
        byId[m.id] = m;
      } else if (!arrivedCids.contains(m.clientMsgId)) {
        unsent.add(m);
      }
    }
    for (final m in incoming) {
      if (m.isSent) byId[m.id] = m;
    }
    final sent = byId.values.toList()..sort((a, b) => b.id.compareTo(a.id));
    return [...unsent, ...sent];
  }
}

final threadControllerProvider =
    NotifierProvider.autoDispose.family<ThreadController, ThreadState, int>(ThreadController.new);

class _ResumeObserver with WidgetsBindingObserver {
  _ResumeObserver(this.onResume);

  final VoidCallback onResume;

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    if (state == AppLifecycleState.resumed) onResume();
  }
}
