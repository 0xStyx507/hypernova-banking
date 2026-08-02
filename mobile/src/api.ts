/**
 * Typed mobile client for the shared Hypernova HTTP contract.
 * Tokens are supplied by the screen layer and never logged or embedded in
 * request errors.
 */
export type Currency = "HNL";
export type OperationMode = "deposit" | "withdrawal" | "transfer";

export interface User { id: string; email: string; full_name: string; created_at: string }
export interface Account { id: string; currency: Currency; type: "checking"; status: string; created_at: string }
export interface Balance { account_id: string; currency: Currency; balance: string; available_balance: string; credits_posted: string; debits_posted: string; credits_pending: string; debits_pending: string }
export interface Transaction { transfer_id: string; type: OperationMode; direction: "credit" | "debit"; amount: string; currency: Currency; created_at: string }
export interface History { items: Transaction[]; has_more: boolean; next_cursor?: string }
export interface Tokens { user: User; access_token: string; refresh_token: string; access_expires_at: string; refresh_expires_at: string }
export interface MFAStatus { enabled: boolean; enrolled: boolean }
export interface MFAEnrollment { secret: string; otpauth_uri: string; expires_at: string }
export interface Operation { id: string; type: OperationMode; status: string; transfer_id: string; amount: string; currency: Currency; created_at: string }
export interface ApiErrorBody { error: string; code: string }

export class MobileApiError extends Error {
  constructor(readonly status: number, readonly body: ApiErrorBody) {
    super(body.error);
    this.name = "MobileApiError";
  }
}

const baseUrl = (process.env.EXPO_PUBLIC_API_BASE_URL ?? "http://localhost:8080/api").replace(/\/$/, "");

async function request<T>(path: string, init: RequestInit = {}, accessToken?: string, idempotencyKey?: string): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body) headers.set("Content-Type", "application/json");
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (idempotencyKey) headers.set("Idempotency-Key", idempotencyKey);
  const response = await fetch(`${baseUrl}/${path.replace(/^\//, "")}`, { ...init, headers, cache: "no-store" });
  if (response.ok) return (await response.json()) as T;
  let payload: ApiErrorBody = { error: "No se pudo completar la solicitud", code: "http_error" };
  try {
    const candidate = (await response.json()) as Partial<ApiErrorBody>;
    if (typeof candidate.error === "string" && typeof candidate.code === "string") payload = candidate as ApiErrorBody;
  } catch { /* Keep a generic public error when the server has no JSON body. */ }
  throw new MobileApiError(response.status, payload);
}

export const mobileApi = {
  register(input: { email: string; password: string; full_name: string }) { return request<{ user: User; account: Account }>("/v1/auth/register", { method: "POST", body: JSON.stringify(input) }); },
  login(input: { email: string; password: string; mfa_code?: string }) { return request<Tokens>("/v1/auth/login", { method: "POST", body: JSON.stringify(input) }); },
  logout(accessToken: string) { return request<void>("/v1/auth/logout", { method: "POST" }, accessToken); },
  mfaStatus(accessToken: string) { return request<MFAStatus>("/v1/auth/mfa", {}, accessToken); },
  enrollMFA(accessToken: string) { return request<MFAEnrollment>("/v1/auth/mfa/enroll", { method: "POST" }, accessToken); },
  verifyMFA(code: string, accessToken: string) { return request<MFAStatus>("/v1/auth/mfa/verify", { method: "POST", body: JSON.stringify({ code }) }, accessToken); },
  accounts(accessToken: string) { return request<{ items: Account[] }>("/v1/accounts", {}, accessToken); },
  balance(accountId: string, accessToken: string) { return request<Balance>(`/v1/accounts/${accountId}/balance`, {}, accessToken); },
  history(accountId: string, accessToken: string) { return request<History>(`/v1/accounts/${accountId}/transactions?limit=8`, {}, accessToken); },
  deposit(accountId: string, amount: string, accessToken: string, key: string) { return request<Operation>(`/v1/accounts/${accountId}/deposits`, { method: "POST", body: JSON.stringify({ amount, currency: "HNL" }) }, accessToken, key); },
  withdraw(accountId: string, amount: string, accessToken: string, key: string) { return request<Operation>(`/v1/accounts/${accountId}/withdrawals`, { method: "POST", body: JSON.stringify({ amount, currency: "HNL" }) }, accessToken, key); },
  transfer(source: string, destination: string, amount: string, accessToken: string, key: string) { return request<Operation>("/v1/transfers", { method: "POST", body: JSON.stringify({ source_account_id: source, destination_account_id: destination, amount, currency: "HNL" }) }, accessToken, key); },
};

/** Idempotency keys are identifiers, not credentials; the fallback is for older Hermes runtimes. */
export function createIdempotencyKey(): string {
  const randomUuid = globalThis.crypto?.randomUUID;
  if (randomUuid) return randomUuid.call(globalThis.crypto);
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function formatMinor(value: string): string {
  try {
    const minor = BigInt(value || "0");
    const digits = minor.toString().padStart(3, "0");
    return `HNL ${digits.slice(0, -2)}.${digits.slice(-2)}`;
  } catch { return "HNL 0.00"; }
}
