import 'package:flutter/material.dart';

const kBackground = Color(0xFF0B0E14);
const kSurface = Color(0xFF10141C);
const kSurfaceHi = Color(0xFF171D28);
const kPrimary = Color(0xFF7AD7FF);
const kSecondary = Color(0xFFE8C547);
const kOk = Color(0xFF4ADE80);
const kWarn = Color(0xFFF2B441);
const kDanger = Color(0xFFF87171);
const kBorder = Color(0xFF232A36);

ThemeData starmeshTheme() {
  final base = ThemeData(
    brightness: Brightness.dark,
    useMaterial3: true,
    colorScheme: const ColorScheme.dark(
      primary: kPrimary,
      secondary: kSecondary,
      surface: kSurface,
      error: kDanger,
    ),
    scaffoldBackgroundColor: kBackground,
  );
  return base.copyWith(
    cardTheme: CardThemeData(
      color: kSurface,
      elevation: 0,
      margin: const EdgeInsets.only(bottom: 10),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(14),
        side: const BorderSide(color: kBorder),
      ),
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: kSurfaceHi,
      hintStyle: TextStyle(color: Colors.white.withValues(alpha: 0.4)),
      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kBorder),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kBorder),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(12),
        borderSide: const BorderSide(color: kPrimary, width: 1.5),
      ),
    ),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        shape:
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        shape:
            RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
        side: const BorderSide(color: kBorder),
        padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 12),
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: kSurface,
      indicatorColor: kPrimary.withValues(alpha: 0.18),
      labelTextStyle: WidgetStatePropertyAll(
        TextStyle(fontSize: 12, color: Colors.white.withValues(alpha: 0.8)),
      ),
    ),
  );
}
