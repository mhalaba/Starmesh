import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';
import 'package:starmesh/screens/qr.dart';

class NetworkScreen extends StatelessWidget {
  const NetworkScreen({super.key});

  @override
  Widget build(BuildContext context) {
    final s = AppScope.of(context);
    return ListenableBuilder(
      listenable: s,
      builder: (context, _) {
        return ListView(
          padding: const EdgeInsets.all(16),
          children: [
            Text('This device: ${s.role}', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            if (s.refuse.isNotEmpty)
              Card(
                color: const Color(0xFF3A1D12),
                child: Padding(
                  padding: const EdgeInsets.all(12),
                  child: Text(s.refuse),
                ),
              ),
            Row(
              children: [
                FilledButton(
                  onPressed: s.becomeHub,
                  child: const Text('Become hub'),
                ),
                const SizedBox(width: 8),
                OutlinedButton(
                  onPressed: s.stopHub,
                  child: const Text('Stop being hub'),
                ),
              ],
            ),
            const SizedBox(height: 16),
            Text('Hubs', style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            if (s.hubs.isEmpty)
              const Text('None yet. Scan a QR, paste IPv6, or wait for a community hub.'),
            ...s.hubs.map(
              (h) => Card(
                child: ListTile(
                  title: Text(h.name.isEmpty ? h.fingerprint : h.name),
                  subtitle: Text(
                    [
                      h.role,
                      if (h.rttMs > 0) '${h.rttMs} ms',
                      if (h.ipv6.isNotEmpty) 'IPv6 ${h.ipv6}',
                      if (h.ipv4.isNotEmpty) 'IPv4 ${h.ipv4}' else 'IPv4 unreachable',
                    ].join(' · '),
                  ),
                  trailing: h.self ? const Chip(label: Text('this')) : null,
                ),
              ),
            ),
            const SizedBox(height: 16),
            if (s.inviteBlob.isNotEmpty)
              FilledButton.tonal(
                onPressed: () => Navigator.of(context).push(
                  MaterialPageRoute(builder: (_) => InviteQrScreen(blob: s.inviteBlob, shortCode: s.shortCode)),
                ),
                child: const Text('Show invite QR'),
              ),
            const SizedBox(height: 12),
            TextField(
              decoration: const InputDecoration(
                labelText: 'Paste starmesh1: invite, 10-char code, or IPv6',
                border: OutlineInputBorder(),
              ),
              onSubmitted: s.ingestInvite,
            ),
          ],
        );
      },
    );
  }
}
