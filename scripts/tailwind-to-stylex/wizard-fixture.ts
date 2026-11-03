import { readFile, writeFile } from 'node:fs/promises'
import { basename, dirname, join, posix } from 'node:path'

/** writeWizardFixture compares the coupled label, control and help owners. */
export async function writeWizardFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Preserve the original frame and recipes together while shared Input keeps its accepted API.
  const names = [
    'WizardFieldFrame.tsx',
    'WizardField.tsx',
    'WizardTextareaField.tsx',
    'wizard-field-styles.ts',
  ]
  for (const name of names) {
    const file = 'app/wizard/' + name
    const source = await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/wizard-' + name
        : file,
      'utf8',
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
  await writeFile(
    join(root, 'WizardSamples.tsx'),
    `
import React from'react';import * as stylex from'@stylexjs/stylex';import{WizardFieldFrame}from'./WizardFieldFrame';import{WizardField}from'./WizardField';import{WizardTextareaField}from'./WizardTextareaField';import{${version === 'before' ? 'wizardInputClassName' : 'wizardFieldStyles'}}from'./wizard-field-styles';
const styles=stylex.create({textarea:{minHeight:'64px',fontFamily:'var(--font-mono)',fontSize:'var(--text-xs)',lineHeight:'var(--text-xs--line-height)',WebkitFontSmoothing:'subpixel-antialiased',MozOsxFontSmoothing:'auto',textRendering:'auto',textShadow:'0 0 0.25px currentColor'},field:{marginTop:'calc(var(--spacing) * 2)'}});
export function WizardSamples(){const disabled=new URLSearchParams(location.search).get('wizard')==='disabled';return <main id="wizard-fixtures" style={{padding:24,width:'100%',maxWidth:480,display:'grid',gap:16}}><WizardField label="Display name" help="Name this Device." value="Example" readOnly disabled={disabled}/><WizardField label="Path" variant="compactMono" defaultValue="/bin/init" disabled={disabled}/><WizardField label="Invalid value" variant="error" defaultValue="example" disabled={disabled}/><WizardTextareaField label="Public key" help="Paste the public key." defaultValue="ssh-ed25519 AAAA" disabled={disabled} ${overrides}/>${select}</main>}
`,
  )
}
