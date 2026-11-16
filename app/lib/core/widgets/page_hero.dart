import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../theme/app_theme.dart';

/// The navy header band of the main tabs: a small cyan [eyebrow], a large [title] and an
/// optional [subtitle], with an optional [trailing] action (for example an edit button).
/// [bottom] sits inside the band below the text (for example a "Host an event" button).
///
/// Use it as the first child of a tab body instead of an `AppBar`; it pads for the status
/// bar itself and keeps the status bar icons light.
class PageHero extends StatelessWidget {
  const PageHero({
    super.key,
    required this.title,
    this.eyebrow,
    this.subtitle,
    this.trailing,
    this.leading,
    this.bottom,
  });

  final String title;
  final String? eyebrow;
  final String? subtitle;
  final Widget? trailing;
  final Widget? leading;
  final Widget? bottom;

  @override
  Widget build(BuildContext context) {
    final top = MediaQuery.paddingOf(context).top;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      value: SystemUiOverlayStyle.light,
      child: Material(
        color: Colors.transparent,
        child: Padding(
          padding: EdgeInsets.fromLTRB(leading == null ? 22 : 8, top + 20, 14, 22),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              Row(
                crossAxisAlignment: CrossAxisAlignment.center,
                children: [
                  if (leading != null) leading!,
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        if (eyebrow != null) ...[
                          Text(eyebrow!.toUpperCase(), style: eyebrowStyle),
                          const SizedBox(height: 8),
                        ],
                        Text(
                          title,
                          style: const TextStyle(
                            fontFamily: AppTheme.headingFont,
                            color: AppTheme.white,
                            fontSize: 26,
                            height: 1.2,
                            fontWeight: FontWeight.w800,
                            letterSpacing: -0.9,
                          ),
                        ),
                        if (subtitle != null) ...[
                          const SizedBox(height: 8),
                          Text(subtitle!, style: const TextStyle(color: AppTheme.heroMuted, fontSize: 13)),
                        ],
                      ],
                    ),
                  ),
                  if (trailing != null) trailing!,
                ],
              ),
              if (bottom != null) ...[const SizedBox(height: 18), bottom!],
            ],
          ),
        ),
      ),
    );
  }

  /// Small spaced capitals in cyan, as above every hero title.
  static const eyebrowStyle = TextStyle(
    fontFamily: AppTheme.headingFont,
    color: AppTheme.cyan,
    fontSize: 11,
    fontWeight: FontWeight.w700,
    letterSpacing: 1.1,
  );
}

/// Square glass-style icon button for the navy band (edit, chat, ...).
class HeroIconButton extends StatelessWidget {
  const HeroIconButton({super.key, required this.icon, required this.tooltip, required this.onPressed});

  final Widget icon;
  final String tooltip;
  final VoidCallback onPressed;

  @override
  Widget build(BuildContext context) => IconButton(
        tooltip: tooltip,
        onPressed: onPressed,
        icon: icon,
        color: AppTheme.white,
        style: IconButton.styleFrom(
          backgroundColor: AppTheme.surface,
          side: const BorderSide(color: AppTheme.softBlueLine),
          shape: const CircleBorder(),
          fixedSize: const Size(48, 48),
        ),
      );
}

/// Two-option pill toggle ("Following | Latest"): dark track with a thin border, the
/// selected option on the cyan gradient.
class PillToggle extends StatelessWidget {
  const PillToggle({super.key, required this.labels, required this.selected, required this.onSelected});

  final List<String> labels;
  final int selected;
  final ValueChanged<int> onSelected;

  @override
  Widget build(BuildContext context) => Container(
        height: 56,
        padding: const EdgeInsets.all(4),
        decoration: BoxDecoration(
          color: AppTheme.surface,
          borderRadius: BorderRadius.circular(28),
          border: Border.all(color: AppTheme.softBlueLine),
        ),
        child: Row(
          children: [
            for (var i = 0; i < labels.length; i++)
              Expanded(
                child: Semantics(
                  selected: i == selected,
                  button: true,
                  child: InkWell(
                    borderRadius: BorderRadius.circular(24),
                    onTap: () => onSelected(i),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 180),
                      alignment: Alignment.center,
                      decoration: BoxDecoration(
                        gradient: i == selected ? AppTheme.cyanGradient : null,
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: Text(
                        labels[i],
                        style: TextStyle(
                          fontSize: 17,
                          color: i == selected ? AppTheme.navy : AppTheme.heroMuted,
                          fontWeight: i == selected ? FontWeight.w600 : FontWeight.w500,
                        ),
                      ),
                    ),
                  ),
                ),
              ),
          ],
        ),
      );
}

