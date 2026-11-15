import 'package:flutter/material.dart';

import '../auth/permissions.dart';

/// One sidebar entry. Shown when the admin has any of [anyOf] (or `*`).
class NavItem {
  const NavItem({required this.path, required this.label, required this.icon, required this.anyOf, required this.section});

  final String path;
  final String label;
  final IconData icon;
  final List<String> anyOf;
  final String section;
}

/// Every screen in the sidebar, in order. Route guards use the same list, so a
/// hidden item is also unreachable by typing its URL.
const List<NavItem> navItems = [
  NavItem(path: '/dashboard', label: 'Dashboard', icon: Icons.dashboard_outlined, anyOf: [Perm.dashboardRead], section: 'Overview'),
  NavItem(path: '/users', label: 'Users', icon: Icons.people_outline, anyOf: [Perm.usersRead], section: 'People'),
  NavItem(path: '/organisers', label: 'Organiser applications', icon: Icons.verified_user_outlined, anyOf: [Perm.organisersReview], section: 'People'),
  NavItem(path: '/location-review', label: 'Location review', icon: Icons.place_outlined, anyOf: [Perm.locationReview], section: 'People'),
  NavItem(path: '/admins', label: 'Admins and roles', icon: Icons.admin_panel_settings_outlined, anyOf: [Perm.adminsManage], section: 'People'),
  NavItem(path: '/events', label: 'Events review', icon: Icons.emoji_events_outlined, anyOf: [Perm.eventsReview], section: 'Content'),
  NavItem(path: '/reports', label: 'Reports', icon: Icons.flag_outlined, anyOf: [Perm.reportsRead], section: 'Content'),
  NavItem(path: '/moderation', label: 'Moderation', icon: Icons.gavel_outlined, anyOf: [Perm.reportsRead, Perm.moderationReadMessages], section: 'Content'),
  NavItem(path: '/communities', label: 'Communities', icon: Icons.groups_outlined, anyOf: [Perm.communitiesModerate], section: 'Content'),
  NavItem(path: '/ads', label: 'Ads', icon: Icons.campaign_outlined, anyOf: [Perm.adsWrite], section: 'Content'),
  NavItem(path: '/broadcast', label: 'Broadcast', icon: Icons.notifications_active_outlined, anyOf: [Perm.notificationsBroadcast], section: 'Content'),
  NavItem(path: '/listing-orders', label: 'Listing payments', icon: Icons.receipt_long_outlined, anyOf: [Perm.paymentsRead], section: 'Money'),
  NavItem(path: '/fee-rules', label: 'Fee rules', icon: Icons.percent, anyOf: [Perm.configRead], section: 'Money'),
  NavItem(path: '/plans', label: 'Premium plans', icon: Icons.workspace_premium_outlined, anyOf: [Perm.configRead], section: 'Money'),
  NavItem(path: '/subscriptions', label: 'Subscriptions', icon: Icons.card_membership_outlined, anyOf: [Perm.paymentsRead], section: 'Money'),
  NavItem(path: '/config', label: 'Config', icon: Icons.tune, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/policies', label: 'Policies', icon: Icons.policy_outlined, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/verification-requirements', label: 'Verification requirements', icon: Icons.rule, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/organiser-steps', label: 'Organiser steps', icon: Icons.checklist, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/sports', label: 'Sports and stats', icon: Icons.sports_cricket_outlined, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/districts', label: 'Districts', icon: Icons.map_outlined, anyOf: [Perm.configRead], section: 'Configuration'),
  NavItem(path: '/audit-log', label: 'Audit log', icon: Icons.history, anyOf: [Perm.auditRead], section: 'Audit'),
  NavItem(path: '/login-events', label: 'Admin sign-ins', icon: Icons.login, anyOf: [Perm.auditRead], section: 'Audit'),
];

/// Items the admin may see.
List<NavItem> visibleNavItems(Iterable<String> granted) =>
    navItems.where((i) => hasAnyPermission(granted, i.anyOf)).toList();

/// The nav item that owns [location] (`/users/42` → Users), or null.
NavItem? navItemFor(String location) {
  for (final i in navItems) {
    if (location == i.path || location.startsWith('${i.path}/')) return i;
  }
  return null;
}

/// Where to land after sign-in: the first visible item, or `/no-access`.
String homePathFor(Iterable<String> granted) {
  final v = visibleNavItems(granted);
  return v.isEmpty ? '/no-access' : v.first.path;
}
