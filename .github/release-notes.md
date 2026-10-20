PR Nanny is a passive GitHub.com pull-request monitor for macOS ARM64, Windows x64 and Linux x64. The changes below are generated from the commits since the previous release.

This release is published only after all required validation jobs and source-attestation checks pass for the exact source commit. The source commit is recorded in SOURCE.txt. Verify downloaded application archives against SHA256SUMS.

**Trust and platform limits:** macOS uses ad-hoc signing, without Developer ID or notarization. Windows is unsigned. The launch/IPC smoke tests exercise actual native executables but do not prove physical tray clicks, notifications, autostart or every desktop configuration. Fedora KDE requires native desktop qualification. OS notification delivery and permissions depend on your environment.

Use the application archives and platform instructions in README.md. Linux requires glibc 2.39+, GTK4, WebKitGTK 6 and Secret Service. Windows requires Microsoft Edge WebView2. Homebrew manifests for the existing alexcatdad/tap are attached for macOS and Linux; Homebrew has no native Windows installation path.

No telemetry or GitHub mutations. See SECURITY.md for the threat model and residual risks.
