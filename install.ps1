# GoCode installer for Windows.
# Usage (PowerShell): irm https://raw.githubusercontent.com/saurabh-sol/cli/main/install.ps1 | iex
$ErrorActionPreference = "Stop"
$repo = "saurabh-sol/cli"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }

$release = Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest"
$tag = $release.tag_name
$url = "https://github.com/$repo/releases/download/$tag/gocode_windows_$arch.zip"

$dir = Join-Path $env:LOCALAPPDATA "gocode"
New-Item -ItemType Directory -Force -Path $dir | Out-Null
$zip = Join-Path $env:TEMP "gocode.zip"
Write-Host "Downloading GoCode $tag for windows/$arch..."
Invoke-WebRequest $url -OutFile $zip
Expand-Archive $zip -DestinationPath $dir -Force
Remove-Item $zip

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dir*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
  $env:Path += ";$dir"
}
Write-Host "Installed to $dir"
Write-Host "Open a new terminal (Windows Terminal recommended) and run: gocode"
