import 'dart:typed_data';

import 'package:dio/dio.dart';

import '../../../core/api/api_error.dart';
import '../../../core/api/api_client.dart';
import '../../../core/api/json.dart';
import '../../../core/api/page.dart';

const adPlacements = ['home_feed', 'events_list', 'search', 'event_detail'];
const adStatuses = ['draft', 'active', 'paused', 'ended'];

class AdCampaign {
  const AdCampaign({
    required this.id,
    required this.advertiserName,
    required this.title,
    this.body,
    this.imageUploadId,
    this.imageUrl,
    required this.targetUrl,
    required this.placement,
    this.sportId,
    this.state,
    required this.priority,
    this.startsAt,
    this.endsAt,
    required this.status,
  });

  final int id;
  final String advertiserName;
  final String title;
  final String? body;
  final int? imageUploadId;
  final String? imageUrl;
  final String targetUrl;
  final String placement;
  final int? sportId;
  final String? state;
  final int priority;
  final DateTime? startsAt;
  final DateTime? endsAt;
  final String status;

  factory AdCampaign.fromJson(Json j) => AdCampaign(
        id: asInt(j['id']),
        advertiserName: asString(j['advertiserName']),
        title: asString(j['title']),
        body: asStringOrNull(j['body']),
        imageUploadId: asIntOrNull(j['imageUploadId']),
        imageUrl: asStringOrNull(j['imageUrl']),
        targetUrl: asString(j['targetUrl']),
        placement: asString(j['placement']),
        sportId: asIntOrNull(j['sportId']),
        state: asStringOrNull(j['state']),
        priority: asInt(j['priority']),
        startsAt: asDate(j['startsAt']),
        endsAt: asDate(j['endsAt']),
        status: asString(j['status']),
      );
}

/// Editable fields. `imageUploadId` comes from [AdsApi.uploadImage] (or is kept
/// from the saved campaign).
class AdCampaignInput {
  const AdCampaignInput({
    required this.advertiserName,
    required this.title,
    this.body,
    this.imageUploadId,
    required this.targetUrl,
    required this.placement,
    this.sportId,
    this.state,
    required this.priority,
    required this.startsAt,
    required this.endsAt,
    required this.status,
  });

  final String advertiserName;
  final String title;
  final String? body;
  final int? imageUploadId;
  final String targetUrl;
  final String placement;
  final int? sportId;
  final String? state;
  final int priority;
  final DateTime startsAt;
  final DateTime endsAt;
  final String status;

  Json toJson() => {
        'advertiserName': advertiserName,
        'title': title,
        'body': body,
        'imageUploadId': imageUploadId,
        'targetUrl': targetUrl,
        'placement': placement,
        'sportId': sportId,
        'state': state,
        'priority': priority,
        'startsAt': startsAt.toUtc().toIso8601String(),
        'endsAt': endsAt.toUtc().toIso8601String(),
        'status': status,
      };
}

class AdDayStats {
  const AdDayStats({required this.date, required this.impressions, required this.clicks});

  final String date;
  final int impressions;
  final int clicks;

  factory AdDayStats.fromJson(Json j) =>
      AdDayStats(date: asString(j['date']), impressions: asInt(j['impressions']), clicks: asInt(j['clicks']));
}

class AdsApi {
  AdsApi(this._client);

  final ApiClient _client;

  /// `GET /admin/ads` (ads.write).
  /// Uploads an ad image and returns its upload id for `imageUploadId`:
  /// `POST /admin/media/uploads` (ads.write, purpose `post_image`; the backend
  /// picks the S3 key) → PUT the bytes to the presigned URL with exactly the
  /// returned headers → `POST /admin/media/uploads/:id/confirm`.
  Future<int> uploadImage({required String contentType, required Uint8List bytes}) async {
    final t = asJson(await _client.post('/admin/media/uploads', body: {
      'purpose': 'post_image',
      'contentType': contentType,
      'sizeBytes': bytes.length,
    }));
    final id = asInt(t['uploadId']);
    final headers = <String, String>{};
    asJson(t['headers']).forEach((k, v) => headers[k] = asString(v));
    // S3 is not our API: a bare client, no bearer token, no JSON unwrapping.
    final s3 = Dio();
    try {
      await s3.request<void>(
        asString(t['uploadUrl']),
        data: Stream.fromIterable([bytes]),
        options: Options(method: asString(t['method'], 'PUT'), headers: {...headers, Headers.contentLengthHeader: bytes.length}),
      );
    } on DioException {
      throw const ApiError(code: 'UPLOAD_FAILED', message: 'The image could not be uploaded. Try again.');
    } finally {
      s3.close();
    }
    await _client.post('/admin/media/uploads/$id/confirm', body: const {});
    return id;
  }

  Future<PageResult<AdCampaign>> list({String? cursor}) => _client.page('/admin/ads', AdCampaign.fromJson, cursor: cursor);

  /// `POST /admin/ads` (ads.write).
  Future<AdCampaign> create(AdCampaignInput input) async =>
      AdCampaign.fromJson(asJson(await _client.post('/admin/ads', body: input.toJson())));

  /// `PUT /admin/ads/:id` (ads.write). Sends the full editable set.
  Future<AdCampaign> update(int id, AdCampaignInput input) async =>
      AdCampaign.fromJson(asJson(await _client.put('/admin/ads/$id', body: input.toJson())));

  /// `PUT /admin/ads/:id` with only `{status}`.
  Future<AdCampaign> setStatus(int id, String status) async =>
      AdCampaign.fromJson(asJson(await _client.put('/admin/ads/$id', body: {'status': status})));

  /// `GET /admin/ads/:id/stats?from&to` (dates as YYYY-MM-DD).
  Future<List<AdDayStats>> stats(int id, {required String from, required String to}) async =>
      asList(await _client.get('/admin/ads/$id/stats', query: {'from': from, 'to': to}), AdDayStats.fromJson);
}
