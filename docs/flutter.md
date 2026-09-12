# Flutter UI (chat, hubs, QR)

The Flutter app in `/app` is **UI only**. It is not the mesh. It never
dials QUIC/TLS and it is not a web host.

It talks to the **local Go starmesh daemon** HTTP API:

```
http://127.0.0.1:7780
```

That is `starmesh hub --api` or `starmesh spoke --api`. Hub/seed dialing
(including the cloud seed) stays inside the Go process.

`168.138.14.210:4433` is a hub/seed on **QUIC/TLS :4433**. Do not open it
in a browser, do not pass it as `--web-hostname`, and do not set it as a
Flutter “server URL”.

## 1. Install Flutter

Need the Flutter SDK (stable, Dart 3.3+):

```bash
# https://docs.flutter.dev/install/manual
export PATH="$PATH:$HOME/flutter/bin"
flutter doctor
```

Desktop and web (Cursor / local):

```bash
flutter config --enable-linux-desktop --enable-web
```

`/app` already has platform folders (`android`, `ios`, `linux`, `macos`,
`windows`, `web`). If you cloned an older tree without them:

```bash
cd app && flutter create . --project-name starmesh
```

That command must not wipe `lib/`.

## 2. Build the Go daemon

From the repo root:

```bash
go test ./...
go build -o dist/starmesh ./hub/cmd/starmesh
```

## 3. Start hub or spoke with `--api` on :7780

One daemon per device. Flutter attaches to whichever local process is
listening on `127.0.0.1:7780`.

**Hub** (lab machine without global IPv6 needs `--dev`):

```bash
./dist/starmesh hub --dev --name "OSP Nadarzyn" --api 127.0.0.1:7780
```

**Spoke** (phones stay outbound-only):

```bash
./dist/starmesh spoke --name alice --invite 'starmesh1:...' --api 127.0.0.1:7780
```

`--api` defaults to `127.0.0.1:7780`. Pass `--api ''` to disable it.
Two daemons on one host cannot share the same port — point Flutter at
one of them.

Paste a `starmesh1:` invite, 10-char fingerprint, or IPv6 in the Network
tab. The UI POSTs it to `/v1/invite`; the **daemon** dials.

## 4. Run the Flutter UI

```bash
cd app
flutter pub get
flutter run
```

Device / target examples:

```bash
flutter run -d linux          # desktop
flutter run -d chrome         # web (CORS is enabled on the loopback API)
flutter run -d android        # USB / emulator
```

Android emulator reaching a daemon on the host:

```bash
flutter run -d android --dart-define=STARMESH_API=http://10.0.2.2:7780
# or: adb reverse tcp:7780 tcp:7780  (then keep the default 127.0.0.1)
```

`--dart-define=STARMESH_API` overrides the loopback URL only. It is not
a seed address.

## Dial order (Go daemon, not Flutter)

Cloud seeds are **last**:

1. Already-open hub session
2. Cached hubs, lowest RTT first
3. Pre-provisioned community hubs
4. Fresh invite just scanned / pasted
5. Cloud seeds (e.g. `168.138.14.210:4433` QUIC/TLS)

Timeouts are 15–20s. Starlink is lossy and prefixes move.

## API the UI uses

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/v1/status` | banner, role, hubs, chat transcript |
| GET | `/v1/invite` | hub invite blob for QR |
| POST | `/v1/invite` | ingest locator (`{"invite":"..."}`) so the daemon dials |
| POST | `/v1/send` | E2E chat (`{"to":"...","text":"..."}`) |
| POST | `/v1/become-hub` | capability probe / already-hub |
| POST | `/v1/stop-hub` | stop the local hub |

The listener must stay on loopback.
