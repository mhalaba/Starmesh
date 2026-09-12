import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';
import 'package:starmesh/screens/qr.dart';
import 'package:starmesh/theme.dart';

class NetworkScreen extends StatefulWidget {
  const NetworkScreen({super.key});
  @override
  State<NetworkScreen> createState() => _NetworkScreenState();
}

class _NetworkScreenState extends State<NetworkScreen> {
  final _invite = TextEditingController();

  @override
  void dispose() {
    _invite.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = AppScope.of(context);
    return ListenableBuilder(
      listenable: s,
      builder: (context, _) {
        return ListView(
          padding: const EdgeInsets.all(14),
          children: [
            _deviceCard(context, s),
            if (s.refuse.isNotEmpty) _refuseCard(s),
            const SizedBox(height: 4),
            _sectionTitle('Hubs & seeds', s.hubs.isEmpty ? null : '${s.hubs.length}'),
            if (s.hubs.isEmpty)
              _hint('No hub yet. Paste an invite below, scan a QR, or wait for a community seed.')
            else
              ...s.hubs.map((h) => _hubCard(h)),
            const SizedBox(height: 8),
            _sectionTitle('Peers', s.peers.isEmpty ? null : '${s.peers.length}'),
            if (s.peers.isEmpty)
              _hint('Nobody else is on your hub yet.')
            else
              ...s.peers.map((p) => _peerTile(p)),
            const SizedBox(height: 12),
            _addInviteCard(context, s),
            const SizedBox(height: 10),
            if (s.inviteBlob.isNotEmpty) _shareInviteButton(context, s),
          ],
        );
      },
    );
  }

