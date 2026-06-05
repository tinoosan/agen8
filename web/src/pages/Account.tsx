import { AccountSettingsSections } from '../components/settings/AccountSettings'

export default function Account() {
  return (
    <div className="h-full overflow-y-auto p-[clamp(16px,4vw,32px)_clamp(16px,5vw,40px)]">
      <div className="mx-auto flex w-full max-w-[980px] flex-col gap-8">
        <div className="flex flex-col gap-1">
          <h1 className="m-0 text-2xl font-bold text-[var(--text-1)]">Settings</h1>
          <p className="m-0 text-[0.8125rem] text-[var(--text-3)]">Account, preferences, and runtime controls.</p>
        </div>
        <AccountSettingsSections />
      </div>
    </div>
  )
}
