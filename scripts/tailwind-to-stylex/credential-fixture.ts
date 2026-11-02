import { readFile, writeFile } from 'node:fs/promises'
import { join } from 'node:path'

/** writeCredentialFixture retains proof combinations and public field styles. */
export async function writeCredentialFixture(
  root: string,
  version: 'before' | 'after',
) {
  // Compare the original credential component with the actual native implementation.
  await writeFile(
    join(root, 'CredentialProofInput.tsx'),
    await readFile(
      version === 'before'
        ? 'scripts/tailwind-to-stylex/testdata/components/CredentialProofInput.tsx'
        : 'web/ui/credential/CredentialProofInput.tsx',
      'utf8',
    ),
  )

  // Password, file, disabled and invalid states retain the same field recipe.
  await writeFile(
    join(root, 'CredentialSamples.tsx'),
    `
import{useRef,useState}from'react';
import{cn}from'@s4wave/web/style/utils.js';
import{CredentialProofInput,${version === 'before' ? 'inputClass' : 'credentialInputStyles'}}from'./CredentialProofInput';
export function CredentialSamples(){const[password,setPassword]=useState('');const file=useRef<HTMLInputElement>(null);return <main id="credential-fixtures" style={{padding:16,maxWidth:360}}>
<CredentialProofInput password={password} onPasswordChange={setPassword} fileInputRef={file}/>
<CredentialProofInput password="secret" onPasswordChange={()=>{}} disabled pemFileName="backup.pem"/>
<CredentialProofInput password="" onPasswordChange={()=>{}} showPem={false} passwordLabel="Password only"/>
<CredentialProofInput password="" onPasswordChange={()=>{}} showPassword={false} pemLabel="File only" error="Choose a readable backup key"/>
<input aria-label="Shared field" ${version === 'before' ? 'className={inputClass}' : 'sx={credentialInputStyles.input}'}/>
<input aria-label="Invalid shared field" ${version === 'before' ? 'className={cn(inputClass,"border-destructive/50")}' : 'sx={[credentialInputStyles.input,credentialInputStyles.invalid]}'}/>
<input aria-label="Verification code" ${version === 'before' ? 'className={cn(inputClass,"text-center font-mono text-lg tracking-brand-extra-wide")}' : 'sx={[credentialInputStyles.input,credentialInputStyles.code]}'}/>
</main>}
`,
  )
}
