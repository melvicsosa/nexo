# nexo installer for Windows PowerShell: downloads the right prebuilt binary
# from the latest GitHub release, verifies its checksum and puts it on your
# PATH. No Go required.
#
#   irm https://raw.githubusercontent.com/melvicsosa/nexo/main/scripts/install.ps1 | iex
#
# Options via environment variables:
#   NEXO_VERSION      tag to install (default: latest release, e.g. v0.1.2)
#   NEXO_INSTALL_DIR  target directory (default: %LOCALAPPDATA%\Programs\nexo)
$ErrorActionPreference = "Stop"

$repo = "melvicsosa/nexo"

# --- platform ---------------------------------------------------------------
$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
    "AMD64" { "amd64" }
    "ARM64" { "arm64" }
    default { throw "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
}

# --- version ----------------------------------------------------------------
$version = $env:NEXO_VERSION
if (-not $version) {
    $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
    if (-not $version) { throw "could not resolve the latest release tag" }
}
$bare = $version.TrimStart("v")

# --- download and verify ----------------------------------------------------
$archive = "nexo_${bare}_windows_${arch}.zip"
$base = "https://github.com/$repo/releases/download/$version"
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) "nexo-install-$([System.Guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmp | Out-Null

try {
    Write-Host "downloading nexo $version (windows/$arch)..."
    Invoke-WebRequest -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive)
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp "checksums.txt")

    $expected = (Select-String -Path (Join-Path $tmp "checksums.txt") -Pattern ([regex]::Escape($archive))).Line.Split(" ")[0]
    $actual = (Get-FileHash -Algorithm SHA256 (Join-Path $tmp $archive)).Hash.ToLower()
    if ($expected -ne $actual) { throw "checksum verification failed for $archive" }

    Expand-Archive -Force (Join-Path $tmp $archive) $tmp

    # --- install ------------------------------------------------------------
    $dir = $env:NEXO_INSTALL_DIR
    if (-not $dir) { $dir = Join-Path $env:LOCALAPPDATA "Programs\nexo" }
    New-Item -ItemType Directory -Force -Path $dir | Out-Null
    Copy-Item -Force (Join-Path $tmp "nexo.exe") (Join-Path $dir "nexo.exe")

    $installed = & (Join-Path $dir "nexo.exe") -version
    Write-Host "installed $installed to $dir\nexo.exe"

    # Add the install dir to the user PATH when it is not there yet.
    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if (($userPath -split ";") -notcontains $dir) {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$dir", "User")
        Write-Host "added $dir to your user PATH. Open a new terminal to pick it up."
    }
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
