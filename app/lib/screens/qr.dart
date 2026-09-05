import 'package:flutter/material.dart';
import 'package:qr_flutter/qr_flutter.dart';

class InviteQrScreen extends StatelessWidget {
  const InviteQrScreen({super.key, required this.blob, required this.shortCode});
  final String blob;
  final String shortCode;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Hub invite')),
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(24),
          child: Column(
            children: [
              Text(shortCode, style: Theme.of(context).textTheme.displaySmall),
              const SizedBox(height: 8),
              const Text('10-char fingerprint — cache / community / just-scanned QR'),
              const SizedBox(height: 24),
              Expanded(
                child: Center(
                  child: QrImageView(
                    data: blob,
                    backgroundColor: Colors.white,
                    size: 320,
                  ),
                ),
              ),
              SelectableText(blob, style: const TextStyle(fontSize: 12)),
            ],
          ),
        ),
      ),
    );
  }
}
