import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});
  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _ctl = TextEditingController();
  final _to = TextEditingController();

  @override
  void dispose() {
    _ctl.dispose();
    _to.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = AppScope.of(context);
    return ListenableBuilder(
      listenable: s,
      builder: (context, _) {
        return Column(
          children: [
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
              child: TextField(
                controller: _to,
                decoration: const InputDecoration(
                  labelText: 'To (name or fingerprint)',
                  border: OutlineInputBorder(),
                ),
              ),
            ),
            Expanded(
              child: ListView.builder(
                padding: const EdgeInsets.all(16),
                itemCount: s.lines.length,
                itemBuilder: (context, i) {
                  final l = s.lines[i];
                  return Align(
                    alignment: l.mine ? Alignment.centerRight : Alignment.centerLeft,
                    child: Container(
                      margin: const EdgeInsets.only(bottom: 8),
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                      decoration: BoxDecoration(
                        color: l.mine ? const Color(0xFF1C3A4A) : const Color(0xFF1B1F28),
                        borderRadius: BorderRadius.circular(12),
                      ),
                      child: Text('${l.from}: ${l.text}'),
                    ),
                  );
                },
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _ctl,
                      decoration: const InputDecoration(
                        hintText: 'Message (E2E, hubs see ciphertext only)',
                        border: OutlineInputBorder(),
                      ),
                      onSubmitted: (t) {
                        s.send(_to.text, t);
                        _ctl.clear();
                      },
                    ),
                  ),
                  IconButton(
                    icon: const Icon(Icons.send),
                    onPressed: () {
                      s.send(_to.text, _ctl.text);
                      _ctl.clear();
                    },
                  ),
                ],
              ),
            ),
          ],
        );
      },
    );
  }
}
