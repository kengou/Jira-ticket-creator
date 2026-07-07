import * as vscode from "vscode";
import { checkBinaryExists } from "./cli.js";
import { registerParticipant } from "./participant.js";
import { registerSecretCommands } from "./secrets.js";
import { registerApplyCommands } from "./commands/apply.js";
import { registerGenerateCommands } from "./commands/generate.js";

export async function activate(
  context: vscode.ExtensionContext,
): Promise<void> {
  registerParticipant(context);
  registerSecretCommands(context);
  registerApplyCommands(context);
  registerGenerateCommands(context);

  const exists = await checkBinaryExists();
  if (!exists) {
    const action = await vscode.window.showWarningMessage(
      "jira-ai-creator binary not found. Install it to use the @jira chat participant.",
      "Install Instructions",
      "Configure Path",
    );

    if (action === "Install Instructions") {
      vscode.env.openExternal(
        vscode.Uri.parse(
          "https://github.com/kengou/jira-ticket-creator#installation",
        ),
      );
    } else if (action === "Configure Path") {
      vscode.commands.executeCommand(
        "workbench.action.openSettings",
        "jiraAiCreator.binaryPath",
      );
    }
  }
}

export function deactivate(): void {}
