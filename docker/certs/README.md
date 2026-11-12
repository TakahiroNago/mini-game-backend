# docker/certs

Optional extra CA certificates (PEM, `*.crt`) that the container **build stage**
should trust. Needed only when the build host sits behind a TLS-inspecting
proxy or antivirus (e.g. corporate proxy, Avast/Kaspersky "HTTPS scanning"),
which otherwise makes `go mod download` / `npm ci` fail with
`x509: certificate signed by unknown authority`.

`*.crt` files here are git-ignored. CI and normal machines need nothing here.

Export on Windows (PowerShell):

```powershell
$c = Get-ChildItem Cert:\LocalMachine\Root, Cert:\CurrentUser\Root | Where-Object { $_.Subject -like '*Avast*' } | Select-Object -First 1
$pem = "-----BEGIN CERTIFICATE-----`n" + [Convert]::ToBase64String($c.RawData, 'InsertLineBreaks') + "`n-----END CERTIFICATE-----`n"
[IO.File]::WriteAllText("docker\certs\local-tls-inspection.crt", $pem)
```
