/// Starmesh invite codec — Dart twin of the Go `invite` package.
///
/// Full locator (QR / paste / Meshtastic): `starmesh1:<crockford>`.
/// 10-char short code is a fingerprint only; it cannot hold IPv6+pubkey.
library starmesh_invite;

import 'dart:convert';
import 'dart:typed_data';
import 'package:crypto/crypto.dart';

const inviteVersion = 1;
const invitePrefix = 'starmesh1:';
const shortLen = 10;
const defaultPort = 4433;
const flagIPv4 = 1 << 0;
const flagSeed = 1 << 1;
const flagName = 1 << 2;

const _crockfordAlphabet = '0123456789ABCDEFGHJKMNPQRSTVWXYZ';

class Invite {
  Invite({
    this.version = inviteVersion,
    required this.pubKey,
    this.ipv6,
    this.ipv4,
    this.port = defaultPort,
    this.expiry,
    this.name = '',
    this.cloudSeed = false,
  });

  final int version;
  final Uint8List pubKey;
  final Uint8List? ipv6;
  final Uint8List? ipv4;
  final int port;
  final DateTime? expiry;
  final String name;
  final bool cloudSeed;

  String shortCode() {
    final raw = _marshal(withCrc: false);
    return _shortFrom(raw);
  }

  String shortDisplay() {
    final s = shortCode();
    if (s.length != shortLen) return s;
    return '${s.substring(0, 5)}-${s.substring(5)}';
  }

  String encode() {
    final raw = _marshal(withCrc: true);
    return '$invitePrefix${_crockfordEncode(raw)}';
  }

  Uint8List _marshal({required bool withCrc}) {
    if (pubKey.length != 32) {
      throw FormatException('pubkey must be 32 bytes');
    }
    if (ipv6 == null && !cloudSeed) {
      throw FormatException('hub invite requires IPv6');
    }
    var flags = 0;
    if (ipv4 != null && ipv4!.length == 4) flags |= flagIPv4;
    if (cloudSeed) flags |= flagSeed;
    final nameBytes = utf8.encode(name);
    final ncut = nameBytes.length > 32 ? nameBytes.sublist(0, 32) : nameBytes;
    if (ncut.isNotEmpty) flags |= flagName;

    final out = BytesBuilder();
    out.addByte(version);
    out.addByte(flags);
    out.add(pubKey);
    final v6 = ipv6 ?? Uint8List(16);
    if (v6.length != 16) {
      throw FormatException('ipv6 must be 16 bytes');
    }
    out.add(v6);
    final p = ByteData(2)..setUint16(0, port == 0 ? defaultPort : port);
    out.add(p.buffer.asUint8List());
    final exp = ByteData(4);
    exp.setUint32(0, expiry == null ? 0 : expiry!.toUtc().millisecondsSinceEpoch ~/ 1000);
    out.add(exp.buffer.asUint8List());
    if (flags & flagIPv4 != 0) out.add(ipv4!);
    if (flags & flagName != 0) {
      out.addByte(ncut.length);
      out.add(ncut);
    }
    var raw = out.toBytes();
    if (withCrc) {
      final crc = ByteData(4)..setUint32(0, crc32IEEE(raw));
      raw = Uint8List.fromList([...raw, ...crc.buffer.asUint8List()]);
    }
    return raw;
  }
}

typedef ShortResolver = Invite? Function(String short);

Invite decodeInvite(String s, {ShortResolver? resolve}) {
  s = s.trim();
  if (s.isEmpty) throw FormatException('empty invite');
  final upper = s.replaceAll(' ', '').toUpperCase();
  if (upper.startsWith('STARMESH1:')) {
    return _decodePayload(upper.substring('STARMESH1:'.length));
  }
  final compact = upper.replaceAll('-', '');
  if (_isShort(compact)) {
    final inv = resolve?.call(compact);
    if (inv != null) return inv;
    throw FormatException(
      '10-char code is a fingerprint; scan QR or paste starmesh1: blob ($compact)',
    );
  }
  return _decodePayload(compact);
}

bool _isShort(String s) {
  if (s.length != shortLen) return false;
  return s.split('').every(_crockfordAlphabet.contains);
}

