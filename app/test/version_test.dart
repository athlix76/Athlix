import 'package:athlix/core/config/version.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('compareVersions is numeric per part', () {
    expect(compareVersions('1.2.10', '1.2.9'), greaterThan(0));
    expect(compareVersions('1.2', '1.2.0'), 0);
    expect(compareVersions('1.0.0+5', '1.0.0'), 0);
    expect(compareVersions('0.9.9', '1.0.0'), lessThan(0));
  });

  test('needsForceUpdate', () {
    expect(needsForceUpdate('1.0.0', '1.1.0'), isTrue);
    expect(needsForceUpdate('1.1.0', '1.1.0'), isFalse);
    expect(needsForceUpdate('1.1.0', null), isFalse);
    expect(needsForceUpdate('1.1.0', ' '), isFalse);
  });
}
