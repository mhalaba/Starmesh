import 'dart:async';
import 'dart:convert';

import 'package:flutter/widgets.dart';
import 'package:http/http.dart' as http;
import 'package:starmesh_invite/invite.dart';

/// Link state derived from the local daemon status, used to colour the header.
enum LinkState { offline, queued, hub, seed }

class HubRow {
  HubRow({
    required this.name,
    required this.role,
    this.rttMs = 0,
    this.ipv6 = '',
    this.ipv4 = '',
    this.self = false,
    this.fingerprint = '',
    this.proto = '',
    this.cloudSeed = false,
    this.spokes = 0,
  });
  final String name;
  final String role;
  final int rttMs;
  final String ipv6;
  final String ipv4;
  final bool self;
  final String fingerprint;
  final String proto;
  final bool cloudSeed;
  final int spokes;
}

class PeerRow {
  PeerRow(this.name, this.fingerprint);
  final String name;
  final String fingerprint;

  String get label => name.isEmpty ? fingerprint : name;
}

class ChatLine {
  ChatLine(this.from, this.text, {this.mine = false, DateTime? ts})
      : ts = ts ?? DateTime.now();
  final String from;
  final String text;
  final bool mine;
  final DateTime ts;
}

class AppState extends ChangeNotifier {
  AppState() {
    _tick = Timer.periodic(const Duration(seconds: 2), (_) => refresh());
    refresh();
    _pumpMessages();
  }

  final api = Uri.parse('http://127.0.0.1:7780');
  Timer? _tick;
  bool _disposed = false;
  int _lastSeq = 0;

  bool daemonUp = false;
  String banner = 'No hub — queued';
  String role = 'spoke';
  String inviteBlob = '';
  String shortCode = '';
  String refuse = '';
  List<HubRow> hubs = [];
  List<PeerRow> peers = [];
  List<ChatLine> lines = [];

  LinkState get link {
    if (!daemonUp) return LinkState.offline;
    if (hubs.isEmpty) return LinkState.queued;
    return hubs.first.cloudSeed ? LinkState.seed : LinkState.hub;
  }

  @override
  void dispose() {
    _disposed = true;
    _tick?.cancel();
    super.dispose();
  }

  Future<void> refresh() async {
    try {
      final r = await http
          .get(api.replace(path: '/v1/status'))
          .timeout(const Duration(seconds: 2));
      if (r.statusCode != 200) return;
      final j = jsonDecode(r.body) as Map<String, dynamic>;
      daemonUp = true;
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
            proto: '${m['proto'] ?? ''}',
            cloudSeed: m['cloud_seed'] == true,
            spokes: (m['spokes'] as num?)?.toInt() ?? 0,
          );
        }).toList();
      }
      final pl = j['peers'];
      if (pl is List) {
        peers = pl.map((e) {
          final m = e as Map<String, dynamic>;
          return PeerRow('${m['name'] ?? ''}', '${m['fingerprint'] ?? ''}');
        }).toList();
      }
      refuse = '';
      notifyListeners();
    } catch (_) {
      daemonUp = false;
      if (hubs.isEmpty && inviteBlob.isEmpty) {
        banner = 'Local daemon offline — start starmesh spoke';
      }
      notifyListeners();
    }
  }

  /// Long-poll the daemon inbox so incoming (and echoed outgoing) chat lines
  /// appear without the user refreshing.
  Future<void> _pumpMessages() async {
    while (!_disposed) {
      try {
        final r = await http
            .get(api.replace(
              path: '/v1/messages',
              queryParameters: {'after': '$_lastSeq'},
            ))
            .timeout(const Duration(seconds: 30));
        if (r.statusCode != 200) {
          await Future.delayed(const Duration(seconds: 1));
          continue;
        }
        daemonUp = true;
        final j = jsonDecode(r.body) as Map<String, dynamic>;
        final msgs = (j['messages'] as List?) ?? [];
        var changed = false;
        for (final m in msgs) {
          final mm = m as Map<String, dynamic>;
          _lastSeq = (mm['seq'] as num?)?.toInt() ?? _lastSeq;
          lines = [
            ...lines,
            ChatLine(
              '${mm['name'] ?? mm['from'] ?? ''}',
              '${mm['text'] ?? ''}',
              mine: mm['mine'] == true,
              ts: DateTime.fromMillisecondsSinceEpoch(
                  (mm['ts'] as num?)?.toInt() ?? DateTime.now().millisecondsSinceEpoch),
            ),
          ];
          changed = true;
        }
        if (changed) notifyListeners();
      } catch (_) {
        await Future.delayed(const Duration(seconds: 2));
      }
    }
  }

  Future<void> becomeHub() async {
    try {
      final r = await http.post(api.replace(path: '/v1/become-hub'));
      final j = jsonDecode(r.body) as Map<String, dynamic>;
      if (j['error'] != null) {
        refuse = '${j['message']}'.isNotEmpty ? '${j['message']}' : '${j['error']}';
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
    try {
      await http.post(
        api.replace(path: '/v1/send'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'to': to, 'text': text}),
      );
      // The daemon echoes the outgoing line via /v1/messages (mine=true),
      // so we do not append optimistically here to avoid duplicates.
    } catch (_) {
      lines = [...lines, ChatLine('system', 'queued (daemon offline)')];
      notifyListeners();
    }
  }

  /// Decode locally for instant feedback, then hand the raw blob to the daemon
  /// so it actually dials the hub/seed.
  Future<void> ingestInvite(String raw) async {
    try {
      final inv = decodeInvite(raw);
      inviteBlob = inv.encode();
      shortCode = inv.shortDisplay();
      banner = 'Invite ${inv.name.isEmpty ? shortCode : inv.name} — dialing';
      notifyListeners();
    } catch (e) {
      refuse = '$e';
      notifyListeners();
    }
    try {
      await http.post(
        api.replace(path: '/v1/invite'),
        headers: {'Content-Type': 'application/json'},
        body: jsonEncode({'invite': raw}),
      );
    } catch (_) {}
  }
}

class AppScope extends InheritedNotifier<AppState> {
  const AppScope({super.key, required AppState notifier, required super.child})
      : super(notifier: notifier);

  static AppState of(BuildContext context) =>
      context.dependOnInheritedWidgetOfExactType<AppScope>()!.notifier!;
}
