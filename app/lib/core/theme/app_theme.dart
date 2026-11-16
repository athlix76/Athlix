import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// Material 3 theme for Athlix. Change colours here only.
///
/// One dark look (approved home screenshot): deep navy background with a soft cyan glow,
/// dark cards with a faint border, cyan gradient for the selected tab / primary action,
/// round outlined icon buttons and a floating pill bottom bar. The navy header band of the
/// tabs ([PageHero]) blends into the background. Headings use Montserrat, body text Manrope
/// (bundled in assets/fonts).
class AppTheme {
  AppTheme._();

  // Palette. Names describe the role, values are the dark look.
  static const Color navy = Color(0xFF070C1A); // page background, text on cyan
  static const Color ice = navy; // kept for older callers: the page background
  static const Color surface = Color(0xFF0D1626); // cards, sheets, bottom bar
  static const Color white = Color(0xFFFFFFFF); // text and icons on dark
  static const Color text = Color(0xFFF3F9FC); // body text
  static const Color cyan = Color(0xFF05CDE0); // primary, selected, accents
  static const Color blue = Color(0xFF19B9F3); // gradient end, focus
  static const Color slate = Color(0xFF8C9BB0); // secondary text, idle icons
  static const Color line = Color(0xFF1E2A3E); // borders, dividers
  static const Color searchFill = Color(0xFF111C2E); // fields, pill tracks
  static const Color softBlue = Color(0xFF111C2E); // secondary buttons
  static const Color softBlueLine = Color(0xFF26354C);
  static const Color link = Color(0xFF22D3EE); // text links
  static const Color avatarBg = Color(0xFF0B2230);
  static const Color avatarFg = cyan;
  static const Color heroMuted = Color(0xFFB9CEDB); // subtitle on the header band
  static const Color red = Color(0xFFF87171);
  static const Color green = Color(0xFF4ADE80);

  /// Selected tab / primary action fill.
  static const LinearGradient cyanGradient = LinearGradient(colors: [cyan, blue]);

  /// Soft cyan glows behind the main tabs (top right and bottom), painted over [navy].
  static const RadialGradient glowTop = RadialGradient(
    center: Alignment(1.15, -1.05),
    radius: 0.85,
    colors: [Color(0x2E05CDE0), Color(0x0005CDE0)],
  );
  static const RadialGradient glowBottom = RadialGradient(
    center: Alignment(0, 1.25),
    radius: 0.7,
    colors: [Color(0x2205CDE0), Color(0x0005CDE0)],
  );

  static const String bodyFont = 'Manrope';
  static const String headingFont = 'Montserrat';

  /// Kept for callers that still ask for a seed colour.
  static const Color seed = cyan;

  /// Same as [dark]: Athlix has a single look.
  static ThemeData light() => dark();

