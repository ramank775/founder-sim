# Licensing

Founder Sim is two things under two licences, deliberately.

## Code: AGPL-3.0-only

Everything outside `content/` — the engine, server, bridge, content tool, built-in plugins, templates and docs — is licensed under the **GNU Affero General Public License v3.0** (`LICENSE`). You can run it, change it and redistribute it; if you modify it and let other people use it over a network (host it), you must make your modified source available to them under the same licence. Improvements to the engine come back to the community; nobody can run a closed hosted fork of it.

SPDX: `AGPL-3.0-only`. Copyright (c) 2026 Raman (ramank775) and contributors.

### Additional permission for plugins and bridged services (AGPL §7)

As an additional permission under section 7 of the GNU AGPL v3, the copyright holders grant the following:

> A program that interacts with Founder Sim **solely** through the plugin protocol (`docs/PLUGIN_PROTOCOL.md`, the HTTP+JSON interface served or consumed by `plugin/httpadapter`) or through the bridge protocol (`bridge/protocol.go`), and that is not linked into the Founder Sim binary, is not considered a work based on Founder Sim for the purposes of this licence. Such programs may be distributed under any terms, including proprietary terms, and using them does not make your Founder Sim deployment a modified version.

In plain words: write a plugin in any language, keep it under any licence. Plugins compiled into the Founder Sim binary (Go packages under `plugins/` that the server imports) are part of the program and are AGPL.

### Contributions

By contributing code you agree it is licensed under AGPL-3.0-only. The project may later offer the code under an additional commercial licence to organisations that cannot accept AGPL; to keep that possible, contributions of substantial size may be asked to sign a simple contributor licence agreement. Until such a CLA exists, contributions are accepted under AGPL-3.0-only alone.

## Content: CC BY-SA 4.0

Everything under `content/` — facts, hidden-checklist items, friction templates, archetypes, scenario cards, stories, manifests and reports — is licensed under **Creative Commons Attribution-ShareAlike 4.0 International** (`content/LICENSE`). Use it, translate it, build on it, sell it; credit "Founder Sim (https://github.com/ramank775/founder-sim)", say what you changed, and license your derived content under the same terms.

Facts marked `provenance: "real"` cite their public sources; the sources themselves are not ours and keep their own terms. Facts marked `estimated` or `invented` are our own work.

SPDX: `CC-BY-SA-4.0`.

## Why the split

Code licences are a poor fit for data, and the content is the part of this project that most deserves to stay open as it improves. AGPL keeps the engine open when it is hosted; share-alike keeps the knowledge base open when it is republished. Both are real open licences, so "open source" here means what it says.
