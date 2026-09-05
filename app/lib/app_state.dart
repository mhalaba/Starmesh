import 'dart:async';
import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:http/http.dart' as http;
import 'package:starmesh_invite/invite.dart';

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
  AppState() {
    _tick = Timer.periodic(const Duration(seconds: 2), (_) => refresh());
    refresh();
  }

  final api = Uri.parse('http://127.0.0.1:7780');
  Timer? _tick;

  String banner = 'No hub — queued';
  String role = 'spoke';
  String inviteBlob = '';
  String shortCode = '';
  String refuse = '';
  List<HubRow> hubs = [];
  List<ChatLine> lines = [];
  String draft = '';

  @override
  void dispose() {
    _tick?.cancel();
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
      await http.post(
        api.replace(path: '/v1/send'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'to': to, 'text': text}),
      );
    } catch (_) {
      lines = [...lines, ChatLine('system', 'queued (no hub)')];
      banner = 'No hub — queued';
      notifyListeners();
    }
  }

  void ingestInvite(String raw) {
    try {
      final inv = decodeInvite(raw);
      inviteBlob = inv.encode();
      shortCode = inv.shortDisplay();
      banner = 'Invite ${inv.name.isEmpty ? shortCode : inv.name} — dialing';
    } catch (e) {
      refuse = '$e';
    }
    notifyListeners();
  }
}

class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState notifier, required super.child})
      : super(notifier: notifier);

  static AppState of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppScope>()!.notifier!;
}
