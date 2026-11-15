import 'package:athlix_admin/core/auth/permissions.dart';
import 'package:athlix_admin/core/router/nav.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('hasPermission', () {
    test('grants an exact match', () {
      expect(hasPermission(['users.read'], Perm.usersRead), isTrue);
    });

    test('denies a missing permission', () {
      expect(hasPermission(['users.read'], Perm.usersWrite), isFalse);
      expect(hasPermission(const [], Perm.usersRead), isFalse);
    });

    test('* grants everything', () {
      for (final p in Perm.values) {
        expect(hasPermission([Perm.all], p), isTrue, reason: p);
      }
    });

    test('no prefix or wildcard matching beyond *', () {
      expect(hasPermission(['users'], Perm.usersRead), isFalse);
      expect(hasPermission(['users.*'], Perm.usersRead), isFalse);
      expect(hasPermission(['USERS.READ'], Perm.usersRead), isFalse);
    });
  });

  group('hasAnyPermission', () {
    test('empty requirement is always allowed', () {
      expect(hasAnyPermission(const [], const []), isTrue);
    });

    test('any one of the list is enough', () {
      expect(hasAnyPermission([Perm.moderationReadMessages], [Perm.reportsRead, Perm.moderationReadMessages]), isTrue);
      expect(hasAnyPermission([Perm.usersRead], [Perm.reportsRead, Perm.moderationReadMessages]), isFalse);
    });
  });

  group('sidebar gating', () {
    test('every nav item needs at least one real permission', () {
      for (final i in navItems) {
        expect(i.anyOf, isNotEmpty, reason: i.path);
        for (final p in i.anyOf) {
          expect(Perm.values, contains(p), reason: '${i.path} uses unknown permission $p');
        }
      }
    });

    test('nav paths are unique', () {
      final paths = navItems.map((i) => i.path).toList();
      expect(paths.toSet().length, paths.length);
    });

    test('* sees every item', () {
      expect(visibleNavItems([Perm.all]).length, navItems.length);
    });

    test('no permissions sees nothing and lands on /no-access', () {
      expect(visibleNavItems(const []), isEmpty);
      expect(homePathFor(const []), '/no-access');
    });

    test('a reviewer sees only their screens', () {
      final paths = visibleNavItems([Perm.locationReview]).map((i) => i.path).toList();
      expect(paths, ['/location-review']);
      expect(homePathFor([Perm.locationReview]), '/location-review');
    });

    test('districts screen needs config.read, not geo.write alone', () {
      expect(visibleNavItems([Perm.geoWrite]).map((i) => i.path), isNot(contains('/districts')));
      expect(visibleNavItems([Perm.configRead]).map((i) => i.path), contains('/districts'));
    });

    test('moderation is visible with either reports.read or moderation.read_messages', () {
      expect(visibleNavItems([Perm.reportsRead]).map((i) => i.path), contains('/moderation'));
      expect(visibleNavItems([Perm.moderationReadMessages]).map((i) => i.path), contains('/moderation'));
    });

    test('dashboard comes first when allowed', () {
      expect(homePathFor([Perm.usersRead, Perm.dashboardRead]), '/dashboard');
      expect(homePathFor([Perm.usersRead]), '/users');
    });
  });

  group('navItemFor', () {
    test('matches a detail path to its list item', () {
      expect(navItemFor('/users/42')?.path, '/users');
      expect(navItemFor('/location-review/7')?.path, '/location-review');
    });

    test('does not match a different screen that shares a prefix', () {
      // '/organiser-steps' must not be owned by '/organisers'.
      expect(navItemFor('/organiser-steps')?.path, '/organiser-steps');
      expect(navItemFor('/unknown'), isNull);
    });
  });
}
