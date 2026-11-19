param(
    [Parameter(Mandatory=$true)][string]$InstallDir,
    [Parameter(Mandatory=$true)][string]$StageDir,
    [Parameter(Mandatory=$true)][int]$ParentPid
)
$ErrorActionPreference = 'Stop'
$updates = Join-Path $env:LOCALAPPDATA 'Cazy Lyre\updates'
New-Item -Path $updates -ItemType Directory -Force | Out-Null
$timestamp = Get-Date -Format 'yyyyMMdd_HHmmss'
$backup = Join-Path $updates ("backup_" + $timestamp)
$log = Join-Path $updates 'last_update.log'
function Log([string]$s) { Add-Content -LiteralPath $log -Value ((Get-Date -Format o) + ' ' + $s); Write-Host $s }
Log "Starting update from $StageDir"
try {
    # After closing Cazy Lyre, allow time for Windows to release the running EXE.
    $deadline = (Get-Date).AddSeconds(70)
    while ((Get-Process -Id $ParentPid -ErrorAction SilentlyContinue) -and ((Get-Date) -lt $deadline)) { Start-Sleep -Milliseconds 350 }
    if (Get-Process -Id $ParentPid -ErrorAction SilentlyContinue) {throw 'Cazy Lyre did not exit before update timeout.'}
    if (-not (Test-Path -LiteralPath (Join-Path $StageDir 'Cazy_Lyre.exe'))) {throw 'Missing release executable.'}
    $source = [IO.Path]::GetFullPath($StageDir).TrimEnd('\') + '\'
    $target = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\') + '\'
    if (-not (Test-Path -LiteralPath $InstallDir)) {throw 'Original Cazy Lyre folder missing.'}
    New-Item -ItemType Directory -Path $backup -Force | Out-Null
    $overwritten = New-Object System.Collections.Generic.List[string]
    $created = New-Object System.Collections.Generic.List[string]
    $done = $false
    try {
        Get-ChildItem -LiteralPath $StageDir -File -Recurse | ForEach-Object {
            $rel = $_.FullName.Substring($source.Length)
            $relLower = $rel.ToLowerInvariant().Replace('/', '\')
            if ($relLower -eq 'library.json' -or $relLower.Contains('api_key') -or $relLower.StartsWith('.venv\') -or $relLower.StartsWith('instrument_samples\')) {throw "Protected user file: $rel"}
            $destination = Join-Path $InstallDir $rel
            $parent = Split-Path -Parent $destination
            New-Item -Path $parent -ItemType Directory -Force | Out-Null
            if (Test-Path -LiteralPath $destination) {
                $saved = Join-Path $backup $rel
                New-Item -Path (Split-Path -Parent $saved) -ItemType Directory -Force | Out-Null
                Copy-Item -LiteralPath $destination -Destination $saved -Force
                $overwritten.Add($rel)
            } else { $created.Add($rel) }
            Copy-Item -LiteralPath $_.FullName -Destination $destination -Force
        }
        $done = $true
    } finally {
        if (-not $done) {
            Log 'Update failed; restoring original files.'
            foreach ($rel in $overwritten) {
                $restore = Join-Path $backup $rel
                if (Test-Path -LiteralPath $restore) { Copy-Item -LiteralPath $restore -Destination (Join-Path $InstallDir $rel) -Force }
            }
            foreach ($rel in $created) { Remove-Item -LiteralPath (Join-Path $InstallDir $rel) -Force -ErrorAction SilentlyContinue }
        }
    }
    Log "Updated successfully. Previous overwritten files backed up at $backup"
    $exe = Join-Path $InstallDir 'Cazy_Lyre.exe'
    Start-Process -FilePath $exe -WorkingDirectory $InstallDir
    Log 'Restarted updated Cazy Lyre.'
} catch {
    Log ("UPDATE FAILED: " + $_.Exception.Message)
    Write-Host 'See last_update.log for details.'
    Start-Sleep -Seconds 5
    exit 1
} finally {
    Remove-Item -LiteralPath $StageDir -Recurse -Force -ErrorAction SilentlyContinue
}
