import 'package:flutter/material.dart';

/// The Athlix glass icon (assets/brand/athlix_icon.png, generated from app/tool/brand).
class AthlixLogo extends StatelessWidget {
  const AthlixLogo({super.key, this.size = 72});

  final double size;

  @override
  Widget build(BuildContext context) => Image.asset(
        'assets/brand/athlix_icon.png',
        width: size,
        height: size,
        filterQuality: FilterQuality.medium,
        semanticLabel: 'Athlix',
      );
}

/// The Athlix mark with the "ATHLIX" word under it, for the navy header band
/// (assets/brand/athlix_wordmark.png, from the UI template with its dark background made
/// transparent).
/// The image is cropped to its centre like the template's `.brand-logo`.
class AthlixWordmark extends StatelessWidget {
  const AthlixWordmark({super.key, this.height = 80});

  final double height;

  @override
  Widget build(BuildContext context) {
    // The template shows a 136x136 image in a 136x104 box; the box is a little narrower here
    // so the source image's edge never shows.
    final side = height * 136 / 104;
    return SizedBox(
      width: height * 1.15,
      height: height,
      child: ClipRect(
        child: OverflowBox(
          minWidth: side,
          minHeight: side,
          maxWidth: side,
          maxHeight: side,
          child: Image.asset(
            'assets/brand/athlix_wordmark.png',
            width: side,
            height: side,
            filterQuality: FilterQuality.medium,
            semanticLabel: 'Athlix',
          ),
        ),
      ),
    );
  }
}

/// The mark alone (assets/brand/athlix_mark.png, transparent) followed by "ATHLIX", as in the
/// Home header.
class AthlixMarkTitle extends StatelessWidget {
  const AthlixMarkTitle({super.key, this.size = 40});

  final double size;

  @override
  Widget build(BuildContext context) => Semantics(
        label: 'Athlix',
        excludeSemantics: true,
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Image.asset('assets/brand/athlix_mark.png', width: size, height: size, filterQuality: FilterQuality.medium),
            const SizedBox(width: 12),
            Text(
              'ATHLIX',
              style: TextStyle(
                color: Colors.white,
                fontSize: size * 0.68,
                fontWeight: FontWeight.w500,
                letterSpacing: 1.2,
              ),
            ),
          ],
        ),
      );
}
