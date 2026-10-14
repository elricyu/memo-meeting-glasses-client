# Requires Python 3 and Android Platform Tools (adb) in PATH.
$ErrorActionPreference = "Stop"
if (Get-Command py -ErrorAction SilentlyContinue) {
    & py -3 "$PSScriptRoot/scripts/manage.py" @args
} else {
    & python "$PSScriptRoot/scripts/manage.py" @args
}
exit $LASTEXITCODE
