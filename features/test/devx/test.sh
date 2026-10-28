#!/usr/bin/env bash
# Dev Container Feature test for devx.
# This test is run against an actual built dev container (see
# https://github.com/devcontainers/cli#features-test) by the "devcontainer
# features test" CLI command / the publish-features workflow.
set -e

# Import the dev container Features test library (provides `check` + `reportResults`).
source dev-container-features-test-lib

check "devx is on PATH" bash -c "command -v devx"
check "devx version runs" bash -c "devx version"

reportResults
