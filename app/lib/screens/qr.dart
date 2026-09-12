import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:qr_flutter/qr_flutter.dart';
import 'package:starmesh/theme.dart';

class InviteQrScreen extends StatelessWidget {
  const InviteQrScreen({super.key, required this.blob, required this.shortCode});
  final String blob;
  final String shortCode;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('My invite'),
        backgroundColor: kSurface,
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(20),
          child: Column(
            children: [
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(22),
                  child: Column(
                    children: [
                      Container(
                        padding: const EdgeInsets.all(16),
                        decoration: BoxDecoration(
                          color: Colors.white,
                          borderRadius: BorderRadius.circular(16),
                        ),
                        child: QrImageView(
                          data: blob,
                          backgroundColor: Colors.white,
                          size: 260,
                        ),
                      ),
                      const SizedBox(height: 20),
                      Text('Short code',
                          style: TextStyle(
                              fontSize: 12,
                              letterSpacing: 1,
                              color: Colors.white.withValues(alpha: 0.5))),
                      const SizedBox(height: 4),
                      SelectableText(
                        shortCode,
                        style: const TextStyle(
                          fontSize: 30,
                          fontWeight: FontWeight.w700,
                          letterSpacing: 2,
                          color: kPrimary,
                        ),
                      ),
                      const SizedBox(height: 6),
                      Text(
                        'Scan the QR or share the code / blob below.\nHubs relay ciphertext only.',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                            fontSize: 12.5,
                            height: 1.4,
                            color: Colors.white.withValues(alpha: 0.5)),
                      ),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  Expanded(
                    child: OutlinedButton.icon(
                      onPressed: () => _copy(context, shortCode, 'Code copied'),
                      icon: const Icon(Icons.tag, size: 18),
                      label: const Text('Copy code'),
                    ),
                  ),
                  const SizedBox(width: 10),
                  Expanded(
                    child: FilledButton.icon(
                      onPressed: () => _copy(context, blob, 'Invite copied'),
                      icon: const Icon(Icons.copy_all, size: 18),
                      label: const Text('Copy invite'),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 16),
              Container(
                width: double.infinity,
                padding: const EdgeInsets.all(14),
                decoration: BoxDecoration(
                  color: kSurfaceHi,
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: kBorder),
                ),
                child: SelectableText(
                  blob,
                  style: TextStyle(
                      fontSize: 12,
                      height: 1.4,
                      color: Colors.white.withValues(alpha: 0.75)),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  void _copy(BuildContext context, String data, String msg) {
    Clipboard.setData(ClipboardData(text: data));
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        content: Text(msg),
        behavior: SnackBarBehavior.floating,
        duration: const Duration(seconds: 2),
      ),
    );
  }
}
