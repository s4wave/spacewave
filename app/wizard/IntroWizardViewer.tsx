import { useCallback, useMemo } from 'react'

import type { ObjectViewerComponentProps } from '@s4wave/web/object/object.js'
import type { ObjectInfo } from '@s4wave/web/object/object.pb.js'
import { ObjectViewer } from '@s4wave/web/object/ObjectViewer.js'
import { LoadingCard } from '@s4wave/web/ui/loading/LoadingCard.js'
import { toast } from '@s4wave/web/ui/toaster.js'
import { IntroWizardConfig } from '@s4wave/sdk/world/wizard/wizard.pb.js'

import { useWizardState } from './useWizardState.js'
import { deleteWizard } from './wizard-space-index.js'
import { IntroWizardOverlay } from './IntroWizardOverlay.js'
import { IntroWizardPresentationContext } from './intro-context.js'

// IntroWizardViewer is the generic new-user introduction viewer. It contains
// the introduced object's standalone ObjectViewer (its own window frame) and
// draws the introduction around that frame. Finishing sets the Space index to
// the introduced object and deletes the wizard object, leaving the normal view.
export function IntroWizardViewer(props: ObjectViewerComponentProps) {
  const ws = useWizardState(props, undefined)
  const { state } = ws

  const config = useMemo(
    () =>
      state?.configData && state.configData.length > 0
        ? IntroWizardConfig.fromBinary(state.configData)
        : undefined,
    [state?.configData],
  )
  const targetObjectKey = state?.targetKeyPrefix ?? ''

  const targetInfo: ObjectInfo = useMemo(
    () => ({
      info: {
        case: 'worldObjectInfo',
        value: {
          objectKey: targetObjectKey,
          ...(state?.targetTypeId ? { objectType: state.targetTypeId } : {}),
        },
      },
    }),
    [targetObjectKey, state?.targetTypeId],
  )

  const handleFinish = useCallback(async () => {
    if (!state || ws.creating || !targetObjectKey) return
    ws.setCreating(true)
    try {
      await deleteWizard(ws, targetObjectKey)
      ws.navigateToObjects([targetObjectKey])
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to finish intro')
    } finally {
      ws.setCreating(false)
    }
  }, [state, ws, targetObjectKey])

  if (!state) {
    return (
      <div className="flex flex-1 items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <LoadingCard
            view={{
              state: 'active',
              title: 'Loading',
              detail: 'Preparing your introduction.',
            }}
          />
        </div>
      </div>
    )
  }

  return (
    <div className="relative flex h-full w-full flex-col overflow-hidden">
      <IntroWizardPresentationContext.Provider value={true}>
        <ObjectViewer
          standalone
          objectInfo={targetInfo}
          worldState={props.worldState}
          spaceContents={props.spaceContents}
          stateNamespace={['intro', targetObjectKey]}
        />
      </IntroWizardPresentationContext.Provider>
      <IntroWizardOverlay
        headline={config?.headline ?? ''}
        subhead={config?.subhead ?? ''}
        finishLabel={config?.finishLabel || 'Continue'}
        callouts={config?.callouts ?? []}
        finishing={ws.creating}
        onFinish={() => void handleFinish()}
        onSkip={() => void handleFinish()}
      />
    </div>
  )
}
