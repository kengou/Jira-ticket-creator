import * as vscode from "vscode";
import { handleValidate, provideFollowups as validateFollowups } from "./commands/validate.js";
import { handlePlan, provideFollowups as planFollowups } from "./commands/plan.js";
import { handleApply, provideFollowups as applyFollowups } from "./commands/apply.js";
import { handleGenerate, provideFollowups as generateFollowups } from "./commands/generate.js";
import { handleEpics, provideFollowups as epicsFollowups } from "./commands/epics.js";
import { handleFields, provideFollowups as fieldsFollowups } from "./commands/fields.js";
import { handleLinktypes, provideFollowups as linktypesFollowups } from "./commands/linktypes.js";

const PARTICIPANT_ID = "jira-ai-creator.jira";

type CommandHandler = (
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
) => Promise<vscode.ChatResult>;

const COMMAND_HANDLERS: Record<string, CommandHandler> = {
  validate: handleValidate,
  plan: handlePlan,
  apply: handleApply,
  generate: handleGenerate,
  epics: handleEpics,
  fields: handleFields,
  linktypes: handleLinktypes,
};

const FOLLOWUP_PROVIDERS = [
  validateFollowups,
  planFollowups,
  applyFollowups,
  generateFollowups,
  epicsFollowups,
  fieldsFollowups,
  linktypesFollowups,
];

export function registerParticipant(
  context: vscode.ExtensionContext,
): void {
  const participant = vscode.chat.createChatParticipant(
    PARTICIPANT_ID,
    handler,
  );

  participant.iconPath = vscode.Uri.joinPath(
    context.extensionUri,
    "icon.png",
  );

  participant.followupProvider = {
    provideFollowups(
      result: vscode.ChatResult,
      _context: vscode.ChatContext,
      _token: vscode.CancellationToken,
    ): vscode.ProviderResult<vscode.ChatFollowup[]> {
      for (const provider of FOLLOWUP_PROVIDERS) {
        const followups = provider(result);
        if (followups.length > 0) return followups;
      }
      return [];
    },
  };

  context.subscriptions.push(participant);
}

async function handler(
  request: vscode.ChatRequest,
  _context: vscode.ChatContext,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const commandHandler = request.command ? COMMAND_HANDLERS[request.command] : undefined;
  if (commandHandler) {
    return commandHandler(request, stream, token);
  }

  if (request.prompt.trim()) {
    return handleGenerate(request, stream, token);
  }

  stream.markdown(
    "### Jira AI Creator\n\n" +
      "| Command | Description | Example |\n" +
      "|---------|-------------|--------|\n" +
      "| `/validate` | Validate the active YAML file | `@jira /validate` |\n" +
      "| `/plan` | Preview issue creation order | `@jira /plan` |\n" +
      "| `/apply` | Create issues in Jira | `@jira /apply --dry-run` |\n" +
      "| `/generate` | Generate YAML from a description | `@jira /generate 5 auth stories` |\n" +
      "| `/epics` | List epics in a project | `@jira /epics -p POM` |\n" +
      "| `/fields` | Search Jira field definitions | `@jira /fields -s epic` |\n" +
      "| `/linktypes` | List issue link types | `@jira /linktypes -s blocks` |\n\n" +
      "Type `help` after any command for full flag details, e.g. `@jira /epics help`\n\n" +
      "Or just type a description to generate a YAML plan:\n" +
      "```\n@jira 5 stories for implementing OAuth2 login\n```\n",
  );

  return {};
}
