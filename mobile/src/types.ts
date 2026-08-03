import { User } from "./api";

export interface MobileSession {
  accessToken: string;
  refreshToken: string;
  user: User;
}

export type DashboardSection = "accounts" | "history" | "operations" | "settings";
