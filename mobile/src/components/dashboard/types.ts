import { Account, Balance, History, MCPAction, OperationMode, User } from "../../api";
import { DashboardSection } from "../../types";

export type MobileFeedbackTone = "error" | "success" | "warning" | "info";

export interface MobileFeedback {
  tone: MobileFeedbackTone;
  message: string;
}

export interface MobileDashboardProps {
  user: User;
  accessToken: string;
  accounts: Account[];
  accountBalances: Record<string, Balance>;
  activeAccount: Account | null;
  balance: Balance | null;
  history: History | null;
  historyPage: number;
  historyBusy: boolean;
  hasMoreHistory: boolean;
  section: DashboardSection;
  operationMode: OperationMode;
  operationAmount: string;
  destinationAccountId: string;
  transferTargetType: "own" | "external";
  transferConfirmationPin: string;
  operationBusy: boolean;
  operationNotice: MobileFeedback | null;
  notice: string;
  accountBusy: boolean;
  accountNotice: string;
  accountRenameBusyId: string;
  profileFullName: string;
  profileBusy: boolean;
  profileNotice: string;
  mcpPin: string;
  mcpPinConfigured: boolean;
  mcpPinExpiresAt?: string;
  mcpPinBusy: boolean;
  mcpPinNotice: string;
  mcpActionPending: boolean;
  mcpAction: MCPAction | null;
  onMCPActionConfirmed: (action: MCPAction) => void;
  onMCPActionExpired: () => void;
  onNavigate: (section: DashboardSection) => void;
  onAccountChange: (accountId: string) => void;
  onCreateAccount: () => void;
  onRenameAccount: (accountId: string, displayName: string) => void;
  onOperationModeChange: (mode: OperationMode) => void;
  onAmountChange: (amount: string) => void;
  onDestinationChange: (accountId: string) => void;
  onTransferTargetTypeChange: (target: "own" | "external") => void;
  onTransferConfirmationPinChange: (pin: string) => void;
  onOperation: () => void;
  onNextHistory: () => void;
  onPreviousHistory: () => void;
  onProfileNameChange: (name: string) => void;
  onProfileSubmit: () => void;
  onMCPPINChange: (pin: string) => void;
  onSetMCPPIN: () => void;
  onLogout: () => void;
  onThemeToggle: () => void;
  onMCPActionPendingChange: (pending: boolean) => void;
}
