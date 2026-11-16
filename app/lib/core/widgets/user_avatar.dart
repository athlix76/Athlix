import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';

import '../theme/app_theme.dart';

/// Round avatar from a signed photo URL, with initials as the fallback.
class UserAvatar extends StatelessWidget {
  const UserAvatar({super.key, required this.name, this.url, this.radius = 20});

  final String name;
  final String? url;
  final double radius;

  String get _initials {
    final parts = name.trim().split(RegExp(r'\s+')).where((p) => p.isNotEmpty).toList();
    if (parts.isEmpty) return '?';
    final first = parts.first[0];
    final last = parts.length > 1 ? parts.last[0] : '';
    return (first + last).toUpperCase();
  }

  @override
  Widget build(BuildContext context) {
    final u = url;
    return CircleAvatar(
      radius: radius,
      backgroundColor: AppTheme.avatarBg,
      foregroundColor: AppTheme.avatarFg,
      backgroundImage: u == null || u.isEmpty ? null : CachedNetworkImageProvider(u),
      child: u == null || u.isEmpty
          ? Text(
              _initials,
              style: TextStyle(
                fontFamily: AppTheme.headingFont,
                fontWeight: FontWeight.w800,
                fontSize: radius * 0.62,
                color: AppTheme.avatarFg,
              ),
            )
          : null,
    );
  }
}

/// Network image for signed URLs (banners, chat and post images).
class SignedImage extends StatelessWidget {
  const SignedImage({super.key, required this.url, this.height, this.fit = BoxFit.cover});

  final String url;
  final double? height;
  final BoxFit fit;

  @override
  Widget build(BuildContext context) {
    return CachedNetworkImage(
      imageUrl: url,
      height: height,
      width: double.infinity,
      fit: fit,
      placeholder: (_, __) => SizedBox(height: height ?? 120),
      errorWidget: (_, __, ___) => SizedBox(
        height: height ?? 120,
        child: const Center(child: Icon(Icons.broken_image_outlined)),
      ),
    );
  }
}
