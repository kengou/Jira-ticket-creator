import * as vscode from "vscode";
import { runCli } from "../cli.js";
import { buildGlobalFlags } from "../config.js";
import { parseArgs } from "../args.js";
import { formatTableOutput, formatErrorOutput } from "../format.js";

const USAGE =
  "### /linktypes — List available issue link types\n\n" +
  "**Usage:**\n" +
  "```\n" +
  "@jira /linktypes [-s <search>]\n" +
  "```\n\n" +
  "| Flag | Description |\n" +
  "|------|-------------|\n" +
  "| `-s`, `--search` | Filter by name, inward, or outward description |\n\n" +
  "**Examples:**\n" +
  "```\n" +
  "@jira /linktypes\n" +
  "@jira /linktypes -s blocks\n" +
  "```\n";

export async function handleLinktypes(
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const parsed = parseArgs(
    request.prompt,
    ["s", "search"],
    [],
  );

  const search =
    parsed.flags["s"] ?? parsed.flags["search"] ?? parsed.positional.join(" ");

  if (request.prompt.trim() === "help") {
    stream.markdown(USAGE);
    return {};
  }

  stream.progress("Fetching link types...");

  const args = ["linktypes", ...buildGlobalFlags()];
  if (search) args.push("-s", search);

  const result = await runCli(args, stream, token);

  const title = search ? `Link Types — "${search}"` : "Link Types";

  if (result.exitCode === 0) {
    stream.markdown(formatTableOutput(result.stdout, result.stderr, title));
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown(
      "### Failed to Fetch Link Types\n\n" + (errors || "Unknown error."),
    );
  }

  return {
    metadata: { command: "linktypes", exitCode: result.exitCode },
  };
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "linktypes") return [];

  if (meta.exitCode === 0) {
    return [
      { prompt: "/linktypes -s blocks", label: "Search: blocks" },
      { prompt: "/linktypes -s relates", label: "Search: relates" },
      { prompt: "/linktypes -s duplicate", label: "Search: duplicate" },
    ];
  }
  return [];
}
