/**
 * Typed mobile client for the shared Hypernova HTTP contract.
 * Tokens are supplied by the screen layer and never logged or embedded in
 * request errors.
 */
export type Currency = "USD";
export type OperationMode = "deposit" | "withdrawal" | "transfer";
export type OAuthProvider = "google" | "github";

export interface User { id: string; email: string; full_name: string; created_at: string }
export interface Account { id: string; display_name: string; currency: Currency; type: "checking"; status: "provisioning" | "active" | "failed" | "closed"; created_at: string }
export interface Balance { account_id: string; currency: Currency; balance: string; available_balance: string; credits_posted: string; debits_posted: string; credits_pending: string; debits_pending: string }
export interface Transaction { transfer_id: string; type: OperationMode; direction: "credit" | "debit"; amount: string; currency: Currency; created_at: string }
export interface History { items: Transaction[]; has_more: boolean; next_cursor?: string }
export interface MCPActionRequest { action: "deposit" | "withdrawal" | "transfer"; account_id?: string; source_account_id?: string; destination_account_id?: string; transfer_type?: "own" | "external"; amount: string; currency: Currency; reason?: string }
export interface MCPAction { id: string; action: "deposit" | "withdrawal" | "transfer"; status: "ready" | "confirming" | "confirmed" | "cancelled" | "expired" | "failed"; payload: MCPActionRequest; expires_at: string; operation?: Operation }
export interface MCPTool { name: string; read_only: boolean; description: string }
export interface MCPToolsResponse { protocol: string; tools: MCPTool[] }
export interface MCPToolCallResponse { name: string; result: unknown }
export interface MCPAccountOption { id: string; display_name: string; currency: Currency }
export interface MCPConversationState { action?: "deposit" | "withdrawal" | "transfer"; account_id?: string; source_account_id?: string; destination_account_id?: string; transfer_type?: "own" | "external"; amount?: string; account_options?: MCPAccountOption[] }
export interface ChatResponse { message: string; requires_confirmation: boolean; read_only_data?: unknown; action?: MCPAction; conversation?: MCPConversationState; account_options?: MCPAccountOption[] }
export interface MCPPINStatus { configured: boolean; expires_at?: string }
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

import Constants from "expo-constants";
import { clientLogger } from "./logger";

/**
 * Resolves the API host for both a simulator and a physical device.
 *
 * `localhost` is correct for an emulator only. Expo Go publishes its LAN
 * host in `hostUri`, so a phone can reach the API container on the same Wi-Fi
 * network without requiring a machine-specific value in source control.
 */
function resolveBaseUrl(): string {
  const configuredUrl = process.env.EXPO_PUBLIC_API_BASE_URL;
  if (configuredUrl) return configuredUrl.replace(/\/$/, "");

  const expoHost = Constants.expoConfig?.hostUri?.split(":")[0];
  if (expoHost) return `http://${expoHost}:8080/api`;

  return "http://localhost:8080/api";
}

const baseUrl = resolveBaseUrl();

async function request<T>(path: string, init: RequestInit = {}, accessToken?: string, idempotencyKey?: string, signal?: AbortSignal): Promise<T> {
  const startedAt = Date.now();
  const method = init.method ?? "GET";
  const safePath = path.split("?", 1)[0];
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");
  if (init.body) headers.set("Content-Type", "application/json");
  if (accessToken) headers.set("Authorization", `Bearer ${accessToken}`);
  if (idempotencyKey) headers.set("Idempotency-Key", idempotencyKey);
  let response: Response;
  try {
    response = await fetch(`${baseUrl}/${path.replace(/^\//, "")}`, { ...init, headers, cache: "no-store", signal });
  } catch (error) {
    clientLogger.error("api request failed", { method, path: safePath, duration_ms: Date.now() - startedAt, error: error instanceof Error ? error.message : String(error) });
    throw error;
  }
  // DELETE endpoints may intentionally return 204 without a response body.
  // Do not attempt to parse JSON in that case; callers still receive a
  // successful typed result and the shared contract remains consistent with
  // the web client.
  if (response.ok) {
    if (method !== "GET") clientLogger.info("api request completed", { method, path: safePath, status: response.status, duration_ms: Date.now() - startedAt });
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }
  let payload: ApiErrorBody = { error: "No se pudo completar la solicitud", code: "http_error" };
  try {
    const candidate = (await response.json()) as Partial<ApiErrorBody>;
    if (typeof candidate.error === "string" && typeof candidate.code === "string") payload = candidate as ApiErrorBody;
  } catch { /* Keep a generic public error when the server has no JSON body. */ }
  clientLogger.warn("api response error", { method, path: safePath, status: response.status, code: payload.code, duration_ms: Date.now() - startedAt });
  throw new MobileApiError(response.status, payload);
}

