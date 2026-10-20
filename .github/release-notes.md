PR Nanny is a passive GitHub.com pull-request monitor for macOS ARM64, Windows x64 and Linux x64.

This release candidate passed its linked native build, frontend, backend, credential-store, packaging and security gates. The source commit is recorded in SOURCE.txt. Verify downloaded application archives against SHA256SUMS.

**Trust and platform limits:** macOS uses ad-hoc signing, without Developer ID or notarization. Windows is unsigned. The launch/IPC smoke tests exercise actual native executables but do not prove physical tray clicks, notifications, autostart or every desktop configuration. Fedora KDE requires native desktop qualification. OS notification delivery and permissions depend on your environment.

Use the application archives and platform instructions in README.md. Linux requires glibc 2.39+, GTK4, WebKitGTK 6 and Secret Service. Windows requires Microsoft Edge WebView2. Homebrew manifests are provided for a local macOS/Linux tap; Homebrew has no native Windows installation path.

No telemetry or GitHub mutations. See SECURITY.md for the threat model and residual risks. This is prerelease software, not a stable release.
