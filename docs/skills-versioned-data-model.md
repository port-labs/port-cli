# Experimental Versioned Skills Data Model

This data model is experimental and is not enabled for all customers. Contact Port to enable the new skills data model with versioning before using the upload and publish commands.

## Structure

The experimental model stores skills across system blueprints:

- `_skill` stores skill metadata and the active version relation.
- `_skill_version` stores version metadata.
- `_skill_file` stores files for each skill version.
- `_skill_group` groups related skills.

This model separates skill identity from skill versions so the CLI can upload new versions and control which version is active.

## CLI Support

Supported commands:

- `port skills upload`
- `port skills publish`
- `port skills unpublish`
- `port skills sync`
- `port skills list`
- `port skills search`

`port skills upload` and `port skills publish` are supported only by this experimental versioned model. They will not work against the main `skill_group => skill` model.

## Uploading Skills

Uploaded skills follow the [Agent Skills specification](https://agentskills.io/specification): each skill is a directory with `SKILL.md` at the root, plus optional `scripts/`, `references/`, and `assets/`.

```sh
# Single skill directory
port skills upload ./my-skill --publish

# Bundle: e.g. ./claude/skills with skill-a/SKILL.md, skill-b/SKILL.md
port skills upload ./claude/skills --publish
```

`upload` upserts through the Skills API: a missing skill is created, and an existing skill receives a new patch version instead of returning a conflict.

The skill folder name, optional `--identifier`, and `SKILL.md` frontmatter `name:` must all match after normalization.

`--publish` makes the uploaded version active. Without it, the active version is unchanged.
