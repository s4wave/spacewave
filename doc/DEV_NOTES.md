## Running bldr on a custom branch

To ensure that all the Bldr components use the HEAD version of your branch:

```bash
rm -rf .bldr && bldr --disable-cleanup --bldr-version=$(git rev-parse HEAD) --bldr-version-sum="" start desktop
```

## Debugging with Delve

Debugging bldr:

```bash
cd ./cmd/bldr
# start web
dlv debug --wd  ../../ -- start web
# dist mode
dlv debug --backend=rr --wd=../../ -- --disable-cleanup --bldr-version=$(git rev-parse HEAD) --bldr-version-sum="" dist
```

Debugging a plugin:

 1. Add `delveAddr: wait` to the plugin compiler config.
 2. Run `bldr`
 3. cd to `./.bldr/plugin/dist/my-plugin`
 4. run the plugin with `dlv debug`

You can use `--backend=rr` if you have Mozilla RR installed to record the plugin
operations, then press ctrl c to interrupt and replay.
