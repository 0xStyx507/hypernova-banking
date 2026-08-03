/**
 * Type-safe HTTP client for the public Hypernova API.
 *
 * Access and refresh tokens deliberately stay outside this module. Callers
 * pass the token they currently hold to authenticated methods, which keeps
 * token persistence and session policy in one application-level boundary.
 */

export type Currency = "USD";
export type AccountStatus = "provisioning" | "active" | "failed" | "closed";
export type OperationType = "deposit" | "withdrawal" | "transfer";
export type OperationStatus = "succeeded";
export type TransactionDirection = "credit" | "debit";

export interface User {
  id: string;
  email: string;
  full_name: string;
  created_at: string;
}

export interface Account {
  id: string;
  display_name: string;
  currency: Currency;
  type: "checking";
  status: AccountStatus;
  created_at: string;
}

export interface Balance {
  account_id: string;
  currency: Currency;
  balance: string;
  available_balance: string;
  credits_posted: string;
  debits_posted: string;
  credits_pending: string;
  debits_pending: string;
}

export interface Transaction {
  transfer_id: string;
  type: OperationType;
  direction: TransactionDirection;
  amount: string;
  currency: Currency;
  created_at: string;
}

export interface HistoryResponse {
  items: Transaction[];
  has_more: boolean;
  next_cursor?: string;
}

export interface ErrorResponse {
  error: string;
  code: string;
  request_id?: string;
}

export interface RegisterRequest {
  email: string;
  password: string;
  full_name: string;
}

export interface LoginRequest {
  email: string;
  password: string;
  mfa_code?: string;
}

export interface MFAStatus {
  enabled: boolean;
  enrolled: boolean;
}

export interface MFAEnrollment {
  secret: string;
  otpauth_uri: string;
  expires_at: string;
}

export interface RefreshRequest {
  refresh_token: string;
}

export interface CreateAccountRequest {
  currency?: Currency;
}

export interface MovementRequest {
  /** Integer minor units encoded as a decimal string. */
  amount: string;
  currency?: Currency;
}

export interface TransferRequest extends MovementRequest {
  source_account_id: string;
  destination_account_id: string;
  transfer_type?: "own" | "external";
  /** Required by the API only when the destination is not owned by the user. */
  confirmation_pin?: string;
}

export interface RegistrationResponse {
  user: User;
  account: Account;
}

export interface TokensResponse {
  user: User;
  access_token: string;
  refresh_token: string;
  access_expires_at: string;
  refresh_expires_at: string;
}

export type OAuthProvider = "google" | "github";

export interface OAuthCallbackResponse {
  user: User;
  exchange_code: string;
  expires_at: string;
  account?: Account;
}

export interface MCPTool {
  name: string;
  read_only: boolean;
  description: string;
}

export interface MCPToolsResponse {
  protocol: string;
  tools: MCPTool[];
}

export interface UpdateProfileRequest {
  full_name: string;
}

export interface MCPPINStatus {
  configured: boolean;
  expires_at?: string;
}

export interface MCPToolCallResponse {
  name: string;
  result: unknown;
}

export type MCPActionType = "deposit" | "withdrawal" | "transfer";

export interface MCPActionRequest {
  action: MCPActionType;
  account_id?: string;
  source_account_id?: string;
  destination_account_id?: string;
  transfer_type?: "own" | "external";
  /** Integer minor units encoded as a decimal string. */
  amount: string;
  currency: Currency;
  reason?: string;
}

export type MCPActionPayload = MCPActionRequest;

export interface MCPAction {
  id: string;
  action: MCPActionType;
  status: "ready" | "confirming" | "confirmed" | "cancelled" | "expired" | "failed";
  payload: MCPActionPayload;
  expires_at: string;
  created_at: string;
  confirmed_at?: string;
  operation?: Operation;
}

export interface ChatResponse {
  message: string;
  requires_confirmation: boolean;
  read_only_data?: unknown;
  action?: MCPAction;
  conversation?: MCPConversationState;
  account_options?: MCPAccountOption[];
}

