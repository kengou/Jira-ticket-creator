# jira-ai-creator

> **Declarative Jira issue creation from YAML** — define once, create anywhere.

A production-ready CLI tool for creating Jira issues from YAML configuration files. Define Epics, Stories, and Bugs declaratively, run one command, and watch your issues appear in Jira with proper dependencies and relationships.

## ⚡ 60-Second Quickstart

```bash
# 1. Build
cd jira-ai-creator && go build -o jira-ai-creator

# 2. Configure credentials
export JIRA_URL="https://jira.yourcompany.com"
export JIRA_TOKEN="your-personal-access-token"

# 3. Create issues
./jira-ai-creator apply -f docs/examples/stories.yaml
```

## Key Features

| Feature | Description |
|---------|-------------|
| **Declarative YAML** | Define issues as code, version control your backlog |
| **Dependency Ordering** | Automatic topological sort respects parent/child and `dependsOn` |
| **Idempotency** | State file prevents duplicate creation on re-runs |
| **Issue Links** | `blocks`, `relates to`, `duplicates` between issues |
| **Dry Run** | Preview changes without touching Jira |
| **Validation** | Catch errors before hitting the API |

## Commands

```bash
jira-ai-creator validate -f issues.yaml   # Check configuration
jira-ai-creator plan     -f issues.yaml   # Preview creation order
jira-ai-creator apply    -f issues.yaml   # Create issues in Jira
jira-ai-creator state    list             # View created issues
```

## Documentation

| Document | Description |
|----------|-------------|
| [**Quickstart**](docs/quickstart.md) | Installation, authentication, first run |
| [**Configuration**](docs/configuration.md) | All config fields with examples |
| [**Schema**](docs/schema.md) | Schema rules, relationships, idempotency |
| [**Troubleshooting**](docs/troubleshooting.md) | Common errors, auth, rate limits |

## Minimal Example

```yaml
schemaVersion: "1.0"

defaults:
  projectKey: MYPROJ

issues:
  - id: EPIC-001
    issueType: Epic
    epicName: "User Authentication"
    summary: "Implement OAuth 2.0 authentication"
    
  - id: STORY-001
    issueType: Story
    parent: EPIC-001
    summary: "Add Google SSO provider"
    description: |
      *As a* user
      *I want to* sign in with Google
      *So that* I don't need another password
```

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

# Jira-ticket-creator
