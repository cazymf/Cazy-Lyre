# Cazy Lyre v3.9 — GitHub Releases auto-updater

Cazy Lyre now checks **GitHub Releases** for `cazymf/Cazy-Lyre` automatically on startup (and via **Updates > CHECK FOR UPDATES**). When a newer release exists, use **DOWNLOAD + PREPARE**, then **INSTALL + RESTART**. Downloads are verified by SHA-256 before they can be installed. An older installation gets backed up; macros, locally encrypted API keys, instrument samples, and exports are not touched.

## First-time GitHub publishing (once)

1. Extract the **v3.9 Windows ZIP** to a permanent folder.
2. Install **Git for Windows** and **GitHub CLI** (via `winget install --id GitHub.cli -e`), if needed.
3. Run `SETUP_GITHUB_UPDATES.cmd` from that folder and sign in to GitHub once. It creates the public **cazymf/Cazy-Lyre** repository and uploads the *source code* (not a copy of your saved macros or credentials).
4. GitHub Actions automatically builds Windows, creates `v3.9.0`, and attaches `Cazy_Lyre_Windows.zip` and the checksum `.sha256` as release assets. Check the repo's **Actions** and **Releases** tabs for status.
5. If ChatGPT's GitHub connector doesn't show the new repo, add `Cazy-Lyre` to the connector's allowed repositories once.

The initial v3.9 release won't show an update if you already run v3.9. Future versions (v3.10, etc.) will appear inside the app automatically after GitHub Actions publishes them.

## Future update publishing

Modify the GitHub repo's source, increment the `lyreVersion` constant in `update_core.go` **and** the version in `release.json`, and commit to the `main` branch. GitHub Actions sees a *new version*, runs tests, builds the Windows EXE, packages the ZIP and `.sha256`, and creates the corresponding GitHub Release automatically. The app detects it the next time it starts or when Check for Updates is clicked. Changes can be made through GitHub with the connected ChatGPT GitHub plugin, provided the connector has write access to the new repository. No manual GitHub Releases upload needed.

The GitHub updater uses *public* releases because it authenticates anonymously. Private repos require a different authentication design (don't put a PAT/token into the app). The install always asks for approval; checking/downloading can occur without additional manual browser steps.

**Notes:** Actual Windows self-update must still be tested on your PC. `SETUP_GITHUB_UPDATES.cmd` prepares GitHub, and is separate from the in-app updater. Current connected GitHub access cannot create a new repo directly, so the one-time bootstrap runs on your PC. No release has been published until bootstrap runs successfully.
