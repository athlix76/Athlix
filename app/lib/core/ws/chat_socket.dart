import 'dart:async';
import 'dart:convert';

import 'package:web_socket_channel/web_socket_channel.dart';

import '../json/json_utils.dart';
import 'backoff.dart';

/// `POST /chat/ws-ticket` → `{ticket, url, expiresAt}`. The ticket is single use.
class WsTicket {
  const WsTicket(
      {required this.ticket, required this.url, required this.expiresAt});

  final String ticket;
  final String url;
  final DateTime? expiresAt;

  factory WsTicket.fromJson(Map<String, dynamic> json) => WsTicket(
        ticket: asString(json['ticket']),
        url: asString(json['url']),
        expiresAt: asDateTimeOrNull(json['expiresAt']),
      );

  @override
  String toString() => 'WsTicket(<redacted>)';
}

/// A server → client frame: `{"type": "...", "data": ...}`.
class ServerFrame {
  const ServerFrame(this.type, this.data);

  final String type;
  final Object? data;

  Map<String, dynamic> get dataMap => asMap(data);
}

/// Frame types from docs/api.md "WebSocket protocol".
abstract final class WsType {
  static const ack = 'ack';
  static const messageNew = 'message.new';
  static const messageDeleted = 'message.deleted';
  static const typing = 'typing';
  static const read = 'read';
  static const error = 'error';
  static const pong = 'pong';
}

enum SocketStatus { disconnected, connecting, connected }

typedef TicketFetcher = Future<WsTicket> Function();
typedef ChannelConnector = WebSocketChannel Function(Uri uri);

/// Real-time chat connection.
///
/// * Gets a fresh ticket for every connect (tickets are single use).
/// * Sends `ping` every [pingInterval] (API Gateway closes idle sockets after 10 min).
/// * On close or error, reconnects with [Backoff] while [start] is in effect.
/// * Never logs frames: they contain message text.
class ChatSocket {
  ChatSocket({
    required TicketFetcher fetchTicket,
    required Uri apiBase,
    ChannelConnector? connector,
    Backoff? backoff,
    this.pingInterval = const Duration(minutes: 5),
  })  : _fetchTicket = fetchTicket,
        _apiBase = apiBase,
        _connector = connector ?? WebSocketChannel.connect,
        _backoff = backoff ?? Backoff();

  final TicketFetcher _fetchTicket;
  final Uri _apiBase;
  final ChannelConnector _connector;
  final Backoff _backoff;
  final Duration pingInterval;

  final StreamController<ServerFrame> _frames =
      StreamController<ServerFrame>.broadcast();
  final StreamController<SocketStatus> _statusCtrl =
      StreamController<SocketStatus>.broadcast();

  WebSocketChannel? _channel;
  // Cancelled in _teardown.
  // ignore: cancel_subscriptions
  StreamSubscription<dynamic>? _sub;
  Timer? _pingTimer;
  Timer? _retryTimer;
  bool _wanted = false;
  bool _connecting = false;
  SocketStatus _status = SocketStatus.disconnected;

  Stream<ServerFrame> get frames => _frames.stream;
  Stream<SocketStatus> get statusChanges => _statusCtrl.stream;
  SocketStatus get status => _status;
  bool get isConnected => _status == SocketStatus.connected;

  /// Keep a connection open until [stop].
  Future<void> start() async {
    _wanted = true;
    if (_status == SocketStatus.disconnected && !_connecting) await _connect();
  }

  /// Reconnect now if we should be connected (e.g. app resumed).
  Future<void> ensureConnected() async {
    if (!_wanted) return;
    if (_status == SocketStatus.disconnected && !_connecting) {
      _retryTimer?.cancel();
      _backoff.reset();
      await _connect();
    }
  }

  Future<void> stop() async {
    _wanted = false;
    _retryTimer?.cancel();
    await _teardown();
    _setStatus(SocketStatus.disconnected);
  }

  Future<void> dispose() async {
    await stop();
    await _frames.close();
    await _statusCtrl.close();
  }

  /// Sends a client frame; false when not connected (caller falls back to REST).
  bool send(String action, [Map<String, dynamic>? data]) {
    final ch = _channel;
    if (ch == null || _status != SocketStatus.connected) return false;
    try {
      ch.sink
          .add(jsonEncode({'action': action, if (data != null) 'data': data}));
      return true;
    } catch (_) {
      return false;
    }
  }

  bool sendMessage({
    required int conversationId,
    required String clientMsgId,
    required String kind,
    String? body,
    int? mediaUploadId,
    int? replyToId,
  }) =>
      send('sendMessage', {
        'conversationId': conversationId,
        'clientMsgId': clientMsgId,
        'kind': kind,
        'body': body,
        'mediaUploadId': mediaUploadId,
        'replyToId': replyToId,
      });

  bool typing(int conversationId) =>
      send('typing', {'conversationId': conversationId});

  bool markRead(int conversationId, int messageId) => send(
      'markRead', {'conversationId': conversationId, 'messageId': messageId});

  /// Builds `url?ticket=<ticket>`. A relative `url` is resolved against the
  /// API base, and http(s) becomes ws(s).
  Uri socketUri(WsTicket t) {
    var uri = Uri.parse(t.url);
    if (!uri.hasScheme) uri = _apiBase.resolveUri(uri);
    final scheme = switch (uri.scheme) {
      'http' => 'ws',
      'https' => 'wss',
      _ => uri.scheme,
    };
    return uri.replace(
      scheme: scheme,
      queryParameters: {...uri.queryParameters, 'ticket': t.ticket},
    );
  }

  Future<void> _connect() async {
    if (!_wanted || _connecting) return;
    _connecting = true;
    _setStatus(SocketStatus.connecting);
    try {
      final ticket = await _fetchTicket();
      if (!_wanted) return;
      final channel = _connector(socketUri(ticket));
      await channel.ready;
      if (!_wanted) {
        await channel.sink.close();
        return;
      }
      _channel = channel;
      _sub = channel.stream.listen(
        _onData,
        onError: (Object _) => _onClosed(),
        onDone: _onClosed,
        cancelOnError: true,
      );
      _backoff.reset();
      _setStatus(SocketStatus.connected);
      _pingTimer?.cancel();
      _pingTimer = Timer.periodic(pingInterval, (_) => send('ping'));
    } catch (_) {
      _scheduleReconnect();
    } finally {
      _connecting = false;
      if (_status == SocketStatus.connecting && !_wanted) {
        _setStatus(SocketStatus.disconnected);
      }
    }
  }

  void _onData(dynamic raw) {
    if (raw is! String) return;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is Map) {
        final type = asString(decoded['type']);
        if (type.isNotEmpty) _frames.add(ServerFrame(type, decoded['data']));
      }
    } catch (_) {
      // Ignore malformed frames.
    }
  }

  void _onClosed() {
    unawaited(_teardown());
    _scheduleReconnect();
  }

  void _scheduleReconnect() {
    _setStatus(SocketStatus.disconnected);
    if (!_wanted) return;
    _retryTimer?.cancel();
    _retryTimer = Timer(_backoff.next(), () => unawaited(_connect()));
  }

  Future<void> _teardown() async {
    _pingTimer?.cancel();
    _pingTimer = null;
    final sub = _sub;
    _sub = null;
    final ch = _channel;
    _channel = null;
    await sub?.cancel();
    try {
      await ch?.sink.close();
    } catch (_) {}
  }

  void _setStatus(SocketStatus s) {
    if (_status == s) return;
    _status = s;
    if (!_statusCtrl.isClosed) _statusCtrl.add(s);
  }
}
