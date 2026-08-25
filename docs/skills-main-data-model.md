# Skills Main Data Model

This is the default skills catalog model for customers unless Port has explicitly enabled the experimental versioned skills model.

## Structure

The main model stores skills as standard Port catalog entities:

- `skill_group` groups related skills.
- `skill` stores the skill content and metadata.
- A skill can belong to a skill group.

This model is suited for managing the current desired state of each skill. Updates replace the content on the skill entity rather than creating CLI-managed skill versions.

## CLI Support

Supported commands:

- `port skills init`
- `port skills sync`
- `port skills list`
- `port skills search`
- `port skills clear`
- Selection commands such as `port skills add`, `port skills remove`, and `port skills select`

Unsupported commands:

- `port skills upload`
- `port skills publish`
- `port skills unpublish`

Upload and publish commands require the experimental versioned skills data model. They will not work against the main `skill_group => skill` model.

## Skill Files

Synced skills are written to disk following the [Agent Skills specification](https://agentskills.io/specification): a skill directory with `SKILL.md` at the root, plus optional `scripts/`, `references/`, and `assets/`.

`port skills sync` reads the catalog state and writes the selected skills under `skills/{skill-name}/`, where `skill-name` is the Agent Skills `name` written in `SKILL.md`. Groups are used for selection only and are not part of the local path.
