# Starmesh invite codec

Go (`invite/`) and Dart (`invite/dart`) encode the same binary layout.

A **full invite** (QR, paste, Meshtastic text) is:

```
starmesh1:<crockford-base32>
```

Fields: version, flags, Ed25519 pubkey (32), IPv6 (16), port, expiry,
optional IPv4, optional name, CRC-32. **No DNS names.**

A **10-character Crockford code** is `SHA-256(payload)[:8]` encoded and
truncated. Fifty bits cannot hold an IPv6 address plus a 32-byte key.
The short code resolves against:

1. last-good hub cache
2. signed pre-provisioned community list
3. a full invite that was just scanned

Starlink hubs always include IPv6. Cloud seeds may be IPv4-only.
