# Git Workflow

## Commit Message Format
```
<type>: <description>

<optional body>
```

Types: feat, fix, refactor, docs, test, chore, perf, ci

## Branch Naming
```
feat/<ticket-id>
```

## Rules
- One commit per ticket — no mixing unrelated changes
- Always `git add -A && git commit && git push origin HEAD` before exiting
- Never work directly on main/master
- Use `git push -u origin HEAD` for new branches
