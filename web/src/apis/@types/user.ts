import type { BaseModelOut } from "@/types";
import { string } from "yup";

export interface PasswordLoginIn {
  username: string;
  password: string;
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
