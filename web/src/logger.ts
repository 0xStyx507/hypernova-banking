type LogContext = Record<string, unknown>;

const enabled = import.meta.env.DEV || import.meta.env.VITE_ENABLE_CLIENT_LOGS === "true";

function write(level: "info" | "warn" | "error", message: string, context?: LogContext) {
  if (!enabled) return;
  const payload = context && Object.keys(context).length > 0 ? context : undefined;
  console[level]("[hypernova]", message, payload ?? "");
}

export const clientLogger = {
  info(message: string, context?: LogContext) { write("info", message, context); },
  warn(message: string, context?: LogContext) { write("warn", message, context); },
  error(message: string, context?: LogContext) { write("error", message, context); },
};

export function installClientErrorLogging() {
  if (!enabled) return () => undefined;
  const onError = (event: ErrorEvent) => clientLogger.error("unhandled browser error", {
    message: event.message,
    source: event.filename?.split("/").pop() ?? "unknown",
    line: event.lineno,
  });
  const onRejection = (event: PromiseRejectionEvent) => clientLogger.error("unhandled promise rejection", {
    message: event.reason instanceof Error ? event.reason.message : String(event.reason ?? "unknown"),
  });
  window.addEventListener("error", onError);
  window.addEventListener("unhandledrejection", onRejection);
  return () => {
    window.removeEventListener("error", onError);
    window.removeEventListener("unhandledrejection", onRejection);
  };
}