  static ThemeData dark() {
    final scheme = ColorScheme.fromSeed(seedColor: cyan, brightness: Brightness.dark).copyWith(
      primary: cyan,
      onPrimary: navy,
      primaryContainer: avatarBg,
      onPrimaryContainer: cyan,
      secondary: link,
      onSecondary: navy,
      secondaryContainer: softBlue,
      onSecondaryContainer: text,
      tertiary: blue,
      surface: navy,
      onSurface: text,
      onSurfaceVariant: slate,
      surfaceContainerLowest: navy,
      surfaceContainerLow: surface,
      surfaceContainer: surface,
      surfaceContainerHigh: searchFill,
      surfaceContainerHighest: const Color(0xFF16233A),
      outline: softBlueLine,
      outlineVariant: line,
      error: red,
      onError: navy,
      errorContainer: const Color(0xFF3B1219),
      onErrorContainer: const Color(0xFFFECACA),
      surfaceTint: Colors.transparent,
    );
    final r12 = BorderRadius.circular(12);
    OutlineInputBorder border(Color c, [double w = 1]) =>
        OutlineInputBorder(borderRadius: r12, borderSide: BorderSide(color: c, width: w));
    const buttonText = TextStyle(fontFamily: bodyFont, fontSize: 15, fontWeight: FontWeight.w800);

    final base = ThemeData(useMaterial3: true, brightness: Brightness.dark, fontFamily: bodyFont);
    TextStyle? heading(TextStyle? s, double size) => s?.copyWith(
          fontFamily: headingFont,
          fontWeight: FontWeight.w800,
          fontSize: size,
          letterSpacing: -0.035 * size,
          color: text,
        );
    TextStyle? title(TextStyle? s) =>
        s?.copyWith(fontFamily: headingFont, fontWeight: FontWeight.w700, color: text, letterSpacing: -0.2);
    final t = base.textTheme;
    final textTheme = t
        .apply(bodyColor: text, displayColor: text, fontFamily: bodyFont)
        .copyWith(
          displaySmall: heading(t.displaySmall, 34),
          headlineLarge: heading(t.headlineLarge, 30),
          headlineMedium: heading(t.headlineMedium, 27),
          headlineSmall: heading(t.headlineSmall, 23),
          titleLarge: title(t.titleLarge)?.copyWith(fontSize: 20, fontWeight: FontWeight.w800),
          titleMedium: title(t.titleMedium),
          titleSmall: title(t.titleSmall),
          bodySmall: t.bodySmall?.copyWith(fontFamily: bodyFont, color: slate),
          labelLarge: t.labelLarge?.copyWith(fontFamily: bodyFont, fontWeight: FontWeight.w700),
        );

    return base.copyWith(
      colorScheme: scheme,
      textTheme: textTheme,
      scaffoldBackgroundColor: navy,
      canvasColor: navy,
      dividerTheme: const DividerThemeData(color: line, space: 1, thickness: 1),
      // Every app bar is the navy band of the template.
      appBarTheme: const AppBarTheme(
        backgroundColor: navy,
        foregroundColor: white,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        scrolledUnderElevation: 0,
        centerTitle: false,
        systemOverlayStyle: SystemUiOverlayStyle.light,
        titleTextStyle: TextStyle(
          fontFamily: headingFont,
          fontSize: 20,
          fontWeight: FontWeight.w800,
          letterSpacing: -0.6,
          color: white,
        ),
        iconTheme: IconThemeData(color: white),
        actionsIconTheme: IconThemeData(color: white),
      ),
      cardTheme: CardThemeData(
        color: surface,
        surfaceTintColor: Colors.transparent,
        elevation: 0,
        margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(18),
          side: const BorderSide(color: line),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: searchFill,
        hintStyle: const TextStyle(color: slate, fontWeight: FontWeight.w500),
        labelStyle: const TextStyle(color: slate, fontWeight: FontWeight.w600),
        floatingLabelStyle: const TextStyle(color: link, fontWeight: FontWeight.w700),
        prefixIconColor: slate,
        suffixIconColor: slate,
        contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 15),
        border: border(line),
        enabledBorder: border(line),
        disabledBorder: border(line),
        focusedBorder: border(cyan, 1.5),
        errorBorder: border(red),
        focusedErrorBorder: border(red, 1.5),
      ),
      filledButtonTheme: FilledButtonThemeData(
        style: FilledButton.styleFrom(
          backgroundColor: cyan,
          foregroundColor: navy,
          disabledBackgroundColor: cyan.withValues(alpha: 0.4),
          disabledForegroundColor: navy.withValues(alpha: 0.5),
          minimumSize: const Size(64, 52),
          shape: RoundedRectangleBorder(borderRadius: r12),
          textStyle: buttonText,
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: cyan,
          foregroundColor: navy,
          elevation: 0,
          shape: RoundedRectangleBorder(borderRadius: r12),
          textStyle: buttonText,
        ),
      ),
      // Secondary button of the template: pale blue with a light-blue border.
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          backgroundColor: softBlue,
          foregroundColor: text,
          side: const BorderSide(color: softBlueLine),
          minimumSize: const Size(48, 46),
          shape: RoundedRectangleBorder(borderRadius: r12),
          textStyle: buttonText.copyWith(fontSize: 14),
        ),
      ),
      textButtonTheme: TextButtonThemeData(
        style: TextButton.styleFrom(
          foregroundColor: link,
          textStyle: const TextStyle(fontFamily: bodyFont, fontWeight: FontWeight.w800),
        ),
      ),
      floatingActionButtonTheme: FloatingActionButtonThemeData(
        backgroundColor: cyan,
        foregroundColor: navy,
        elevation: 2,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(18)),
      ),
      tabBarTheme: const TabBarThemeData(
        labelColor: text,
        unselectedLabelColor: slate,
        indicatorColor: cyan,
        indicatorSize: TabBarIndicatorSize.tab,
        dividerColor: line,
        labelStyle: TextStyle(fontFamily: bodyFont, fontWeight: FontWeight.w800, fontSize: 14),
        unselectedLabelStyle: TextStyle(fontFamily: bodyFont, fontWeight: FontWeight.w700, fontSize: 14),
        indicator: UnderlineTabIndicator(borderSide: BorderSide(color: cyan, width: 3)),
      ),
      chipTheme: const ChipThemeData(
        backgroundColor: searchFill,
        selectedColor: cyan,
        checkmarkColor: navy,
        side: BorderSide(color: line),
        labelStyle: TextStyle(fontFamily: bodyFont, color: text, fontWeight: FontWeight.w600),
        secondaryLabelStyle: TextStyle(fontFamily: bodyFont, color: navy, fontWeight: FontWeight.w800),
        shape: StadiumBorder(),
        padding: EdgeInsets.symmetric(horizontal: 6, vertical: 4),
      ),
      segmentedButtonTheme: SegmentedButtonThemeData(
        style: ButtonStyle(
          backgroundColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? cyan : searchFill),
          foregroundColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? navy : text),
          side: const WidgetStatePropertyAll(BorderSide(color: line)),
        ),
      ),
      checkboxTheme: CheckboxThemeData(
        fillColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? cyan : null),
        checkColor: const WidgetStatePropertyAll(navy),
      ),
      switchTheme: SwitchThemeData(
        trackColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? cyan : null),
        thumbColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? white : null),
      ),
      badgeTheme: const BadgeThemeData(
        backgroundColor: cyan,
        textColor: navy,
        textStyle: TextStyle(fontFamily: bodyFont, fontSize: 10, fontWeight: FontWeight.w800),
      ),
      bottomSheetTheme: const BottomSheetThemeData(
        backgroundColor: surface,
        surfaceTintColor: Colors.transparent,
        showDragHandle: true,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(24))),
      ),
      dialogTheme: DialogThemeData(
        backgroundColor: surface,
        surfaceTintColor: Colors.transparent,
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(20)),
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: const Color(0xFF16233A),
        contentTextStyle: const TextStyle(fontFamily: bodyFont, color: white, fontWeight: FontWeight.w600),
        actionTextColor: cyan,
        behavior: SnackBarBehavior.floating,
        shape: RoundedRectangleBorder(borderRadius: r12),
      ),
      listTileTheme: const ListTileThemeData(
        contentPadding: EdgeInsets.symmetric(horizontal: 16),
        iconColor: slate,
        titleTextStyle: TextStyle(fontFamily: bodyFont, color: text, fontSize: 15, fontWeight: FontWeight.w700),
        subtitleTextStyle: TextStyle(fontFamily: bodyFont, color: slate, fontSize: 13),
      ),
      progressIndicatorTheme: const ProgressIndicatorThemeData(color: cyan),
      textSelectionTheme: const TextSelectionThemeData(cursorColor: cyan, selectionHandleColor: cyan),
      popupMenuTheme: const PopupMenuThemeData(color: Color(0xFF16233A), surfaceTintColor: Colors.transparent),
      iconTheme: const IconThemeData(color: text),
    );
  }
}
