// ROOT_BOOT_VISIBILITY_SCRIPT selects the loading shell before first paint
// when the URL or stored session requires application startup.
export const ROOT_BOOT_VISIBILITY_SCRIPT =
  "<script>try{if(location.hash.length>1||localStorage.getItem('spacewave-has-session'))document.documentElement.setAttribute('data-sw-boot-visibility','loading')}catch(_){}</script>"

export const ROOT_BOOT_VISIBILITY_CSS =
  'html[data-sw-boot-visibility="loading"] #sw-landing{display:none!important}html[data-sw-boot-visibility="loading"] #sw-loading{display:block!important}'

// ROOT_LOADING_STYLE keeps the hidden first-start loading shell sized as a
// full-width flex item after hydrate.tsx or boot.mjs reveals it.
export const ROOT_LOADING_STYLE =
  'display:none;flex:1 1 0%;width:100%;min-width:0'
