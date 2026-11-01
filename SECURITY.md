# Security policy

SluiceOps controls the configuration of a load balancer, so security issues are
taken seriously.

Please **do not open a public issue** for vulnerabilities. Report them privately
through GitHub's "Report a vulnerability" function (Security tab of the
repository). You will get an answer within a few days.

Supported versions: the latest release.

Hardening recommendations:

- Do not expose SluiceOps to the internet. Bind it to localhost or your LAN and
  put it behind a reverse proxy with additional access control.
- Use a strong `SLUICEOPS_ADMIN_PASSWORD`.
- Use `-auth none` only behind a proxy that authenticates every request.
- Use `-read-only` for monitoring-only deployments.
