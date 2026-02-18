# dothuntcli Homebrew Release Playbook

This repo publishes binaries on tags and updates the Homebrew tap manually.

## 0) Prereqs
- Clean git tree on `main`.
- Tap repo available at `../homebrew-tap`.
- `gh` authenticated (`gh auth status`).

## 1) Create and push release tag
```sh
git checkout main
git pull
git tag v0.1.0
git push origin v0.1.0
```

This triggers `.github/workflows/release.yml` and creates GitHub release assets via GoReleaser.

## 2) Generate formula fields for the tag
```sh
scripts/release-homebrew.sh v0.1.0
```

Copy the printed `version`, `url`, and `sha256`.

## 3) Update the tap formula
Edit `../homebrew-tap/Formula/dothuntcli.rb` and replace:
- `version`
- `url`
- `sha256`

Then commit and push the tap:
```sh
git -C ../homebrew-tap add Formula/dothuntcli.rb README.md
git -C ../homebrew-tap commit -m "dothuntcli v0.1.0"
git -C ../homebrew-tap push origin main
```

## 4) Verify install
```sh
brew untap benithors/tap || true
brew tap benithors/tap
brew reinstall benithors/tap/dothuntcli
dothuntcli --version
brew test benithors/tap/dothuntcli
```
