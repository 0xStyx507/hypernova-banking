export type PasswordStrength = { label: "Baja" | "Media" | "Alta"; percent: number; tone: "low" | "medium" | "high" } | null;

function passwordStrength(value: string): PasswordStrength {
  if (!value) return null;
  let score = 0;
  if (value.length >= 8) score += 1;
  if (value.length >= 12) score += 1;
  if (/[a-záéíóúüñ]/u.test(value) && /[A-ZÁÉÍÓÚÜÑ]/u.test(value)) score += 1;
  if (/\d/u.test(value)) score += 1;
  if (/[^A-Za-zÁÉÍÓÚÜÑáéíóúüñ\d]/u.test(value)) score += 1;
  if (score <= 2) return { label: "Baja", percent: 35, tone: "low" };
  if (score <= 3) return { label: "Media", percent: 65, tone: "medium" };
  return { label: "Alta", percent: 100, tone: "high" };
}

/** Shows password guidance without persisting or logging the password. */
export function PasswordStrengthMeter({ value }: { value: string }) {
  const strength = passwordStrength(value);
  if (!strength) return null;

  return (
    <div className="password-strength" aria-live="polite">
      <div className="password-strength-track" role="progressbar" aria-label="Seguridad de la contraseña" aria-valuemin={0} aria-valuemax={100} aria-valuenow={strength.percent}><span className={`password-strength-bar strength-${strength.tone}`} style={{ width: `${strength.percent}%` }} /></div>
      <span className={`password-strength-label strength-text-${strength.tone}`}>Seguridad {strength.label.toLowerCase()} · {strength.percent}%</span>
    </div>
  );
}
