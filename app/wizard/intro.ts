import {
  IntroWizardConfig,
  IntroWizardRegion,
} from '@s4wave/sdk/world/wizard/wizard.pb.js'

// IntroWizardTypeID is the generic new-user introduction wizard type. One
// viewer parameterizes over every quickstart: the wizard object records the
// introduced object key in WizardState.targetKeyPrefix and the per-quickstart
// intro content in WizardState.configData as a serialized IntroWizardConfig.
export const IntroWizardTypeID = 'wizard/intro'

// driveIntroConfig is the Drive parameterization of the new-user introduction:
// the first quickstart to supply its content as wizard data.
export function driveIntroConfig(): IntroWizardConfig {
  return {
    headline: 'Welcome to your Drive',
    subhead: 'The Drive securely stores your files.',
    finishLabel: 'Got it, start exploring',
    callouts: [
      {
        region: IntroWizardRegion.TOP,
        title: 'Add files',
        detail: 'Upload or drag files in, or make a new folder.',
      },
      {
        region: IntroWizardRegion.CENTER,
        title: 'Your files',
        detail: 'Everything you add shows up here to open or organize.',
      },
      {
        region: IntroWizardRegion.BOTTOM_RIGHT,
        title: 'Upload progress',
        detail: 'Uploads report their progress here in the corner.',
      },
    ],
  }
}
