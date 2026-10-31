# devx Dev Container Features

This directory contains a [Dev Container Feature](https://containers.dev/implementors/features/)
that installs the `devx` CLI into a dev container, so any `devcontainer.json`
can pull it in without hand-rolling an install script.

## Usage

Add it to your `devcontainer.json`:

```json
"features": {
    "ghcr.io/dever-labs/devx/cli:1": {}
}
```

Pin to a specific devx release instead of the latest one:

```json
"features": {
    "ghcr.io/dever-labs/devx/cli:1": {
        "version": "v1.2.3"
    }
}
```

The feature downloads the matching `devx` Linux binary (amd64/arm64) from
[GitHub Releases](https://github.com/dever-labs/devx/releases) and installs it
to `/usr/local/bin/devx` — no Go toolchain or build step required in the
container.

## Publishing

Features are packaged and pushed to `ghcr.io/dever-labs/devx/<feature-id>` by
`.github/workflows/publish-features.yml` whenever `features/src/**` changes on
`main`. Bump the `version` in `features/src/cli/devcontainer-feature.json`
(semver) to publish a new Feature version — the publish action skips versions
that are already published.

Per the [Features distribution spec](https://containers.dev/implementors/features-distribution/),
the published path is `<registry>/<namespace>/<id>`; `devcontainers/action`
defaults `namespace` to the full source repo path (`dever-labs/devx`), which
this repo uses as-is (no `features-namespace` override). The Feature's `id`
is kept distinct from the repo name (`cli`, not `devx`) specifically so the
published path reads as `dever-labs/devx/cli` — not the confusing, duplicated
`dever-labs/devx/devx` that would result if the id matched the repo name.
This also keeps the CLI's separate, auto-generated
`devcontainer-collection.json` push (always targets `<registry>/<namespace>`
with no `id` suffix — here `ghcr.io/dever-labs/devx`) meaningful: it's just
this repo's own package, not a generic throwaway name.
