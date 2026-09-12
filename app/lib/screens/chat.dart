import 'package:flutter/material.dart';
import 'package:starmesh/app_state.dart';
import 'package:starmesh/theme.dart';

class ChatScreen extends StatefulWidget {
  const ChatScreen({super.key});
  @override
  State<ChatScreen> createState() => _ChatScreenState();
}

class _ChatScreenState extends State<ChatScreen> {
  final _ctl = TextEditingController();
  final _scroll = ScrollController();
  String _to = '';
  int _lastCount = 0;

  @override
  void dispose() {
    _ctl.dispose();
    _scroll.dispose();
    super.dispose();
  }

  void _autoscroll(int count) {
    if (count == _lastCount) return;
    _lastCount = count;
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (!_scroll.hasClients) return;
      _scroll.animateTo(
        _scroll.position.maxScrollExtent,
        duration: const Duration(milliseconds: 220),
        curve: Curves.easeOut,
      );
    });
  }

  void _submit(AppState s) {
    final text = _ctl.text.trim();
    if (text.isEmpty) return;
    s.send(_to, text);
    _ctl.clear();
  }

  @override
  Widget build(BuildContext context) {
    final s = AppScope.of(context);
    return ListenableBuilder(
      listenable: s,
      builder: (context, _) {
        _autoscroll(s.lines.length);
        return Column(
          children: [
            _recipientBar(context, s),
            Expanded(
              child: s.lines.isEmpty
                  ? _empty(s)
                  : ListView.builder(
                      controller: _scroll,
                      padding: const EdgeInsets.fromLTRB(14, 8, 14, 8),
                      itemCount: s.lines.length,
                      itemBuilder: (context, i) => _bubble(s.lines[i]),
                    ),
            ),
            _composer(s),
          ],
        );
      },
    );
  }

  Widget _recipientBar(BuildContext context, AppState s) {
    final choices = <String>['', ...s.peers.map((p) => p.label)];
    final current = choices.contains(_to) ? _to : '';
    return Container(
      padding: const EdgeInsets.fromLTRB(14, 10, 14, 10),
      decoration: const BoxDecoration(
        color: kSurface,
        border: Border(bottom: BorderSide(color: kBorder)),
      ),
      child: Row(
        children: [
          Icon(Icons.alternate_email, size: 18, color: kPrimary.withValues(alpha: 0.9)),
          const SizedBox(width: 10),
          Expanded(
            child: DropdownButtonHideUnderline(
              child: DropdownButton<String>(
                value: current,
                isExpanded: true,
                dropdownColor: kSurfaceHi,
                borderRadius: BorderRadius.circular(12),
                items: [
                  for (final c in choices)
                    DropdownMenuItem(
                      value: c,
                      child: Text(
                        c.isEmpty ? 'Everyone on this hub' : c,
                        style: const TextStyle(fontSize: 14),
                      ),
                    ),
                ],
                onChanged: (v) => setState(() => _to = v ?? ''),
              ),
            ),
          ),
          if (s.peers.isNotEmpty)
            Text('${s.peers.length} peer${s.peers.length == 1 ? '' : 's'}',
                style: TextStyle(fontSize: 12, color: Colors.white.withValues(alpha: 0.45))),
        ],
      ),
    );
  }

  Widget _empty(AppState s) {
    final hint = s.link == LinkState.offline
        ? 'Local daemon offline. Start it with:  starmesh spoke'
        : s.link == LinkState.queued
            ? 'No hub yet. Add an invite on the Network tab; messages queue until a hub is reached.'
            : 'End-to-end encrypted. Say hello — hubs only ever relay ciphertext.';
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.lock_outline, size: 42, color: kPrimary.withValues(alpha: 0.7)),
            const SizedBox(height: 14),
            Text(
              hint,
              textAlign: TextAlign.center,
              style: TextStyle(color: Colors.white.withValues(alpha: 0.6), height: 1.4),
            ),
          ],
        ),
      ),
    );
  }

  Widget _bubble(ChatLine l) {
    final isSystem = l.from == 'system';
    if (isSystem) {
      return Padding(
        padding: const EdgeInsets.symmetric(vertical: 6),
        child: Center(
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
            decoration: BoxDecoration(
              color: kWarn.withValues(alpha: 0.12),
              borderRadius: BorderRadius.circular(999),
            ),
            child: Text(l.text,
                style: TextStyle(fontSize: 12, color: kWarn.withValues(alpha: 0.95))),
          ),
        ),
      );
    }
    final mine = l.mine;
    return Align(
      alignment: mine ? Alignment.centerRight : Alignment.centerLeft,
      child: Container(
        constraints: const BoxConstraints(maxWidth: 320),
        margin: const EdgeInsets.only(bottom: 8),
        padding: const EdgeInsets.fromLTRB(14, 9, 14, 7),
        decoration: BoxDecoration(
          color: mine ? kPrimary.withValues(alpha: 0.16) : kSurfaceHi,
          border: Border.all(
            color: mine ? kPrimary.withValues(alpha: 0.5) : kBorder,
          ),
          borderRadius: BorderRadius.only(
            topLeft: const Radius.circular(14),
            topRight: const Radius.circular(14),
            bottomLeft: Radius.circular(mine ? 14 : 4),
            bottomRight: Radius.circular(mine ? 4 : 14),
          ),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            if (!mine)
              Padding(
                padding: const EdgeInsets.only(bottom: 3),
                child: Text(l.from,
                    style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        color: kPrimary.withValues(alpha: 0.9))),
              ),
            Text(l.text, style: const TextStyle(fontSize: 15, height: 1.3)),
            const SizedBox(height: 3),
            Row(
              mainAxisSize: MainAxisSize.min,
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                Text(_hhmm(l.ts),
                    style: TextStyle(
                        fontSize: 10.5, color: Colors.white.withValues(alpha: 0.4))),
                if (mine) ...[
                  const SizedBox(width: 4),
                  Icon(Icons.done_all, size: 13, color: kPrimary.withValues(alpha: 0.7)),
                ],
              ],
            ),
          ],
        ),
      ),
    );
  }

  Widget _composer(AppState s) {
    return SafeArea(
      top: false,
      child: Container(
        padding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
        decoration: const BoxDecoration(
          color: kSurface,
          border: Border(top: BorderSide(color: kBorder)),
        ),
        child: Row(
          children: [
            Expanded(
              child: TextField(
                controller: _ctl,
                textInputAction: TextInputAction.send,
                minLines: 1,
                maxLines: 4,
                decoration: const InputDecoration(
                  hintText: 'Message — encrypted end-to-end',
                ),
                onSubmitted: (_) => _submit(s),
              ),
            ),
            const SizedBox(width: 8),
            _SendButton(onTap: () => _submit(s)),
          ],
        ),
      ),
    );
  }

  String _hhmm(DateTime t) {
    final h = t.hour.toString().padLeft(2, '0');
    final m = t.minute.toString().padLeft(2, '0');
    return '$h:$m';
  }
}

class _SendButton extends StatelessWidget {
  const _SendButton({required this.onTap});
  final VoidCallback onTap;
  @override
  Widget build(BuildContext context) {
    return Material(
      color: kPrimary,
      borderRadius: BorderRadius.circular(12),
      child: InkWell(
        borderRadius: BorderRadius.circular(12),
        onTap: onTap,
        child: const Padding(
          padding: EdgeInsets.all(12),
          child: Icon(Icons.send_rounded, color: Color(0xFF06121A), size: 22),
        ),
      ),
    );
  }
}
