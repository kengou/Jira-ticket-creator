import * as vscode from "vscode";
import * as fs from "fs";
import { runCli } from "../cli.js";
import { getActiveYamlFile, buildGlobalFlags } from "../config.js";
import { parseArgs } from "../args.js";
import { formatApplyOutput, formatErrorOutput } from "../format.js";

const USAGE =
  "### /apply — Create issues in Jira\n\n" +
  "**Usage:**\n" +
  "```\n" +
  "@jira /apply [--dry-run] [--continue-on-error]\n" +
  "```\n\n" +
  "| Flag | Description |\n" +
  "|------|-------------|\n" +
  "| `--dry-run` | Simulate creation without making API calls |\n" +
  "| `--continue-on-error` | Continue creating issues even if one fails |\n\n" +
  "The active YAML file in the editor is used as input.\n\n" +
  "**Examples:**\n" +
  "```\n" +
  "@jira /apply\n" +
  "@jira /apply --dry-run\n" +
  "@jira /apply --continue-on-error\n" +
  "```\n";

export async function handleApply(
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const parsed = parseArgs(
    request.prompt,
    [],
    ["dry-run", "continue-on-error", "help"],
  );

  if (parsed.boolFlags.has("help") || request.prompt.trim() === "help") {
    stream.markdown(USAGE);
    return {};
  }

  const file = getActiveYamlFile();
  if (!file) {
    stream.markdown(
      "No YAML file is open in the active editor. Open a jira-ai-creator YAML config file and try again.",
    );
    return {};
  }

  const dryRun = parsed.boolFlags.has("dry-run");

  const content = fs.readFileSync(file, "utf-8");
  const needsReview = content.includes("# reviewed: false");

  if (needsReview && !dryRun) {
    stream.markdown(
      "### Review Required\n\n" +
        "This file is AI-generated and contains `# reviewed: false`.\n\n" +
        "Review the file, then mark it as reviewed before applying.\n\n" +
        "Or use `@jira /apply --dry-run` to preview without creating issues.\n",
    );
    stream.button({
      command: "jira-ai-creator.markReviewed",
      title: "Mark as Reviewed",
      arguments: [file],
    });
    return {
      metadata: { command: "apply", blocked: true },
    };
  }

  const label = dryRun ? "Simulating issue creation..." : "Creating issues in Jira...";
  stream.progress(label);

  const args = ["apply", "-f", file, "--yes", ...buildGlobalFlags()];
  if (dryRun) args.push("--dry-run");
  if (parsed.boolFlags.has("continue-on-error")) args.push("--continue-on-error");

  const result = await runCli(args, stream, token);

  const combined = [result.stdout, result.stderr].filter(Boolean).join("\n");

  if (result.exitCode === 0) {
    stream.markdown(formatApplyOutput(combined));
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown("### Apply Failed\n\n" + (errors || "Unknown error."));
  }

  return {
    metadata: {
      command: "apply",
      file,
      exitCode: result.exitCode,
    },
  };
}

export function registerApplyCommands(
  context: vscode.ExtensionContext,
): void {
  context.subscriptions.push(
    vscode.commands.registerCommand(
      "jira-ai-creator.markReviewed",
      (filePath: string) => {
        const content = fs.readFileSync(filePath, "utf-8");
        const updated = content.replace(
          "# reviewed: false",
          "# reviewed: true",
        );
        fs.writeFileSync(filePath, updated, "utf-8");
        vscode.window.showInformationMessage(
          "File marked as reviewed. You can now run /apply.",
        );
      },
    ),
  );
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "apply") return [];

  if (meta.blocked) {
    return [
      { prompt: "/apply --dry-run", label: "Dry run instead" },
      { prompt: "/validate", label: "Validate first" },
    ];
  }
  return [];
}
