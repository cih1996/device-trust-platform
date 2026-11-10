param([switch]$Quiet)
$ErrorActionPreference='Stop'
Set-Location $PSScriptRoot
foreach($port in @(8099,9443,9444)){
    if(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction SilentlyContinue){throw "Port $port is occupied. See Stop-Demo.ps1 for this package's processes."}
}
New-Item -ItemType Directory -Force data,logs | Out-Null
& .\setup-demo.exe
if($LASTEXITCODE -ne 0){throw 'Demo initialization failed'}
& .\trust-sync.exe -out data/trust
if($LASTEXITCODE -ne 0){throw 'Official attestation trust refresh failed'}
$owned=@()
try {
    $platform=Start-Process -FilePath (Join-Path $PWD 'server.exe') -WorkingDirectory $PWD -WindowStyle Hidden -PassThru -RedirectStandardOutput logs/platform.out.log -RedirectStandardError logs/platform.err.log -ArgumentList @('-addr','127.0.0.1:9443','-admin-addr','127.0.0.1:8099','-data','data/platform','-web','web/dist','-admin-token-file','data/admin-token','-tls-cert','config/localhost-cert.pem','-tls-key','config/localhost-key.pem','-attestation-roots','data/trust/attestation-roots.pem','-attestation-revocations','data/trust/attestation-status.json')
    $owned+=@{id=$platform.Id;path=(Join-Path $PWD 'server.exe')}
    $business=Start-Process -FilePath (Join-Path $PWD 'demo-business.exe') -WorkingDirectory $PWD -WindowStyle Hidden -PassThru -RedirectStandardOutput logs/business.out.log -RedirectStandardError logs/business.err.log -ArgumentList @('-config','data/demo.json','-tls-cert','config/localhost-cert.pem','-tls-key','config/localhost-key.pem')
    $owned+=@{id=$business.Id;path=(Join-Path $PWD 'demo-business.exe')}
    ConvertTo-Json -InputObject @($owned) | Set-Content data/processes.json -Encoding UTF8
    & .\setup-demo.exe -check
    if($LASTEXITCODE -ne 0){throw 'Service startup failed. Read logs/*.err.log for the original error.'}
} catch {
    foreach($item in $owned){$p=Get-Process -Id $item.id -ErrorAction SilentlyContinue;if($p -and $p.Path -eq $item.path){Stop-Process -Id $p.Id}}
    throw
}
if(!$Quiet){
    $config=Get-Content data/demo.json -Raw | ConvertFrom-Json
    Write-Host 'Console: http://127.0.0.1:8099'
    Write-Host ('Admin token: '+(Get-Content data/admin-token -Raw))
    Write-Host ('App demo password: '+$config.password)
    Write-Host 'App account: choose a test account name. Phone: adb reverse tcp:9443 tcp:9443; adb reverse tcp:9444 tcp:9444'
}
