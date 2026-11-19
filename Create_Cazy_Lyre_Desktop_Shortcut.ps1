$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$Target = Join-Path $ScriptDir 'Cazy_Lyre.exe'
$Icon = Join-Path $ScriptDir 'Cazy_Pink_Icon.ico'
$Desktop = [Environment]::GetFolderPath('Desktop')
$ShortcutPath = Join-Path $Desktop 'Cazy Lyre.lnk'
$WshShell = New-Object -ComObject WScript.Shell
$Shortcut = $WshShell.CreateShortcut($ShortcutPath)
$Shortcut.TargetPath = $Target
$Shortcut.WorkingDirectory = $ScriptDir
$Shortcut.IconLocation = "$Icon,0"
$Shortcut.Description = 'Cazy Lyre - AI + Macro Manager'
$Shortcut.Save()
Write-Host "Desktop shortcut created: $ShortcutPath"
