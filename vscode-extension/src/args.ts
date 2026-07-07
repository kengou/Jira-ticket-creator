export interface ParsedArgs {
  flags: Partial<Record<string, string>>;
  boolFlags: Set<string>;
  positional: string[];
}

export function parseArgs(
  input: string,
  knownFlags: string[],
  knownBoolFlags: string[],
): ParsedArgs {
  const flags: Record<string, string> = {};
  const boolFlags = new Set<string>();
  const positional: string[] = [];

  const tokens = tokenize(input);
  const flagSet = new Set(knownFlags);
  const boolFlagSet = new Set(knownBoolFlags);

  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i];

    const flagName = parseFlagName(token);
    if (flagName && boolFlagSet.has(flagName)) {
      boolFlags.add(flagName);
      continue;
    }

    if (flagName && flagSet.has(flagName) && i + 1 < tokens.length) {
      flags[flagName] = tokens[++i];
      continue;
    }

    if (!token.startsWith("-")) {
      positional.push(token);
    }
  }

  return { flags, boolFlags, positional };
}

function parseFlagName(token: string): string | undefined {
  if (token.startsWith("--")) return token.slice(2);
  if (token.startsWith("-") && token.length === 2) return token.slice(1);
  return undefined;
}

function tokenize(input: string): string[] {
  const tokens: string[] = [];
  let current = "";
  let inQuote: string | null = null;

  for (const ch of input) {
    if (inQuote) {
      if (ch === inQuote) {
        inQuote = null;
      } else {
        current += ch;
      }
      continue;
    }

    if (ch === '"' || ch === "'") {
      inQuote = ch;
      continue;
    }

    if (ch === " " || ch === "\t") {
      if (current) {
        tokens.push(current);
        current = "";
      }
      continue;
    }

    current += ch;
  }

  if (current) {
    tokens.push(current);
  }

  return tokens;
}
