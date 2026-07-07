import * as vscode from "vscode";
import { getSecret } from "./secrets.js";

export function getActiveYamlFile(): string | undefined {
  const editor = vscode.window.activeTextEditor;
  if (!editor) return undefined;

  const filePath = editor.document.uri.fsPath;
  if (filePath.endsWith(".yaml") || filePath.endsWith(".yml")) {
    return filePath;
  }
  return undefined;
}

export function getJiraUrl(): string {
  const setting = vscode.workspace
    .getConfiguration("jiraAiCreator")
    .get<string>("jiraUrl", "");
  return setting || process.env["JIRA_URL"] || "";
}

export function isCloud(): boolean {
  return vscode.workspace
    .getConfiguration("jiraAiCreator")
    .get<boolean>("cloud", false);
}

export function getAiProvider(): string {
  return vscode.workspace
    .getConfiguration("jiraAiCreator")
    .get<string>("aiProvider", "");
}

export function getWorkspaceRoot(): string | undefined {
  return vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;
}

export async function getJiraToken(
  secrets: vscode.SecretStorage,
): Promise<string> {
  const stored = await getSecret(secrets, "jiraToken");
  return stored || process.env["JIRA_TOKEN"] || "";
}

export async function getAnthropicKey(
  secrets: vscode.SecretStorage,
): Promise<string> {
  const stored = await getSecret(secrets, "anthropicKey");
  return stored || process.env["ANTHROPIC_API_KEY"] || "";
}

export function buildGlobalFlags(): string[] {
  const flags: string[] = [];

  const jiraUrl = getJiraUrl();
  if (jiraUrl) {
    flags.push("--jira-url", jiraUrl);
  }

  if (isCloud()) {
    flags.push("--cloud");
  }

  return flags;
}

export async function buildEnv(
  secrets: vscode.SecretStorage,
): Promise<Record<string, string>> {
  const env: Record<string, string> = { ...process.env } as Record<
    string,
    string
  >;

  const jiraToken = await getJiraToken(secrets);
  if (jiraToken) {
    env["JIRA_TOKEN"] = jiraToken;
  }

  const anthropicKey = await getAnthropicKey(secrets);
  if (anthropicKey) {
    env["ANTHROPIC_API_KEY"] = anthropicKey;
  }

  return env;
}
