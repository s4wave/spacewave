import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix, resolve } from 'node:path'

/** writeDividersFixture retains complete class-based row families from the original interface. */
export async function writeDividersFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Compare the complete row cascade, including its former parent-owned borders.
  for (const file of [
    'web/forge/ForgeEntityList.tsx',
    'web/transform/TransformConfigDisplay.tsx',
    'web/debug/ForgeViewerDebug.tsx',
    'web/debug/HDRDebug.tsx',
    'web/debug/UnixFSBrowserDebug.tsx',
    'web/forge/ForgeViewerShell.tsx',
    ...(version === 'before' ? ['web/ui/DashboardButton.tsx'] : []),
  ]) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/dividers-${basename(file)}`
        : file,
      'utf8',
    )
    const relocated = source.replace(
      /(from\s*['"])(\.[^'"]+)(['"])/g,
      (_, prefix, ref, suffix) =>
        `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
    )
    await writeFile(
      join(root, basename(file)),
      version === 'before'
        ? relocated
            .replaceAll("'@s4wave/web/ui/tooltip.js'", "'./tooltip'")
            .replaceAll(
              "'@s4wave/web/ui/DashboardButton.js'",
              "'./DashboardButton'",
            )
            .replaceAll("'@s4wave/web/ui/button.js'", "'./button'")
        : relocated,
    )
  }

  // Static transform metadata reaches both the encryption and the ordinary row layouts.
  await writeFile(
    join(root, 'DividerSamples.tsx'),
    `
import React from'react';import * as stylex from'@stylexjs/stylex';
import{UnixFSBrowserDebug}from'./UnixFSBrowserDebug';import{ForgeViewerShell}from'./ForgeViewerShell';import{StateNamespaceProvider,atom}from'@s4wave/web/state/persist.js';import{HDRDebug}from'./HDRDebug';import{ForgeEntityList}from'./ForgeEntityList';import{TransformConfigDisplay}from'./TransformConfigDisplay';import{ForgeViewerDebug}from'./ForgeViewerDebug';import{RouterProvider}from'@s4wave/web/router/router.js';
import{LuBox}from${JSON.stringify(resolve('web/images/icons.ts'))};
const state=atom({});const icon=stylex.create({small:{width:14,height:14}});const entities=['first','second','third'].map(name=>({objectKey:'task/'+name,typeId:'example-task'}));
export function DividerSamples(){const mode=new URLSearchParams(location.search).get('dividers')??'default';return <main id="divider-fixtures" style={{padding:16,width:'100%',display:'grid',gap:16,alignContent:'start'}}>{mode.startsWith('unixfs')?<div style={{height:900,width:'100%'}}><RouterProvider path="/" onNavigate={()=>{}}><UnixFSBrowserDebug/></RouterProvider></div>:mode.startsWith('shell')?<StateNamespaceProvider rootAtom={state}><div style={{height:700,width:'100%'}}><ForgeViewerShell title="Job" stateKey="parity" icon={<LuBox sx={icon.small}/>} headerActions={[{label:'Create Job',onClick:()=>{}}]} actions={[{label:'Start Worker',onClick:()=>{}}]} tabs={[{id:'overview',label:'Overview',content:'Overview'},{id:'tasks',label:'Tasks',content:'Tasks'}]}/></div></StateNamespaceProvider>:mode.startsWith('hdr')?<div style={{height:900,width:'100%'}}><RouterProvider path="/" onNavigate={()=>{}}><HDRDebug/></RouterProvider></div>:mode.startsWith('forge')?<div style={{height:900,width:'100%'}}><RouterProvider path="/" onNavigate={()=>{}}><ForgeViewerDebug/></RouterProvider></div>:<><ForgeEntityList entities={[]} loading={false} icon={<LuBox sx={icon.small}/>}/><ForgeEntityList entities={entities.slice(0,1)} loading={false} icon={<LuBox sx={icon.small}/>}/><ForgeEntityList entities={entities} loading={false} icon={<LuBox sx={icon.small}/>}/><TransformConfigDisplay info={{steps:[{id:'hydra/transform/lz4'},{id:'hydra/transform/chksum'}],storageBytes:1024n,grantCount:1}}/><TransformConfigDisplay info={{steps:[{id:'hydra/transform/gzip'},{id:'hydra/transform/blockenc'}],storageBytes:4096n,grantCount:3}}/></>}</main>}
`,
  )
}
