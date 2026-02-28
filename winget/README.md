# WinGet manifests for devx
#
# To submit a new release to winget-pkgs, create a PR to microsoft/winget-pkgs
# adding these three files under:
#   manifests/d/dever-labs/devx/<version>/
#
# The release workflow (.github/workflows/release.yml) can be extended to
# auto-submit this PR using the wingetcreate tool.
#
# Template files:
#   dever-labs.devx.yaml          — version manifest
#   dever-labs.devx.installer.yaml — installer manifest
#   dever-labs.devx.locale.en-US.yaml — locale manifest
#
# Replace {{VERSION}} and {{SHA256_*}} placeholders with real values.
