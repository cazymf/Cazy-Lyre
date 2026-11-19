# One-time bootstrap: publish Cazy Lyre as its OWN GitHub repository.
# Run in PowerShell after extracting the Windows/source release folder.
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $PSScriptRoot
$repo = 'cazymf/Cazy-Lyre'
Write-Host "Cazy Lyre | GitHub Releases setup" -ForegroundColor Cyan
Write-Host "Target: https://github.com/$repo (PUBLIC repository)"
if (-not (Get-Command git -ErrorAction SilentlyContinue)) { throw 'Install Git for Windows (https://git-scm.com/download/win), then run this script again.' }
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw 'Install GitHub CLI: winget install --id GitHub.cli -e; reopen PowerShell and rerun this script.' }
& gh auth status *> $null
if ($LASTEXITCODE -ne 0) {
    Write-Host 'Sign into YOUR GitHub account in the browser once.'
    & gh auth login --hostname github.com --web --git-protocol https
    if ($LASTEXITCODE -ne 0) { throw 'GitHub login failed.' }
}
$user = (& gh api user --jq '.login').Trim()
if ($user -ne 'cazymf') { throw "Signed into GitHub as '$user'; expected cazymf. Switch accounts before publishing." }
$confirmation = Read-Host "Create/publish a PUBLIC GitHub repository '$repo' and enable automatic releases? (type YES)"
if ($confirmation -cne 'YES') { Write-Host 'Canceled.'; exit 0 }
if (-not (Test-Path '.git')) {
    & git init -b main
    if ($LASTEXITCODE -ne 0) { throw 'git init failed.' }
}
& git config user.name 'cazymf'
& git config user.email '353448847+cazymf@users.noreply.github.com'
& git add .
& git commit -m 'Initialize Cazy Lyre GitHub updater v3.9.0'
if ($LASTEXITCODE -ne 0) { throw 'Could not create the initial source commit.' }
& gh repo view $repo --json name *> $null
if ($LASTEXITCODE -ne 0) {
    & gh repo create $repo --public --source . --remote origin --push
    if ($LASTEXITCODE -ne 0) { throw 'Could not create/push GitHub repository.' }
} else {
    & git remote get-url origin *> $null
    if ($LASTEXITCODE -ne 0) {
        & git remote add origin "https://github.com/$repo.git"
    } else {
        $remote = (& git remote get-url origin).Trim()
        if ($remote -ne "https://github.com/$repo.git" -and $remote -ne "git@github.com:$repo.git") { throw "Unexpected origin remote: $remote" }
    }
    & git push -u origin main
    if ($LASTEXITCODE -ne 0) { throw 'GitHub push failed; no force-push was attempted.' }
}
Write-Host 'Repository published. GitHub Actions will build and publish v3.9.0 automatically.' -ForegroundColor Green
Write-Host "https://github.com/$repo/actions"
Write-Host "https://github.com/$repo/releases"
Write-Host 'If ChatGPT cannot see this new repo, allow Cazy-Lyre in your GitHub connector repository access settings.'
