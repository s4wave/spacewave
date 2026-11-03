import { writeViewerFixture } from './viewer-fixture.js'
import { writeAppChildFixture } from './app-child-fixture.js'
import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix } from 'node:path'

/** writeAppHelperFixture compares the actual views that consume native helper values. */
export async function writeAppHelperFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Keep the child-owner comparisons beside the helper family.
  await writeAppChildFixture(root, version)
  await writeViewerFixture(root, version)

  // Only this fixture exports private renderers; production modules retain their APIs.
  const files: [string, string][] = [
    ['app/wizard/IntroWizardOverlay.tsx', 'IntroCallout'],
    [
      'app/org/OrganizationDetails.tsx',
      'RecoverySummaryCard,getRecoverySummary',
    ],
    ['app/session/RecoveryPage.tsx', 'PasswordStrength'],
    ['app/billing/UsageBars.tsx', 'UsageBar'],
    ['app/session/setup/PhaseChecklist.tsx', ''],
    ['app/session/setup/SetupPageLayout.tsx', ''],
  ]
  const names = new Set(files.map(([file]) => basename(file)))
  for (const [file, exposed] of files) {
    const source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/app-helper-' +
            basename(file)
        : file,
      'utf8',
    )
    await writeFile(
      join(root, 'helper-' + basename(file)),
      source.replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          names.has(basename(ref).replace(/\.js$/, '.tsx'))
            ? `${prefix}./helper-${basename(ref)}${suffix}`
            : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ) + (exposed ? '\nexport {' + exposed + '}\n' : ''),
    )
  }

  // Fixed protobuf health and region inputs reach each renderer's real state branches.
  const custom =
    version === 'before'
      ? 'barClassName="bg-blue-500"'
      : 'barSx={styles.storage}'
  const checklist =
    version === 'before' ? 'className="mt-4"' : 'sx={styles.checklist}'
  await writeFile(
    join(root, 'AppHelperSamples.tsx'),
    `
import{ViewerSamples}from'./ViewerSamples';import{AppChildSamples}from'./AppChildSamples';import React from'react';import * as stylex from'@stylexjs/stylex';import{IntroCallout}from'./helper-IntroWizardOverlay';import{RecoverySummaryCard,getRecoverySummary}from'./helper-OrganizationDetails';import{PasswordStrength}from'./helper-RecoveryPage';import{UsageBar}from'./helper-UsageBars';import{PhaseChecklist}from'./helper-PhaseChecklist';import{SetupPageLayout}from'./helper-SetupPageLayout';
import{IntroWizardRegion as Region}from'@s4wave/sdk/world/wizard/wizard.pb.js';import{SharedObjectHealthStatus as Status,SharedObjectHealthLayer as Layer,SharedObjectHealthCommonReason as Reason,SharedObjectHealthRemediationHint as Hint}from'@s4wave/core/sobject/sobject.pb.js';
const styles=stylex.create({storage:{backgroundColor:'var(--color-blue-500)'},checklist:{marginTop:'calc(var(--spacing) * 4)'}});const phases=[{label:'Prepare',done:true},{label:'Download',done:false,active:true},{label:'Start',done:false}];
export function AppHelperSamples(){const mode=new URLSearchParams(location.search).get('app-helper')??'values';if(mode.startsWith('viewer-'))return <ViewerSamples/>;if(['members','actions','build','lists','workbench','credentials'].includes(mode))return <AppChildSamples mode={mode}/>;if(mode.startsWith('callout')){const region=({'callout-top':Region.TOP,'callout-left':Region.LEFT,'callout-bottom':Region.BOTTOM_RIGHT})[mode]??Region.CENTER;return <main id="app-helper-fixtures" style={{position:'relative',height:800,width:'100%'}}><IntroCallout callout={{region,title:'Create a Space',detail:'Choose what this Space should do.'}}/></main>};if(mode.startsWith('setup'))return <main id="app-helper-fixtures" style={{display:'flex',height:900,width:'100%'}}><SetupPageLayout title="Set up this Device" subtitle="Connect it to a Space."><PhaseChecklist phases={mode==='setup-empty'?[]:phases} ${checklist}/></SetupPageLayout></main>;return <main id="app-helper-fixtures" style={{padding:24,display:'grid',gap:16,maxWidth:540}}>{[Status.LOADING,Status.DEGRADED,Status.CLOSED].map(status=><RecoverySummaryCard key={status} summary={getRecoverySummary({status,layer:Layer.SHARED_OBJECT,commonReason:Reason.INITIAL_STATE_REJECTED,remediationHint:Hint.RETRY,error:'Example root could not load.'})} healthError={status===Status.CLOSED?'Example root could not load.':''}/>)}{['','a','abcdefgh','abcdefghijkl'].map(password=><PasswordStrength key={password} password={password}/>)}{[0,50,80,100].map(used=><UsageBar key={used} label={'Usage '+used} used={used} baseline={100} formatValue={value=>String(value)}/>)}<UsageBar label="Storage" used={40} baseline={100} formatValue={value=>String(value)} ${custom}/></main>}
`,
  )
}
