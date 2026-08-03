type LogContext = Record<string, unknown>;

const enabled = process.env.NODE_ENV !== "production" || process.env.EXPO_PUBLIC_ENABLE_CLIENT_LOGS === "true";

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
