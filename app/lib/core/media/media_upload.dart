import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../api/api_client.dart';
import '../api/api_exception.dart';
import '../config/app_config.dart';
import '../config/config_providers.dart';
import '../json/json_utils.dart';
import '../providers.dart';

/// Upload `purpose` values (docs/api.md "Media").
abstract final class MediaPurpose {
  static const profilePhoto = 'profile_photo';
  static const achievementCertificate = 'achievement_certificate';
  static const eventBanner = 'event_banner';
  static const chatImage = 'chat_image';
  static const postImage = 'post_image';
  static const communityAvatar = 'community_avatar';
  static const organiserDocument = 'organiser_document';

  /// Location identity proof (PDF/JPEG/PNG, document limits). Never shown back to users.
  static const locationProof = 'location_proof';

  static bool isDocument(String purpose) =>
      purpose == achievementCertificate || purpose == organiserDocument || purpose == locationProof;
}

/// `UploadTicket`: PUT the raw bytes to [uploadUrl] with exactly [headers].
class UploadTicket {
  const UploadTicket({
    required this.uploadId,
    required this.uploadUrl,
    required this.method,
    required this.headers,
    required this.expiresAt,
  });

  final int uploadId;
  final String uploadUrl;
  final String method;
  final Map<String, String> headers;
  final DateTime? expiresAt;

  factory UploadTicket.fromJson(Map<String, dynamic> json) => UploadTicket(
        uploadId: asInt(json['uploadId']),
        uploadUrl: asString(json['uploadUrl']),
        method: asString(json['method'], 'PUT'),
        headers: asMap(json['headers']).map((k, v) => MapEntry(k, '$v')),
        expiresAt: asDateTimeOrNull(json['expiresAt']),
      );
}

/// A picked file, ready to upload.
class UploadFile {
  const UploadFile({required this.bytes, required this.contentType, this.name});

  final Uint8List bytes;
  final String contentType;
  final String? name;
}

/// Guess the MIME type from a file name (image_picker does not always tell).
String contentTypeForName(String name, {String? reported}) {
  if (reported != null && reported.contains('/')) return reported;
  final lower = name.toLowerCase();
  if (lower.endsWith('.png')) return 'image/png';
  if (lower.endsWith('.webp')) return 'image/webp';
  if (lower.endsWith('.heic')) return 'image/heic';
  if (lower.endsWith('.pdf')) return 'application/pdf';
  return 'image/jpeg';
}

/// The three-step upload flow: ticket → PUT bytes → confirm. Returns the
/// `uploadId` to pass as `...UploadId`. The app never sees an S3 key.
class MediaUploader {
  MediaUploader(this._api, this._config, {Dio? rawDio}) : _raw = rawDio ?? Dio();

  final ApiClient _api;
  final AppConfig _config;

  /// Plain client for the presigned PUT: **no** auth interceptor, so our
  /// bearer token is never sent to the storage host.
  final Dio _raw;

  /// Client-side check against public config; null = OK. The server checks again.
  String? validate(String purpose, UploadFile file) {
    final isDoc = MediaPurpose.isDocument(purpose);
    final allowed = isDoc ? _config.allowedDocumentTypes : _config.allowedImageTypes;
    if (allowed != null && allowed.isNotEmpty && !allowed.contains(file.contentType)) {
      return 'This file type is not allowed.';
    }
    final maxMb = isDoc ? _config.documentMaxMb : _config.imageMaxMb;
    if (maxMb != null && file.bytes.length > maxMb * 1024 * 1024) {
      return 'File is too large (max $maxMb MB).';
    }
    return null;
  }

  Future<int> upload(String purpose, UploadFile file) async {
    final problem = validate(purpose, file);
    if (problem != null) {
      throw ApiException(code: ApiErrorCode.validationFailed, message: problem);
    }
    final ticket = await _api.post(
      '/media/uploads',
      (d) => UploadTicket.fromJson(asMap(d)),
      body: {'purpose': purpose, 'contentType': file.contentType, 'sizeBytes': file.bytes.length},
    );
    try {
      await _raw.request<void>(
        ticket.uploadUrl,
        data: Stream<List<int>>.fromIterable([file.bytes]),
        options: Options(
          method: ticket.method,
          headers: {...ticket.headers, Headers.contentLengthHeader: file.bytes.length},
          responseType: ResponseType.plain,
        ),
      );
    } on DioException {
      throw const ApiException(
        code: ApiErrorCode.upstreamError,
        message: 'Upload failed. Please try again.',
      );
    }
    await _api.postVoid('/media/uploads/${ticket.uploadId}/confirm');
    return ticket.uploadId;
  }
}

final mediaUploaderProvider = Provider<MediaUploader>(
  (ref) => MediaUploader(ref.watch(apiClientProvider), ref.watch(appConfigProvider)),
);
