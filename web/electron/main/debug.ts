// BLDR_DEBUG is set if this is a debug build.
declare const BLDR_DEBUG: boolean | undefined

// debugWhenReady is either a stub or the debug function depending on BLDR_DEBUG.
let debugWhenReady: () => void

if (BLDR_DEBUG) {
  debugWhenReady = () => {
    // Not working: Electron doesn't support devtools v3.
    // https://github.com/electron/electron/issues/36545
    // https://github.com/electron/electron/issues/37876
    /*
    installExtension(REACT_DEVELOPER_TOOLS, {
      loadExtensionOptions: {
        allowFileAccess: true,
      }
    }).then((name) => console.log(`Added Electron extension: ${name}`))
      .catch((err) => console.log('An error occurred: ', err));
    */
  }
} else {
  debugWhenReady = () => {}
}

export default debugWhenReady
