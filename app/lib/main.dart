import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';
import 'package:starmesh/screens/chat.dart';
import 'package:starmesh/screens/network.dart';
import 'package:starmesh/theme.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const StarmeshApp());
}

class StarmeshApp extends StatelessWidget {
  const StarmeshApp({super.key});

  @override
  Widget build(BuildContext context) {
    return AppScope(
      notifier: AppState(),
      child: MaterialApp(
        title: 'Starmesh',
        debugShowCheckedModeBanner: false,
        theme: starmeshTheme(),
        home: const Shell(),
      ),
    );
  }
}

class Shell extends StatefulWidget {
  const Shell({super.key});
  @override
  State<Shell> createState() => _ShellState();
}

class _ShellState extends State<Shell> {
  int _i = 0;

  @override
  Widget build(BuildContext context) {
    final state = AppScope.of(context);
    return ListenableBuilder(
      listenable: state,
      builder: (context, _) {
        return Scaffold(
          body: SafeArea(
            bottom: false,
            child: Column(
              children: [
                _Header(state: state),
                Expanded(
                  child: IndexedStack(
                    index: _i,
                    children: const [ChatScreen(), NetworkScreen()],
                  ),
                ),
              ],
            ),
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _i,
            onDestinationSelected: (v) => setState(() => _i = v),
            destinations: const [
              NavigationDestination(
                icon: Icon(Icons.forum_outlined),
                selectedIcon: Icon(Icons.forum),
                label: 'Chat',
              ),
              NavigationDestination(
                icon: Icon(Icons.hub_outlined),
                selectedIcon: Icon(Icons.hub),
                label: 'Network',
              ),
            ],
          ),
        );
      },
    );
  }
}

class _Header extends StatelessWidget {
  const _Header({required this.state});
  final AppState state;

  @override
  Widget build(BuildContext context) {
    final (color, icon, label) = _linkStyle(state.link);
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 14),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [kSurface, kBackground],
        ),
        border: Border(bottom: BorderSide(color: color.withValues(alpha: 0.4))),
      ),
      child: Row(
        children: [
          const _Logo(),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Text('Starmesh',
                    style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 0.3)),
                const SizedBox(height: 2),
                Text(
                  state.banner,
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                      fontSize: 12, color: Colors.white.withValues(alpha: 0.6)),
                ),
              ],
            ),
          ),
          _StatusPill(color: color, icon: icon, label: label),
        ],
      ),
    );
  }

  (Color, IconData, String) _linkStyle(LinkState s) {
    switch (s) {
      case LinkState.hub:
        return (kOk, Icons.hub, 'Hub');
      case LinkState.seed:
        return (kPrimary, Icons.cloud_done, 'Seed');
      case LinkState.queued:
        return (kWarn, Icons.schedule, 'Queued');
      case LinkState.offline:
        return (kDanger, Icons.cloud_off, 'Offline');
    }
  }
}

class _Logo extends StatelessWidget {
  const _Logo();
  @override
  Widget build(BuildContext context) {
    return Container(
      width: 36,
      height: 36,
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(10),
        gradient: const LinearGradient(colors: [kPrimary, Color(0xFF3E7CB1)]),
      ),
      child: const Icon(Icons.travel_explore, color: Color(0xFF06121A), size: 22),
    );
  }
}

class _StatusPill extends StatelessWidget {
  const _StatusPill(
      {required this.color, required this.icon, required this.label});
  final Color color;
  final IconData icon;
  final String label;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.15),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: 0.6)),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(icon, size: 16, color: color),
          const SizedBox(width: 6),
          Text(label,
              style: TextStyle(
                  color: color, fontWeight: FontWeight.w700, fontSize: 12)),
        ],
      ),
    );
  }
}
