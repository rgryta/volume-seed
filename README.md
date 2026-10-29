# volume-seed

A static `/pause` binary for `scratch` sidecar images that share files with another container through anonymous volumes and `volumes_from`. For example, [homelab-tools](https://github.com/rgryta/homelab-tools) and [android-sdk](https://github.com/rgryta/android-sdk) give their toolchains to code-server in this way.

## The problem

Docker fills an anonymous volume from the image only when it creates the volume. A plain `docker compose up -d` after a tag change keeps the old volume. Thus the container starts on the new image but serves the old files. Before this tool, only `--renew-anon-volumes` fixed that, and dockge and wud do not use that flag.

## What it does

`/pause <target>...` does these steps for each target, then waits for a signal:

1. It calculates a fingerprint of `/seed<target>` (the path, mode, size and mtime of each entry).
2. If `<target>/.seed-stamp` has the same fingerprint, it does nothing.
3. If not, it empties `<target>`, copies `/seed<target>` into it, and writes the stamp last. If the copy stops before it completes, the next start copies again.

Mount points in a target stay. For example, a named volume at `/opt/tools/gcloud-config` and its contents are not removed.

## Use

```dockerfile
FROM scratch
COPY --from=ghcr.io/rgryta/volume-seed:1.0.0 /pause /pause
# An empty target directory that uid 1000 owns, so that uid 1000 can write to a new volume
COPY --from=ghcr.io/rgryta/volume-seed:1.0.0 --chown=1000:1000 /empty /opt/tools
# Put the payload under /seed, not at the target
COPY --from=builder --chown=1000:1000 /opt/tools/ /seed/opt/tools/
USER 1000:1000
CMD ["/pause", "/opt/tools"]
```

```yaml
services:
  tools:
    image: ghcr.io/you/tools:1.2.3
    volumes:
      - /opt/tools
  app:
    volumes_from: [tools]
```

## Release

Push a `vX.Y.Z` tag. The workflow then publishes `ghcr.io/rgryta/volume-seed:X.Y.Z` and `:X`. Each build runs `go vet` and `go test`.
