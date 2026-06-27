package memory

// memoryGuidelines is injected into the system prompt when memory is enabled.
// It tells the model when and how to use the remember tool.
const memoryGuidelines = `## Long-Term Memory Guidelines

You have a ` + "`remember`" + ` tool to persist durable facts across sessions. To keep the store useful and avoid noise, follow these rules strictly.

### When to remember
Call ` + "`remember`" + ` when the user provides information with reuse value across future sessions:
- **user** type: Name, role, tech stack, work habits, preferences (e.g. "I prefer Rust over Python").
- **feedback** type: Explicit corrections to your behavior or output style (e.g. "Don't call me boss, call me Xiao Chen", "Always add comments in Chinese").
- **project** type: Active projects, key technical decisions, business context, migration plans (e.g. "We are migrating from MySQL to PostgreSQL next week").
- **reference** type: External resources relevant to the project (docs, dashboards, API endpoints, team contacts).

### What NOT to remember
- Transient small talk, greetings, or casual chat.
- Context specific to the current conversation only (e.g. an error trace you just fixed, a file you just read).
- Sensitive secrets (passwords, API keys, personal IDs, tokens).
- Facts already obvious from repo files, code, or git history.

### Format requirements
Each memory must be atomic and distilled — never copy the user's words verbatim:
- ` + "`name`" + `: Short kebab-case slug, one per fact.
- ` + "`title`" + `: Human-readable label for the memory index.
- ` + "`description`" + `: One-line hook shown in the index.
- ` + "`type`" + `: One of ` + "`user`" + `, ` + "`feedback`" + `, ` + "`project`" + `, ` + "`reference`" + `.
- ` + "`body`" + `: The durable fact in Markdown, written as an objective third-person statement.
- One core fact per memory. Use declarative sentences.

### Thinking process
On every user input, ask yourself: "Does this contain user profile, preferences, corrections, or key facts with future reuse value?" If yes, call ` + "`remember`" + ` before or alongside your reply.
`
