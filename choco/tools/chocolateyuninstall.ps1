$ErrorActionPreference = 'Stop'

$packageName = 'tfoutdated'
$installDir = "$(Split-Path -Parent $MyInvocation.MyCommand.Definition)"

Remove-Item -Path "$installDir\tfoutdated.exe" -Force -ErrorAction SilentlyContinue
Remove-Item -Path "$installDir\tfoutdated-mcp.exe" -Force -ErrorAction SilentlyContinue
