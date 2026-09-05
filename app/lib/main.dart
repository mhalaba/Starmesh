import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';
import 'package:starmesh/screens/chat.dart';
import 'package:starmesh/screens/network.dart';

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
        theme: ThemeData(
          brightness: Brightness.dark,
          colorScheme: const ColorScheme.dark(
            primary: Color(0xFF7AD7FF),
            secondary: Color(0xFFE8C547),
            surface: Color(0xFF10141C),
          ),
          scaffoldBackgroundColor: const Color(0xFF0B0E14),
          useMaterial3: true,
        ),
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
          body: Column(
            children: [
              _Banner(text: state.banner),
              Expanded(
                child: IndexedStack(
                  index: _i,
                  children: const [ChatScreen(), NetworkScreen()],
                ),
              ),
            ],
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _i,
            onDestinationSelected: (v) => setState(() => _i = v),
            destinations: const [
              NavigationDestination(icon: Icon(Icons.forum_outlined), label: 'Chat'),
              NavigationDestination(icon: Icon(Icons.hub_outlined), label: 'Network'),
            ],
          ),
        );
      },
    );
  }
}

class _Banner extends StatelessWidget {
  const _Banner({required this.text});
  final String text;
  @override
  Widget build(BuildContext context) {
    final ok = !text.toLowerCase().contains('no hub');
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.fromLTRB(16, 48, 16, 12),
      color: ok ? const Color(0xFF123524) : const Color(0xFF3A1D12),
      child: Text(
        text,
        style: TextStyle(
          color: ok ? const Color(0xFFB6F3C8) : const Color(0xFFFFD0B5),
          fontWeight: FontWeight.w600,
        ),
      ),
    );
  }
}
