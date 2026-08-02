import { FormEvent } from "react";
import { Account, Balance, HistoryResponse, MFAEnrollment, MFAStatus, MCPAction, MCPActionRequest, MCPTool, OAuthProvider, Operation, Transaction, User } from "./api";

export type AuthMode = "login" | "register";
export type OperationMode = "deposit" | "withdraw" | "transfer";
export type AuthField = "fullName" | "email" | "password" | "mfaCode";
export type AuthFieldErrors = Partial<Record<AuthField, string>>;

export interface AuthForm {
  email: string;
  password: string;
  fullName: string;
  mfaCode: string;
}

export interface Session {
  accessToken: string;
  refreshToken: string;
  user: User;
  accessExpiresAt?: string;
  refreshExpiresAt?: string;
}

export interface FormNotice {
  tone: "error" | "success";
  message: string;
}

export interface OAuthPending {
  provider: OAuthProvider;
  code: string;
}

export interface AssistantReply {
  message: string;
  data?: unknown;
  confirmation: boolean;
}

export interface DashboardData {
  accounts: Account[];
  activeAccount?: Account;
  balance: Balance | null;
  history: HistoryResponse | null;
  historyPage: number;
  dashboardLoading: boolean;
  dashboardError: string;
  operationMode: OperationMode;
  operationAmount: string;
  destinationAccountId: string;
  operationBusy: boolean;
  operationNotice: FormNotice | null;
  exportBusy: boolean;
  mcpTools: MCPTool[];
  mcpLoading: boolean;
  mcpError: string;
  assistantInput: string;
  assistantReply: AssistantReply | null;
  assistantBusy: boolean;
  mcpAction: MCPAction | null;
  mcpActionBusy: boolean;
  mcpActionNotice: FormNotice | null;
  onAccountChange: (accountId: string) => void;
  onOperationModeChange: (mode: OperationMode) => void;
  onAmountChange: (amount: string) => void;
  onDestinationChange: (accountId: string) => void;
  onOperation: (event: FormEvent<HTMLFormElement>) => void;
  onExport: () => void;
  onPreviousHistory: () => void;
  onNextHistory: () => void;
  historyHasMore: boolean;
  historyPageBusy: boolean;
  onAssistantInput: (value: string) => void;
  onAssistantSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onPrepareMCPAction: (request: MCPActionRequest) => void;
  onConfirmMCPAction: () => void;
  onCancelMCPAction: () => void;
  onLogout: () => void;
}

export type { MFAEnrollment, MFAStatus, Operation, Transaction, User };
