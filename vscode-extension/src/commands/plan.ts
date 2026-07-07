import * as vscode from "vscode";
import { runCli } from "../cli.js";
import { getActiveYamlFile, buildGlobalFlags } from "../config.js";
import { formatPlanOutput, formatErrorOutput } from "../format.js";

export async function handlePlan(
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

  stream.progress("Building creation plan...");

  const args = ["plan", "-f", file, ...buildGlobalFlags()];
  const result = await runCli(args, stream, token);

  if (result.exitCode === 0) {
    stream.markdown(formatPlanOutput(result.stdout));
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown("### Plan Failed\n\n" + (errors || "Unknown error."));
  }

  return {
    metadata: {
      command: "plan",
      file,
      exitCode: result.exitCode,
    },
  };
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "plan") return [];

  if (meta.exitCode === 0) {
    return [
      { prompt: "/apply", label: "Apply to Jira" },
      { prompt: "/apply --dry-run", label: "Dry run first" },
    ];
  }
  return [{ prompt: "/validate", label: "Validate configuration" }];
}
