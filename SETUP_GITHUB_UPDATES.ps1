# Cazy Lyre GitHub publishing bootstrap — repaired 2026-11-19.
# Place this file beside Cazy_Lyre.exe and SETUP_GITHUB_UPDATES.cmd.
$ErrorActionPreference = 'Continue' # Native git/gh write normal status to stderr in Windows PowerShell 5.1.
Set-Location -LiteralPath $PSScriptRoot
$repo = 'cazymf/Cazy-Lyre'
Write-Host 'Cazy Lyre | GitHub publishing setup (FIXED)' -ForegroundColor Cyan
Write-Host "Target: https://github.com/$repo"

if (-not (Get-Command git -ErrorAction SilentlyContinue)) { throw 'Git for Windows is missing: https://git-scm.com/download/win' }
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) { throw 'GitHub CLI missing. Install via winget install --id GitHub.cli -e, then reopen the terminal.' }

# A missing GitHub repository is expected on first run. Use a successful list
# operation rather than a failing `gh repo view`, which terminates PowerShell 5.1.
$loggedInUser = & gh api user --jq '.login'
if ($LASTEXITCODE -ne 0) { throw 'GitHub login needed. Run: gh auth login' }
$loggedInUser = ($loggedInUser | Out-String).Trim()
if ($loggedInUser -ne 'cazymf') { throw "GitHub is signed in as '$loggedInUser'; expected 'cazymf'." }

if (-not (Test-Path -LiteralPath (Join-Path $PSScriptRoot 'release.json'))) {
    throw 'release.json not found. Put both setup files inside the extracted Cazy_Lyre_v3_9_0 folder before running.'
}

$answer = Read-Host "Publish source files to PUBLIC repository '$repo'? (type YES)"
if ($answer -ine 'YES') { Write-Host 'Canceled. Nothing published.'; exit 0 }

if (-not (Test-Path -LiteralPath '.git')) {
    & git init -b main
    if ($LASTEXITCODE -ne 0) { throw 'Git init failed.' }
}
& git config user.name 'cazymf'
& git config user.email '353448847+cazymf@users.noreply.github.com'
if ($LASTEXITCODE -ne 0) { throw 'Git config failed.' }

# The original setup may have created the commit already: do not fail or duplicate it.
& git add -A
if ($LASTEXITCODE -ne 0) { throw 'git add failed.' }
& git diff --cached --quiet
$stagedDiffCode = $LASTEXITCODE
if ($stagedDiffCode -eq 1) {
    & git commit -m 'Initialize/update Cazy Lyre GitHub auto-updater'
    if ($LASTEXITCODE -ne 0) { throw 'Git commit failed.' }
} elseif ($stagedDiffCode -eq 0) {
    Write-Host 'Already committed. Reusing existing local commit.' -ForegroundColor DarkCyan
} else { throw 'Git staging inspection failed.' }

# The repo is not currently published; querying the full repo list is non-error.
$reposJson = & gh repo list cazymf --limit 1000 --json name
if ($LASTEXITCODE -ne 0) { throw 'Could not list repositories. Run gh auth status.' }
$repos = @()
if (($reposJson | Out-String).Trim()) { $repos = @($reposJson | ConvertFrom-Json) }
$exists = [bool](@($repos | Where-Object { $_.name -eq 'Cazy-Lyre' }).Count)

if (-not $exists) {
    Write-Host 'Creating GitHub repository and publishing local commit...' -ForegroundColor Cyan
    & gh repo create $repo --public --source . --remote origin --push
    if ($LASTEXITCODE -ne 0) { throw 'GitHub repository creation/push failed. Check the error printed above.' }
} else {
    Write-Host 'Repository exists: syncing the source...' -ForegroundColor Cyan
    $remoteNames = @(& git remote)
    if ($LASTEXITCODE -ne 0) { throw 'Could not list git remotes.' }
    if ($remoteNames -notcontains 'origin') {
        & git remote add origin "https://github.com/$repo.git"
        if ($LASTEXITCODE -ne 0) { throw 'Could not set GitHub remote.' }
    } else {
        $origin = (& git remote get-url origin | Out-String).Trim()
        if ($LASTEXITCODE -ne 0) { throw 'Could not read GitHub remote.' }
        if ($origin -ne "https://github.com/$repo.git" -and $origin -ne "git@github.com:$repo.git") {
            throw "Refusing to push to an unexpected remote: $origin"
        }
    }
    & git push -u origin main
    if ($LASTEXITCODE -ne 0) { throw 'Git push failed. Check the error printed above.' }
}

Write-Host ''
Write-Host 'SUCCESS: Source published to GitHub.' -ForegroundColor Green
Write-Host "https://github.com/$repo"
Write-Host "https://github.com/$repo/actions"
Write-Host "https://github.com/$repo/releases"
Write-Host 'The first release is built by GitHub Actions; check its Actions tab for status.'
Write-Host 'Grant the ChatGPT GitHub connector permission to this repository for future updates.'
