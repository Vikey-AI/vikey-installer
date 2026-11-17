# Vikey installer for Windows.
# Downloads the vikey CLI, checks its SHA-256, then starts the setup with a
# one-time code. The code works once and expires after 15 minutes. Your API
# key is never part of this script.
# Source: https://github.com/Vikey-AI/vikey-installer
#
# Runs through "irm ... | iex", so everything sits in a script block: no
# variable or preference leaks into your PowerShell session, and the window
# never closes on an error.
& {
  $ErrorActionPreference = 'Stop'
  $ProgressPreference = 'SilentlyContinue'
  $code = '__CODE__'
  $version = '__VERSION__'
  $panelUrl = '__PANEL_URL__'
  $downloadBase = '__DOWNLOAD_BASE__'
  $assets = @{
__ASSET_HASH__
  }

  function Stop-Vikey([string]$message) {
    Write-Host ''
    Write-Host "Vikey: $message" -ForegroundColor Red
  }

  $arch = $env:PROCESSOR_ARCHITEW6432
  if (-not $arch) { $arch = $env:PROCESSOR_ARCHITECTURE }
  switch ($arch) {
    'AMD64' { $arch = 'amd64' }
    'ARM64' { $arch = 'arm64' }
    default { Stop-Vikey "Arsitektur $arch belum didukung."; return }
  }
  $target = "windows-$arch"
  $sha = $assets[$target]
  if (-not $sha) { Stop-Vikey "Installer untuk $target belum tersedia."; return }

  # Windows PowerShell 5.1 on older Windows 10 builds may not offer TLS 1.2 by default.
  try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  } catch {}

  $dir = Join-Path $env:USERPROFILE '.vikey\bin'
  New-Item -ItemType Directory -Force -Path $dir | Out-Null
  $tmp = Join-Path $dir ".download-$PID.exe"
  $exe = Join-Path $dir 'vikey.exe'

  Write-Host "Mengunduh Vikey CLI $version untuk $target"
  try {
    Invoke-WebRequest -UseBasicParsing -Uri "$downloadBase/vikey-$target.exe" -OutFile $tmp
  } catch {
    Stop-Vikey "Gagal mengunduh. $($_.Exception.Message)"
    return
  }
  $got = (Get-FileHash -Algorithm SHA256 -LiteralPath $tmp).Hash.ToLower()
  if ($got -ne $sha) {
    Remove-Item -Force -LiteralPath $tmp
    Stop-Vikey 'File unduhan tidak cocok dengan checksum. Coba lagi beberapa saat lagi.'
    return
  }
  try {
    Move-Item -Force -LiteralPath $tmp -Destination $exe
  } catch {
    Remove-Item -Force -LiteralPath $tmp -ErrorAction SilentlyContinue
    Stop-Vikey 'vikey.exe sedang dipakai. Tutup jendela Vikey lain lalu jalankan perintahnya lagi.'
    return
  }

  # VIKEY_SETUP_ARGS adds flags, for example "--yes --tools claude,codex" in CI.
  $extra = @()
  if ($env:VIKEY_SETUP_ARGS) { $extra = @($env:VIKEY_SETUP_ARGS -split '\s+' | Where-Object { $_ }) }
  & $exe setup --code $code --panel-url $panelUrl @extra
}
