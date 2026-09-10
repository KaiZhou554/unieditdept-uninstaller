# Build ued-uninstaller and inject today's date as the version number.
#
# The version looks like 260910 (yyMMdd) and is shown at the bottom-right of
# the TUI, as well as by --version. Kept ASCII-only on purpose: Windows
# PowerShell 5.1 reads script files as ANSI and would mangle non-ASCII text.
$ErrorActionPreference = "Stop"

$date = Get-Date -Format "yyMMdd"
$pkg = "github.com/unieditdept/ued-uninstaller/internal/version"

go build -ldflags "-s -w -X $pkg.buildDate=$date" -o ued-uninstaller.exe .
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

Write-Host "Built ued-uninstaller.exe (version $date)"
