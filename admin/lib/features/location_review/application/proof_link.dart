/// When a signed proof URL stops working, measured from when the detail was
/// fetched (so the admin's clock skew does not matter).
///
/// Reads the lifetime from the S3 presign query (`X-Amz-Expires`, seconds)
/// and subtracts [margin]. Returns null when the URL does not say; the screen
/// then relies on the image error handler and the "Show again" button.
DateTime? proofLinkExpiry(String url, DateTime fetchedAt, {Duration margin = const Duration(seconds: 5)}) {
  final uri = Uri.tryParse(url);
  if (uri == null) return null;
  String? raw;
  uri.queryParameters.forEach((k, v) {
    if (k.toLowerCase() == 'x-amz-expires') raw = v;
  });
  final seconds = int.tryParse(raw ?? '');
  if (seconds == null || seconds <= 0) return null;
  final life = Duration(seconds: seconds) - margin;
  return fetchedAt.add(life.isNegative ? Duration.zero : life);
}
