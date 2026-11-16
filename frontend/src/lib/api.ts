// Thin wrapper over the generated Wails bindings.
import { Events } from "@wailsio/runtime";
import * as MailService from "@bindings/github.com/HamzaBaghdadi/pigeon/services/mailservice";
import * as AccountService from "@bindings/github.com/HamzaBaghdadi/pigeon/services/accountservice";
import * as SettingsService from "@bindings/github.com/HamzaBaghdadi/pigeon/services/settingsservice";
import type { AddAccountInput, GoogleSetup, MoveResult, SyncStatus } from "@bindings/github.com/HamzaBaghdadi/pigeon/services/models";
import type { Settings, Theme } from "@bindings/github.com/HamzaBaghdadi/pigeon/internal/settings/models";
import type { Settings as ProviderSettings } from "@bindings/github.com/HamzaBaghdadi/pigeon/internal/providers/models";
import type {
  Account,
  Folder,
  FolderRole,
  MessageBody,
  MessageSummary,
} from "@bindings/github.com/HamzaBaghdadi/pigeon/internal/mail/models";

export type {
  Account,
  AddAccountInput,
  Folder,
  FolderRole,
  GoogleSetup,
  MessageBody,
  MessageSummary,
  MoveResult,
  ProviderSettings,
  Settings,
  SyncStatus,
};

export const api = {
  // mail
  listAccounts: async () => (await MailService.ListAccounts()) ?? [],
  listFolders: async (accountId: string) => (await MailService.ListFolders(accountId)) ?? [],
  listMessages: async (accountId: string, role: FolderRole, query: string) =>
    (await MailService.ListMessages(accountId, role, query)) ?? [],
  getMessage: (id: string) => MailService.GetMessage(id),
  setRead: (id: string, read: boolean) => MailService.SetRead(id, read),
  setStarred: (id: string, starred: boolean) => MailService.SetStarred(id, starred),
  move: (id: string, role: FolderRole): Promise<MoveResult> => MailService.Move(id, role),
  openExternal: (url: string) => MailService.OpenExternal(url),

  // accounts
  detectSettings: (email: string): Promise<ProviderSettings> => AccountService.DetectSettings(email),
  addAccount: (input: AddAccountInput): Promise<Account> => AccountService.AddAccount(input),
  removeAccount: (id: string) => AccountService.RemoveAccount(id),
  loadSampleData: () => AccountService.LoadSampleData(),
  removeSampleData: () => AccountService.RemoveSampleData(),
  syncNow: () => AccountService.SyncNow(),
  getSyncStatus: (): Promise<SyncStatus> => AccountService.GetSyncStatus(),
  getGoogleSetup: (): Promise<GoogleSetup> => AccountService.GetGoogleSetup(),
  saveGoogleSetup: (clientId: string, clientSecret: string) => AccountService.SaveGoogleSetup(clientId, clientSecret),
  signInWithGoogle: (loginHint: string, name: string): Promise<Account> => AccountService.SignInWithGoogle(loginHint, name),
  reconnectGoogle: (accountId: string) => AccountService.ReconnectGoogle(accountId),
  cancelSignIn: () => AccountService.CancelSignIn(),

  // settings
  getSettings: (): Promise<Settings> => SettingsService.GetSettings(),
  setTheme: (theme: string): Promise<Settings> => SettingsService.SetTheme(theme as Theme),
};

/** Subscribe to a backend event; returns an unsubscribe function. */
export function onEvent<T>(name: "mail:changed" | "sync:status", cb: (data: T) => void): () => void {
  return Events.On(name, (e: { data: unknown }) => cb(e.data as T));
}

/** Turns a rejected Wails call into a readable sentence. */
export function errorMessage(err: unknown): string {
  const raw = err instanceof Error ? err.message : typeof err === "string" ? err : JSON.stringify(err);
  try {
    const parsed = JSON.parse(raw);
    if (parsed && typeof parsed.message === "string") return capitalize(parsed.message);
  } catch {
    /* not JSON */
  }
  return capitalize(raw);
}

function capitalize(s: string) {
  return s ? s[0].toUpperCase() + s.slice(1) : s;
}
