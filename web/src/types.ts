import { FormEvent } from "react";
import { Account, Balance, HistoryResponse, MFAEnrollment, MFAStatus, MCPAction, MCPActionRequest, MCPAccountOption, MCPConversationState, OAuthProvider, Operation, Transaction, User } from "./api";
import { ThemeMode } from "./theme";

export type AuthMode = "login" | "register";
export type OperationMode = "deposit" | "withdraw" | "transfer";
export type TransferTargetType = "own" | "external";
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
  id: string;
  message: string;
  data?: unknown;
  confirmation: boolean;
  conversation?: MCPConversationState;
  accountOptions?: MCPAccountOption[];
}

export interface DashboardData {
  themeMode: ThemeMode;
  onThemeModeChange: (mode: ThemeMode) => void;
  accounts: Account[];
  accountBalances: Record<string, Balance>;
  activeAccount?: Account;
  balance: Balance | null;
  history: HistoryResponse | null;
  historyPage: number;
  dashboardLoading: boolean;
  dashboardError: string;
  operationMode: OperationMode;
  operationAmount: string;
  destinationAccountId: string;
  transferTargetType: TransferTargetType;
  transferConfirmationPin: string;
  operationBusy: boolean;
  operationNotice: FormNotice | null;
  exportBusy: boolean;
  mcpError: string;
  assistantInput: string;
  assistantReply: AssistantReply | null;
  assistantConversation: MCPConversationState | null;
  assistantAccountOptions: MCPAccountOption[];
  assistantBusy: boolean;
  mcpAction: MCPAction | null;
  mcpActionBusy: boolean;
  mcpActionNotice: FormNotice | null;
  mcpPinConfigured: boolean;
  mcpPinExpiresAt?: string;
  mcpPin: string;
  mcpConfirmationPin: string;
  mcpPinBusy: boolean;
  mcpPinNotice: FormNotice | null;
  accountBusy: boolean;
  accountNotice: FormNotice | null;
  accountRenameBusyId: string;
  accountDeleteBusyId: string;
  profileFullName: string;
  profileBusy: boolean;
  profileNotice: FormNotice | null;
  onAccountChange: (accountId: string) => void;
  onCreateAccount: () => void;
  onRenameAccount: (accountId: string, displayName: string) => void;
  onDeleteAccount: (accountId: string) => void;
  onOperationModeChange: (mode: OperationMode) => void;
  onAmountChange: (amount: string) => void;
  onDestinationChange: (accountId: string) => void;
  onTransferTargetTypeChange: (target: TransferTargetType) => void;
  onTransferConfirmationPinChange: (pin: string) => void;
  onOperation: (event: FormEvent<HTMLFormElement>) => void;
  onExport: () => void;
  onPreviousHistory: () => void;
  onNextHistory: () => void;
  historyHasMore: boolean;
  historyPageBusy: boolean;
  onAssistantInput: (value: string) => void;
  onAssistantSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onAssistantAccountSelect: (accountId: string) => void;
  onPrepareMCPAction: (request: MCPActionRequest) => void;
  onConfirmMCPAction: () => void;
  onCancelMCPAction: () => void;
  onMCPPINChange: (pin: string) => void;
  onMCPConfirmationPINChange: (pin: string) => void;
  onSetMCPPIN: (event: FormEvent<HTMLFormElement>) => void;
  onProfileNameChange: (name: string) => void;
  onProfileSubmit: (event: FormEvent<HTMLFormElement>) => void;
  onLogout: () => void;
}

export type { MFAEnrollment, MFAStatus, Operation, Transaction, User };
