/* eslint-disable no-misleading-character-class, no-control-regex */
const EMOJI_PATTERN = /[\u{1F300}-\u{1FAFF}\u{2600}-\u{27BF}\u{FE00}-\u{FE0F}\u{1F900}-\u{1F9FF}]\s*/gu;
const ANSI_PATTERN = /\x1B\[[0-9;]*[A-Za-z]/g;
/* eslint-enable no-misleading-character-class, no-control-regex */

function stripEmojisAndAnsi(text: string): string {
  return text.replace(ANSI_PATTERN, "").replace(EMOJI_PATTERN, "").trim();
}

function isBlankOrSeparator(line: string): boolean {
  const stripped = stripEmojisAndAnsi(line);
  return stripped === "" || /^[=\-]{3,}$/.test(stripped); // eslint-disable-line no-useless-escape
}

export function formatValidateOutput(
  stderr: string,
  success: boolean,
): string {
  const lines = stderr.split("\n");
  const parts: string[] = [];

  if (success) {
    parts.push("### Validation Passed\n");
  } else {
    parts.push("### Validation Failed\n");
  }

  let inSummary = false;
  const summaryLines: string[] = [];
  const errorLines: string[] = [];
  const warningLines: string[] = [];

  for (const raw of lines) {
    const line = stripEmojisAndAnsi(raw);
    if (!line) continue;

    if (line.startsWith("Validating:")) continue;

    if (line === "Configuration is valid!") continue;

    if (line.startsWith("Summary:")) {
      inSummary = true;
      continue;
    }

    if (inSummary && line.startsWith("- ")) {
      summaryLines.push(line);
      continue;
    }

    if (inSummary && !line.startsWith("- ") && !line.startsWith("  ")) {
      inSummary = false;
    }

    if (/error/i.test(raw) && !line.startsWith("- ")) {
      errorLines.push(line);
    } else if (/warn/i.test(raw)) {
      warningLines.push(line);
    }
  }

  if (summaryLines.length > 0) {
    parts.push("| Property | Value |");
    parts.push("|----------|-------|");
    for (const sl of summaryLines) {
      const match = /^-\s+(.+?):\s+(.+)$/.exec(sl);
      if (match) {
        parts.push(`| ${match[1]} | ${match[2]} |`);
      } else {
        const sub = /^\s+-\s+(.+?):\s+(.+)$/.exec(sl);
        if (sub) {
          parts.push(`|   ${sub[1]} | ${sub[2]} |`);
        }
      }
    }
    parts.push("");
  }

  if (errorLines.length > 0) {
    parts.push("**Errors:**\n");
    for (const e of errorLines) {
      parts.push(`- ${e}`);
    }
    parts.push("");
  }

  if (warningLines.length > 0) {
    parts.push("**Warnings:**\n");
    for (const w of warningLines) {
      parts.push(`- ${w}`);
    }
    parts.push("");
  }

  return parts.join("\n");
}

interface PlanIssue {
  index: string;
  type: string;
  id: string;
  summary: string;
  parent?: string;
}

export function formatPlanOutput(stderr: string): string {
  const lines = stderr.split("\n");
  const issues: PlanIssue[] = [];
  let totalCount = "";

  for (let i = 0; i < lines.length; i++) {
    const line = stripEmojisAndAnsi(lines[i]);

    const countMatch = /Will create (\d+ issues)/.exec(line);
    if (countMatch) {
      totalCount = countMatch[1];
      continue;
    }

    const issueMatch = /^(\d+)\.\s+\[(\w+)]\s+(\S+):\s+(.+)$/.exec(line);
    if (issueMatch) {
      const issue: PlanIssue = {
        index: issueMatch[1],
        type: issueMatch[2],
        id: issueMatch[3],
        summary: issueMatch[4],
      };

      const nextLine = i + 1 < lines.length ? stripEmojisAndAnsi(lines[i + 1]) : "";
      const parentMatch = /Parent:\s+(\S+)/.exec(nextLine);
      if (parentMatch) {
        issue.parent = parentMatch[1];
        i++;
      }

      issues.push(issue);
    }
  }

  const parts: string[] = [];
  parts.push(`### Creation Plan (${totalCount || `${issues.length} issues`})\n`);

  if (issues.length > 0) {
    parts.push("| # | Type | ID | Summary | Parent |");
    parts.push("|---|------|-----|---------|--------|");
    for (const issue of issues) {
      parts.push(
        `| ${issue.index} | \`${issue.type}\` | \`${issue.id}\` | ${issue.summary} | ${issue.parent ? `\`${issue.parent}\`` : "—"} |`,
      );
    }
    parts.push("");
  }

  return parts.join("\n");
}

interface ApplyIssue {
  index: string;
  total: string;
  id: string;
  summary: string;
  type: string;
  jiraKey?: string;
  status: "created" | "skipped" | "failed" | "dry-run";
}

export function formatApplyOutput(stderr: string): string {
  const lines = stderr.split("\n");
  const issues: ApplyIssue[] = [];
  let dryRun = false;
  const linkInfo: string[] = [];

  for (let i = 0; i < lines.length; i++) {
    const line = stripEmojisAndAnsi(lines[i]);

    if (/DRY RUN/.test(line) && /MODE/.test(line)) {
      dryRun = true;
      continue;
    }

    const issueMatch = /^\[(\d+)\/(\d+)]\s+(\S+):\s+(.+?)\s+\((\w+)\)/.exec(line);
    if (issueMatch) {
      const issue: ApplyIssue = {
        index: issueMatch[1],
        total: issueMatch[2],
        id: issueMatch[3],
        summary: issueMatch[4],
        type: issueMatch[5],
        status: dryRun ? "dry-run" : "created",
      };

      for (let j = i + 1; j < lines.length && j <= i + 3; j++) {
        const next = stripEmojisAndAnsi(lines[j]);
        const keyMatch = /Created:\s+(\S+)/.exec(next);
        if (keyMatch) {
          issue.jiraKey = keyMatch[1];
        }
        if (/Skipped/.test(next)) {
          issue.status = "skipped";
        }
        if (/Failed|Error/.test(next)) {
          issue.status = "failed";
        }
        if (/DRY RUN/.test(next)) {
          issue.status = "dry-run";
        }
      }

      issues.push(issue);
      continue;
    }

    if (/link/i.test(line) && /created|creating/i.test(line)) {
      linkInfo.push(line);
    }
  }

  const parts: string[] = [];
  if (dryRun) {
    parts.push("### Dry Run Results\n");
  } else {
    const created = issues.filter((i) => i.status === "created").length;
    const failed = issues.filter((i) => i.status === "failed").length;
    const skipped = issues.filter((i) => i.status === "skipped").length;
    parts.push(`### Apply Results — ${created} created, ${skipped} skipped, ${failed} failed\n`);
  }

  if (issues.length > 0) {
    parts.push("| # | Type | ID | Summary | Status |");
    parts.push("|---|------|-----|---------|--------|");
    for (const issue of issues) {
      const statusLabel = formatStatus(issue);
      parts.push(
        `| ${issue.index} | \`${issue.type}\` | \`${issue.id}\` | ${issue.summary} | ${statusLabel} |`,
      );
    }
    parts.push("");
  }

  if (linkInfo.length > 0) {
    parts.push("**Links:**\n");
    for (const l of linkInfo) {
      parts.push(`- ${l}`);
    }
    parts.push("");
  }

  return parts.join("\n");
}

function formatStatus(issue: ApplyIssue): string {
  switch (issue.status) {
    case "created":
      return issue.jiraKey ? `\`${issue.jiraKey}\`` : "Created";
    case "skipped":
      return "Skipped";
    case "failed":
      return "Failed";
    case "dry-run":
      return "Would create";
  }
}

export function formatTableOutput(
  stdout: string,
  stderr: string,
  title: string,
): string {
  const parts: string[] = [];
  parts.push(`### ${title}\n`);

  const infoLines = stderr
    .split("\n")
    .map(stripEmojisAndAnsi)
    .filter((l) => l && !isBlankOrSeparator(l));

  for (const line of infoLines) {
    if (/found \d+/i.test(line) || /showing/i.test(line)) {
      parts.push(`*${line}*\n`);
    }
  }

  const stdoutClean = stdout.replace(ANSI_PATTERN, "").trim();
  if (stdoutClean) {
    parts.push("```");
    parts.push(stdoutClean);
    parts.push("```\n");
  }

  return parts.join("\n");
}

export function formatErrorOutput(stderr: string): string {
  const lines = stderr
    .split("\n")
    .map(stripEmojisAndAnsi)
    .filter((l) => l.length > 0);

  const errors = lines.filter(
    (l) => !l.startsWith("Usage:") && !l.startsWith("Flags:") && !l.startsWith("Global Flags:") && !/^\s+-/.test(l) && !/^\s+--/.test(l),
  );

  if (errors.length === 0) return "";

  const parts = ["**Error:**\n"];
  for (const e of errors) {
    if (/^Error:/.test(e)) {
      parts.push(`- ${e.replace(/^Error:\s*/, "")}`);
    } else {
      parts.push(`- ${e}`);
    }
  }
  return parts.join("\n");
}
