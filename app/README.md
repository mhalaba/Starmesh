# Starmesh Flutter UI

Chat, hubs, and QR only. This app is **not** the mesh.

It talks to the local Go daemon HTTP API at
`http://127.0.0.1:7780` (`starmesh hub|spoke --api`).

Exact run path (install Flutter, build the Go binary, start `--api`, then
`flutter run`): **[docs/flutter.md](../docs/flutter.md)**.

```bash
cd app
flutter pub get
flutter run
```

Do not point the UI at `168.138.14.210:4433`. That address is a QUIC/TLS
hub/seed, last in the daemon dial order — not a Flutter web host.
