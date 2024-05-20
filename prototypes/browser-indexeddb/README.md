# Debugging

This demo is useful for debugging the IndexedDB tests.

See: https://blog.noops.land/debugging-webAssembly-from-go-sources-in-chrome-devtools

Download: https://chromewebstore.google.com/detail/cc++-devtools-support-dwa/pdcpmagijalfljmkmjngeonclgbbannb?pli=1

Finally, enable WebAssembly debugging in the DevTools Experiments. Open Chrome
DevTools, click the gear (⚙) icon in the top right corner of DevTools pane, go
to the Experiments panel and tick WebAssembly Debugging: Enable DWARF support:

Run `serve.bash` and browse to localhost:8080

In Chrome DevTools, open the Sources tab. In the Page panel, open main.go from
the file:// tree.

Note: breakpoints and stepping do not work quite as expected currently.

## Listing Keys in the DB

In the js console:

```js
window.listDbKeys()
```
