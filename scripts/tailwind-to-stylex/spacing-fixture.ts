import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix, resolve } from 'node:path'

/** writeSpacingFixture compares actual settings and keybinding child owners. */
export async function writeSpacingFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the coupled keybinding children local while sharing their existing context.
  const files = [
    'web/debug/SessionSettingsDebug.tsx',
    'web/command/KeybindingBindingsSection.tsx',
    'web/command/KeybindingConflictList.tsx',
    'web/command/KeybindingCommandDetails.tsx',
  ]
  const names = new Set(files.map((file) => basename(file)))
  for (const file of files) {
    const source = await readFile(
      version === 'before'
        ? `scripts/tailwind-to-stylex/testdata/components/spacing-${basename(file)}`
        : file,
      'utf8',
    )
    const relocated = source.replace(
      /(from\s*['"])(\.[^'"]+)(['"])/g,
      (whole, prefix, ref, suffix) => {
        const name = basename(ref).replace(/\.js$/, '.tsx')
        return names.has(name)
          ? `${prefix}./${basename(ref)}${suffix}`
          : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`
      },
    )
    await writeFile(
      join(root, basename(file)),
      version === 'before'
        ? relocated.replaceAll(
            "'@s4wave/web/images/icons.js'",
            JSON.stringify(resolve('web/images/icons.ts')),
          )
        : relocated,
    )
  }

  // Exercise mutually exclusive child lists without replacing the production rendering code.
  await writeFile(
    join(root, 'SpacingSamples.tsx'),
    `
import React from 'react';
import{SessionSettingsDebug}from'./SessionSettingsDebug';import{KeybindingCommandDetails}from'./KeybindingCommandDetails';
import{KeybindingEditorContext}from'@s4wave/web/command/KeybindingEditorContext.js';
import type{KeybindingEditorContextValue,KeybindingLayerController}from'@s4wave/web/command/component.js';
import type{ResolvedCommandBinding}from'@s4wave/web/command/KeybindingResolver.js';
import{CommandFocusContext,CommandSurface}from'@s4wave/sdk/command/command.pb.js';
import{RouterProvider}from'@s4wave/web/router/router.js';import{StateNamespaceProvider,atom}from'@s4wave/web/state/persist.js';
const noop=()=>{};const state=atom({});
const controller:KeybindingLayerController={scope:'local',label:'Local',overrideSet:{overrides:{},settings:{}},layer:null,available:true,readOnly:false,loading:false,error:null,settingsEditable:true,setSettings:noop,setOverrideSet:noop,setCommandBindings:noop,addCommandBinding:noop,clearCommandBindings:noop,clearCommandBindingId:noop,removeCommandBinding:noop,resetCommand:noop,resetLayer:noop};
const bindings:ResolvedCommandBinding[]=['CmdOrCtrl+K','CmdOrCtrl+P'].map((combo,index)=>({commandId:'parity',label:'Open command',bindingId:String(index),kind:'combo',context:CommandFocusContext.GLOBAL,combo,display:combo,command:{commandId:'parity',label:'Open command'},source:{id:String(index),surface:CommandSurface.WEB,binding:{case:'combo',value:{combo}}},sourceLayer:'local',sourceLayerLabel:'Local'}));
function model(mode:string):KeybindingEditorContextValue{return{accountOverridesAvailable:false,spaceOverridesAvailable:false,rows:[],query:'',selectedCommandId:'parity',selectedScope:'local',selectedRow:{state:{command:{commandId:'parity',label:'Open command'}},commandId:'parity',label:'Open command',menuPath:'Tools/Open',displayBindings:['CmdOrCtrl+K']},selectedBindings:mode==='empty'?[]:bindings,selectedController:controller,selectedLayerStatus:'Local overrides',selectedLayerEditable:true,selectedSettingsEditable:true,commandConflicts:[{context:CommandFocusContext.GLOBAL,kind:'combo',key:'CmdOrCtrl+K',bindings},{context:CommandFocusContext.GLOBAL,kind:'combo',key:'CmdOrCtrl+P',bindings}],pendingConflict:undefined,pendingBinding:mode==='pending'?bindings[0]!.source:null,pendingReplace:false,capture:mode==='capture'?{kind:'sequence',replace:false,steps:['CmdOrCtrl+K'],heldModifiers:['Shift']}:null,captureError:mode==='error'?'That key combination is unavailable.':null,setQuery:noop,setSelectedCommandId:noop,setSelectedScope:noop,startCapture:noop,cancelCapture:noop,replacePendingConflict:noop,pendingConflictReplaceable:false,savePendingBinding:noop,clearSelectedBindings:noop,resetSelectedCommand:noop,removeBinding:noop,updateLeaderCombo:noop,updateWhichKeyDelay:noop}}
export function SpacingSamples(){const mode=new URLSearchParams(location.search).get('spacing')??'settings';return <main id="spacing-fixtures" style={{display:'flex',width:'100%',height:760}}><StateNamespaceProvider rootAtom={state}><RouterProvider path="/" onNavigate={noop}>{mode==='settings'||mode==='expanded'?<SessionSettingsDebug/>:<KeybindingEditorContext.Provider value={model(mode)}><KeybindingCommandDetails/></KeybindingEditorContext.Provider>}</RouterProvider></StateNamespaceProvider></main>}
`,
  )
}
