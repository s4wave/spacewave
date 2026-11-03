import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix } from 'node:path'

/** writeWizardFixture compares the coupled label, control and help owners. */
export async function writeWizardFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Preserve the original frame and recipes together while shared Input keeps its accepted API.
  const names = [
    'WizardShell.tsx',
    'WizardFieldFrame.tsx',
    'WizardField.tsx',
    'WizardTextareaField.tsx',
    'wizard-field-styles.ts',
  ]
  for (const name of names) {
    const file = 'app/wizard/' + name
    let source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/wizard-' + name
        : file,
      'utf8',
    )
    if (version === 'before')
      source = source
        .replaceAll(
          "'@s4wave/web/ui/DashboardButton.js'",
          JSON.stringify(join(root, 'DashboardButton.tsx')),
        )
        .replaceAll(
          "'@s4wave/web/ui/button.js'",
          JSON.stringify(join(root, 'button.tsx')),
        )
    await writeFile(
      join(root, name),
      source.replace(
        /(from\s*['"])(\.[^'"]+)(['"])/g,
        (_, prefix, ref, suffix) =>
          names.some(
            (name) =>
              basename(ref).replace(/\.js$/, '') ===
              name.replace(/\.tsx?$/, ''),
          )
            ? `${prefix}./${basename(ref)}${suffix}`
            : `${prefix}@s4wave/${posix.normalize(join(dirname(file), ref))}${suffix}`,
      ),
    )
  }

  // The native select is the same public frame composition used by the VM wizard.
  const select =
    version === 'before'
      ? '<WizardFieldFrame label="Filesystem" help="Programs run on a copy."><select className={`${wizardInputClassName} w-full rounded-md border px-2`} disabled={disabled} defaultValue="root"><option value="root">Root filesystem</option></select></WizardFieldFrame>'
      : '<WizardFieldFrame label="Filesystem" help="Programs run on a copy.">{controlSx=><select sx={[controlSx,wizardFieldStyles.select]} disabled={disabled} defaultValue="root"><option value="root">Root filesystem</option></select>}</WizardFieldFrame>'
  const overrides =
    version === 'before'
      ? 'className="min-h-16 font-mono text-xs" fieldClassName="mt-2"'
      : 'sx={styles.textarea} fieldSx={styles.field}'
  const content =
    version === 'before'
      ? '<WizardField label="Configuration" value="Example" readOnly/>'
      : '{childSx=><WizardField fieldSx={childSx} label="Configuration" value="Example" readOnly/>}'
  await writeFile(
    join(root, 'WizardSamples.tsx'),
    `
import{WizardShell}from'./WizardShell';import React from'react';import * as stylex from'@stylexjs/stylex';import{WizardFieldFrame}from'./WizardFieldFrame';import{WizardField}from'./WizardField';import{WizardTextareaField}from'./WizardTextareaField';import{${version === 'before' ? 'wizardInputClassName' : 'wizardFieldStyles'}}from'./wizard-field-styles';
const styles=stylex.create({textarea:{minHeight:'64px',fontFamily:'var(--font-mono)',fontSize:'var(--text-xs)',lineHeight:'var(--text-xs--line-height)',WebkitFontSmoothing:'subpixel-antialiased',MozOsxFontSmoothing:'auto',textRendering:'auto',textShadow:'0 0 0.25px currentColor'},field:{marginTop:'calc(var(--spacing) * 2)'}});
export function WizardSamples(){const mode=new URLSearchParams(location.search).get('wizard')??'default';const disabled=mode==='disabled';if(mode.startsWith('shell'))return <main id="wizard-fixtures" style={{height:900,width:'100%'}}><WizardShell title="New example" step={mode==='shell-next'?1:0} totalSteps={2} stepName="Configuration" width={mode==='shell-wide'?'wide':'default'} localName="Example" onUpdateName={()=>{}} onBack={()=>{}} onCancel={()=>{}} creating={false} onFinalize={()=>{}} onNext={()=>{}} finalizeStep={1} nameHelp="Name this example.">${content}</WizardShell></main>;return <main id="wizard-fixtures" style={{padding:24,width:'100%',maxWidth:480,display:'grid',gap:16}}><WizardField label="Display name" help="Name this Device." value="Example" readOnly disabled={disabled}/><WizardField label="Path" variant="compactMono" defaultValue="/bin/init" disabled={disabled}/><WizardField label="Invalid value" variant="error" defaultValue="example" disabled={disabled}/><WizardTextareaField label="Public key" help="Paste the public key." defaultValue="ssh-ed25519 AAAA" disabled={disabled} ${overrides}/>${select}</main>}
`,
  )
}