export interface MCPAccountOption {
  id: string;
  display_name: string;
  currency: Currency;
}

export interface MCPConversationState {
  action?: MCPActionType;
  account_id?: string;
  source_account_id?: string;
  destination_account_id?: string;
  transfer_type?: "own" | "external";
  amount?: string;
  account_options?: MCPAccountOption[];
}

export interface Operation {
  id: string;
  type: OperationType;
  status: OperationStatus;
  transfer_id: string;
  amount: string;
  currency: Currency;
  created_at: string;
}

export interface AccountListResponse {
  items: Account[];
}

export interface RequestOptions {
  signal?: AbortSignal;
}

export interface AuthenticatedRequestOptions extends RequestOptions {
  accessToken: string;
}

export interface IdempotentRequestOptions extends AuthenticatedRequestOptions {
  /** Required by the API for every money movement. */
  idempotencyKey: string;
}

/** Error raised for a non-successful HTTP response. */
export class ApiError extends Error {
  readonly status: number;
  readonly response: ErrorResponse;

  constructor(status: number, response: ErrorResponse) {
    super(response.error);
    this.name = "ApiError";
    this.status = status;
    this.response = response;
  }
}

type RequestConfig = RequestOptions & {
  accessToken?: string;
  idempotencyKey?: string;
  body?: unknown;
};

const apiBaseUrl = (import.meta.env.VITE_API_BASE_URL ?? "/api").replace(/\/$/, "");

function isErrorResponse(value: unknown): value is ErrorResponse {
  if (typeof value !== "object" || value === null) return false;

  const candidate = value as Partial<ErrorResponse>;
  return typeof candidate.error === "string" && typeof candidate.code === "string";
}

function joinApiPath(path: string): string {
  return `${apiBaseUrl}/${path.replace(/^\//, "")}`;
}

/**
 * Small fetch wrapper shared by all endpoints. It does not retry mutations:
 * retrying a financial request without the same idempotency key could create
 * a new operation instead of replaying the original intent.
 */
async function request<T>(path: string, init: RequestInit = {}, config: RequestConfig = {}): Promise<T> {
  const headers = new Headers(init.headers);
  headers.set("Accept", "application/json");

  if (config.accessToken) headers.set("Authorization", `Bearer ${config.accessToken}`);
  if (config.idempotencyKey) headers.set("Idempotency-Key", config.idempotencyKey);

  let body = init.body;
  if (config.body !== undefined) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(config.body);
  }

  const response = await fetch(joinApiPath(path), {
    ...init,
    body,
    headers,
    cache: "no-store",
    referrerPolicy: "no-referrer",
    signal: config.signal,
  });

  if (response.ok) {
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }

  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    payload = undefined;
  }

  const errorResponse: ErrorResponse = isErrorResponse(payload)
    ? payload
    : {
        error: response.statusText || "Request failed",
        code: "http_error",
      };

  throw new ApiError(response.status, errorResponse);
}

export class ApiClient {
  /** Registers identity and the default USD account in one API response. */
  register(input: RegisterRequest, options: RequestOptions = {}): Promise<RegistrationResponse> {
    return request<RegistrationResponse>("/v1/auth/register", { method: "POST" }, { ...options, body: input });
  }

  login(input: LoginRequest, options: RequestOptions = {}): Promise<TokensResponse> {
    return request<TokensResponse>("/v1/auth/login", { method: "POST" }, { ...options, body: input });
  }

  /** Starts the provider redirect; credentials never pass through the SPA. */
  startOAuth(provider: OAuthProvider): void {
    const endpoint = new URL(joinApiPath(`/v1/auth/oauth/${provider}/start`), window.location.origin);
    // The callback returns a short-lived exchange code to the same web origin.
    // Keeping this target explicit prevents the API from returning JSON instead
    // of sending the user back to the SPA after provider authorization.
    endpoint.searchParams.set("return_to", `${window.location.origin}/`);
    window.location.assign(endpoint.toString());
  }

