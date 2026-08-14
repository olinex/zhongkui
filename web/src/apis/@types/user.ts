import type { BaseModelOut } from "@/types";

export interface PasswordLoginIn {
  username: string;
  password: string;
}

export interface UserTokenOut {
  token: string;
}

export interface PasswordResetIn {
  oldPassword: string;
  newPassword: string;
}

export interface profileChangeIn {
  nickleName: string;
  email: string;
}

export interface accessKeyCreateIn {
  enabled: boolean;
  description: string;
  expiredAt: Date | null;
}

export interface accessKeyOut extends BaseModelOut {
  enabled: boolean;
  accessKeyID: string;
  accessKeySecret: string;
  description: string;
  expiredAt: string;
  lastAccessAt: string;
}
