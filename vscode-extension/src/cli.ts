import { execFileSync, spawn } from "child_process";
import * as vscode from "vscode";

export interface CliResult {
  exitCode: number;
  stdout: string;
  stderr: string;
}

const BINARY_NAME = "jira-ai-creator";

let resolvedPath: string | undefined;

export function getBinaryPath(): string {
  const configured = vscode.workspace
    .getConfiguration("jiraAiCreator")
    .get<string>("binaryPath", "");
  if (configured) return configured;

  if (resolvedPath) return resolvedPath;

  try {
    resolvedPath = execFileSync("which", [BINARY_NAME], {
      encoding: "utf-8",
      timeout: 3000,
    }).trim();
  } catch {
    resolvedPath = BINARY_NAME;
  }
  return resolvedPath;
}

export async function checkBinaryExists(): Promise<boolean> {
  return new Promise((resolve) => {
    const proc = spawn(getBinaryPath(), ["--help"], {
      stdio: "ignore",
      timeout: 5000,
    });
    proc.on("error", () => { resolve(false); });
    proc.on("close", (code) => { resolve(code === 0); });
  });
}

export async function runCli(
  args: string[],
  stream: vscode.ChatResponseStream,
  token: vscode.CancellationToken,
): Promise<CliResult> {
  const binaryPath = getBinaryPath();
  const cwd = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  return new Promise((resolve) => {
    const proc = spawn(binaryPath, args, {
      env: { ...process.env },
      cwd,
    });

    let stdout = "";
    let stderr = "";

    proc.stdout.on("data", (data: Buffer) => {
      const text = data.toString();
      stdout += text;
    });

    proc.stderr.on("data", (data: Buffer) => {
      const text = data.toString();
      stderr += text;
    });

    proc.on("error", (err) => {
      stream.markdown(
        `\n**Error:** Could not run \`${binaryPath}\`: ${err.message}\n\n` +
          "Make sure `jira-ai-creator` is installed and on your PATH.\n",
      );
      resolve({ exitCode: 1, stdout, stderr: err.message });
    });

    proc.on("close", (code) => {
      resolve({ exitCode: code ?? 1, stdout, stderr });
    });

    token.onCancellationRequested(() => {
      proc.kill();
    });
  });
}

export async function runCliSilent(args: string[]): Promise<CliResult> {
  const binaryPath = getBinaryPath();
  const cwd = vscode.workspace.workspaceFolders?.[0]?.uri.fsPath;

  return new Promise((resolve) => {
    const proc = spawn(binaryPath, args, {
      env: { ...process.env },
      cwd,
    });

    let stdout = "";
    let stderr = "";

    proc.stdout.on("data", (data: Buffer) => {
      stdout += data.toString();
    });

    proc.stderr.on("data", (data: Buffer) => {
      stderr += data.toString();
    });

    proc.on("error", (err) => {
      resolve({ exitCode: 1, stdout, stderr: err.message });
    });

    proc.on("close", (code) => {
      resolve({ exitCode: code ?? 1, stdout, stderr });
    });
  });
}
