# Building from Source

Most users should download a prebuilt binary from the
[Releases](https://github.com/degeer/locksmith/releases) page instead — see the
README. This document is for building Locksmith yourself.

## Requirements

- macOS
- Go 1.25 or later

## Install with go install

```bash
go install github.com/degeer/locksmith@latest
```

## Building from Source

To build an optimized binary with reduced size:

```bash
go build -ldflags="-s -w" -o locksmith .
```

Build flags explained:
- `-ldflags="-s -w"`: Strips debugging information and symbol tables for a smaller binary

For development builds with debug logging (writes to `locksmith.log`):

```bash
go build -tags debug -o locksmith .
```
