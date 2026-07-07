import * as vscode from "vscode";
import { runCli } from "../cli.js";
import { buildGlobalFlags } from "../config.js";
import { parseArgs } from "../args.js";
import { formatTableOutput, formatErrorOutput } from "../format.js";

const USAGE =
  "### /epics — List existing epics in a Jira project\n\n" +
  "**Usage:**\n" +
  "```\n" +
  "@jira /epics -p <PROJECT_KEY> [-s <status>] [-o <output.yaml>]\n" +
  "```\n\n" +
  "| Flag | Description |\n" +
  "|------|-------------|\n" +
  "| `-p`, `--project` | Jira project key **(required)** |\n" +
  "| `-s`, `--status` | Filter by status (e.g. `Done`, `\"In Progress\"`, `NOT:Done`) |\n" +
  "| `-o`, `--output` | Save epics to a YAML file |\n\n" +
  "**Examples:**\n" +
  "```\n" +
  "@jira /epics -p POM\n" +
  '@jira /epics -p POM -s "NOT:Done"\n' +
  "@jira /epics -p POM -s Done -o epics.yaml\n" +
  "```\n";

export async function handleEpics(
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const parsed = parseArgs(
    request.prompt,
    ["p", "project", "s", "status", "o", "output"],
    [],
  );

  const project =
    parsed.flags["p"] ?? parsed.flags["project"] ?? parsed.positional.at(0) ?? "";

  if (!project || project === "help") {
    stream.markdown(USAGE);
    return { metadata: { command: "epics", needsProject: true } };
  }

  const status = parsed.flags["s"] ?? parsed.flags["status"];
  const output = parsed.flags["o"] ?? parsed.flags["output"];

  stream.progress(`Fetching epics for ${project}...`);

  const args = ["epics", "-p", project, ...buildGlobalFlags()];
  if (status) args.push("-s", status);
  if (output) args.push("-o", output);

  const result = await runCli(args, stream, token);

  if (result.exitCode === 0) {
    stream.markdown(
      formatTableOutput(result.stdout, result.stderr, `Epics — ${project}`),
    );
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown(
      "### Failed to Fetch Epics\n\n" + (errors || "Unknown error."),
    );
  }

  return {
    metadata: { command: "epics", exitCode: result.exitCode },
  };
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "epics") return [];

  if (meta.needsProject) {
    return [
      { prompt: "/epics -p PROJECT_KEY", label: "Set project key" },
    ];
  }

  if (meta.exitCode === 0) {
    return [
      { prompt: '/epics -p POM -s "NOT:Done"', label: "Filter by status" },
      { prompt: "/epics -p POM -o epics.yaml", label: "Save as YAML" },
    ];
  }
  return [];
}
