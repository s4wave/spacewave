import { parseSync, traverse } from '@babel/core'
import * as t from '@babel/types'
import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix, resolve } from 'node:path'

/** writeAppChildFixture exercises native inputs on actual member, action, build and inspector owners. */
export async function writeAppChildFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Original modules retain their whole row cascade; only the fixture exports private views.
  const files: [string, string][] = [
    ['app/device/AddDeviceWizardViewer.tsx', 'SshCredentialCard'],
    ['app/org/OrgMemberList.tsx', ''],
    ['app/org/OrgInviteSection.tsx', ''],
    ['app/billing/BillingPage.tsx', 'BillingStatusRow'],
    ['app/session/agent/AgentConnectPanel.tsx', 'PanelButton'],
    ['app/space/SpacePluginBuild.tsx', 'BuildSelect'],
    ['app/space/SpacePlugins.tsx', 'SpacePluginAddPanel'],
    ['app/system/SyncInspector.tsx', ''],
    ['app/system/RuntimeInspector.tsx', 'Listing,PluginRow'],
    ['app/space/SpacePluginWorkbench.tsx', ''],
  ]
  const names = new Set(files.map(([file]) => basename(file)))
  for (const [file, exposed] of files) {
    let source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/app-child-' +
            basename(file)
        : file,
      'utf8',
    )
    if (version === 'before')
      source = source.replaceAll(
        "'@s4wave/web/ui/input.js'",
        JSON.stringify(join(root, 'input.tsx')),
      )
    await writeFile(
      join(root, 'child-' + basename(file)),
      source.replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          ['WizardField', 'WizardTextareaField', 'WizardShell'].includes(
            basename(ref).replace(/\.js$/, ''),
          )
            ? `${prefix}${join(root, basename(ref).replace(/\.js$/, '.tsx'))}${suffix}`
            : names.has(basename(ref).replace(/\.js$/, '.tsx'))
              ? `${prefix}./child-${basename(ref)}${suffix}`
              : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ) + (exposed ? '\nexport {' + exposed + '}\n' : ''),
    )
  }

  // Slice the actual pane markup so the responsive divider has no Resource or editor state dependency.
  const workbench = await readFile(
    version === 'before'
      ? 'scripts/tailwind-to-stylex/testdata/components/app-child-SpacePluginWorkbench.tsx'
      : 'app/space/SpacePluginWorkbench.tsx',
    'utf8',
  )
  const parsed = parseSync(workbench, {
    filename: 'SpacePluginWorkbench.tsx',
    configFile: false,
    babelrc: false,
    parserOpts: { plugins: ['typescript', 'jsx'] },
  })!
  const styles: string[] = []
  let panes = ''
  traverse(parsed, {
    VariableDeclarator(p) {
      const n = p.node
      if (
        t.isIdentifier(n.id) &&
        t.isCallExpression(n.init) &&
        t.isMemberExpression(n.init.callee) &&
        t.isIdentifier(n.init.callee.property, { name: 'create' })
      )
        styles.push(n.id.name)
    },
    JSXElement(p) {
      const sections = p.node.children.filter(
        (n): n is t.JSXElement =>
          t.isJSXElement(n) &&
          t.isJSXIdentifier(n.openingElement.name, { name: 'section' }),
      )
      if (
        !sections.some((n) =>
          n.openingElement.attributes.some(
            (a) =>
              t.isJSXAttribute(a) &&
              t.isJSXIdentifier(a.name, { name: 'aria-label' }) &&
              t.isStringLiteral(a.value, { value: 'Plugin source' }),
          ),
        )
      )
        return
      panes =
        workbench.slice(
          p.node.openingElement.start!,
          p.node.openingElement.end!,
        ) +
        sections
          .map(
            (n, i) =>
              workbench.slice(n.openingElement.start!, n.openingElement.end!) +
              (i ? 'Preview content' : 'Source content') +
              '</section>',
          )
          .join('') +
        '</div>'
    },
  })
  if (!panes) throw Error('Missing actual workbench pane markup')
  await writeFile(
    join(root, 'child-SpacePluginWorkbench.tsx'),
    (await readFile(join(root, 'child-SpacePluginWorkbench.tsx'), 'utf8')) +
      '\nexport {' +
      styles.join(',') +
      '}\n',
  )
  await writeFile(
    join(root, 'WorkbenchPanes.tsx'),
    "import React from'react';import{" +
      styles.join(',') +
      "}from'./child-SpacePluginWorkbench';export function WorkbenchPanes(){return " +
      panes +
      '}',
  )

  // Reusable data covers empty and populated lists, finite tones, and an optional trailing action.
  const icon =
    version === 'before'
      ? '<LuBox className="size-6"/>'
      : '<LuBox sx={samples.icon}/>'
  const trailing =
    version === 'before'
      ? '<span>Trailing action</span>'
      : '{childSx=><span sx={childSx}>Trailing action</span>}'
  const rows = `{plugins.map(plugin=><PluginRow key={plugin.id} plugin={plugin} ${version === 'before' ? '' : 'sx={childSx}'}/>)} `
  const listing = version === 'before' ? rows : '{childSx=><>' + rows + '</>}'
  await writeFile(
    join(root, 'AppChildSamples.tsx'),
    `
import{SshCredentialCard}from'./child-AddDeviceWizardViewer';import{WorkbenchPanes}from'./WorkbenchPanes';import React from'react';import * as stylex from'@stylexjs/stylex';import{OrgMemberList}from'./child-OrgMemberList';import{OrgInviteSection}from'./child-OrgInviteSection';import{BillingStatusRow}from'./child-BillingPage';import{PanelButton}from'./child-AgentConnectPanel';import{BuildSelect}from'./child-SpacePluginBuild';import{SpacePluginAddPanel}from'./child-SpacePlugins';import{SyncInspector}from'./child-SyncInspector';import{Listing,PluginRow}from'./child-RuntimeInspector';import{LuBox}from${JSON.stringify(version === 'before' ? 'react-icons/lu' : resolve('web/images/icons.ts'))};import{BillingStatus}from'@s4wave/sdk/provider/spacewave/spacewave.pb.js';import{buildSessionSyncStatusView}from'@s4wave/app/session/SessionSyncStatusContext.js';
const samples=stylex.create({icon:{width:24,height:24}});const noop=()=>{};const members=['Ada','Bo','Cy'].map((name,i)=>({id:String(i),entityId:name,subjectId:'account-'+i,roleId:i?'member':'owner'}));const invites=['First','Second'].map((token,i)=>({id:String(i),token,maxUses:3,uses:i}));const plugins=['running','requested','pending'].map((state,i)=>({key:String(i),id:'example/'+i,state,instanceKey:'',spaceId:'',host:'System'}));const sync={...buildSessionSyncStatusView(null,false,null),detailLabel:'Up to date',summaryLabel:'Ready',transportLabel:'Connected',p2pLabel:'Enabled',localCopies:[{id:'one',name:'Local disk',status:'Ready',detail:'All blocks available',error:''},{id:'two',name:'Backup',status:'Retrying',detail:'Remote copy',error:'Connection paused'}],peers:[{id:'one',name:'Desktop',state:'Connected',traffic:'1 KiB sent'},{id:'two',name:'Laptop',state:'Waiting',traffic:'No traffic'}]};
export function AppChildSamples({mode}:{mode:string}){return <main id="app-helper-fixtures" style={{padding:24,display:'grid',gap:16,maxWidth:720}}>{mode==='credentials'?<>{(['password','private-key']as const).map(authMode=><SshCredentialCard key={authMode} authMode={authMode} credentialDraft={{password:'',privateKey:'',passphrase:''}} onConfigChange={noop} onCredentialChange={noop}/>)}</>:mode==='workbench'?<div style={{height:180}}><WorkbenchPanes/></div>:mode==='members'?<><OrgMemberList members={[]} isOwner/><OrgMemberList members={members} isOwner onRemove={async()=>{}}/><OrgInviteSection invites={[]} onRevoke={async()=>{}}/><OrgInviteSection invites={invites} onRevoke={async()=>{}}/></>:mode==='actions'?<><div style={{display:'flex',gap:8}}><PanelButton icon={${icon}} label="Connect" primary onClick={noop}/><PanelButton icon={${icon}} label="Cancel" onClick={noop}/></div><BillingStatusRow billing={{status:BillingStatus.BillingStatus_ACTIVE}}/><BillingStatusRow billing={{status:BillingStatus.BillingStatus_ACTIVE,cancelAt:1893456000n,currentPeriodEnd:1893456000n}}/></>:mode==='build'?<><BuildSelect label="Source" value="first" options={['first','second']} placeholder="Choose source" disabled={false} onChange={noop}>${trailing}</BuildSelect><BuildSelect label="Device" value="" options={[]} placeholder="No devices" disabled onChange={noop}/><SpacePluginAddPanel draftId="sample-plugin" draftError="" pending="" canSubmitDraft trimmedDraft="sample-plugin" suggestions={[]} onDraftChange={noop} onAdd={noop}>${trailing}</SpacePluginAddPanel></>:<><SyncInspector sync={sync}/><Listing loading={false} empty="No plugins" count={plugins.length}>${listing}</Listing><Listing loading={false} empty="No plugins" count={0}>${version === 'before' ? '' : '{()=>null}'}</Listing></>}</main>}
`,
  )
}
