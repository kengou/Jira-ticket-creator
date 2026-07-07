import * as vscode from "vscode";
import * as path from "path";
import * as os from "os";
import { runCli } from "../cli.js";
import {
  getAiProvider,
  getWorkspaceRoot,
  buildGlobalFlags,
} from "../config.js";
import { parseArgs } from "../args.js";
import { formatErrorOutput } from "../format.js";

const USAGE =
  "### /generate — Generate YAML plan from a description\n\n" +
  "**Usage:**\n" +
  "```\n" +
  "@jira /generate <description>\n" +
  "@jira /generate -p <description> [-o <output.yaml>] [-d <source-dir>] [-e <epics.yaml>] [--model <model>]\n" +
  "```\n\n" +
  "| Flag | Description |\n" +
  "|------|-------------|\n" +
  "| `-p`, `--prompt` | Description of issues to generate (or just type it directly) |\n" +
  "| `-o`, `--output` | Save to file (default: temp file) |\n" +
  "| `-d`, `--source-dir` | Include source code as context (default: workspace root) |\n" +
  "| `-e`, `--epics` | Existing epics YAML for linking |\n" +
  "| `--model` | AI model override |\n" +
  "| `--no-source` | Skip including workspace source code |\n\n" +
  "**Examples:**\n" +
  "```\n" +
  "@jira /generate 5 stories for OAuth2 login\n" +
  '@jira /generate -p "migration to PostgreSQL" -o migration.yaml\n' +
  "@jira /generate -p auth stories -e epics.yaml --model claude-sonnet-4-5\n" +
  "```\n";

export async function handleGenerate(
  request: vscode.ChatRequest,
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<vscode.ChatResult> {
  const parsed = parseArgs(
    request.prompt,
    ["p", "prompt", "o", "output", "d", "source-dir", "e", "epics", "model"],
    ["no-source"],
  );

  const prompt =
    parsed.flags["p"] ??
    parsed.flags["prompt"] ??
    parsed.positional.join(" ");

  if (!prompt || prompt === "help") {
    stream.markdown(USAGE);
    return {};
  }

  const provider = getAiProvider();
  if (!provider) {
    stream.markdown(
      "### No AI Provider Configured\n\n" +
        "Set `jiraAiCreator.aiProvider` in VS Code settings " +
        "to one of: `claude`, `copilot`, `opencode`.\n\n" +
        "Or run the command **Jira AI Creator: Set Anthropic API Key** from the Command Palette.",
    );
    return {};
  }

  const outputFile =
    parsed.flags["o"] ??
    parsed.flags["output"] ??
    path.join(os.tmpdir(), `jira-ai-${Date.now()}.yaml`);

  stream.progress(`Generating YAML with ${provider}...`);

  const args = [
    "ai",
    `--${provider}`,
    "-p",
    prompt,
    "-o",
    outputFile,
    "--yes",
    ...buildGlobalFlags(),
  ];

  const noSource = parsed.boolFlags.has("no-source");
  const sourceDir =
    parsed.flags["d"] ?? parsed.flags["source-dir"] ?? (noSource ? undefined : getWorkspaceRoot());
  if (sourceDir) args.push("-d", sourceDir);

  const epics = parsed.flags["e"] ?? parsed.flags["epics"];
  if (epics) args.push("-e", epics);

  const model = parsed.flags["model"];
  if (model) args.push("--model", model);

  const result = await runCli(args, stream, token);

  if (result.exitCode === 0) {
    stream.markdown("### YAML Generated\n\n");

    try {
      const content = await vscode.workspace.fs.readFile(
        vscode.Uri.file(outputFile),
      );
      const yaml = Buffer.from(content).toString("utf-8");
      stream.markdown("```yaml\n" + yaml + "```\n\n");
    } catch {
      stream.markdown(`Output saved to \`${outputFile}\`\n\n`);
    }

    stream.button({
      command: "jira-ai-creator.openFile",
      title: "Open in Editor",
      arguments: [outputFile],
    });
  } else {
    const errors = formatErrorOutput(result.stderr);
    stream.markdown(
      "### Generation Failed\n\n" + (errors || "Unknown error."),
    );
  }

  return {
    metadata: {
      command: "generate",
      outputFile,
      exitCode: result.exitCode,
    },
  };
}

export function registerGenerateCommands(
  context: vscode.ExtensionContext,
): void {
  context.subscriptions.push(
    vscode.commands.registerCommand(
      "jira-ai-creator.openFile",
      async (filePath: string) => {
        const doc = await vscode.workspace.openTextDocument(filePath);
        await vscode.window.showTextDocument(doc);
      },
    ),
  );
}

export function provideFollowups(
  result: vscode.ChatResult,
): vscode.ChatFollowup[] {
  const meta: Record<string, unknown> | undefined = result.metadata;
  if (meta?.command !== "generate") return [];

  if (meta.exitCode === 0) {
    return [
      { prompt: "/validate", label: "Validate generated YAML" },
      { prompt: "/plan", label: "Preview creation plan" },
    ];
  }
  return [];
}