  exchangeOAuth(provider: OAuthProvider, code: string, mfaCode?: string): Promise<TokensResponse> {
    return request<TokensResponse>(`/v1/auth/oauth/${provider}/exchange`, { method: "POST" }, {
      body: { code, mfa_code: mfaCode || undefined },
    });
  }

  refresh(refreshToken: string, options: RequestOptions = {}): Promise<TokensResponse> {
    const body: RefreshRequest = { refresh_token: refreshToken };
    return request<TokensResponse>("/v1/auth/refresh", { method: "POST" }, { ...options, body });
  }

  logout(accessToken: string, options: RequestOptions = {}): Promise<void> {
    return request<void>("/v1/auth/logout", { method: "POST" }, { ...options, accessToken });
  }

  getMFAStatus(options: AuthenticatedRequestOptions): Promise<MFAStatus> {
    return request<MFAStatus>("/v1/auth/mfa", {}, options);
  }

  enrollMFA(options: AuthenticatedRequestOptions): Promise<MFAEnrollment> {
    return request<MFAEnrollment>("/v1/auth/mfa/enroll", { method: "POST" }, options);
  }

  verifyMFA(code: string, options: AuthenticatedRequestOptions): Promise<MFAStatus> {
    return request<MFAStatus>("/v1/auth/mfa/verify", { method: "POST" }, { ...options, body: { code } });
  }

  listMCPTools(options: AuthenticatedRequestOptions): Promise<MCPToolsResponse> {
    return request<MCPToolsResponse>("/v1/mcp/tools", {}, options);
  }

  callMCPTool(name: string, arguments_: unknown, options: AuthenticatedRequestOptions): Promise<MCPToolCallResponse> {
    return request<MCPToolCallResponse>("/v1/mcp/tools/call", { method: "POST" }, {
      ...options,
      body: { name, arguments: arguments_ },
    });
  }

  prepareMCPAction(input: MCPActionRequest, options: AuthenticatedRequestOptions): Promise<MCPAction> {
    return request<MCPAction>("/v1/mcp/actions", { method: "POST" }, { ...options, body: input });
  }

