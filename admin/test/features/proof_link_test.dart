import 'package:athlix_admin/features/location_review/application/proof_link.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final fetched = DateTime.utc(2026, 1, 1, 10);

  test('reads X-Amz-Expires and subtracts the margin', () {
    const url = 'https://bucket.s3.ap-south-1.amazonaws.com/k?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Expires=60&X-Amz-Signature=abc';
    expect(proofLinkExpiry(url, fetched), fetched.add(const Duration(seconds: 55)));
  });

  test('parameter name is case-insensitive', () {
    expect(proofLinkExpiry('https://h/k?x-amz-expires=120', fetched, margin: Duration.zero), fetched.add(const Duration(seconds: 120)));
  });

  test('never goes before the fetch time', () {
    expect(proofLinkExpiry('https://h/k?X-Amz-Expires=2', fetched), fetched);
  });

  test('null when the URL does not say', () {
    expect(proofLinkExpiry('https://h/k', fetched), isNull);
    expect(proofLinkExpiry('https://h/k?X-Amz-Expires=abc', fetched), isNull);
    expect(proofLinkExpiry('https://h/k?X-Amz-Expires=0', fetched), isNull);
  });
}
