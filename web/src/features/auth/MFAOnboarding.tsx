import { FormEvent, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { MFAEnrollment, User } from "../../api";
import { FormNotice } from "../../types";
import { maskEmail } from "../../components/auth/maskEmail";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";

interface MFAOnboardingProps {
  user: User;
  enrollment: MFAEnrollment | null;
  code: string;
  busy: boolean;
  loading: boolean;
  notice: FormNotice | null;
  onCodeChange: (value: string) => void;
  onBegin: () => void;
  onVerify: (event: FormEvent<HTMLFormElement>) => void;
  onLogout: () => void;
}

/** Authenticated MFA enrollment. QR material never appears in the login challenge. */
export function MFAOnboarding({ user, enrollment, code, busy, loading, notice, onCodeChange, onBegin, onVerify, onLogout }: MFAOnboardingProps) {
  const [secretCopied, setSecretCopied] = useState(false);

  async function copySecret(secret: string) {
    if (!navigator.clipboard) return;
    await navigator.clipboard.writeText(secret);
    setSecretCopied(true);
    window.setTimeout(() => setSecretCopied(false), 1800);
  }

  return <main className="mfa-page px-5 py-8 text-ink sm:px-10 sm:py-12"><div className="mfa-shell"><header className="mfa-header"><div><HyperBankWordmark className="auth-brand mfa-brand-center" /><p className="mt-2 text-center text-sm text-slate-500">Cuenta de {maskEmail(user.email)}</p></div><button className="secondary-button" onClick={onLogout} type="button">Salir</button></header><section className="mfa-layout"><div className="mfa-intro"><span className="mfa-kicker">Protección de cuenta</span><h1>Un último paso para mantener tu cuenta protegida.</h1><p>Usa un código temporal además de tu contraseña para confirmar que eres tú.</p><div className="mfa-benefits"><p>✓ Funciona con Google Authenticator</p><p>✓ Funciona con Microsoft Authenticator</p><p>✓ Tu código cambia cada pocos segundos</p></div></div><div className="mfa-card"><div className="flex items-start justify-between gap-4"><div><p className="text-xs font-bold uppercase tracking-[0.18em] text-slate-400">Activación requerida</p><h2 className="mt-2 text-2xl font-semibold">Autenticación multifactor</h2></div><span className="status-pill status-pill-neutral">Pendiente</span></div>{loading && !enrollment ? <div className="mfa-loading">Preparando tu configuración segura…</div> : enrollment ? <div className="mfa-enrollment"><div className="qr-card" role="img" aria-label="Código QR para configurar tu autenticador"><QRCodeSVG value={enrollment.otpauth_uri} size={192} includeMargin /></div><div className="mfa-form"><p className="text-sm font-semibold">Escanea el código QR</p><p className="mt-2 text-sm leading-6 text-slate-500">Abre tu autenticador, agrega una cuenta y confirma el código de seis dígitos que aparece.</p><details className="mt-4"><summary className="cursor-pointer text-xs font-bold text-slate-500">¿No puedes escanear?</summary><p className="mt-2 break-all rounded-xl bg-slate-50 p-3 font-mono text-xs text-slate-600">{enrollment.secret}</p><button className="secondary-button mt-3 w-full" onClick={() => void copySecret(enrollment.secret)} type="button">{secretCopied ? "Clave copiada" : "Copiar clave manual"}</button></details><form className="mt-5 space-y-3" onSubmit={onVerify}><label><span className="field-label">Código actual</span><input className={notice?.tone === "error" ? "field-invalid" : ""} required inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => onCodeChange(event.target.value.replace(/\D/g, ""))} placeholder="000000" autoComplete="one-time-code" aria-label="Código MFA" aria-invalid={notice?.tone === "error"} /></label><button className="primary-button w-full" disabled={busy || code.length !== 6} type="submit">{busy ? "Verificando…" : "Activar y entrar"}</button>{notice?.tone === "error" && <p className="status-message status-error" role="alert">{notice.message}</p>}</form></div></div> : <button className="primary-button mt-7 w-full" disabled={busy} onClick={onBegin} type="button">Generar nuevo código</button>}{notice?.tone === "success" && <p className="status-message status-success mt-5" role="alert">{notice.message}</p>}</div></section></div></main>;
}