export const mobileApi = {
  register(input: { email: string; password: string; full_name: string }) { return request<{ user: User; account: Account }>("/v1/auth/register", { method: "POST", body: JSON.stringify(input) }); },
  login(input: { email: string; password: string; mfa_code?: string }) { return request<Tokens>("/v1/auth/login", { method: "POST", body: JSON.stringify(input) }); },
  refresh(refreshToken: string) { return request<Tokens>("/v1/auth/refresh", { method: "POST", body: JSON.stringify({ refresh_token: refreshToken }) }); },
  /** Builds the browser redirect URL without putting credentials in the deep link. */
  oauthStartUrl(provider: OAuthProvider, returnTo: string) { return `${baseUrl}/v1/auth/oauth/${provider}/start?return_to=${encodeURIComponent(returnTo)}`; },
  exchangeOAuth(provider: OAuthProvider, code: string, mfaCode?: string) { return request<Tokens>(`/v1/auth/oauth/${provider}/exchange`, { method: "POST", body: JSON.stringify({ code, mfa_code: mfaCode || undefined }) }); },
  logout(accessToken: string) { return request<void>("/v1/auth/logout", { method: "POST" }, accessToken); },
  mfaStatus(accessToken: string) { return request<MFAStatus>("/v1/auth/mfa", {}, accessToken); },
  enrollMFA(accessToken: string) { return request<MFAEnrollment>("/v1/auth/mfa/enroll", { method: "POST" }, accessToken); },
  verifyMFA(code: string, accessToken: string) { return request<MFAStatus>("/v1/auth/mfa/verify", { method: "POST", body: JSON.stringify({ code }) }, accessToken); },
  accounts(accessToken: string) { return request<{ items: Account[] }>("/v1/accounts", {}, accessToken); },
  account(accountId: string, accessToken: string) { return request<Account>(`/v1/accounts/${encodeURIComponent(accountId)}`, {}, accessToken); },
  createAccount(accessToken: string, key: string) { return request<Account>("/v1/accounts", { method: "POST", body: JSON.stringify({ currency: "USD" }) }, accessToken, key); },
  renameAccount(accountId: string, displayName: string, accessToken: string) { return request<Account>(`/v1/accounts/${encodeURIComponent(accountId)}`, { method: "PATCH", body: JSON.stringify({ display_name: displayName }) }, accessToken); },
  closeAccount(accountId: string, accessToken: string) { return request<void>(`/v1/accounts/${encodeURIComponent(accountId)}`, { method: "DELETE" }, accessToken); },
  balance(accountId: string, accessToken: string) { return request<Balance>(`/v1/accounts/${accountId}/balance`, {}, accessToken); },
  history(accountId: string, accessToken: string, options: { limit?: number; cursor?: string } = {}) {
    const query = new URLSearchParams();
    query.set("limit", String(options.limit ?? 5));
    if (options.cursor) query.set("cursor", options.cursor);
    return request<History>(`/v1/accounts/${accountId}/transactions?${query.toString()}`, {}, accessToken);
  },
  updateProfile(fullName: string, accessToken: string) {
    return request<User>("/v1/auth/profile", { method: "PUT", body: JSON.stringify({ full_name: fullName }) }, accessToken);
  },
  chat(message: string, accessToken: string, accountId?: string, conversation?: MCPConversationState | null) {
    return request<ChatResponse>("/v1/chat/messages", { method: "POST", body: JSON.stringify({ message, account_id: accountId, conversation: conversation ?? undefined }) }, accessToken);
  },
  prepareMCPAction(action: MCPActionRequest, accessToken: string) { return request<MCPAction>("/v1/mcp/actions", { method: "POST", body: JSON.stringify(action) }, accessToken); },
  mcpTools(accessToken: string) { return request<MCPToolsResponse>("/v1/mcp/tools", {}, accessToken); },
  callMCPTool(name: string, args: unknown, accessToken: string) { return request<MCPToolCallResponse>("/v1/mcp/tools/call", { method: "POST", body: JSON.stringify({ name, arguments: args }) }, accessToken); },
  getMCPAction(actionId: string, accessToken: string) { return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}`, {}, accessToken); },
  getPendingMCPAction(accessToken: string) { return request<{ action: MCPAction | null }>("/v1/mcp/actions/pending", {}, accessToken); },
  confirmMCPAction(actionId: string, pin: string, accessToken: string) { const controller = new AbortController(); const timeout = setTimeout(() => controller.abort(), 15000); return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}/confirm`, { method: "POST", body: JSON.stringify({ pin }) }, accessToken, undefined, controller.signal).finally(() => clearTimeout(timeout)); },
  cancelMCPAction(actionId: string, accessToken: string) { return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}/cancel`, { method: "POST" }, accessToken); },
  mcpPINStatus(accessToken: string) { return request<MCPPINStatus>("/v1/auth/mcp-pin", {}, accessToken); },
  setMCPPIN(pin: string, accessToken: string) { return request<MCPPINStatus>("/v1/auth/mcp-pin", { method: "POST", body: JSON.stringify({ pin }) }, accessToken); },
  deposit(accountId: string, amount: string, accessToken: string, key: string) { return request<Operation>(`/v1/accounts/${accountId}/deposits`, { method: "POST", body: JSON.stringify({ amount, currency: "USD" }) }, accessToken, key); },
  withdraw(accountId: string, amount: string, accessToken: string, key: string) { return request<Operation>(`/v1/accounts/${accountId}/withdrawals`, { method: "POST", body: JSON.stringify({ amount, currency: "USD" }) }, accessToken, key); },
  transfer(source: string, destination: string, amount: string, accessToken: string, key: string, confirmationPin?: string, transferType: "own" | "external" = "own") { return request<Operation>("/v1/transfers", { method: "POST", body: JSON.stringify({ source_account_id: source, destination_account_id: destination, transfer_type: transferType, amount, currency: "USD", confirmation_pin: confirmationPin || undefined }) }, accessToken, key); },
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
    return `USD ${digits.slice(0, -2)}.${digits.slice(-2)}`;
  } catch { return "USD 0.00"; }
}