  Widget _deviceCard(BuildContext context, AppState s) {
    final isHub = s.role == 'hub';
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(isHub ? Icons.hub : Icons.smartphone,
                    color: isHub ? kOk : kPrimary),
                const SizedBox(width: 10),
                Text('This device',
                    style: Theme.of(context).textTheme.titleMedium),
                const Spacer(),
                _RoleBadge(role: s.role),
              ],
            ),
            const SizedBox(height: 6),
            Text(
              s.banner,
              style: TextStyle(color: Colors.white.withValues(alpha: 0.6), fontSize: 13),
            ),
            const SizedBox(height: 14),
            Row(
              children: [
                Expanded(
                  child: FilledButton.icon(
                    onPressed: isHub ? null : s.becomeHub,
                    icon: const Icon(Icons.cell_tower, size: 18),
                    label: const Text('Become hub'),
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: isHub ? s.stopHub : null,
                    icon: const Icon(Icons.stop_circle_outlined, size: 18),
                    label: const Text('Stop hub'),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _refuseCard(AppState s) {
    return Card(
      color: kDanger.withValues(alpha: 0.10),
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(14),
        side: BorderSide(color: kDanger.withValues(alpha: 0.5)),
      ),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Icon(Icons.error_outline, color: kDanger, size: 20),
            const SizedBox(width: 10),
            Expanded(
              child: Text(s.refuse,
                  style: const TextStyle(color: kDanger, height: 1.35)),
            ),
          ],
        ),
      ),
    );
  }

  Widget _hubCard(HubRow h) {
    final subtitle = <String>[
      if (h.rttMs > 0) '${h.rttMs} ms',
      if (h.ipv6.isNotEmpty) 'IPv6' else if (h.ipv4.isNotEmpty) 'IPv4',
      if (h.spokes > 0) '${h.spokes} spoke${h.spokes == 1 ? '' : 's'}',
    ].join('  •  ');
    return Card(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(14, 12, 12, 12),
        child: Row(
          children: [
            _hubAvatar(h),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Flexible(
                        child: Text(
                          h.name.isEmpty ? h.fingerprint : h.name,
                          overflow: TextOverflow.ellipsis,
                          style: const TextStyle(
                              fontWeight: FontWeight.w700, fontSize: 15),
                        ),
                      ),
                      const SizedBox(width: 8),
                      if (h.proto.isNotEmpty) _protoChip(h.proto),
                    ],
                  ),
                  if (subtitle.isNotEmpty) ...[
                    const SizedBox(height: 3),
                    Text(subtitle,
                        style: TextStyle(
                            fontSize: 12.5,
                            color: Colors.white.withValues(alpha: 0.55))),
                  ],
                  if (h.ipv6.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(h.ipv6,
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                        style: TextStyle(
                            fontSize: 11,
                            color: Colors.white.withValues(alpha: 0.4))),
                  ],
                ],
              ),
            ),
            if (h.self) const _RoleBadge(role: 'this') else _RoleBadge(role: h.cloudSeed ? 'seed' : 'hub'),
          ],
        ),
      ),
    );
  }

  Widget _hubAvatar(HubRow h) {
    final (color, icon) = h.cloudSeed
        ? (kPrimary, Icons.cloud_done)
        : (kOk, Icons.hub);
    return Container(
      width: 40,
      height: 40,
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: color.withValues(alpha: 0.4)),
      ),
      child: Icon(icon, color: color, size: 20),
    );
  }

  Widget _protoChip(String proto) {
    final isQuic = proto.toLowerCase() == 'quic';
    final color = isQuic ? kOk : kSecondary;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(6),
      ),
      child: Text(proto.toUpperCase(),
          style: TextStyle(
              fontSize: 10, fontWeight: FontWeight.w700, color: color)),
    );
  }

  Widget _peerTile(PeerRow p) {
    return Card(
      child: ListTile(
        leading: CircleAvatar(
          backgroundColor: kSurfaceHi,
          child: Text(
            (p.label.isNotEmpty ? p.label[0] : '?').toUpperCase(),
            style: const TextStyle(color: kPrimary, fontWeight: FontWeight.w700),
          ),
        ),
        title: Text(p.label, style: const TextStyle(fontWeight: FontWeight.w600)),
        subtitle: p.fingerprint.isEmpty
            ? null
            : Text(p.fingerprint,
                style: TextStyle(
                    fontSize: 12, color: Colors.white.withValues(alpha: 0.45))),
        trailing: Icon(Icons.circle, size: 10, color: kOk.withValues(alpha: 0.8)),
      ),
    );
  }

  Widget _addInviteCard(BuildContext context, AppState s) {
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Icon(Icons.add_link, size: 18, color: kPrimary.withValues(alpha: 0.9)),
                const SizedBox(width: 8),
                const Text('Add a hub',
                    style: TextStyle(fontWeight: FontWeight.w700, fontSize: 14)),
              ],
            ),
            const SizedBox(height: 10),
            TextField(
              controller: _invite,
              minLines: 1,
              maxLines: 3,
              decoration: const InputDecoration(
                hintText: 'Paste starmesh1: invite, 10-char code, or IPv6',
              ),
              onSubmitted: (v) => _connect(s, v),
            ),
            const SizedBox(height: 10),
            SizedBox(
              width: double.infinity,
              child: FilledButton.icon(
                onPressed: () => _connect(s, _invite.text),
                icon: const Icon(Icons.link, size: 18),
                label: const Text('Connect'),
              ),
            ),
          ],
        ),
      ),
    );
  }

  void _connect(AppState s, String v) {
    final raw = v.trim();
    if (raw.isEmpty) return;
    s.ingestInvite(raw);
    _invite.clear();
    FocusScope.of(context).unfocus();
  }

  Widget _shareInviteButton(BuildContext context, AppState s) {
    return SizedBox(
      width: double.infinity,
      child: OutlinedButton.icon(
        onPressed: () => Navigator.of(context).push(
          MaterialPageRoute(
            builder: (_) =>
                InviteQrScreen(blob: s.inviteBlob, shortCode: s.shortCode),
          ),
        ),
        icon: const Icon(Icons.qr_code_2, size: 20),
        label: const Text('Show my invite QR'),
      ),
    );
  }

  Widget _sectionTitle(String text, String? count) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 6, 4, 8),
      child: Row(
        children: [
          Text(text,
              style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.4,
                  color: Colors.white.withValues(alpha: 0.55))),
          if (count != null) ...[
            const SizedBox(width: 8),
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 1),
              decoration: BoxDecoration(
                color: kSurfaceHi,
                borderRadius: BorderRadius.circular(999),
              ),
              child: Text(count,
                  style: TextStyle(
                      fontSize: 11, color: Colors.white.withValues(alpha: 0.6))),
            ),
          ],
        ],
      ),
    );
  }

  Widget _hint(String text) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(4, 0, 4, 12),
      child: Text(text,
          style: TextStyle(
              fontSize: 13, color: Colors.white.withValues(alpha: 0.45), height: 1.4)),
    );
  }
}

class _RoleBadge extends StatelessWidget {
  const _RoleBadge({required this.role});
  final String role;

  @override
  Widget build(BuildContext context) {
    final (color, label) = switch (role) {
      'hub' => (kOk, 'HUB'),
      'seed' => (kPrimary, 'SEED'),
      'this' => (kSecondary, 'THIS'),
      _ => (Colors.white70, 'SPOKE'),
    };
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.14),
        borderRadius: BorderRadius.circular(999),
        border: Border.all(color: color.withValues(alpha: 0.5)),
      ),
      child: Text(label,
          style: TextStyle(
              fontSize: 11, fontWeight: FontWeight.w800, color: color, letterSpacing: 0.5)),
    );
  }
}
