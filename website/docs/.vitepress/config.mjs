// VitePress site config for the Conduit documentation site.
//
// Pick VitePress over Docusaurus / Astro because:
//   * single small dep tree (Vite + Vue) — fits a Go monorepo without bloat
//   * markdown-first, in-repo, PR-reviewable (issue #137 acceptance criteria)
//   * trivial to wire into CI for broken-link checks
export default {
  title: 'Conduit',
  description: 'A personal AI agent harness for macOS — local-first, model-agnostic, owned not rented.',
  lang: 'en-US',
  cleanUrls: true,
  lastUpdated: true,
  ignoreDeadLinks: false,
  themeConfig: {
    logo: '/logo.svg',
    nav: [
      { text: 'Getting Started', link: '/getting-started/' },
      { text: 'Guides', link: '/guides/' },
      { text: 'Reference', link: '/reference/configuration' },
      { text: 'API', link: '/reference/api' },
      { text: 'Plugins', link: '/plugins/' },
      { text: 'Architecture', link: '/architecture/' },
      { text: 'FAQ', link: '/faq' },
    ],
    sidebar: {
      '/getting-started/': [
        {
          text: 'Getting Started',
          items: [
            { text: 'Overview', link: '/getting-started/' },
            { text: 'Install', link: '/getting-started/install' },
            { text: 'First run', link: '/getting-started/first-run' },
            { text: 'Concepts', link: '/getting-started/concepts' },
          ],
        },
      ],
      '/guides/': [
        {
          text: 'Guides',
          items: [
            { text: 'Overview', link: '/guides/' },
            { text: 'Coding agent', link: '/guides/coding-agent' },
            { text: 'Workflows', link: '/guides/workflows' },
            { text: 'Memory & sessions', link: '/guides/memory-sessions' },
          ],
        },
      ],
      '/reference/': [
        {
          text: 'Reference',
          items: [
            { text: 'Configuration', link: '/reference/configuration' },
            { text: 'CLI', link: '/reference/cli' },
            { text: 'API', link: '/reference/api' },
            { text: 'Keybindings', link: '/reference/keybindings' },
          ],
        },
      ],
      '/plugins/': [
        {
          text: 'Plugins',
          items: [
            { text: 'Overview', link: '/plugins/' },
            { text: 'Plugin development', link: '/plugins/development' },
          ],
        },
      ],
      '/architecture/': [
        {
          text: 'Architecture',
          items: [
            { text: 'Overview', link: '/architecture/' },
            { text: 'Decision records', link: '/architecture/adr' },
          ],
        },
      ],
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/jabreeflor/conduit' },
    ],
    editLink: {
      pattern: 'https://github.com/jabreeflor/conduit/edit/main/website/docs/:path',
      text: 'Edit this page on GitHub',
    },
    footer: {
      message: 'Released under the MIT License.',
      copyright: 'Copyright © Conduit contributors',
    },
    search: { provider: 'local' },
  },
}
