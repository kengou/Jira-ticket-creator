import * as vscode from "vscode";
import { runCli } from "../cli.js";
import { getActiveYamlFile, buildGlobalFlags } from "../config.js";
import { formatValidateOutput, formatErrorOutput } from "../format.js";

export async function handleValidate(
  _request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const file = getActiveYamlFile();
  if (!file) {
    stream.markdown(
      "No YAML file is open in the active editor. Open a jira-ai-creator YAML config file and try again.",
    );
    return {};
  }

  stream.progress("Validating configuration...");

  const args = ["validate", "-f", file, ...buildGlobalFlags()];
  const result = await runCli(args, stream, token);

  if (result.exitCode === 0) {
    stream.markdown(formatValidateOutput(result.stderr, true));
  } else {
    const formatted = formatValidateOutput(result.stderr, false);
    const errors = formatErrorOutput(result.stderr);
    stream.markdown(formatted + (errors ? "\n" + errors : ""));
  }

  return {
    metadata: {
      command: "validate",
      file,
      exitCode: result.exitCode,
    },
  };
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "validate") return [];

  if (meta.exitCode === 0) {
    return [
      { prompt: "/plan", label: "Preview creation plan" },
      { prompt: "/apply", label: "Apply to Jira" },
    ];
  }
  return [];
}
