import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/application/session_controller.dart';
import '../../features/auth/presentation/change_password_dialog.dart';
import '../auth/idle_watcher.dart';
import '../config/env.dart';
import '../router/nav.dart';

/// Sidebar layout around every signed-in screen. The menu is filtered by the
/// admin's permissions; the idle timer signs out after inactivity.
class AdminShell extends ConsumerWidget {
  const AdminShell({super.key, required this.location, required this.child});

  final String location;
  final Widget child;

  static const _wide = 1000.0;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final session = ref.watch(sessionProvider);
    final items = visibleNavItems(session.permissions);
    final current = navItemFor(location);
    final wide = MediaQuery.sizeOf(context).width >= _wide;
    final sidebar = _Sidebar(items: items, current: current, closeOnTap: !wide);

    return IdleWatcher(
      timeout: const Duration(minutes: Env.idleTimeoutMinutes),
      onIdle: () => ref.read(sessionProvider).logout(notice: 'Signed out after ${Env.idleTimeoutMinutes} minutes without activity.'),
      child: Scaffold(
        appBar: AppBar(
          title: Text(current?.label ?? 'Athlix admin'),
          automaticallyImplyLeading: !wide,
          actions: [
            if (session.me != null)
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Center(child: Text('${session.me!.name} · ${session.me!.roleName}')),
              ),
            IconButton(
              tooltip: 'Change password',
              icon: const Icon(Icons.key_outlined),
              onPressed: () => showChangePasswordDialog(context),
            ),
            IconButton(
              tooltip: 'Sign out',
              icon: const Icon(Icons.logout),
              onPressed: () => ref.read(sessionProvider).logout(notice: 'You signed out.'),
            ),
          ],
        ),
        drawer: wide ? null : Drawer(child: sidebar),
        body: Row(children: [
          if (wide) SizedBox(width: 260, child: Material(elevation: 1, child: sidebar)),
          Expanded(child: child),
        ]),
      ),
    );
  }
}

class _Sidebar extends StatelessWidget {
  const _Sidebar({required this.items, required this.current, required this.closeOnTap});

  final List<NavItem> items;
  final NavItem? current;
  final bool closeOnTap;

  @override
  Widget build(BuildContext context) {
    final children = <Widget>[];
    String? section;
    for (final i in items) {
      if (i.section != section) {
        section = i.section;
        children.add(Padding(
          padding: const EdgeInsets.fromLTRB(16, 16, 16, 4),
          child: Text(section.toUpperCase(), style: Theme.of(context).textTheme.labelSmall),
        ));
      }
      children.add(ListTile(
        dense: true,
        leading: Icon(i.icon, size: 20),
        title: Text(i.label),
        selected: identical(i, current),
        onTap: () {
          if (closeOnTap) Navigator.of(context).pop();
          context.go(i.path);
        },
      ));
    }
    if (items.isEmpty) {
      children.add(const Padding(padding: EdgeInsets.all(16), child: Text('Your role has no screens yet.')));
    }
    return SafeArea(child: ListView(children: children));
  }
}
