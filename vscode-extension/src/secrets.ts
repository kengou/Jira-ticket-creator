import * as vscode from "vscode";

const KEY_PREFIX = "jira-ai-creator";

export async function getSecret(
  storage: vscode.SecretStorage,
  key: string,
): Promise<string | undefined> {
  return storage.get(`${KEY_PREFIX}.${key}`);
}

export async function setSecret(
  storage: vscode.SecretStorage,
  key: string,
  value: string,
): Promise<void> {
  await storage.store(`${KEY_PREFIX}.${key}`, value);
}

export function registerSecretCommands(
  context: vscode.ExtensionContext,
): void {
  context.subscriptions.push(
    vscode.commands.registerCommand(
      "jira-ai-creator.setJiraToken",
      async () => {
        const token = await vscode.window.showInputBox({
          prompt: "Enter your Jira Personal Access Token",
          password: true,
          placeHolder: "Your Jira PAT",
        });
        if (token) {
          await setSecret(context.secrets, "jiraToken", token);
          vscode.window.showInformationMessage(
            "Jira token saved securely.",
          );
        }
      },
    ),
  );

  context.subscriptions.push(
    vscode.commands.registerCommand(
      "jira-ai-creator.setAnthropicKey",
      async () => {
        const key = await vscode.window.showInputBox({
          prompt: "Enter your Anthropic API Key",
          password: true,
          placeHolder: "sk-ant-...",
        });
        if (key) {
          await setSecret(context.secrets, "anthropicKey", key);
          vscode.window.showInformationMessage(
            "Anthropic API key saved securely.",
          );
        }
      },
    ),
  );
}
