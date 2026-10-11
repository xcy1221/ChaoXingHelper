param(
    [Parameter(Mandatory=$true)][ValidatePattern('^[A-Za-z0-9_-]{1,40}$')][string]$User,
    [string]$OutputDirectory = "$env:USERPROFILE\AigcDevice"
)
$ErrorActionPreference = 'Stop'
New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
$inf = Join-Path $OutputDirectory "$User.inf"
$csr = Join-Path $OutputDirectory "$User.csr"
if (Test-Path -LiteralPath $csr) { throw 'CSR already exists; use another directory for a new device.' }
@"
[Version]
Signature="`$Windows NT`$"
[NewRequest]
Subject="CN=$User"
KeyAlgorithm=RSA
KeyLength=2048
KeySpec=0
Exportable=FALSE
MachineKeySet=FALSE
ProviderName="Microsoft Software Key Storage Provider"
RequestType=PKCS10
HashAlgorithm=SHA256
KeyUsage=0x80
UserProtected=FALSE
[Extensions]
2.5.29.37="{text}1.3.6.1.5.5.7.3.2"
"@ | Set-Content -LiteralPath $inf -Encoding ascii
& certreq.exe -new -user -q $inf $csr
if ($LASTEXITCODE -ne 0) { throw 'Certificate request failed.' }
Write-Output "Send only this CSR to the administrator: $csr"
