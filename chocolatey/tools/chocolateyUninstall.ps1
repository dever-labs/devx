$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
Remove-Item "$toolsDir\devx.exe" -Force -ErrorAction SilentlyContinue
