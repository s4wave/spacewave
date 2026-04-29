import { LuArrowRight, LuTriangleAlert } from 'react-icons/lu'

// MissingBillingAccountBanner prompts when an org has no billing account.
// Owners get a "Configure billing" CTA that opens the org details billing
// section so they can pick or detach a managed BA. Non-owners see a
// read-only message asking them to contact an org admin.
export function MissingBillingAccountBanner(props: {
  isOwner: boolean
  onConfigureBilling: () => void
}) {
  const message =
    'This organization has no billing account. Spaces in this org cannot be created until billing is configured.'

  if (!props.isOwner) {
    return (
      <div className="border-warning/20 bg-warning/5 relative z-10 flex w-full items-center border-b">
        <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
          <LuTriangleAlert className="text-warning h-3.5 w-3.5 shrink-0" />
          <div className="min-w-0">
            <p className="text-foreground/80 text-xs font-medium">{message}</p>
            <p className="text-foreground-alt/60 mt-0.5 text-[11px]">
              Ask an organization admin to assign a billing account.
            </p>
          </div>
        </div>
      </div>
    )
  }

  return (
    <button
      type="button"
      onClick={props.onConfigureBilling}
      className="border-warning/20 bg-warning/5 hover:bg-warning/10 relative z-10 flex w-full items-center border-b text-left transition-colors"
    >
      <div className="flex min-w-0 flex-1 items-start gap-2 px-3 py-1.5">
        <LuTriangleAlert className="text-warning h-3.5 w-3.5 shrink-0" />
        <div className="min-w-0">
          <p className="text-foreground/80 text-xs font-medium">{message}</p>
          <p className="text-foreground-alt/60 mt-0.5 text-[11px]">
            Assign one of your managed billing accounts to keep cloud features
            available.
          </p>
        </div>
      </div>
      <div className="group flex shrink-0 items-center gap-1 px-3 py-1.5 transition-colors">
        <span className="text-foreground/70 group-hover:text-foreground text-xs font-medium transition-colors">
          Configure billing
        </span>
        <LuArrowRight className="text-foreground-alt group-hover:text-foreground h-3 w-3 shrink-0 transition-colors" />
      </div>
    </button>
  )
}