Invite _decodePayload(String s) {
  final raw = _crockfordDecode(_normalizeCrockford(s.replaceAll('-', '')));
  if (raw.length < 2 + 32 + 16 + 2 + 4 + 4) {
    throw FormatException('truncated');
  }
  final body = raw.sublist(0, raw.length - 4);
  final want = ByteData.sublistView(raw, raw.length - 4).getUint32(0);
  if (crc32IEEE(body) != want) throw FormatException('checksum mismatch');
  if (body[0] != inviteVersion) throw FormatException('unsupported version');
  final flags = body[1];
  var off = 2;
  final pk = Uint8List.fromList(body.sublist(off, off + 32));
  off += 32;
  var ipv6 = Uint8List.fromList(body.sublist(off, off + 16));
  off += 16;
  final port = ByteData.sublistView(body, off, off + 2).getUint16(0);
  off += 2;
  final exp = ByteData.sublistView(body, off, off + 4).getUint32(0);
  off += 4;
  Uint8List? ipv4;
  if (flags & flagIPv4 != 0) {
    ipv4 = Uint8List.fromList(body.sublist(off, off + 4));
    off += 4;
  }
  var name = '';
  if (flags & flagName != 0) {
    final n = body[off];
    off += 1;
    name = utf8.decode(body.sublist(off, off + n));
  }
  final allZero = ipv6.every((b) => b == 0);
  return Invite(
    pubKey: pk,
    ipv6: allZero ? null : ipv6,
    ipv4: ipv4,
    port: port,
    expiry: exp == 0 ? null : DateTime.fromMillisecondsSinceEpoch(exp * 1000, isUtc: true),
    name: name,
    cloudSeed: flags & flagSeed != 0,
  );
}

String _shortFrom(Uint8List raw) {
  final h = sha256.convert(raw).bytes;
  var s = _crockfordEncode(Uint8List.fromList(h.sublist(0, 8)));
  if (s.length > shortLen) s = s.substring(0, shortLen);
  return s;
}

String _crockfordEncode(Uint8List data) {
  const alphabet = _crockfordAlphabet;
  if (data.isEmpty) return '';
  // RFC 4648 bit order, custom alphabet, no padding.
  final out = StringBuffer();
  var buffer = 0;
  var bits = 0;
  for (final b in data) {
    buffer = (buffer << 8) | b;
    bits += 8;
    while (bits >= 5) {
      bits -= 5;
      out.write(alphabet[(buffer >> bits) & 31]);
    }
  }
  if (bits > 0) {
    out.write(alphabet[(buffer << (5 - bits)) & 31]);
  }
  return out.toString();
}

Uint8List _crockfordDecode(String s) {
  const alphabet = _crockfordAlphabet;
  var buffer = 0;
  var bits = 0;
  final out = BytesBuilder();
  for (final r in s.toUpperCase().codeUnits) {
    final i = alphabet.codeUnits.indexOf(r);
    if (i < 0) continue;
    buffer = (buffer << 5) | i;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      out.addByte((buffer >> bits) & 0xff);
    }
  }
  return out.toBytes();
}

String _normalizeCrockford(String s) {
  final b = StringBuffer();
  for (final c in s.toUpperCase().split('')) {
    switch (c) {
      case 'I':
      case 'L':
        b.write('1');
      case 'O':
        b.write('0');
      case 'U':
        b.write('V');
      default:
        b.write(c);
    }
  }
  return b.toString();
}

int crc32IEEE(List<int> p) {
  var crc = 0xFFFFFFFF;
  for (final b in p) {
    crc = _crcTable[(crc ^ b) & 0xFF] ^ (crc >> 8);
  }
  return (crc ^ 0xFFFFFFFF) & 0xFFFFFFFF;
}

final List<int> _crcTable = _makeCrc();

List<int> _makeCrc() {
  const poly = 0xEDB88320;
  return List<int>.generate(256, (i) {
    var crc = i;
    for (var j = 0; j < 8; j++) {
      if (crc & 1 != 0) {
        crc = poly ^ (crc >> 1);
      } else {
        crc >>= 1;
      }
    }
    return crc;
  });
}
