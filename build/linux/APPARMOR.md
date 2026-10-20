# Ubuntu 24.04+ user namespaces

Ubuntu restricts unprivileged user namespaces by default. WebKitGTK uses Bubblewrap to sandbox its subprocesses; an unregistered application can fail with `bwrap: setting up uid map: Permission denied`.

For Ubuntu installations that encounter this error, generate a profile for the **installed executable's exact path**, review it, and have an administrator load it:

```sh
sh scripts/apparmor-profile.sh "$HOME/.local/bin/pr-nanny" > pr-nanny.apparmor
sudo install -m 644 pr-nanny.apparmor /etc/apparmor.d/pr-nanny
sudo apparmor_parser -r /etc/apparmor.d/pr-nanny
```

The generator changes no system settings. Repeat for the real executable path if it moves (including versioned Homebrew Cellar paths). This permits this application to create its WebKit sandbox; it does not disable WebKit sandboxing or relax the system-wide user-namespace policy. It follows the per-application profile approach in the [Ubuntu release notes](https://documentation.ubuntu.com/release-notes/24.04/#unprivileged-user-namespace-restrictions).

A system administrator can remove the allowance with `sudo apparmor_parser -R /etc/apparmor.d/pr-nanny` followed by removing that profile file. Fedora normally uses SELinux rather than this Ubuntu policy; do not install an AppArmor profile there without a corresponding policy need.

CI loads a profile for its exact build executable before the Xvfb launch smoke test. The application never loads security profiles or requests elevated privileges itself.
