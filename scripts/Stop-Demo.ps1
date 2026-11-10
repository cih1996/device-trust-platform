$ErrorActionPreference='Stop'
Set-Location $PSScriptRoot
if(!(Test-Path -LiteralPath data/processes.json)){throw 'No process record exists for this package'}
$items=Get-Content data/processes.json -Raw | ConvertFrom-Json
foreach($item in $items){
    $p=Get-Process -Id $item.id -ErrorAction SilentlyContinue
    if($p){
        if($p.Path -ne $item.path){throw "Process $($item.id) belongs to another executable"}
        Stop-Process -Id $p.Id
    }
}
Remove-Item -LiteralPath data/processes.json
Write-Host 'Demo stopped.'
