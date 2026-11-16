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
