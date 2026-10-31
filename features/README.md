# devx Dev Container Features

This directory contains a [Dev Container Feature](https://containers.dev/implementors/features/)
that installs the `devx` CLI into a dev container, so any `devcontainer.json`
can pull it in without hand-rolling an install script.

## Usage

Add it to your `devcontainer.json`:

```json
"features": {
    "ghcr.io/dever-labs/features/devx:1": {}
}
```

Pin to a specific devx release instead of the latest one:

```json
"features": {
    "ghcr.io/dever-labs/features/devx:1": {
        "version": "v1.2.3"
    }
}
```

The feature downloads the matching `devx` Linux binary (amd64/arm64) from
[GitHub Releases](https://github.com/dever-labs/devx/releases) and installs it
to `/usr/local/bin/devx` — no Go toolchain or build step required in the
container.

## Publishing

Features are packaged and pushed to `ghcr.io/dever-labs/features/<feature-id>`
by `.github/workflows/publish-features.yml` whenever `features/src/**` changes
on `main`. Bump the `version` in `features/src/devx/devcontainer-feature.json`
(semver) to publish a new Feature version — the publish action skips versions
that are already published.

Per the [Features distribution spec](https://containers.dev/implementors/features-distribution/),
the published path is `<registry>/<namespace>/<id>`, and `devcontainers/action`
defaults `namespace` to the full source repo path (`dever-labs/devx`). This
repo overrides `features-namespace` to `dever-labs/features` instead, because
this repo's name happens to equal its one Feature's `id` (`devx`), and the
default would otherwise publish to the confusing, duplicated
`ghcr.io/dever-labs/devx/devx`. `dever-labs/features` keeps the namespace at
two path segments, which also keeps the CLI's separate, auto-generated
`devcontainer-collection.json` push (always targets `<registry>/<namespace>`
with no `id` suffix) valid — GHCR rejects single-segment package names with
`NAME_INVALID`.
