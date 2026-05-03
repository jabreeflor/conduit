# Conduit documentation site

This directory holds the source for the [Conduit] documentation site, built
with [VitePress].

## Why VitePress?

The [#137 issue] gave us a choice between VitePress, Docusaurus, and Astro.
We picked VitePress because:

- **Lightest footprint.** One dev dep (Vite + Vue), no React tree, fast cold
  builds — appropriate for a Go monorepo where the docs site is a side
  concern.
- **Markdown-first.** Authors edit `.md` files; docs are reviewed in PRs
  alongside the code that changed.
- **Local search out of the box.**

## Run locally

```sh
cd website
npm install
npm run dev          # http://localhost:5173
npm run build
npm run check-links  # CI runs this too
```

## Layout

```
website/
├── docs/                       # markdown sources
│   ├── .vitepress/config.mjs   # site config, nav, sidebar
│   ├── index.md                # home
│   ├── getting-started/        # install, first run, concepts
│   ├── guides/                 # task walkthroughs
│   ├── reference/              # config, CLI, API, keybindings
│   ├── plugins/                # plugin dev guide
│   ├── architecture/           # arch overview + ADR index
│   └── faq.md
├── scripts/check-links.mjs     # zero-dep in-repo broken-link checker
└── package.json
```

## Adding a page

1. Create `docs/<section>/<slug>.md`.
2. Add it to the sidebar in `docs/.vitepress/config.mjs`.
3. Run `npm run check-links` before pushing.

[Conduit]: https://github.com/jabreeflor/conduit
[VitePress]: https://vitepress.dev
[#137 issue]: https://github.com/jabreeflor/conduit/issues/137
