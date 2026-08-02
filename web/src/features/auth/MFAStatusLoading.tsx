import { FormNotice } from "../../types";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";

export function MFAStatusLoading({ notice, onLogout }: { notice: FormNotice | null; onLogout: () => void }) {
  return <main className="mfa-page mfa-status-loading px-5 py-8 text-ink sm:px-10 sm:py-12"><div className="mfa-loading-card surface"><HyperBankWordmark className="auth-brand mfa-brand-center" /><div className="mfa-loading" role="status">Verificando la protección de tu cuenta…</div>{notice && <p className="status-message status-error mt-5" role="alert">{notice.message}</p>}<button className="secondary-button mt-5 w-full" onClick={onLogout} type="button">Volver al inicio</button></div></main>;
}
