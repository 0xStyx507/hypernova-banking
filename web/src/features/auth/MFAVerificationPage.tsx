import { FormEvent } from "react";
import { maskEmail } from "../../components/auth/maskEmail";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";

interface FormNotice {
  tone: "error" | "success";
  message: string;
}

interface MFAVerificationPageProps {
  email: string;
  code: string;
  busy: boolean;
  notice: FormNotice | null;
  fieldError?: string;
  oauth: boolean;
  onCodeChange: (value: string) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onBack: () => void;
}

/** Verifies the second factor after credentials or OAuth have succeeded. */
export function MFAVerificationPage({
  email,
  code,
  busy,
  notice,
  fieldError,
  oauth,
  onCodeChange,
  onSubmit,
  onBack,
}: MFAVerificationPageProps) {
  return (
    <main className="mfa-verify-page px-5 py-8 text-ink sm:px-8 sm:py-12">
      <div className="mfa-verify-card surface">
        <HyperBankWordmark className="auth-brand mfa-brand-center" />
        <div className="mfa-verify-icon" aria-hidden="true">✓</div>
        <p className="eyebrow">Verificación de seguridad</p>
        <h1>{oauth ? "Confirma tu acceso" : "Confirma tu identidad"}</h1>
        <p className="mfa-verify-copy">{oauth ? "Escribe el código de tu autenticador para terminar de entrar." : `Tu cuenta ${email ? `(${maskEmail(email)}) ` : ""}necesita un código de seguridad antes de continuar.`}</p>
        <form className="mfa-verify-form" onSubmit={onSubmit}>
          {(fieldError || notice) && <p id="login-mfa-error" className={`status-message ${(fieldError || notice?.tone === "error") ? "status-error" : "status-success"}`} role="alert">{fieldError ?? notice?.message}</p>}
          <label><span className="field-label">Código de 6 dígitos</span><input id="login-mfa-code" className={fieldError ? "field-invalid" : ""} autoFocus required inputMode="numeric" pattern="[0-9]{6}" maxLength={6} value={code} onChange={(event) => onCodeChange(event.target.value.replace(/\D/g, ""))} autoComplete="one-time-code" placeholder="000000" aria-invalid={Boolean(fieldError)} aria-describedby={fieldError ? "login-mfa-error" : undefined} /></label>
          <button className="primary-button w-full" disabled={busy || code.length !== 6} type="submit">{busy ? "Validando…" : "Continuar"}</button>
        </form>
        <button className="secondary-button mfa-verify-back" onClick={onBack} type="button">Volver al inicio de sesión</button>
      </div>
    </main>
  );
}
