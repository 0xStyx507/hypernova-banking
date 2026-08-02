import { FormEvent } from "react";
import { OAuthProvider } from "../../api";
import { PasswordStrengthMeter } from "../../components/auth/PasswordStrengthMeter";
import { HyperBankWordmark } from "../../components/brand/HyperBankWordmark";
import { AuthField, AuthFieldErrors, AuthForm, AuthMode, FormNotice, OAuthPending } from "../../types";

interface AuthPageProps {
  mode: AuthMode;
  busy: boolean;
  notice: FormNotice | null;
  fieldErrors: AuthFieldErrors;
  form: AuthForm;
  oauthPending: OAuthPending | null;
  onModeChange: (mode: AuthMode) => void;
  onFieldChange: (field: AuthField, value: string) => void;
  onFullNameChange: (value: string) => void;
  onFieldBlur: (field: AuthField) => void;
  onSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onOAuth: (provider: OAuthProvider) => void;
}

/** Public authentication shell. It owns presentation only; App owns auth state and API calls. */
export function AuthPage({ mode, busy, notice, fieldErrors, form, oauthPending, onModeChange, onFieldChange, onFullNameChange, onFieldBlur, onSubmit, onOAuth }: AuthPageProps) {
  return (
    <main className="auth-page px-5 py-8 text-ink sm:px-8 sm:py-12">
      <div className="auth-shell">
        <HyperBankWordmark className="auth-brand" />
        <section className="auth-intro"><h1 className="max-w-xl text-5xl font-semibold leading-[0.98] tracking-tight sm:text-6xl">Banca digital para tu día a día</h1></section>
        <section className="auth-card surface p-6 sm:p-8">
          <div className="auth-tabs" role="tablist" aria-label="Acceso">
            {(["login", "register"] as AuthMode[]).map((nextMode) => <button key={nextMode} className={`flex-1 rounded-full px-4 py-2.5 text-sm font-bold ${mode === nextMode ? "bg-blue text-white shadow-sm" : "text-slate-500"}`} onClick={() => onModeChange(nextMode)} role="tab" aria-selected={mode === nextMode} type="button">{nextMode === "login" ? "Iniciar sesión" : "Crear cuenta"}</button>)}
          </div>
          <div className="mb-7 mt-7"><h2 className="text-2xl font-semibold">{oauthPending ? "Confirma tu acceso" : mode === "login" ? "Qué bueno verte" : "Comienza hoy"}</h2><p className="mt-2 text-sm leading-6 text-slate-500">{oauthPending ? "Solo falta validar tu código de seguridad." : mode === "login" ? "Ingresa para continuar con tu cuenta." : "Crea tu cuenta y empieza a organizar mejor tu dinero."}</p></div>
          <form className="space-y-5" onSubmit={onSubmit}>
            {mode === "register" && <label><span className="field-label">Nombre completo</span><input id="full-name" className={fieldErrors.fullName ? "field-invalid" : ""} required minLength={2} maxLength={120} value={form.fullName} onChange={(event) => onFullNameChange(event.target.value)} onBlur={() => onFieldBlur("fullName")} autoComplete="name" autoCapitalize="words" placeholder="Tu nombre" aria-invalid={Boolean(fieldErrors.fullName)} aria-describedby={fieldErrors.fullName ? "full-name-error" : undefined} />{fieldErrors.fullName && <span id="full-name-error" className="field-error">{fieldErrors.fullName}</span>}</label>}
            {!oauthPending && <label><span className="field-label">Correo electrónico</span><input id="email" className={fieldErrors.email ? "field-invalid" : ""} required type="email" inputMode="email" value={form.email} onChange={(event) => onFieldChange("email", event.target.value.trim())} onBlur={() => onFieldBlur("email")} autoComplete={mode === "login" ? "username" : "email"} autoCapitalize="none" spellCheck={false} placeholder="nombre@gmail.com" aria-invalid={Boolean(fieldErrors.email)} aria-describedby={fieldErrors.email ? "email-error" : undefined} />{fieldErrors.email && <span id="email-error" className="field-error">{fieldErrors.email}</span>}</label>}
            {!oauthPending && <label><span className="field-label">Contraseña</span><input id="password" className={fieldErrors.password ? "field-invalid" : ""} required minLength={mode === "register" ? 8 : 1} maxLength={72} type="password" value={form.password} onChange={(event) => onFieldChange("password", event.target.value)} onBlur={() => onFieldBlur("password")} autoComplete={mode === "login" ? "current-password" : "new-password"} aria-invalid={Boolean(fieldErrors.password)} aria-describedby={fieldErrors.password ? "password-error" : undefined} />{mode === "register" && <PasswordStrengthMeter value={form.password} />}{fieldErrors.password && <span id="password-error" className="field-error">{fieldErrors.password}</span>}</label>}
            {notice && <p className={`status-message ${notice.tone === "error" ? "status-error" : "status-success"}`} role="alert">{notice.message}</p>}
            <button className="primary-button w-full" disabled={busy} type="submit">{busy ? "Procesando…" : oauthPending ? "Confirmar acceso" : mode === "login" ? "Iniciar sesión" : "Crear mi cuenta"}</button>
          </form>
          <div className="my-6 flex items-center gap-3 text-xs font-bold uppercase tracking-[0.16em] text-slate-400"><span className="h-px flex-1 bg-slate-200" />También puedes<span className="h-px flex-1 bg-slate-200" /></div>
          <div className="grid gap-3 sm:grid-cols-2"><button className="secondary-button auth-provider-button w-full" onClick={() => onOAuth("google")} type="button"><GoogleIcon /> <span>Google</span></button><button className="secondary-button auth-provider-button w-full" onClick={() => onOAuth("github")} type="button"><GitHubIcon /> <span>GitHub</span></button></div>
          <p className="mt-6 text-center text-xs leading-5 text-slate-500">Al continuar aceptas nuestros términos de uso y política de privacidad.</p>
        </section>
        <section className="auth-trust" aria-label="Beneficios de Hyper Bank"><p>Una forma más clara de administrar tus fondos, revisar tu actividad y tomar mejores decisiones.</p><div className="flex flex-wrap justify-center gap-x-4 gap-y-2 text-sm font-semibold text-slate-600"><span><i>✓</i> Todo en un solo lugar</span><span><i>✓</i> Control cuando lo necesitas</span><span><i>✓</i> Seguridad que acompaña</span></div></section>
      </div>
    </main>
  );
}

