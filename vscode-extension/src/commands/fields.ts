import * as vscode from "vscode";
import { runCli } from "../cli.js";
import { buildGlobalFlags } from "../config.js";
import { parseArgs } from "../args.js";
import { formatTableOutput, formatErrorOutput } from "../format.js";

const USAGE =
  "### /fields — Search Jira field definitions\n\n" +
  "**Usage:**\n" +
  "```\n" +
  "@jira /fields [-s <search>] [--custom-only]\n" +
  "```\n\n" +
  "| Flag | Description |\n" +
  "|------|-------------|\n" +
  "| `-s`, `--search` | Filter fields by name (case-insensitive) |\n" +
  "| `--custom-only` | Show only custom fields |\n\n" +
  "**Examples:**\n" +
  "```\n" +
  "@jira /fields -s epic\n" +
  "@jira /fields --custom-only\n" +
  "@jira /fields -s epic --custom-only\n" +
  "```\n";

export async function handleFields(
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const parsed = parseArgs(
    request.prompt,
    ["s", "search"],
    ["custom-only"],
  );

  const search =
    parsed.flags["s"] ?? parsed.flags["search"] ?? parsed.positional.join(" ");
  const customOnly = parsed.boolFlags.has("custom-only");

  if (!search && !customOnly && request.prompt.trim() === "help") {
    stream.markdown(USAGE);
    return {};
  }

  stream.progress("Fetching Jira fields...");

  const args = ["fields", ...buildGlobalFlags()];
  if (search) args.push("-s", search);
  if (customOnly) args.push("--custom-only");

  const result = await runCli(args, stream, token);

  let title = "Jira Fields";
  if (search && customOnly) title = `Custom Fields — "${search}"`;
  else if (search) title = `Jira Fields — "${search}"`;
  else if (customOnly) title = "Custom Fields";

  if (result.exitCode === 0) {
    stream.markdown(formatTableOutput(result.stdout, result.stderr, title));
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown(
      "### Failed to Fetch Fields\n\n" + (errors || "Unknown error."),
    );
  }

  return {
    metadata: { command: "fields", exitCode: result.exitCode },
  };
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "fields") return [];

  if (meta.exitCode === 0) {
    return [
      { prompt: "/fields -s epic", label: "Search for epic fields" },
      { prompt: "/fields --custom-only", label: "Custom fields only" },
      { prompt: "/fields -s epic --custom-only", label: "Custom epic fields" },
    ];
  }
  return [];
}
