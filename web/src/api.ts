/**
 * Type-safe HTTP client for the public Hypernova API.
 *
 * Access and refresh tokens deliberately stay outside this module. Callers
 * pass the token they currently hold to authenticated methods, which keeps
 * token persistence and session policy in one application-level boundary.
 */

export type Currency = "HNL";
export type AccountStatus = "provisioning" | "active" | "failed";
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
  /** Registers identity and the default HNL account in one API response. */
  register(input: RegisterRequest, options: RequestOptions = {}): Promise<RegistrationResponse> {
    return request<RegistrationResponse>("/v1/auth/register", { method: "POST" }, { ...options, body: input });
  }

  login(input: LoginRequest, options: RequestOptions = {}): Promise<TokensResponse> {
    return request<TokensResponse>("/v1/auth/login", { method: "POST" }, { ...options, body: input });
  }

  refresh(refreshToken: string, options: RequestOptions = {}): Promise<TokensResponse> {
    const body: RefreshRequest = { refresh_token: refreshToken };
    return request<TokensResponse>("/v1/auth/refresh", { method: "POST" }, { ...options, body });
  }

  logout(accessToken: string, options: RequestOptions = {}): Promise<void> {
    return request<void>("/v1/auth/logout", { method: "POST" }, { ...options, accessToken });
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
