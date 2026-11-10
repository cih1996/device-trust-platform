param(
    [Parameter(Mandatory=$true)][string]$NdkPath,
    [Parameter(Mandatory=$true)][string]$ObfuscatorPath
)
$ErrorActionPreference='Stop'
$repoRoot=Split-Path $PSScriptRoot -Parent
$source=Join-Path $repoRoot 'android/sdk-secure/src/main/cpp/frame.cpp'
$output=Join-Path $repoRoot 'android/sdk-secure/src/main/jniLibs'
$buildRoot=Join-Path $repoRoot 'work/native'
$stock=Join-Path $NdkPath 'toolchains/llvm/prebuilt/windows-x86_64'
$clang=Join-Path $ObfuscatorPath 'bin/clang++.exe'
$opt=Join-Path $ObfuscatorPath 'bin/opt.exe'
$abis=@(
    @{Name='arm64-v8a';Target='aarch64-linux-android24';Include='aarch64-linux-android'},
    @{Name='armeabi-v7a';Target='armv7a-linux-androideabi24';Include='arm-linux-androideabi'},
    @{Name='x86_64';Target='x86_64-linux-android24';Include='x86_64-linux-android'},
    @{Name='x86';Target='i686-linux-android24';Include='i686-linux-android'}
)
New-Item -ItemType Directory -Force -Path $buildRoot | Out-Null
$manifest=@()
foreach($abi in $abis){
    $name=$abi.Name; $dir=Join-Path $output $name; New-Item -ItemType Directory -Force -Path $dir | Out-Null
    $ir=Join-Path $buildRoot ($name+'.ll'); $obfuscated=Join-Path $buildRoot ($name+'.obfuscated.bc'); $object=Join-Path $buildRoot ($name+'.o')
    $flags=@(('--target='+$abi.Target),('--sysroot='+$stock+'/sysroot'),'-isystem',($stock+'/sysroot/usr/include/'+$abi.Include),'-O1','-fPIC','-fvisibility=hidden','-fno-exceptions','-fno-rtti','-fstack-protector-strong')
    & $clang @flags -S -emit-llvm $source -o $ir
    if($LASTEXITCODE -ne 0){throw ('IR compilation failed: '+$name)}
    & $opt '-passes=obfuscation,obf-metrics' '-obf-seed=41971' $ir -o $obfuscated > (Join-Path $buildRoot ($name+'.obfuscation.log'))
    if($LASTEXITCODE -ne 0){throw ('Obfuscation failed: '+$name)}
    # The released obfuscator injects portable crypto IR with its Windows-host
    # CPU attributes. Remove those attributes before Android code generation;
    # the requested Android target determines the actual instruction set.
    $portable=Join-Path $buildRoot ($name+'.portable.ll')
    & $opt -S $obfuscated -o $portable
    if($LASTEXITCODE -ne 0){throw ('IR export failed: '+$name)}
    $text=[IO.File]::ReadAllText($portable)
    $text=[regex]::Replace($text,'"(?:target-cpu|tune-cpu|target-features)"="[^"]*"','')
    [IO.File]::WriteAllText($portable,$text,[Text.UTF8Encoding]::new($false))
    & $clang ('--target='+$abi.Target) ('--sysroot='+$stock+'/sysroot') -O1 -fPIC -fstack-protector-strong -c $portable -o $object
    if($LASTEXITCODE -ne 0){throw ('Object compilation failed: '+$name)}
    & ($stock+'/bin/clang++.exe') ('--target='+$abi.Target) ('--sysroot='+$stock+'/sysroot') -shared -nostdlib++ $object '-Wl,-z,relro,-z,now,-z,noexecstack,-z,max-page-size=16384' ('-Wl,--version-script='+$repoRoot+'/android/sdk-secure/src/main/cpp/exports.map') -o ($dir+'/libdtpsecure.so')
    if($LASTEXITCODE -ne 0){throw ('Native linking failed: '+$name)}
    & ($stock+'/bin/llvm-strip.exe') --strip-unneeded ($dir+'/libdtpsecure.so')
    if($LASTEXITCODE -ne 0){throw ('Native stripping failed: '+$name)}
    $manifest+=@{abi=$name;sha256=(Get-FileHash ($dir+'/libdtpsecure.so') -Algorithm SHA256).Hash;obfuscator='xollvm-v0.12.0-llvm22';pageAlignment=16384}
}
$manifest|ConvertTo-Json -Depth 4|Set-Content (Join-Path $buildRoot 'manifest.json') -Encoding utf8
Write-Output 'Four obfuscated Android native libraries built.'