function GoogleIcon() {
  return <svg aria-hidden="true" className="provider-icon" viewBox="0 0 24 24"><path fill="#4285F4" d="M21.35 12.27c0-.73-.07-1.43-.2-2.1H12v3.98h5.24a4.48 4.48 0 0 1-1.94 2.94v2.45h3.14c1.84-1.7 2.91-4.2 2.91-7.27Z" /><path fill="#34A853" d="M12 21.7c2.63 0 4.84-.87 6.45-2.36l-3.14-2.45c-.87.58-1.98.92-3.31.92-2.54 0-4.7-1.72-5.47-4.03H3.29v2.53A9.74 9.74 0 0 0 12 21.7Z" /><path fill="#FBBC05" d="M6.53 13.78a5.86 5.86 0 0 1 0-3.56V7.69H3.29a9.74 9.74 0 0 0 0 8.62l3.24-2.53Z" /><path fill="#EA4335" d="M12 6.19c1.43 0 2.72.49 3.73 1.46l2.8-2.8C16.84 3.3 14.63 2.3 12 2.3a9.74 9.74 0 0 0-8.71 5.39l3.24 2.53C7.3 7.91 9.46 6.19 12 6.19Z" /></svg>;
}

function GitHubIcon() {
  return <svg aria-hidden="true" className="provider-icon provider-icon-github" viewBox="0 0 24 24" fill="currentColor"><path d="M12 .6a11.4 11.4 0 0 0-3.6 22.22c.57.1.78-.25.78-.55v-2.16c-3.17.69-3.84-1.34-3.84-1.34-.52-1.32-1.27-1.67-1.27-1.67-1.04-.71.08-.7.08-.7 1.15.08 1.75 1.18 1.75 1.18 1.02 1.74 2.67 1.24 3.32.95.1-.74.4-1.24.72-1.52-2.53-.29-5.19-1.27-5.19-5.65 0-1.25.45-2.27 1.18-3.07-.12-.29-.51-1.45.11-3.02 0 0 .96-.31 3.14 1.17a10.9 10.9 0 0 1 5.72 0c2.18-1.48 3.14-1.17 3.14-1.17.62 1.57.23 2.73.11 3.02.73.8 1.18 1.82 1.18 3.07 0 4.39-2.66 5.35-5.2 5.64.41.36.77 1.06.77 2.14v3.16c0 .3.2.66.78.55A11.4 11.4 0 0 0 12 .6Z" /></svg>;
}
