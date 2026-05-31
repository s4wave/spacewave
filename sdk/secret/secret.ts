import type { ClientResourceRef } from '@aptre/bldr-sdk/resource/client.js'
import { Resource } from '@aptre/bldr-sdk/resource/resource.js'

import { SecretResourceServiceClient } from './secret_srpc.pb.js'
import type { SecretState, WatchStateResponse } from './secret.pb.js'

// SecretTypeID is the ObjectType id for Secret objects.
export const SecretTypeID = 'spacewave/secret'

// SecretKindMatrixAccessToken is the kind for Matrix access tokens.
export const SecretKindMatrixAccessToken = 'matrix_access_token'

// SecretKindSSHPrivateKey is the kind for SSH private-key credentials.
export const SecretKindSSHPrivateKey = 'ssh_private_key'

// SecretKindSSHPassword is the kind for SSH password credentials.
export const SecretKindSSHPassword = 'ssh_password'

// SecretKindSSHPassphrase is the kind for SSH private-key passphrases.
export const SecretKindSSHPassphrase = 'ssh_passphrase'

// SSHPrivateKeyContentType is the content type for SSH private-key payloads.
export const SSHPrivateKeyContentType = 'application/x-pem-file'

// SSHTextCredentialContentType is the content type for text SSH credentials.
export const SSHTextCredentialContentType = 'text/plain; charset=utf-8'

// SecretHandle represents a redacted Secret resource.
export class SecretHandle extends Resource {
  private service: SecretResourceServiceClient

  constructor(resourceRef: ClientResourceRef) {
    super(resourceRef)
    this.service = new SecretResourceServiceClient(resourceRef.client)
  }

  // watchState streams redacted Secret metadata and grant status.
  public async *watchState(
    abortSignal?: AbortSignal,
  ): AsyncIterable<SecretState | undefined> {
    const stream = this.service.WatchState({}, abortSignal)
    for await (const resp of stream as AsyncIterable<WatchStateResponse>) {
      yield resp.state
    }
  }
}
