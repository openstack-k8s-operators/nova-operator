# AI assistance for nova-operator

Repo-local skills and agents for AI coding assistants working on nova-operator.
Layout matches [devskills](https://github.com/openstack-k8s-operators/devskills):
skills are thin entry points; agents hold the detailed methodology.

## Layout

```
ai/
├── skills/<name>/SKILL.md    # User-facing entry point (/explainer)
└── agents/<name>/AGENT.md    # Domain knowledge and workflow
```

## Skills

| Skill | Agent | Purpose |
|-------|-------|---------|
| [explainer](skills/explainer/SKILL.md) | [explainer](agents/explainer/AGENT.md) | Onboarding buddy — operator fundamentals and nova-operator specifics |

## Install (local discovery)

Symlink the whole `ai/skills` and `ai/agents` trees into `.claude/` and
`.cursor/` so Claude Code and Cursor pick them up automatically. Run once per
clone — new skills added under `ai/` appear without re-running make:

```bash
make install-ai-skills
```

Restart Cursor (or **Developer: Reload Window**) after the first install.

Remove the symlinks:

```bash
make uninstall-ai-skills
```

This creates project-scoped links only (not committed; see `.gitignore`):

| Source | Linked to |
|--------|-----------|
| `ai/skills/` | `.claude/skills/`, `.cursor/skills/` |
| `ai/agents/` | `.claude/agents/`, `.cursor/agents/` |

## Usage without install

Point the agent at the skill file directly:

```
Follow ai/skills/explainer/SKILL.md — I'm new to nova-operator
```

Or invoke `/explainer` after running `make install-ai-skills`.

Interactive menus use **AskQuestion** in Cursor and **AskUserQuestion** in Claude Code.

## Org-wide skills

Generic operator workflows (`/test-operator`, `/debug-operator`, `/code-review`,
etc.) live in the org-wide
[devskills](https://github.com/openstack-k8s-operators/devskills) plugin — install
that separately via Claude Code marketplace.
