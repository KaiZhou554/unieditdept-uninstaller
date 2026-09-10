# Build ued-uninstaller and inject today's date as the version number.
#
# The version looks like 260910 (yyMMdd) and is shown at the top-right of the
# TUI, as well as by --version. The same date is embedded into the Windows
# VERSIONINFO resource, so Explorer's Properties -> Details tab is filled in.
# Kept ASCII-only on purpose: Windows PowerShell 5.1 reads script files as
# ANSI and would mangle non-ASCII text.
$ErrorActionPreference = "Stop"

$root = $PSScriptRoot
$date = Get-Date -Format "yyMMdd"
$pkg = "github.com/unieditdept/ued-uninstaller/internal/version"

# Windows version resources need four numeric components; yy/MM/dd map onto
# Major/Minor/Patch and Build stays 0.
$major = [int]$date.Substring(0, 2)
$minor = [int]$date.Substring(2, 2)
$patch = [int]$date.Substring(4, 2)

$template = Join-Path $root "versioninfo.json"
$generated = Join-Path $root "versioninfo.gen.json"

$json = Get-Content $template -Raw -Encoding UTF8
$json = $json.Replace("__MAJOR__", "$major")
$json = $json.Replace("__MINOR__", "$minor")
$json = $json.Replace("__PATCH__", "$patch")
# The string version fields must be dotted (x.y.z); goversioninfo rejects the
# compact yyMMdd form and would leave them empty.
$json = $json.Replace("__VERSION_DOTTED__", "$major.$minor.$patch")

# Write without a BOM: goversioninfo rejects a leading UTF-8 BOM.
$utf8 = New-Object System.Text.UTF8Encoding($false)
[System.IO.File]::WriteAllText($generated, $json, $utf8)

try {
    # -platform-specific writes resource_windows_<arch>.syso next to main.go;
    # go build picks .syso files up automatically. The platform suffix keeps
    # non-Windows builds from trying to link a PE resource.
    go run github.com/josephspurrier/goversioninfo/cmd/goversioninfo@v1.7.0 `
        -platform-specific $generated
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

    go build -ldflags "-s -w -X $pkg.buildDate=$date" -o (Join-Path $root "ued-uninstaller.exe") $root
    if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
}
finally {
    Remove-Item $generated -ErrorAction SilentlyContinue
}

Write-Host "Built ued-uninstaller.exe (version $date)"
