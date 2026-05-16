import { FormEvent, ReactNode, useState } from 'react';
import { LockKeyhole, Loader2, LogOut, ShieldCheck } from 'lucide-react';
import { usePassphraseAuth } from '../hooks/usePassphraseAuth';

interface PassphraseGateProps {
  children: ReactNode;
}

export function PassphraseGate({ children }: PassphraseGateProps) {
  const { status, verifying, error, verify, logout } = usePassphraseAuth();
  const [passphrase, setPassphrase] = useState('');

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const verified = await verify(passphrase);
    if (verified) setPassphrase('');
  };

  if (status === 'checking') {
    return <GateShell icon={<Loader2 className="h-6 w-6 animate-spin" />} title="認証状態を確認中" />;
  }

  if (status === 'authenticated') {
    return (
      <>
        {children}
        <button
          type="button"
          onClick={logout}
          className="absolute right-4 top-4 z-[60] inline-flex items-center gap-2 rounded-full border border-white/20 bg-black/45 px-3 py-2 text-xs font-medium text-white shadow-lg backdrop-blur transition hover:bg-black/65"
          aria-label="認証を解除"
        >
          <LogOut className="h-3.5 w-3.5" />
          Lock
        </button>
      </>
    );
  }

  if (status === 'unconfigured') {
    return (
      <GateShell
        icon={<LockKeyhole className="h-6 w-6" />}
        title="パスフレーズ認証が未設定です"
        description="GitHub Pages 用にビルドする前に VITE_AUTH_PASSPHRASE_HASH と VITE_AUTH_PASSPHRASE_SALT を設定してください。"
      />
    );
  }

  return (
    <GateShell
      icon={<ShieldCheck className="h-6 w-6" />}
      title="Object Lens を開く"
      description="事前に共有されたパスフレーズを入力すると、カメラ検索機能にアクセスできます。"
    >
      <form onSubmit={handleSubmit} className="mt-6 space-y-4">
        <label className="block text-left text-sm font-medium text-white/80" htmlFor="passphrase">
          パスフレーズ
        </label>
        <input
          id="passphrase"
          type="password"
          value={passphrase}
          onChange={(event) => setPassphrase(event.target.value)}
          className="w-full rounded-2xl border border-white/15 bg-white/10 px-4 py-3 text-base text-white outline-none transition placeholder:text-white/35 focus:border-blue-300 focus:bg-white/15 focus:ring-4 focus:ring-blue-400/20"
          placeholder="Enter passphrase"
          autoComplete="current-password"
          disabled={verifying}
        />
        {error && <p className="rounded-xl bg-red-500/15 px-3 py-2 text-sm text-red-100 ring-1 ring-red-400/30">{error}</p>}
        <button
          type="submit"
          disabled={verifying}
          className="inline-flex w-full items-center justify-center gap-2 rounded-2xl bg-blue-500 px-4 py-3 font-semibold text-white shadow-xl shadow-blue-500/20 transition hover:bg-blue-400 disabled:cursor-not-allowed disabled:opacity-60"
        >
          {verifying ? <Loader2 className="h-5 w-5 animate-spin" /> : <LockKeyhole className="h-5 w-5" />}
          Unlock
        </button>
      </form>
    </GateShell>
  );
}

function GateShell({ icon, title, description, children }: { icon: ReactNode; title: string; description?: string; children?: ReactNode }) {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-black px-6 text-white">
      <div className="absolute inset-0 bg-[radial-gradient(circle_at_top_left,rgba(59,130,246,0.35),transparent_36%),radial-gradient(circle_at_bottom_right,rgba(14,165,233,0.2),transparent_42%)]" />
      <div className="absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:40px_40px] opacity-30" />
      <section className="relative w-full max-w-md rounded-[2rem] border border-white/15 bg-white/10 p-8 text-center shadow-2xl backdrop-blur-xl">
        <div className="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-2xl bg-white text-neutral-950 shadow-lg">
          {icon}
        </div>
        <h1 className="text-2xl font-bold tracking-tight">{title}</h1>
        {description && <p className="mt-3 text-sm leading-6 text-white/70">{description}</p>}
        {children}
        <p className="mt-6 text-xs leading-5 text-white/45">静的サイト向けの簡易ゲートです。機密情報の保護にはサーバー側認証を使ってください。</p>
      </section>
    </div>
  );
}
