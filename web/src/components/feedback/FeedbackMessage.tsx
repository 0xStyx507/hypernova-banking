import { ReactNode } from "react";

export type FeedbackTone = "error" | "success" | "warning" | "info";

interface Props {
  tone: FeedbackTone;
  message: string;
  title?: string;
  action?: ReactNode;
}

const defaultTitles: Record<FeedbackTone, string> = {
  error: "No pudimos completar la acción",
  success: "Listo",
  warning: "Revisa esta información",
  info: "Información",
};

/** Consistent, accessible feedback for contextual forms and dashboard actions. */
export function FeedbackMessage({ tone, message, title, action }: Props) {
  const assertive = tone === "error" || tone === "warning";
  return <div className={`feedback-message feedback-${tone}`} role={assertive ? "alert" : "status"} aria-live={assertive ? "assertive" : "polite"}>
    <span className="feedback-icon" aria-hidden="true">{tone === "error" ? "!" : tone === "success" ? "✓" : "i"}</span>
    <div className="feedback-copy"><strong>{title ?? defaultTitles[tone]}</strong><p>{message}</p></div>
    {action}
  </div>;
}
