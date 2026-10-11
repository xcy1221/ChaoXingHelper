param([Parameter(Mandatory=$true)][string]$PackageDirectory)
$ErrorActionPreference = 'Stop'
$device = Join-Path $PackageDirectory 'device.crt'
$root = Join-Path $PackageDirectory 'server-root.crt'
Import-Certificate -FilePath $root -CertStoreLocation Cert:\CurrentUser\Root | Select-Object Subject, Thumbprint
Import-Certificate -FilePath (Join-Path $PackageDirectory 'client-ca.crt') -CertStoreLocation Cert:\CurrentUser\Root | Select-Object Subject, Thumbprint
& certreq.exe -accept -user -q $device
if ($LASTEXITCODE -ne 0) { throw 'Device certificate installation failed; use the computer that generated the CSR.' }
Write-Output 'Installed. Open the URL in connection.txt and choose your device certificate. No password is required.'
