import { SPACEWAVE_PUBLIC_BASE_URL } from '@s4wave/app/urls.js'

// FUNNEL_BEACON_SOURCE reports a visit to the landing, pricing or a
// quickstart page of the production website to the product funnel. Other
// hosts, such as staging and local builds, report nothing. A visit with an
// app route or a stored session boots the app, so it is not a landing.
export const FUNNEL_BEACON_SOURCE = `try{var p=location.pathname,e=p==='/'||p==='/pricing'?'LANDING':p.indexOf('/quickstart/')===0?'INTENT':'';if(e&&location.hostname===${JSON.stringify(new URL(SPACEWAVE_PUBLIC_BASE_URL).hostname)}&&location.hash.length<2&&!localStorage.getItem('spacewave-has-session'))navigator.sendBeacon('/api/funnel/event',e)}catch(_){}`

// FUNNEL_BEACON_SCRIPT is the inline script tag of FUNNEL_BEACON_SOURCE.
export const FUNNEL_BEACON_SCRIPT = `<script>${FUNNEL_BEACON_SOURCE}</script>`