/// Round outlined icon button of the dark look (header chat, post actions).
class RoundIconButton extends StatelessWidget {
  const RoundIconButton({super.key, required this.icon, required this.tooltip, required this.onPressed, this.size = 52});

  final Widget icon;
  final String tooltip;
  final VoidCallback? onPressed;
  final double size;

  @override
  Widget build(BuildContext context) => IconButton(
        tooltip: tooltip,
        onPressed: onPressed,
        icon: icon,
        color: AppTheme.white,
        style: IconButton.styleFrom(
          backgroundColor: AppTheme.surface,
          side: const BorderSide(color: AppTheme.softBlueLine),
          fixedSize: Size(size, size),
          shape: const CircleBorder(),
        ),
      );
}

/// Cyan gradient rounded-square action button with a glow (Home "new post").
class GlowFab extends StatelessWidget {
  const GlowFab({super.key, required this.tooltip, required this.onPressed, this.icon = Icons.add});

  final String tooltip;
  final VoidCallback onPressed;
  final IconData icon;

  @override
  Widget build(BuildContext context) => Tooltip(
        message: tooltip,
        child: Container(
          width: 68,
          height: 68,
          decoration: BoxDecoration(
            gradient: AppTheme.cyanGradient,
            borderRadius: BorderRadius.circular(22),
            boxShadow: const [BoxShadow(color: Color(0x6605CDE0), blurRadius: 24, offset: Offset(0, 6))],
          ),
          child: Material(
            color: Colors.transparent,
            child: InkWell(
              borderRadius: BorderRadius.circular(22),
              onTap: onPressed,
              child: Icon(icon, size: 34, color: AppTheme.navy),
            ),
          ),
        ),
      );
}

/// Horizontal row of filter pills (white, the selected one cyan).
class PillChips extends StatelessWidget {
  const PillChips({super.key, required this.labels, required this.selected, required this.onSelected});

  final List<String> labels;
  final int selected;
  final ValueChanged<int> onSelected;

  @override
  Widget build(BuildContext context) => SingleChildScrollView(
        scrollDirection: Axis.horizontal,
        padding: const EdgeInsets.symmetric(horizontal: 16),
        child: Row(
          children: [
            for (var i = 0; i < labels.length; i++)
              Padding(
                padding: const EdgeInsets.only(right: 8),
                child: ChoiceChip(
                  label: Text(labels[i]),
                  selected: i == selected,
                  showCheckmark: false,
                  onSelected: (_) => onSelected(i),
                ),
              ),
          ],
        ),
      );
}

/// Input decoration of the template's search fields (pale blue fill, search icon).
InputDecoration searchFieldDecoration(String hint, {Widget? suffix}) => InputDecoration(
      hintText: hint,
      prefixIcon: const Icon(Icons.search),
      suffixIcon: suffix,
      fillColor: AppTheme.searchFill,
      isDense: true,
      contentPadding: const EdgeInsets.symmetric(vertical: 14),
      border: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: AppTheme.line)),
      enabledBorder:
          OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: const BorderSide(color: AppTheme.line)),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(14),
        borderSide: const BorderSide(color: AppTheme.blue, width: 1.5),
      ),
    );

/// White button on the navy band ("Host an event").
class HeroButton extends StatelessWidget {
  const HeroButton({super.key, required this.icon, required this.label, required this.onPressed, this.primary = false});

  final IconData icon;
  final String label;
  final VoidCallback onPressed;

  /// Cyan instead of pale blue.
  final bool primary;

  @override
  Widget build(BuildContext context) => FilledButton.icon(
        onPressed: onPressed,
        icon: Icon(icon, size: 20),
        label: Text(label),
        style: FilledButton.styleFrom(
          backgroundColor: primary ? AppTheme.cyan : AppTheme.softBlue,
          foregroundColor: primary ? AppTheme.navy : AppTheme.text,
          side: primary ? null : const BorderSide(color: AppTheme.softBlueLine),
          minimumSize: const Size(0, 46),
          padding: const EdgeInsets.symmetric(horizontal: 16),
          textStyle: const TextStyle(fontFamily: AppTheme.bodyFont, fontWeight: FontWeight.w800, fontSize: 14),
        ),
      );
}
