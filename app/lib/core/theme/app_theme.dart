import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// Material 3 theme for Athlix. Change colours here only.
///
/// The app uses one dark look everywhere (see `app.dart`, `themeMode: dark`):
/// deep navy background, slightly lighter navy cards and fields, cyan accent.
/// Colours were taken from the approved login design.
class AppTheme {
  AppTheme._();

  // Brand palette.
  static const Color cyan = Color(0xFF00D4EC); // primary buttons, links, focus
  static const Color navy = Color(0xFF031727); // screen background
  static const Color navyDeep = Color(0xFF020F1C); // deepest layer
  static const Color teal = Color(0xFF002F3F); // glow / tinted containers
  static const Color card = Color(0xFF0A1424); // cards, sheets, nav bar
  static const Color field = Color(0xFF0D1D34); // text fields
  static const Color line = Color(0xFF303C50); // field borders, outlines
  static const Color lineSoft = Color(0xFF1A2638); // dividers, card borders
  static const Color text = Color(0xFFF2F6FA);
  static const Color textMuted = Color(0xFF9AA8B8);

  /// Kept for callers that still ask for a seed colour.
  static const Color seed = cyan;

  /// Same as [dark]: Athlix has a single dark look.
  static ThemeData light() => dark();

  static ThemeData dark() {
    final scheme = ColorScheme.fromSeed(seedColor: cyan, brightness: Brightness.dark).copyWith(
      primary: cyan,
      onPrimary: const Color(0xFF00242B),
      primaryContainer: teal,
      onPrimaryContainer: const Color(0xFF9EEFFB),
      secondary: const Color(0xFF5FD4E3),
      onSecondary: const Color(0xFF00242B),
      secondaryContainer: const Color(0xFF0F3442),
      onSecondaryContainer: const Color(0xFFBDF3FA),
      surface: navy,
      onSurface: text,
      onSurfaceVariant: textMuted,
      surfaceContainerLowest: navyDeep,
      surfaceContainerLow: card,
      surfaceContainer: card,
      surfaceContainerHigh: field,
      surfaceContainerHighest: const Color(0xFF14263F),
      outline: line,
      outlineVariant: lineSoft,
      surfaceTint: Colors.transparent,
    );
    final shape12 = RoundedRectangleBorder(borderRadius: BorderRadius.circular(12));
    OutlineInputBorder border(Color c, [double w = 1]) =>
        OutlineInputBorder(borderRadius: BorderRadius.circular(12), borderSide: BorderSide(color: c, width: w));

    return ThemeData(
      useMaterial3: true,
      brightness: Brightness.dark,
      colorScheme: scheme,
      scaffoldBackgroundColor: navy,
      canvasColor: navy,
      dividerTheme: const DividerThemeData(color: lineSoft, space: 1),
      appBarTheme: const AppBarTheme(
        backgroundColor: navy,
        foregroundColor: text,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        scrolledUnderElevation: 0,
        systemOverlayStyle: SystemUiOverlayStyle.light,
      ),
      cardTheme: CardThemeData(
        color: card,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(16),
          side: const BorderSide(color: lineSoft),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: field,
        hintStyle: const TextStyle(color: textMuted),
        border: border(line),
        enabledBorder: border(line),
        focusedBorder: border(cyan, 1.5),
        errorBorder: border(scheme.error),
        focusedErrorBorder: border(scheme.error, 1.5),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: cyan,
          foregroundColor: const Color(0xFF00242B),
          minimumSize: const Size(64, 52),
          shape: const StadiumBorder(),
          textStyle: const TextStyle(fontSize: 16, fontWeight: FontWeight.w600),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: cyan,
          foregroundColor: const Color(0xFF00242B),
          shape: const StadiumBorder(),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: cyan,
          side: const BorderSide(color: line),
          shape: const StadiumBorder(),
        ),
      ),
      textButtonTheme: TextButtonThemeData(style: TextButton.styleFrom(foregroundColor: cyan)),
      floatingActionButtonTheme: const FloatingActionButtonThemeData(
        backgroundColor: cyan,
        foregroundColor: Color(0xFF00242B),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: card,
        surfaceTintColor: Colors.transparent,
        indicatorColor: cyan.withValues(alpha: 0.16),
        iconTheme: WidgetStateProperty.resolveWith(
          (s) => IconThemeData(color: s.contains(WidgetState.selected) ? cyan : textMuted),
        ),
        labelTextStyle: WidgetStateProperty.resolveWith(
          (s) => TextStyle(
            fontSize: 12,
            color: s.contains(WidgetState.selected) ? cyan : textMuted,
            fontWeight: s.contains(WidgetState.selected) ? FontWeight.w600 : FontWeight.w400,
          ),
        ),
      ),
      tabBarTheme: const TabBarThemeData(
        labelColor: cyan,
        unselectedLabelColor: textMuted,
        indicatorColor: cyan,
        dividerColor: lineSoft,
      ),
      chipTheme: ChipThemeData(
        backgroundColor: card,
        side: const BorderSide(color: line),
        labelStyle: const TextStyle(color: text),
        shape: shape12,
      ),
      bottomSheetTheme: const BottomSheetThemeData(backgroundColor: card, surfaceTintColor: Colors.transparent),
      dialogTheme: const DialogThemeData(backgroundColor: card, surfaceTintColor: Colors.transparent),
      snackBarTheme: const SnackBarThemeData(
        backgroundColor: Color(0xFF14263F),
        contentTextStyle: TextStyle(color: text),
        actionTextColor: cyan,
      ),
      listTileTheme: const ListTileThemeData(
        contentPadding: EdgeInsets.symmetric(horizontal: 16),
        iconColor: textMuted,
      ),
      progressIndicatorTheme: const ProgressIndicatorThemeData(color: cyan),
    );
  }
}
