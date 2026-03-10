$ErrorActionPreference = 'Stop'

$packageName = 'tfoutdated'
$version = $env:chocolateyPackageVersion
$url64 = "https://github.com/AnassKartit/tfoutdated/releases/download/v${version}/tfoutdated_${version}_windows_amd64.zip"

$packageArgs = @{
  packageName    = $packageName
  unzipLocation  = "$(Split-Path -Parent $MyInvocation.MyCommand.Definition)"
  url64bit       = $url64
  checksumType64 = 'sha256'
  checksum64     = '[[CHECKSUM64]]'
}

Install-ChocolateyZipPackage @packageArgs
