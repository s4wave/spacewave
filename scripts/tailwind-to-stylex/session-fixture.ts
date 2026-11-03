import { readFile, writeFile } from 'node:fs/promises'
import { dirname, join, posix, resolve } from 'node:path'

/** writeSessionFixture compares the real recovery card across its visible health and action states. */
export async function writeSessionFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Expose the existing private card only in this build, without adding a production seam.
  const file = 'app/session/SessionSharedObjectContainer.tsx'
  const source = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/session-SessionSharedObjectContainer.tsx'
      : file,
    'utf8',
  )
  await writeFile(
    join(root, 'SessionHealth.tsx'),
    source
      .replaceAll(
        "'@s4wave/web/images/icons.js'",
        JSON.stringify(resolve('web/images/icons.ts')),
      )
      .replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ) + '\nexport {SharedObjectHealthCard}\n',
  )

  // Keep each original input and its shared class recipe together.
  for (const name of ['OptionalPinLock', 'auth-flow-shared']) {
    const file = 'app/provider/spacewave/' + name + '.tsx'
    let source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/session-' +
            name +
            '.tsx'
        : file,
      'utf8',
    )
    source = source
      .replaceAll(
        "'@s4wave/web/images/icons.js'",
        JSON.stringify(resolve('web/images/icons.ts')),
      )
      .replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      )
    source = source.replaceAll(
      "'@s4wave/app/provider/spacewave/auth-flow-shared.js'",
      "'./auth-flow-shared'",
    )
    await writeFile(join(root, name + '.tsx'), source)
  }

  // The card receives the same protobuf states and callbacks as its watched route owner.
  await writeFile(
    join(root, 'SessionSamples.tsx'),
    `
import React from'react';import{OptionalPinLock}from'./OptionalPinLock';import{SharedObjectHealthCard}from'./SessionHealth';import{RouterProvider}from'@s4wave/web/router/router.js';
import{SharedObjectHealthStatus as Status,SharedObjectHealthLayer as Layer,SharedObjectHealthCommonReason as Reason,SharedObjectHealthRemediationHint as Hint}from'@s4wave/core/sobject/sobject.pb.js';
const action=()=>{};
export function SessionSamples(){const mode=new URLSearchParams(location.search).get('session')??'closed';const health={status:mode==='loading'?Status.LOADING:mode==='degraded'?Status.DEGRADED:Status.CLOSED,layer:Layer.SHARED_OBJECT,commonReason:Reason.INITIAL_STATE_REJECTED,remediationHint:Hint.RETRY,error:'The root signature could not be verified.'};if(mode.startsWith('pin'))return <main id="session-fixtures" style={{padding:24,maxWidth:440}}><OptionalPinLock pin="" confirmPin="" pinError={mode==='pin-error'||mode==='pin-error-focus'?'PINs do not match':''} onPinChange={action} onConfirmPinChange={action} onSubmit={action} disabled={mode==='pin-disabled'} pinInputId="parity-pin" keyHeldWithoutPin/></main>;return <main id="session-fixtures" style={{height:900,width:'100%'}}><RouterProvider path="/" onNavigate={action}><SharedObjectHealthCard health={health} onRetry={action} onRepair={action} onReinitialize={action} onBack={action} mutationPermission={{canMutate:mode!=='forbidden',disabledReason:mode==='forbidden'?'Owner access required':''}} mutationPending={mode==='pending'} mutationError={mode==='error'?'Recovery request is unavailable.':''}/></RouterProvider></main>}
`,
  )
}
