import 'dart:async';
import 'dart:convert';

import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;
import 'package:starmesh_invite/invite.dart';

/// Loopback Go daemon (`starmesh hub|spoke --api`). Not a mesh or seed URL.
/// Override with `--dart-define=STARMESH_API=http://...` (e.g. Android emulator
/// `http://10.0.2.2:7780`). Never point this at `168.138.14.210:4433`.
const localAPIBase = String.fromEnvironment(
  'STARMESH_API',
  defaultValue: 'http://127.0.0.1:7780',
);

class HubRow {
  HubRow({
    required this.name,
    required this.role,
    this.rttMs = 0,
    this.ipv6 = '',
    this.ipv4 = '',
    this.self = false,
    this.fingerprint = '',
  });
  final String name;
  final String role;
  final int rttMs;
  final String ipv6;
  final String ipv4;
  final bool self;
  final String fingerprint;
}

class ChatLine {
  ChatLine(this.from, this.text, {this.mine = false});
  final String from;
  final String text;
  final bool mine;
}

class AppState extends ChangeNotifier {
  AppState({Uri? api}) : api = api ?? Uri.parse(localAPIBase);

  final Uri api;
  Timer? _tick;

  String banner = 'No hub — queued';
  String role = 'spoke';
  String inviteBlob = '';
  String shortCode = '';
  String refuse = '';
  List<HubRow> hubs = [];
  List<ChatLine> lines = [];
  String draft = '';

  void start() {
    if (_tick != null) return;
    _tick = Timer.periodic(const Duration(seconds: 2), (_) => refresh());
    refresh();
  }

  @override
  void dispose() {
    _tick?.cancel();
    _tick = null;
    super.dispose();
  }

  Future<void> refresh() async {
    try {
      final r = await http.get(api.replace(path: '/v1/status')).timeout(const Duration(seconds: 2));
      if (r.statusCode != 200) return;
      final j = jsonDecode(r.body) as Map<String, dynamic>;
      banner = (j['banner'] as String?) ?? banner;
      role = (j['role'] as String?) ?? role;
      inviteBlob = (j['invite'] as String?) ?? inviteBlob;
      shortCode = (j['short'] as String?) ?? shortCode;
      final list = j['hubs'];
      if (list is List) {
        hubs = list.map((e) {
          final m = e as Map<String, dynamic>;
          return HubRow(
            name: '${m['name'] ?? ''}',
            role: '${m['role'] ?? ''}',
            rttMs: (m['rtt_ms'] as num?)?.toInt() ?? 0,
            ipv6: '${m['ipv6'] ?? ''}',
            ipv4: '${m['ipv4'] ?? ''}',
            self: m['self'] == true,
            fingerprint: '${m['fingerprint'] ?? ''}',
          );
        }).toList();
      }
      final msgs = j['messages'];
      if (msgs is List) {
        lines = msgs.map((e) {
          final m = e as Map<String, dynamic>;
          return ChatLine(
            '${m['from'] ?? ''}',
            '${m['text'] ?? ''}',
            mine: m['mine'] == true,
          );
        }).toList();
      }
      refuse = '';
      notifyListeners();
    } catch (_) {
      // Local Go daemon not running — UI still works; actions will explain.
      if (hubs.isEmpty && inviteBlob.isEmpty) {
        banner = 'No hub — queued';
        notifyListeners();
      }
    }
  }

  Future<void> becomeHub() async {
    try {
      final r = await http.post(api.replace(path: '/v1/become-hub'));
      final j = jsonDecode(r.body) as Map<String, dynamic>;
      if (j['error'] != null) {
        refuse = '${j['error']}';
        if ('${j['message']}'.isNotEmpty) refuse = '${j['message']}';
      } else {
        refuse = '';
        await refresh();
      }
    } catch (_) {
      refuse =
          'This dish cannot be a hub. Put the app on a PC behind a bypass router or use a Priority public IP.';
    }
    notifyListeners();
  }

  Future<void> stopHub() async {
    try {
      await http.post(api.replace(path: '/v1/stop-hub'));
    } catch (_) {}
    role = 'spoke';
    await refresh();
  }

  Future<void> send(String to, String text) async {
    if (text.trim().isEmpty) return;
    lines = [...lines, ChatLine('me', text, mine: true)];
    notifyListeners();
    try {
      final r = await http.post(
        api.replace(path: '/v1/send'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'to': to, 'text': text}),
      );
      if (r.statusCode != 200) {
        lines = [...lines, ChatLine('system', 'queued (no hub)')];
        banner = 'No hub — queued';
        notifyListeners();
        return;
      }
      final j = jsonDecode(r.body);
      if (j is Map && j['error'] != null) {
        lines = [...lines, ChatLine('system', '${j['error']}')];
        notifyListeners();
      }
    } catch (_) {
      lines = [...lines, ChatLine('system', 'queued (no hub)')];
      banner = 'No hub — queued';
      notifyListeners();
    }
  }

  /// Decode locally for QR display, then hand the locator to the Go daemon.
  /// Flutter never dials hubs or the cloud seed.
  Future<void> ingestInvite(String raw) async {
    raw = raw.trim();
    if (raw.isEmpty) return;
    try {
      final inv = decodeInvite(raw);
      inviteBlob = inv.encode();
      shortCode = inv.shortDisplay();
      final label = inv.name.isEmpty ? shortCode : inv.name;
      banner = inv.cloudSeed ? 'Seed $label — last in dial order' : 'Invite $label — dialing';
      refuse = '';
    } catch (e) {
      refuse = '$e';
    }
    notifyListeners();
    try {
      final r = await http.post(
        api.replace(path: '/v1/invite'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'invite': raw}),
      );
      if (r.statusCode != 200) {
        refuse = refuse.isEmpty ? 'daemon rejected invite (HTTP ${r.statusCode})' : refuse;
        notifyListeners();
        return;
      }
      final j = jsonDecode(r.body);
      if (j is Map && j['error'] != null) {
        refuse = '${j['error']}';
        notifyListeners();
        return;
      }
      await refresh();
    } catch (_) {
      if (refuse.isEmpty) {
        refuse = 'Local daemon not running on $localAPIBase (starmesh --api)';
      }
      notifyListeners();
    }
  }
}

class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState notifier, required super.child})
      : super(notifier: notifier);

  static AppState of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppScope>()!.notifier!;
}
