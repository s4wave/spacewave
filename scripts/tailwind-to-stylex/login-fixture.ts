import { readFile, writeFile } from 'node:fs/promises'
import { join, resolve } from 'node:path'

/** writeLoginFixture compares the real login lifecycle without making authentication requests. */
export async function writeLoginFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the actual form implementation and the original child styling contracts.
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/login-form.tsx'
      : 'web/ui/login-form.tsx',
    'utf8',
  )
  let output = source.replace('@s4wave/app/urls.js', resolve('app/urls.ts'))
  if (version === 'before') {
    // Preserve the class-based child contracts consumed by the original form.
    for (const name of ['Spinner', 'ProgressBar', 'AuthProgressCard']) {
      let child = await readFile(
        `scripts/tailwind-to-stylex/testdata/components/${name}.tsx`,
        'utf8',
      )
      for (const dependency of ['Spinner', 'ProgressBar'])
        child = child.replace(
          `@s4wave/web/ui/loading/${dependency}.js`,
          `./login-${dependency}`,
        )
      await writeFile(join(root, `login-${name}.tsx`), child)
      output = output.replace(
        `@s4wave/web/ui/${name === 'AuthProgressCard' ? 'credential' : 'loading'}/${name}.js`,
        `./login-${name}`,
      )
    }
  }
  await writeFile(join(root, 'login-form.tsx'), output)
  await writeFile(
    join(root, 'LoginSamples.tsx'),
    `
import React from 'react';
import {LoginForm} from './login-form';
export function LoginSamples(){const mode=new URLSearchParams(location.search).get('login');return <main id="login-fixtures" style={{padding:16,maxWidth:380}}><LoginForm initialUsername="alice" cloudProviderConfig={{turnstileSiteKey:'1x00000000000000000000AA',googleSsoEnabled:true,githubSsoEnabled:true}} onContinueWithoutAccount={()=>{}} onContinueWithPasskey={()=>{}} onSignInWithSSO={()=>{}} onLoginWithPassword={async()=>mode==='busy'?new Promise(()=>{}):mode==='error'?{type:'error',errorCode:'wrong_password'}:{type:'new_account'}} onCreateAccountWithPassword={async()=>({sessionIndex:1})}/></main>}
`,
  )
}
