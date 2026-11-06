# APTEX Mac client

A native, universal macOS app for connecting to the APTEX server on your desktop.
The Mac runs the client only. The app contains no APTEX server, Docker stack,
model runtime, Python environment, or repository-path dependency.

## Connect

1. Download `APTEX-macOS-universal.zip` from the macOS client build artifacts and
   move `APTEX.app` into Applications.
2. Open APTEX and enter your desktop server's HTTPS address or private LAN/VPN
   address, such as `http://desktop.local:8787`.
3. Sign in with your APTEX account. Your Mac keeps its own app session and theme
   preferences. **APTEX → Server…** changes the desktop address later.

Apple Silicon and Intel Macs are supported on macOS 13.3 or newer. The app uses
the system WebKit engine, so Chrome is not required. Standard Edit shortcuts,
back/forward navigation, reload, file upload, and downloads are supported.
External HTTPS links open in your normal browser. Server pages have no native
JavaScript bridge or access to a local command executor.

## Reach the desktop server

APTEX's default Docker configuration publishes its server to desktop loopback
only. A Mac cannot reach that address directly over the network. Use an existing
private-network reverse proxy/VPN endpoint, or an SSH tunnel to the desktop:

```sh
ssh -N -L 127.0.0.1:8787:127.0.0.1:8787 desktop-user@desktop-address
```

With that tunnel running, enter `http://127.0.0.1:8787` in the Mac client. This
forwards the connection to the desktop and does not run an APTEX server on the
Mac. The desktop needs an SSH service and must already be running APTEX.

Public server addresses require HTTPS. Private LAN, VPN, and loopback HTTP
addresses are accepted. Certificate validation stays enabled. Credentials must
be entered in the APTEX sign-in page, rather than embedded in the server URL.

## Build and verify

On a Mac with Xcode Command Line Tools:

```sh
swift test --package-path clients/macos
clients/macos/build.sh
python3 clients/macos/tools/smoke.py dist/macos/APTEX.app
```

Run these commands from the verified repository root. Build paths are resolved
from the script's own location; the finished app works after being moved to
Applications. The build produces both architectures, validates the universal
binary and code signature, and writes a ZIP and SHA-256 checksum to `dist/macos`.
CI builds and renders the actual app on Apple Silicon and Intel runners.

The default build is ad-hoc signed, rather than Developer ID notarized. macOS may
require approval in **System Settings → Privacy & Security** for a downloaded
build. Set `APTEX_SIGN_IDENTITY` to an available Developer ID identity when
building a distribution-signed version; notarization is a separate release step.
