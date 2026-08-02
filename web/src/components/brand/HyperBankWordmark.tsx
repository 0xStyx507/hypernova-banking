interface HyperBankWordmarkProps {
  className?: string;
}

/** Shared brand primitive used by every authenticated and public surface. */
export function HyperBankWordmark({ className = "" }: HyperBankWordmarkProps) {
  return <div className={`hyperbank-wordmark ${className}`.trim()} aria-label="Hyper Bank"><span className="auth-brand-mark" aria-hidden="true">H</span><span>Hyper Bank</span></div>;
}