  getMCPAction(actionId: string, options: AuthenticatedRequestOptions): Promise<MCPAction> {
    return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}`, {}, options);
  }
  getPendingMCPAction(options: AuthenticatedRequestOptions): Promise<{ action: MCPAction | null }> {
    return request<{ action: MCPAction | null }>("/v1/mcp/actions/pending", {}, options);
  }

  confirmMCPAction(actionId: string, pin: string, options: AuthenticatedRequestOptions): Promise<MCPAction> {
    const controller = new AbortController();
    const timeout = window.setTimeout(() => controller.abort(), 15000);
    return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}/confirm`, { method: "POST" }, { ...options, body: { pin }, signal: controller.signal }).finally(() => window.clearTimeout(timeout));
  }

  updateProfile(input: UpdateProfileRequest, options: AuthenticatedRequestOptions): Promise<User> {
    return request<User>("/v1/auth/profile", { method: "PUT" }, { ...options, body: input });
  }

  cancelMCPAction(actionId: string, options: AuthenticatedRequestOptions): Promise<MCPAction> {
    return request<MCPAction>(`/v1/mcp/actions/${encodeURIComponent(actionId)}/cancel`, { method: "POST" }, options);
  }

  getMCPPINStatus(options: AuthenticatedRequestOptions): Promise<MCPPINStatus> {
    return request<MCPPINStatus>("/v1/auth/mcp-pin", {}, options);
  }

  setMCPPIN(pin: string, options: AuthenticatedRequestOptions): Promise<MCPPINStatus> {
    return request<MCPPINStatus>("/v1/auth/mcp-pin", { method: "POST" }, { ...options, body: { pin } });
  }

  sendChatMessage(message: string, accountId: string | undefined, conversation: MCPConversationState | null, options: AuthenticatedRequestOptions): Promise<ChatResponse> {
    return request<ChatResponse>("/v1/chat/messages", { method: "POST" }, { ...options, body: { message, account_id: accountId, conversation: conversation ?? undefined } });
  }

  createAccount(
    input: CreateAccountRequest = {},
    options: AuthenticatedRequestOptions,
  ): Promise<Account> {
    return request<Account>("/v1/accounts", { method: "POST" }, { ...options, body: input });
  }

  listAccounts(options: AuthenticatedRequestOptions): Promise<AccountListResponse> {
    return request<AccountListResponse>("/v1/accounts", {}, options);
  }

  getAccount(accountId: string, options: AuthenticatedRequestOptions): Promise<Account> {
    return request<Account>(`/v1/accounts/${encodeURIComponent(accountId)}`, {}, options);
  }

  renameAccount(accountId: string, displayName: string, options: AuthenticatedRequestOptions): Promise<Account> {
    return request<Account>(`/v1/accounts/${encodeURIComponent(accountId)}`, { method: "PATCH" }, { ...options, body: { display_name: displayName } });
  }

  closeAccount(accountId: string, options: AuthenticatedRequestOptions): Promise<void> {
    return request<void>(`/v1/accounts/${encodeURIComponent(accountId)}`, { method: "DELETE" }, options);
  }

  getBalance(accountId: string, options: AuthenticatedRequestOptions): Promise<Balance> {
    return request<Balance>(`/v1/accounts/${encodeURIComponent(accountId)}/balance`, {}, options);
  }

  getHistory(
    accountId: string,
    options: AuthenticatedRequestOptions & { limit?: number; cursor?: string },
  ): Promise<HistoryResponse> {
    const query = new URLSearchParams();
    if (options.limit !== undefined) query.set("limit", String(options.limit));
    if (options.cursor) query.set("cursor", options.cursor);

    const queryString = query.toString();
    const path = `/v1/accounts/${encodeURIComponent(accountId)}/transactions${queryString ? `?${queryString}` : ""}`;
    return request<HistoryResponse>(path, {}, options);
  }

  /** Downloads the bounded, ownership-checked CSV history export. */
  async exportHistory(accountId: string, options: AuthenticatedRequestOptions): Promise<Blob> {
    const response = await fetch(joinApiPath(`/v1/accounts/${encodeURIComponent(accountId)}/transactions.csv`), {
      headers: {
        Accept: "text/csv",
        Authorization: `Bearer ${options.accessToken}`,
      },
      cache: "no-store",
      referrerPolicy: "no-referrer",
      signal: options.signal,
    });
    if (response.ok) return response.blob();

    let payload: unknown;
    try {
      payload = await response.json();
    } catch {
      payload = undefined;
    }
    throw new ApiError(response.status, isErrorResponse(payload) ? payload : { error: response.statusText || "Request failed", code: "http_error" });
  }

  deposit(
    accountId: string,
    input: MovementRequest,
    options: IdempotentRequestOptions,
  ): Promise<Operation> {
    return request<Operation>(
      `/v1/accounts/${encodeURIComponent(accountId)}/deposits`,
      { method: "POST" },
      { ...options, body: input },
    );
  }

  withdraw(
    accountId: string,
    input: MovementRequest,
    options: IdempotentRequestOptions,
  ): Promise<Operation> {
    return request<Operation>(
      `/v1/accounts/${encodeURIComponent(accountId)}/withdrawals`,
      { method: "POST" },
      { ...options, body: input },
    );
  }

  transfer(input: TransferRequest, options: IdempotentRequestOptions): Promise<Operation> {
    return request<Operation>("/v1/transfers", { method: "POST" }, { ...options, body: input });
  }
}

/** Shared stateless client; authentication state remains managed by the app. */
export const apiClient = new ApiClient();

export default apiClient;
