# Port CLI

<img src="./docs/hero.png" width="600" />

<br />

A modular command-line interface for Port that enables authentication, API operations, data import/export, organization migration — using a pluggable module architecture.

## Features

- **Authenticate**: log in to multiple organizations simultaneously
- **API Operations**: Direct CRUD operations on Port resources
- **Import**: Restore data from backups
- **Export**: Backup Port data (blueprints, entities, scorecards, actions, teams, automations, pages, integrations)
- **Migrate**: Transfer data between Port organizations
- **Compare**: Diff two Port organizations and generate reports (text, JSON, HTML)
- **Clear**: Bulk-delete org resources (blueprints, entities, actions, scorecards, automations, pages)
- **Skills**: Sync AI skills from Port into your local AI coding tools (Cursor, Claude Code, Gemini CLI, OpenAI Codex, Windsurf, GitHub Copilot)

For full documentation, see the [Port CLI docs](https://docs.port.io/context-lake/consuming-the-lake/port-cli/overview):

- [Installation and configuration](https://docs.port.io/context-lake/consuming-the-lake/port-cli/installation)
- [Usage and examples](https://docs.port.io/context-lake/consuming-the-lake/port-cli/examples) (including AI skill hooks)

For deeper skill-hook setup notes that are still CLI-repo specific, see [docs/skills-setup.md](docs/skills-setup.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## Release Process

See [RELEASE.md](RELEASE.md) for release procedures.

## License

MIT License - see [LICENSE](LICENSE)

## References

- [Port Documentation](https://docs.port.io)
- [Port API Reference](https://docs.port.io/api-reference/port-api)

---

<picture>
  <source media="(prefers-color-scheme: dark)"
  srcset="./docs/Port_logo_black.svg">
  <img width="300" src="./docs/Port_logo_white.svg">
</picture>
