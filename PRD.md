# Conduit GUI — Product Requirements (Gherkin Scenarios)

This document specifies the observable behavior of the Conduit GUI as
**Gherkin-style acceptance scenarios**. It is a requirements/specification
artifact: it enumerates *what* each part of the GUI must do and the exact cases
worth testing against. It deliberately contains **no test code** — only
`Feature` / `Scenario` / `Scenario Outline` documentation that an engineer or QA
author can later implement against.

## Scope — two GUI surfaces

Conduit has two distinct GUI surfaces, and both are documented here:

1. **Desktop app (`spike/gui`)** — a runnable Tauri + React/TypeScript macOS
   application. It has nine top-level views (welcome, chat, projects, new
   project, workspace, soul, settings, plugins, automations) plus cross-cutting
   chrome (sidebar, top bar, Spotlight command palette, theming). Sections 1–5.

2. **Native GUI model layer (`internal/gui`)** — a Go package (`package gui`,
   mirrored identically in `internal/surface/gui`) that holds the **headless
   state/layout view-models** for the production native macOS GUI. The rendering
   layer reads these models to decide what to draw. Its behavior is pinned by
   table-driven `_test.go` files. Sections 6–7.

## How these scenarios were derived

- **React/Tauri side:** grounded directly in the current component source
  (`spike/gui/src/**`). This surface currently ships **no automated tests**, so
  every scenario reflects observed source behavior, not a pre-existing test.
- **Go native side:** grounded in the source under `internal/gui/**` *and* the
  exact constants, clamping ranges, fractions, enum ordinals, and state
  transitions pinned by the matching `_test.go` files. Table-driven Go tests are
  rendered as `Scenario Outline` blocks with `Examples` tables.

## Conventions

- Scenarios use standard Gherkin: `Feature`, `Background`, `Scenario`,
  `Scenario Outline` + `Examples`, and `Given` / `When` / `Then` / `And` / `But`
  steps. Tags (e.g. `@navigation`, `@streaming`, `@native-gui`) group related
  behavior.
- Scenarios are descriptive of present behavior. Where the code reveals an
  unfinished or placeholder state, that is captured honestly as a scenario
  rather than as an aspirational feature (see caveats below).

## Coverage summary

| # | Area | Scenarios |
|---|------|-----------|
| 1 | App Shell, Navigation & Theming | 81 |
| 2 | Spotlight & Global Search | 61 |
| 3 | Welcome, Chat & Agent Streaming | 99 |
| 4 | Projects, Workspace & Agent Sessions | 89 |
| 5 | Settings, Soul, Plugins, Automations & Integrations | 67 |
| 6 | Native GUI — Layout & Core Views | 105 |
| 7 | Native GUI — Developer Tooling Views | 137 |
| | **Total** | **639** |

## Known caveats surfaced while authoring (current spike state)

These reflect the present implementation and are captured so the scenarios are
honest about what exists today:

- **New Project persistence is partial.** The form persists only
  name / description / infrastructure / visibility (the `NewProjectDraft`) to
  `localStorage` under `conduit.localProjects`. Contextual assets and agent
  sessions configured in the form are presentational spike state and are **not**
  saved.
- **`AgentSessionsPage` is not wired in.** It imports `getAgents` /
  `createAgent` / `AgentTemplate` from `../api`, which are not exported there,
  and the component is not mounted from `App.tsx`. Documented with that caveat.
- **`IntegrationCard` is presentational.** It has no connected/disconnected
  state; clicks are a no-op. No connect/disconnect flow is invented.
- **Plugins and Automations pages are "coming soon"** empty states with
  permanently-disabled call-to-action buttons.
- **Theme changes are local-only** (no network); the backend `/api/settings`
  PATCH validates only the provider name (`anthropic` / `codex` / `echo`), not
  the chosen model. The front end hard-codes the provider→models mapping and
  per-provider defaults.
- **`internal/gui` and `internal/surface/gui` are identical duplicates;** the
  native scenarios apply to both.

## Table of contents

1. [App Shell, Navigation & Theming](#app-shell-navigation--theming)
2. [Spotlight & Global Search](#spotlight--global-search)
3. [Welcome, Chat & Agent Streaming](#welcome-chat--agent-streaming)
4. [Projects, Workspace & Agent Sessions](#projects-workspace--agent-sessions)
5. [Settings, Soul, Plugins, Automations & Integrations](#settings-soul-plugins-automations--integrations)
6. [Native GUI — Layout & Core Views](#native-gui--layout--core-views)
7. [Native GUI — Developer Tooling Views](#native-gui--developer-tooling-views)

---

## App Shell, Navigation & Theming

These scenarios describe the observable behavior of the Conduit GUI app shell:
top-level view routing, the back/forward history stack, sidebar navigation and
active-state highlighting, theme cycling and persistence, global keyboard
shortcuts (Spotlight), the `?demo=` URL seeding, and the TopBar search/history
chrome. Behavior is grounded in `App.tsx`, `Sidebar.tsx`, `TopBar.tsx`,
`BrandRow.tsx`, `Icon.tsx`, and `useTheme.ts`.

---

@navigation @app-shell
### Feature: Initial view and app shell layout

  The app boots into a single-page shell containing a persistent sidebar and a
  main content area with a TopBar. The view shown on launch is derived from the
  `?demo=` URL parameter, defaulting to the welcome screen.

  Background:
    Given the Conduit GUI has loaded
    And the app shell renders a sidebar, a content column, and a TopBar

  Scenario: Default launch shows the welcome screen
    Given the URL has no "demo" query parameter
    When the app initializes
    Then the active view is "welcome"
    And the history stack contains only the welcome view
    And the back button is disabled
    And the forward button is disabled
    And the welcome screen is shown with the branch label "feat/projects-shell"
    And the right-hand TeamActivity panel is shown

  Scenario: Welcome view auto-focuses only when chat state is reset to "new"
    Given the active view is "welcome"
    When the active chat id is "new"
    Then the welcome screen requests autofocus
    But when the active chat id is null
    Then the welcome screen does not request autofocus

  Scenario Outline: The "?demo=" parameter seeds the initial view
    Given the URL query parameter "demo" is "<demo>"
    When the app initializes
    Then the active view is "<view>"
    And the history stack contains only that single seeded view

    Examples:
      | demo        | view       |
      | chat        | chat       |
      | spotlight   | chat       |
      | projects    | projects   |
      | new-project | newProject |
      | workspace   | workspace  |
      | soul        | soul       |
      | settings    | settings   |
      | plugins     | plugins    |
      | automations | automations|

  Scenario: An unrecognized demo value falls back to welcome
    Given the URL query parameter "demo" is "bogus"
    When the app initializes
    Then the active view is "welcome"

  Scenario: The "?demo=chat" seed marks the chat as a demo chat
    Given the URL query parameter "demo" is "chat"
    When the app initializes
    Then the active view is "chat"
    And the active chat id is "demo"
    And the chat panel renders in demo mode

  Scenario: The "?demo=spotlight" seed opens Spotlight over a demo chat
    Given the URL query parameter "demo" is "spotlight"
    When the app initializes
    Then the active view is "chat"
    And the active chat id is "demo"
    And the Spotlight overlay is open
    And the chat panel renders in demo mode

  Scenario Outline: Each view contributes its own TopBar label
    Given the active view is "<view>"
    Then the TopBar label reads "<label>"

    Examples:
      | view       | label       |
      | welcome    | Conduit     |
      | chat       | Chat        |
      | projects   | Projects    |
      | newProject | New Project |
      | workspace  | Workspace   |
      | soul       | Soul        |
      | settings   | Settings    |
      | plugins    | Plugins     |
      | automations| Automations |

  Scenario Outline: Conditional side panels render only on specific views
    Given the active view is "<view>"
    Then the right-hand panel shown is "<panel>"

    Examples:
      | view       | panel        |
      | welcome    | TeamActivity |
      | chat       | SoulPanel    |
      | projects   | none         |
      | newProject | none         |
      | workspace  | none         |
      | soul       | none         |
      | settings   | none         |
      | plugins    | none         |
      | automations| none         |

---

@navigation
### Feature: Top-level view navigation between the nine views

  The shell routes between nine views: welcome, chat, projects, newProject,
  workspace, soul, settings, plugins, and automations. Navigation pushes onto a
  history stack via the navigate function; navigating to the currently active
  view is a no-op.

  Background:
    Given the Conduit GUI has loaded with the default welcome view

  Scenario: Navigating to a new view advances the history
    Given the active view is "welcome"
    When the user navigates to "projects"
    Then the active view is "projects"
    And the history position advances by one
    And the back button becomes enabled

  Scenario: Navigating to the current view is a no-op
    Given the active view is "projects"
    When the user navigates to "projects"
    Then the active view remains "projects"
    And the history stack is unchanged
    And the history position is unchanged

  Scenario: Browsing projects then creating a new project
    Given the active view is "projects"
    When the user chooses to create a new project
    Then the active view is "newProject"

  Scenario: Cancelling new project returns to the projects list
    Given the active view is "newProject"
    When the user cancels project creation
    Then the active view is "projects"

  Scenario: Creating a project saves it locally and returns to the list
    Given the active view is "newProject"
    When the user creates a project with a valid draft
    Then the project is added to local projects
    And the active view is "projects"

  Scenario: Entering a project opens its workspace
    Given the active view is "projects"
    When the user enters a project titled "Atlas"
    Then the active project becomes "Atlas"
    And the active view is "workspace"

  Scenario: The workspace back action returns to the projects list
    Given the active view is "workspace"
    When the user chooses the workspace back action
    Then the active view is "projects"

  Scenario: Browsing projects from the welcome screen
    Given the active view is "welcome"
    When the user chooses to browse projects from the welcome screen
    Then the active view is "projects"

---

@navigation @chat
### Feature: Starting and selecting chats

  Chats are reached by starting a chat from the welcome screen, selecting an
  existing chat in the sidebar, or starting a session from the workspace.

  Scenario: Starting a chat from the welcome screen
    Given the active view is "welcome"
    When the user starts a chat with the prompt "Refactor the parser"
    Then the pending prompt is set to "Refactor the parser"
    And the active chat id is "active"
    And the active view is "chat"

  Scenario: Starting a chat with an associated project path
    Given the active view is "welcome"
    When the user starts a chat with a prompt and a project path
    Then the pending project path is recorded
    And the active view is "chat"

  Scenario: Starting a chat without a project path clears any prior path
    Given the active view is "welcome"
    When the user starts a chat with a prompt and no project path
    Then the pending project path is cleared
    And the active view is "chat"

  Scenario: Selecting an existing chat from the sidebar
    Given a chat with id "c-123" exists in the sidebar
    When the user selects chat "c-123"
    Then the pending prompt is cleared
    And the active chat id is "c-123"
    And the active view is "chat"

  Scenario: Starting a session from the workspace resets to a new chat
    Given the active view is "workspace"
    When the user starts a session from the workspace
    Then the pending prompt is cleared
    And the active chat id is reset to "new"
    And the active view is "welcome"

  Scenario: The chat panel remounts when the active chat id changes
    Given the active view is "chat" with active chat id "c-1"
    When the user selects a different chat "c-2"
    Then the chat panel is keyed by the new active chat id

---

@navigation @reset
### Feature: "Go to welcome" resets chat state

  The "go" entry point used by the sidebar and Spotlight resets chat state
  before navigating, but only when the target is the welcome view.

  Scenario: Going to welcome clears the pending prompt and starts a new chat
    Given the active view is "chat" with a pending prompt and an active chat id
    When the user goes to "welcome"
    Then the pending prompt is cleared
    And the active chat id is set to "new"
    And the active view is "welcome"

  Scenario: Going to a non-welcome view does not reset chat state
    Given the active view is "welcome" with a pending prompt
    When the user goes to "projects"
    Then the pending prompt is unchanged
    And the active chat id is unchanged
    And the active view is "projects"

  Scenario: Going to the already-active welcome view still resets chat state but does not push history
    Given the active view is "welcome"
    When the user goes to "welcome"
    Then the active chat id is set to "new"
    And the history stack is unchanged because navigating to the current view is a no-op

---

@navigation @history
### Feature: Back/forward history stack

  Navigation maintains a linear history stack with a position cursor. Back and
  forward move the cursor without mutating the stack. Navigating after going
  back truncates the forward branch.

  Background:
    Given the Conduit GUI has loaded with the default welcome view

  Scenario: Back is disabled at the start of history
    Given the history position is at the first entry
    Then the back button is disabled
    And the forward button is disabled

  Scenario: Going back re-enables forward
    Given the user has navigated welcome -> projects
    When the user presses back
    Then the active view is "welcome"
    And the back button is disabled
    And the forward button is enabled

  Scenario: Going forward after going back
    Given the user has navigated welcome -> projects and then pressed back
    When the user presses forward
    Then the active view is "projects"
    And the forward button is disabled
    And the back button is enabled

  Scenario: Forward is disabled at the end of history
    Given the history position is at the last entry
    Then the forward button is disabled

  Scenario: Back at the first entry is clamped and does not underflow
    Given the history position is at the first entry
    When the back handler is invoked
    Then the position remains at the first entry

  Scenario: Forward at the last entry is clamped and does not overflow
    Given the history position is at the last entry
    When the forward handler is invoked
    Then the position remains at the last entry

  Scenario: Navigating after going back truncates the forward branch
    Given the user has navigated welcome -> projects -> soul
    And the user has pressed back twice so the active view is "welcome"
    When the user navigates to "settings"
    Then the forward entries "projects" and "soul" are discarded
    And the history becomes welcome -> settings
    And the active view is "settings"
    And the forward button is disabled

  Scenario: Deep linear navigation keeps a full back trail
    Given the user navigates welcome -> projects -> newProject -> workspace
    Then pressing back four checks reaches welcome step by step
    And forward is available at every step until the most recent view

---

@navigation @sidebar
### Feature: Sidebar navigation and active-state highlighting

  The sidebar exposes six top-level nav rows (New chat, Projects, Plugins,
  Automations, Soul, Settings). Project sub-views all highlight "Projects", and
  both welcome and chat highlight "New chat".

  Background:
    Given the sidebar is rendered with the nav rows

  Scenario Outline: Nav rows navigate to their target view via the reset-aware "go" handler
    When the user clicks the "<label>" nav row
    Then the navigation target is "<view>"

    Examples:
      | label       | view        |
      | New chat    | welcome     |
      | Projects    | projects    |
      | Plugins     | plugins     |
      | Automations | automations |
      | Soul        | soul        |
      | Settings    | settings    |

  Scenario Outline: "New chat" is highlighted for both welcome and chat views
    Given the active view is "<view>"
    Then the "New chat" nav row is highlighted

    Examples:
      | view    |
      | welcome |
      | chat    |

  Scenario Outline: "Projects" is highlighted for all project sub-views
    Given the active view is "<view>"
    Then the "Projects" nav row is highlighted

    Examples:
      | view       |
      | projects   |
      | newProject |
      | workspace  |

  Scenario Outline: Standalone views highlight their own nav row only
    Given the active view is "<view>"
    Then the "<label>" nav row is highlighted
    And no other nav row is highlighted

    Examples:
      | view        | label       |
      | soul        | Soul        |
      | settings    | Settings    |
      | plugins     | Plugins     |
      | automations | Automations |

  Scenario: The active nav row uses the filled icon variant
    Given the active view is "projects"
    Then the "Projects" nav row renders its icon filled
    And inactive nav rows render their icons unfilled

  Scenario: The sidebar footer "Settings" gear navigates to settings
    When the user clicks the footer settings gear
    Then the navigation target is "settings"

  Scenario: The brand header shows the wordmark and version
    Given the sidebar is rendered with version "0.4.2"
    Then the brand header shows the "Conduit" wordmark
    And the brand header shows "v0.4.2"
    And the brand logo tile shows the "terminal" glyph

  Scenario: The profile identity shows a placeholder until backend info loads
    Given the backend info has not loaded
    Then the profile plan label reads "local session"
    When backend info loads with provider "anthropic" and model "claude"
    Then the profile plan label reads "anthropic · claude"

  Scenario: The profile identity keeps the placeholder when the backend is offline
    Given the backend info request fails
    Then the profile plan label remains "local session"

---

@sidebar @chat-list
### Feature: Sidebar projects and chats lists

  The sidebar lists backend projects (with nested chats) and orphan chats, each
  with empty and error states and add affordances.

  Scenario: Projects load failure shows a serve hint
    Given the projects request fails
    Then the Projects section shows "Start `conduit serve` to load"

  Scenario: No projects shows an empty state
    Given the projects request succeeds with zero projects
    Then the Projects section shows "No projects yet"

  Scenario: No orphan chats shows an empty state
    Given there are zero orphan chats
    Then the Chats section shows "No orphan chats"

  Scenario: Adding a project from the Projects section header
    When the user clicks the add button in the Projects section
    Then the navigation target is "newProject"

  Scenario: Adding a chat from the Chats section header
    When the user clicks the add button in the Chats section
    Then the navigation target is "welcome"

  Scenario: A project row toggles its nested chats open and closed
    Given a project row is rendered expanded by default
    When the user clicks the project row
    Then its nested chats collapse
    When the user clicks the project row again
    Then its nested chats expand

  Scenario: The active chat row is highlighted
    Given the active chat id is "c-9"
    Then the chat row for "c-9" is highlighted
    And other chat rows are not highlighted

  Scenario Outline: Relative timestamps collapse to compact units
    Given a chat created "<ago>"
    Then its timestamp renders as "<text>"

    Examples:
      | ago             | text |
      | under 1 minute  | now  |
      | 5 minutes       | 5m   |
      | 3 hours         | 3h   |
      | 2 days          | 2d   |
      | 3 weeks         | 3w   |
      | 4 months        | 4mo  |

  Scenario: An unparseable timestamp renders empty
    Given a chat with an invalid ISO timestamp
    Then its timestamp renders as an empty string

---

@theming
### Feature: Theme values, cycling, and persistence

  The app supports three themes: light (cream default), dark (warm charcoal),
  and hc (high contrast). The current theme is applied to the document's
  data-theme attribute and persisted to localStorage under "conduit.theme".

  Scenario: The default theme is light when nothing is stored
    Given localStorage has no "conduit.theme" value
    When the theme initializes
    Then the theme is "light"

  Scenario Outline: A stored theme is restored on launch
    Given localStorage "conduit.theme" is "<stored>"
    When the theme initializes
    Then the theme is "<stored>"

    Examples:
      | stored |
      | light  |
      | dark   |
      | hc     |

  Scenario: An invalid stored theme falls back to light
    Given localStorage "conduit.theme" is "neon"
    When the theme initializes
    Then the theme is "light"

  Scenario: The theme drives the document attribute and is persisted
    When the theme is set to "dark"
    Then the document element has data-theme "dark"
    And localStorage "conduit.theme" is "dark"

  Scenario Outline: Cycling advances through the theme order and wraps
    Given the current theme is "<from>"
    When the user cycles the theme
    Then the theme becomes "<to>"

    Examples:
      | from  | to    |
      | light | dark  |
      | dark  | hc    |
      | hc    | light |

  Scenario: Cycling the theme three times returns to the start
    Given the current theme is "light"
    When the user cycles the theme three times
    Then the theme is "light" again

  Scenario: The sidebar theme toggle cycles the theme
    Given the current theme is "light"
    When the user clicks the sidebar theme toggle button
    Then the theme becomes "dark"

  Scenario Outline: The theme toggle exposes the current theme via label and glyph
    Given the current theme is "<theme>"
    Then the theme toggle title reads "Theme: <label>"
    And its accessible label reads "Theme: <label>. Click to switch."
    And it renders the "<icon>" glyph

    Examples:
      | theme | label          | icon       |
      | light | Light          | light_mode |
      | dark  | Dark           | dark_mode  |
      | hc    | High contrast  | contrast   |

  Scenario: The settings page sets a specific theme directly
    Given the active view is "settings"
    When the user selects the "hc" theme on the settings page
    Then the theme is set directly to "hc"
    And the document element has data-theme "hc"

---

@keyboard @spotlight
### Feature: Global keyboard shortcuts for Spotlight

  A global keydown listener toggles the Spotlight overlay with Cmd+K or
  Alt+Space, and dismisses it with Escape.

  Background:
    Given the Conduit GUI has loaded
    And the Spotlight overlay is closed

  Scenario: Cmd+K opens Spotlight
    When the user presses Cmd+K
    Then the default key action is prevented
    And the Spotlight overlay is open

  Scenario: Cmd+K toggles Spotlight closed when already open
    Given the Spotlight overlay is open
    When the user presses Cmd+K
    Then the Spotlight overlay is closed

  Scenario: Alt+Space opens Spotlight
    When the user presses Alt+Space
    Then the default key action is prevented
    And the Spotlight overlay is open

  Scenario: Alt+Space toggles Spotlight closed when already open
    Given the Spotlight overlay is open
    When the user presses Alt+Space
    Then the Spotlight overlay is closed

  Scenario: Escape closes an open Spotlight
    Given the Spotlight overlay is open
    When the user presses Escape
    Then the Spotlight overlay is closed

  Scenario: Escape while Spotlight is closed is a no-op
    Given the Spotlight overlay is closed
    When the user presses Escape
    Then the Spotlight overlay stays closed

  Scenario: Closing Spotlight via its close control
    Given the Spotlight overlay is open
    When the user closes Spotlight via its close control
    Then the Spotlight overlay is closed

---

@spotlight @navigation
### Feature: Spotlight actions

  Spotlight emits action ids that the shell maps to navigation, new chat,
  session opening, and theme cycling.

  Scenario: A session action id opens a fresh welcome chat
    Given the Spotlight overlay is open
    When Spotlight emits the action "ses:abc"
    Then the chat state resets to a new chat
    And the active view is "welcome"

  Scenario Outline: Navigation actions route to their views
    Given the Spotlight overlay is open
    When Spotlight emits the action "<action>"
    Then the active view is "<view>"

    Examples:
      | action          | view       |
      | nav:new-chat    | welcome    |
      | nav:projects    | projects   |
      | nav:new-project | newProject |
      | nav:soul        | soul       |
      | nav:settings    | settings   |

  Scenario: The new-chat Spotlight action resets chat state
    Given the active view is "chat" with a pending prompt
    When Spotlight emits the action "nav:new-chat"
    Then the pending prompt is cleared
    And the active chat id is reset to "new"
    And the active view is "welcome"

  Scenario: The cycle-theme Spotlight action advances the theme without navigating
    Given the current theme is "light"
    And the active view is "projects"
    When Spotlight emits the action "act:cycle-theme"
    Then the theme becomes "dark"
    And the active view remains "projects"

  Scenario: An unrecognized Spotlight action is ignored
    Given the active view is "projects"
    When Spotlight emits an unknown action id
    Then nothing changes

---

@topbar @search
### Feature: TopBar search and history chrome

  The TopBar shows back/forward buttons, a search field whose placeholder
  varies by view, and a context label. The search field is read-only and opens
  Spotlight on focus or Enter.

  Scenario Outline: The search placeholder varies by active view
    Given the active view is "<view>"
    Then the TopBar search placeholder reads "<placeholder>"

    Examples:
      | view       | placeholder                       |
      | welcome    | Search resources or actions…      |
      | projects   | Search systems or workspaces…     |
      | newProject | Search systems or workspaces…     |
      | workspace  | Search systems or workspaces…     |
      | chat       | Search conversations…             |
      | soul       | Search conversations…             |
      | settings   | Search conversations…             |
      | plugins    | Search conversations…             |
      | automations| Search conversations…             |

  Scenario: Focusing the search field opens Spotlight with an empty query
    When the user focuses the TopBar search field
    Then Spotlight opens with an empty query

  Scenario: Pressing Enter in the search field opens Spotlight with the typed query
    When the user types "deploy" and presses Enter in the search field
    Then Spotlight opens seeded with the query "deploy"

  Scenario: The search field is read-only when a search handler is wired
    Given the TopBar has an onSearch handler
    Then the search input is read-only

  Scenario: The back button reflects history availability
    Given the back button receives canBack false
    Then the back button is disabled and shows a not-allowed cursor with reduced opacity

  Scenario: The forward button reflects history availability
    Given the forward button receives canForward false
    Then the forward button is disabled

  Scenario: Clicking back invokes the back handler when enabled
    Given the back button is enabled
    When the user clicks the back button
    Then the history back handler is invoked

  Scenario: Clicking forward invokes the forward handler when enabled
    Given the forward button is enabled
    When the user clicks the forward button
    Then the history forward handler is invoked

  Scenario: Split-view and dock controls are presentational
    Then the TopBar shows non-interactive "Split view" and "Dock" indicators


---

## Spotlight & Global Search

This document specifies the acceptance criteria for Conduit's command palette
("Spotlight") and the global search affordance in the TopBar. All scenarios are
grounded in the observed behavior of `spike/gui/src/Spotlight.tsx`,
`spike/gui/src/App.tsx`, `spike/gui/src/components/TopBar.tsx`, and
`spike/gui/src/api.ts`. No automated test files exist for these areas at the
time of writing; behavior is derived from source.

---

### Feature: Opening and closing Spotlight via keyboard shortcuts

  Spotlight is a modal command palette that overlays the entire app shell. A
  global keydown listener (registered on `window` in App) toggles it open and
  closed, and Escape always forces it closed.

  Background:
    Given the Conduit app shell is rendered
    And Spotlight is closed

  @spotlight @keyboard
  Scenario: Open Spotlight with Cmd+K
    When I press the Meta (Command) key together with "k"
    Then the default browser action for that key combination is prevented
    And Spotlight opens
    And the Spotlight overlay (backdrop and panel) is rendered

  @spotlight @keyboard
  Scenario: Open Spotlight with Alt+Space
    When I press the Alt (Option) key together with the Space key
    Then the default browser action for that key combination is prevented
    And Spotlight opens

  @spotlight @keyboard @toggle
  Scenario: Cmd+K toggles Spotlight closed when already open
    Given Spotlight is open
    When I press Meta together with "k"
    Then Spotlight closes
    And the Spotlight overlay is no longer rendered

  @spotlight @keyboard @toggle
  Scenario: Alt+Space toggles Spotlight closed when already open
    Given Spotlight is open
    When I press Alt together with Space
    Then Spotlight closes

  @spotlight @keyboard @toggle
  Scenario Outline: Repeated shortcut presses flip the open state each time
    Given Spotlight is "<starting>"
    When I press the toggle shortcut "<shortcut>" one time
    Then Spotlight is "<ending>"

    Examples:
      | starting | shortcut  | ending |
      | closed   | Cmd+K     | open   |
      | open     | Cmd+K     | closed |
      | closed   | Alt+Space | open   |
      | open     | Alt+Space | closed |

  @spotlight @keyboard @close
  Scenario: Escape closes an open Spotlight
    Given Spotlight is open
    When I press the Escape key
    Then Spotlight closes

  @spotlight @keyboard @close
  Scenario: Escape while Spotlight is already closed has no effect
    Given Spotlight is closed
    When I press the Escape key
    Then Spotlight remains closed
    And no Spotlight overlay is rendered

  @spotlight @keyboard
  Scenario: Cmd alone or K alone does not open Spotlight
    When I press "k" without holding the Meta key
    Then Spotlight remains closed

  @spotlight @keyboard
  Scenario: Space alone or Alt alone does not open Spotlight
    When I press the Space key without holding the Alt key
    Then Spotlight remains closed

---

### Feature: Opening Spotlight from the TopBar search field

  The TopBar renders a search input. When App passes an `onSearch` handler the
  input is read-only and acts purely as an entry point into Spotlight. Focusing
  the field opens Spotlight with an empty query; pressing Enter opens Spotlight
  seeded with the typed value. The placeholder text is contextual to the
  current view.

  Background:
    Given the Conduit app shell is rendered with the TopBar search wired to openSearch
    And Spotlight is closed

  @search @topbar
  Scenario: Focusing the TopBar search opens Spotlight with an empty query
    When I focus the TopBar search input
    Then openSearch is called with an empty string
    And the persisted Spotlight query is set to empty
    And Spotlight opens
    And the Spotlight input is empty

  @search @topbar
  Scenario: The TopBar search input is read-only when wired to Spotlight
    Given the TopBar search has an onSearch handler
    Then the TopBar search input is read-only
    And typing directly into the TopBar field does not change its value

  @search @topbar @keyboard
  Scenario: Pressing Enter in the TopBar search seeds the Spotlight query
    Given the TopBar search input holds the value "design review"
    When I press Enter in the TopBar search input
    Then openSearch is called with "design review"
    And Spotlight opens
    And the Spotlight input is pre-filled with "design review"

  @search @topbar
  Scenario Outline: The TopBar search placeholder reflects the active view
    Given the active view is "<view>"
    Then the TopBar search placeholder is "<placeholder>"

    Examples:
      | view       | placeholder                       |
      | welcome    | Search resources or actions…      |
      | projects   | Search systems or workspaces…     |
      | newProject | Search systems or workspaces…     |
      | workspace  | Search systems or workspaces…     |
      | chat       | Search conversations…             |
      | soul       | Search conversations…             |
      | settings   | Search conversations…             |

  @search @topbar
  Scenario: Default search placeholder when no view-specific text applies
    Given the TopBar is rendered without a searchPlaceholder prop
    Then the TopBar search placeholder is "Search conversations…"

---

### Feature: Spotlight input focus and initial state

  When the Spotlight panel mounts it auto-focuses its text input and selects any
  existing text, so a seeded query can be immediately overwritten.

  @spotlight @focus
  Scenario: The Spotlight input is focused on open
    When Spotlight opens
    Then the Spotlight text input receives focus

  @spotlight @focus
  Scenario: A seeded query is pre-selected for quick replacement
    Given Spotlight is opened with an initial query of "projects"
    Then the Spotlight input shows "projects"
    And the text "projects" is selected
    When I type "soul"
    Then the selected text is replaced and the input shows "soul"

  @spotlight @ui
  Scenario: Spotlight chrome shows the prompt icon, placeholder, hints, and model label
    When Spotlight is open with an empty query
    Then the input placeholder reads "Ask Conduit…"
    And a terminal prompt icon is shown beside the input
    And the footer shows the hint "ESC to close"
    And the footer shows the hint "↑↓ to navigate"
    And the footer shows the model label "codex/gpt-5.5"

---

### Feature: Static navigation and action commands

  Spotlight always offers a fixed set of built-in commands. Selecting one calls
  the action handler with the command id and then closes the palette. Each id
  maps to a specific navigation or app action in App's handleSpotlightAction.

  Background:
    Given Spotlight is open
    And the query is empty

  @spotlight @commands
  Scenario: The built-in commands are listed in order with their kinds and subtitles
    Then the following commands are present in this order:
      | title           | kind    | subtitle             |
      | New chat        | command | /new                 |
      | Go to Projects  | command | /projects            |
      | New Project     | command | /projects new        |
      | Open Soul       | memory  | /soul                |
      | Settings        | command | /settings            |
      | Switch theme    | command | light · dark · hc    |

  @spotlight @commands @navigation
  Scenario Outline: Selecting a command fires its action id and closes Spotlight
    When I activate the "<title>" command
    Then the action handler is called with id "<id>"
    And Spotlight closes

    Examples:
      | title          | id              |
      | New chat       | nav:new-chat    |
      | Go to Projects | nav:projects    |
      | New Project    | nav:new-project |
      | Open Soul      | nav:soul        |
      | Settings       | nav:settings    |
      | Switch theme   | act:cycle-theme |

  @spotlight @commands @navigation
  Scenario: "New chat" routes to the welcome view and resets chat state
    When I activate the "New chat" command
    Then the app navigates to the "welcome" view
    And the pending prompt is cleared
    And the active chat id becomes "new"

  @spotlight @commands @navigation
  Scenario: "Go to Projects" navigates to the projects view
    When I activate the "Go to Projects" command
    Then the app navigates to the "projects" view

  @spotlight @commands @navigation
  Scenario: "New Project" navigates to the new-project view
    When I activate the "New Project" command
    Then the app navigates to the "newProject" view

  @spotlight @commands @navigation
  Scenario: "Open Soul" navigates to the soul view
    When I activate the "Open Soul" command
    Then the app navigates to the "soul" view

  @spotlight @commands @navigation
  Scenario: "Settings" navigates to the settings view
    When I activate the "Settings" command
    Then the app navigates to the "settings" view

  @spotlight @commands @action
  Scenario: "Switch theme" cycles the theme without changing the view
    Given the current view is "welcome"
    When I activate the "Switch theme" command
    Then the theme is cycled to the next theme (light, dark, then high contrast)
    And the active view remains "welcome"
    And Spotlight closes

  @spotlight @commands @navigation
  Scenario: Navigating to the view the app is already on is a no-op for the history stack
    Given the current view is "projects"
    When I activate the "Go to Projects" command
    Then the action handler is called with id "nav:projects"
    And the history stack is unchanged because the target equals the current view
    And Spotlight closes

---

### Feature: Recent session results

  On open, Spotlight fetches recent sessions from GET /api/sessions and merges
  up to the first 8 into the result list as "session" entries, appended after
  the static commands. Each entry is prefixed with "ses:". The fetch is
  best-effort; failures leave only the static commands.

  @spotlight @sessions @search
  Scenario: Recent sessions are loaded and appended after the commands
    Given the sessions endpoint returns a list of sessions
    When Spotlight opens
    Then up to 8 sessions are shown as "session" results
    And the session results appear after the six static commands
    And each session id is prefixed with "ses:"

  @spotlight @sessions
  Scenario: Only the first eight sessions are shown
    Given the sessions endpoint returns 12 sessions
    When Spotlight opens
    Then exactly 8 session results are listed

  @spotlight @sessions
  Scenario: A session uses its summary as the title and a relative time as the subtitle
    Given a session with summary "Refactor auth flow" created 5 minutes ago
    When Spotlight opens
    Then a session result titled "Refactor auth flow" is shown
    And its subtitle reads "5m ago"

  @spotlight @sessions
  Scenario: A session without a summary falls back to its id for title and search
    Given a session with an empty summary and id "sess-abc123"
    When Spotlight opens
    Then a session result titled "sess-abc123" is shown
    And its searchable text is the lowercased id

  @spotlight @sessions
  Scenario Outline: Relative time subtitle formatting
    Given a session created "<age>"
    Then its subtitle reads "<subtitle>"

    Examples:
      | age                       | subtitle  |
      | less than 1 minute ago    | just now  |
      | 30 minutes ago            | 30m ago   |
      | 3 hours ago               | 3h ago    |
      | 2 days ago                | 2d ago    |
      | an unparseable timestamp  | session   |

  @spotlight @sessions @offline
  Scenario: Sessions endpoint failure leaves only the static commands
    Given the sessions endpoint is unavailable or errors
    When Spotlight opens
    Then no session results are shown
    And only the six static commands are listed

  @spotlight @sessions @navigation
  Scenario: Activating any session result routes to the welcome view
    Given a session result with id "ses:sess-abc123" is present
    When I activate that session result
    Then the action handler is called with id "ses:sess-abc123"
    And the app navigates to the "welcome" view
    And the pending prompt is cleared
    And the active chat id becomes "new"
    And Spotlight closes

  @spotlight @sessions @async
  Scenario: Sessions arriving after a stale open are discarded
    Given Spotlight is opened and then closed and unmounted before sessions resolve
    When the sessions request finally resolves
    Then the cancelled flag prevents updating state on the unmounted palette

---

### Feature: Filtering results by query

  Typing in the input filters the merged list (commands + sessions) by a
  case-insensitive substring match against each result's searchable text. The
  query is trimmed before matching. An empty query shows everything.

  Background:
    Given Spotlight is open
    And recent sessions have loaded

  @spotlight @search @filter
  Scenario: Empty query shows all commands and sessions
    Given the query is empty
    Then every static command is shown
    And every loaded session is shown

  @spotlight @search @filter
  Scenario: Query is matched case-insensitively against searchable text
    When I type "PROJECT"
    Then "Go to Projects" is shown because its search text includes "project"
    And "New Project" is shown
    And "New chat" is hidden

  @spotlight @search @filter
  Scenario: Leading and trailing whitespace in the query is trimmed before matching
    When I type "  theme  "
    Then the query is trimmed to "theme"
    And "Switch theme" is shown

  @spotlight @search @filter
  Scenario: Matching uses the hidden search field, not only the visible title
    When I type "preferences"
    Then "Settings" is shown because its search text includes "preferences"

  @spotlight @search @filter
  Scenario: Matching against the high-contrast keyword in Switch theme
    When I type "high contrast"
    Then "Switch theme" is shown because its search text includes "high contrast"

  @spotlight @search @filter
  Scenario: Sessions are filtered by their summary text
    Given a session with summary "Refactor auth flow" is loaded
    When I type "auth"
    Then that session result is shown
    And commands that do not include "auth" are hidden

  @spotlight @search @no-results
  Scenario: A query matching nothing shows the empty-state row
    When I type "xyzzy-nonexistent"
    Then no command or session rows are shown
    And a single "No matches" empty row is displayed

  @spotlight @search @no-results
  Scenario: Clearing a no-match query restores the full list
    Given I have typed "xyzzy-nonexistent" and see "No matches"
    When I clear the input
    Then all commands and sessions are shown again
    And the "No matches" row is removed

---

### Feature: Keyboard navigation through results

  The arrow keys move a highlighted cursor through the visible results with
  wrap-around. Enter activates the highlighted result. The cursor resets to the
  top whenever the query changes.

  Background:
    Given Spotlight is open
    And the query is empty
    And the first result is highlighted

  @spotlight @keyboard @navigation
  Scenario: ArrowDown moves the highlight to the next result
    When I press ArrowDown
    Then the default action is prevented
    And the second result is highlighted

  @spotlight @keyboard @navigation
  Scenario: ArrowUp moves the highlight to the previous result
    Given the second result is highlighted
    When I press ArrowUp
    Then the default action is prevented
    And the first result is highlighted

  @spotlight @keyboard @navigation @wrap
  Scenario: ArrowDown wraps from the last result to the first
    Given the last result is highlighted
    When I press ArrowDown
    Then the first result is highlighted

  @spotlight @keyboard @navigation @wrap
  Scenario: ArrowUp wraps from the first result to the last
    Given the first result is highlighted
    When I press ArrowUp
    Then the last result is highlighted

  @spotlight @keyboard @navigation @edge
  Scenario: Arrow keys with no results keep the cursor at index zero
    Given the query is "xyzzy-nonexistent" so there are no results
    When I press ArrowDown
    Then the cursor stays at index 0
    When I press ArrowUp
    Then the cursor stays at index 0

  @spotlight @keyboard @activate
  Scenario: Enter activates the currently highlighted result
    Given the "Go to Projects" command is highlighted
    When I press Enter
    Then the default action is prevented
    And the action handler is called with id "nav:projects"
    And Spotlight closes

  @spotlight @keyboard @activate @edge
  Scenario: Enter with no highlighted result does nothing
    Given the query is "xyzzy-nonexistent" so there are no results
    When I press Enter
    Then no action handler is called
    And Spotlight stays open

  @spotlight @keyboard @navigation
  Scenario: Changing the query resets the highlight to the first result
    Given the third result is highlighted
    When I type an additional character into the query
    Then the highlight resets to the first matching result

  @spotlight @keyboard @navigation
  Scenario: Enter after filtering activates the first remaining result by default
    When I type "soul"
    Then the highlight is on the first matching result
    And the matching "Open Soul" result is highlighted
    When I press Enter
    Then the action handler is called with id "nav:soul"
    And Spotlight closes

---

### Feature: Mouse interaction with results

  Hovering a row makes it the active (highlighted) row, and clicking a row
  activates it. Clicking the panel itself does not dismiss the palette, but
  clicking the backdrop outside the panel does.

  Background:
    Given Spotlight is open
    And the query is empty

  @spotlight @mouse @navigation
  Scenario: Hovering a row highlights it
    When I move the mouse over the "Settings" row
    Then the "Settings" row becomes the active highlighted row

  @spotlight @mouse @activate
  Scenario: Clicking a row activates that result
    When I click the "Open Soul" row
    Then the action handler is called with id "nav:soul"
    And Spotlight closes

  @spotlight @mouse @activate
  Scenario: Clicking a session row routes to the welcome view
    Given a session result with id "ses:sess-xyz" is present
    When I click that session row
    Then the action handler is called with id "ses:sess-xyz"
    And the app navigates to the "welcome" view
    And Spotlight closes

---

### Feature: Closing Spotlight by clicking outside

  The backdrop closes the palette on click, while clicks inside the panel are
  prevented from bubbling so they do not close it.

  @spotlight @mouse @close
  Scenario: Clicking the backdrop closes Spotlight
    Given Spotlight is open
    When I click the backdrop area outside the panel
    Then Spotlight closes

  @spotlight @mouse @close
  Scenario: Clicking inside the panel does not close Spotlight
    Given Spotlight is open
    When I click inside the Spotlight panel (for example the input row)
    Then the click does not propagate to the backdrop
    And Spotlight stays open

  @spotlight @close
  Scenario: Activating any result closes Spotlight via onClose
    Given Spotlight is open
    When I activate any command or session result
    Then onClose is invoked after the action handler
    And Spotlight closes

---

### Feature: Query persistence and seeding between opens

  App keeps the last search query in state (spotlightQuery) and passes it to
  Spotlight as initialQuery. Because Spotlight initializes its local input from
  initialQuery only on mount, a fresh open re-seeds the input from the persisted
  value, while edits made within an open session are local to that session.

  @spotlight @search @persistence
  Scenario: Opening from the TopBar Enter seeds the persisted query into the next open
    Given Spotlight is closed
    And I type "auth flow" into the TopBar search and press Enter
    Then the persisted query becomes "auth flow"
    And Spotlight opens with its input pre-filled with "auth flow"

  @spotlight @search @persistence
  Scenario: Focusing the TopBar search resets the persisted query to empty
    Given the persisted query is "auth flow"
    When I focus the TopBar search input
    Then the persisted query is reset to empty
    And Spotlight opens with an empty input

  @spotlight @search @persistence
  Scenario: Toggling open via keyboard uses the last persisted query as the seed
    Given the persisted query is "projects" from a prior TopBar search
    And Spotlight is closed
    When I open Spotlight with Cmd+K
    Then the Spotlight input is pre-filled with "projects"

  @spotlight @search @persistence
  Scenario: Edits typed inside Spotlight do not change the persisted App query
    Given Spotlight is open seeded with "projects"
    When I edit the input to read "soul" within the same open session
    Then the in-session input shows "soul"
    But the persisted App query remains "projects" until the next openSearch call


---

## Welcome, Chat & Agent Streaming

These acceptance scenarios are derived directly from the source of
`WelcomeScreen.tsx`, `ChatPanel.tsx`, `SoulPanel.tsx`, `TeamActivity.tsx`, and
the websocket/REST protocol in `api.ts`. Every step is grounded in observed
behavior of that code; no behavior is invented.

---

### Feature: Welcome screen composer
  As a user opening Conduit with no chat selected
  I want a prompt composer with a headline, context pills and a send control
  So that I can describe what to build and start a chat

  Background:
    Given no chat is selected
    And the WelcomeScreen is rendered as the empty-state body of the content column

  @welcome @layout
  Scenario: Welcome screen chrome is shown
    Then a decorative glow element marked aria-hidden is rendered
    And a centered terminal mark icon is shown
    And the headline reads "What should we build in conduit?"
    And a composer with a 3-row textarea is shown
    And the textarea placeholder reads "Ask Conduit anything. @ to mention files or tools"

  @welcome @autofocus
  Scenario: Composer autofocuses when requested
    Given autoFocus is true
    When the WelcomeScreen mounts
    Then the composer textarea receives keyboard focus

  @welcome @autofocus
  Scenario: Composer does not autofocus when not requested
    Given autoFocus is false
    When the WelcomeScreen mounts
    Then the composer textarea does not request focus

  @welcome @autofocus
  Scenario: Autofocus re-applies when the flag flips to true
    Given autoFocus was false
    When autoFocus changes to true
    Then the composer textarea receives keyboard focus

  @welcome @branch
  Scenario: Branch label displays the provided branch name
    Given the branch prop is "feature/x"
    Then the branch context pill displays "feature/x"

  @welcome @branch
  Scenario: Branch label falls back to main when empty
    Given the branch prop is an empty string
    Then the branch context pill displays "main"

---

### Feature: Composing and sending a prompt from the welcome screen
  As a user on the welcome screen
  I want to type a prompt and start a chat
  So that the agent begins working on my request

  Background:
    Given the WelcomeScreen is rendered

  @welcome @compose
  Scenario: Send button is disabled while the draft is empty
    Given the draft is empty
    Then the send button is disabled

  @welcome @compose
  Scenario: Send button is disabled when the draft is only whitespace
    Given the draft contains only spaces
    Then the send button is disabled

  @welcome @compose
  Scenario: Send button is enabled once non-whitespace text is entered
    Given I type "build a parser" into the composer
    Then the send button is enabled

  @welcome @compose @start-chat
  Scenario: Clicking send starts a chat with the trimmed prompt and selected project path
    Given a project named "conduit" is selected with absolute path "/home/u/conduit"
    And I type "  refactor the auth module  " into the composer
    When I click the send button
    Then onStartChat is called with prompt "refactor the auth module" and projectPath "/home/u/conduit"
    And the composer textarea is cleared

  @welcome @compose @start-chat
  Scenario: Enter without shift submits the prompt
    Given I type "hello" into the composer
    When I press Enter without holding shift
    Then the default newline insertion is prevented
    And onStartChat is called with prompt "hello"
    And the composer textarea is cleared

  @welcome @compose
  Scenario: Shift+Enter inserts a newline instead of submitting
    Given I type "line one" into the composer
    When I press Enter while holding shift
    Then onStartChat is not called
    And a newline is inserted into the draft

  @welcome @compose
  Scenario: Submitting an empty or whitespace-only draft is a no-op
    Given the draft is empty or only whitespace
    When submit is triggered
    Then onStartChat is not called
    And the composer is left unchanged

  @welcome @compose @start-chat
  Scenario: Starting a chat with no project selected omits the project path
    Given no project is selected
    And I type "scratch task" into the composer
    When I click the send button
    Then onStartChat is called with prompt "scratch task" and an undefined projectPath

---

### Feature: Welcome screen pills and project selection
  As a user on the welcome screen
  I want to choose sandbox, model, project, machine and branch options
  So that I can configure the context before starting

  Background:
    Given the WelcomeScreen is rendered

  @welcome @pills
  Scenario: Default pill values are shown
    Then the sandbox pill reads "Sandboxed"
    And the model pill reads "gpt-5.5 Medium"
    And the machine context pill reads "Work locally"

  @welcome @pills
  Scenario Outline: Opening a pill menu toggles its option list
    When I click the "<pill>" pill
    Then the "<pill>" option list is shown with aria-expanded true
    When I click the "<pill>" pill again
    Then the "<pill>" option list is hidden

    Examples:
      | pill    |
      | sandbox |
      | model   |
      | project |

  @welcome @pills
  Scenario Outline: Selecting a sandbox option updates the pill and closes the menu
    Given the sandbox menu is open
    When I select "<option>"
    Then the sandbox pill reads "<option>"
    And the sandbox menu is closed

    Examples:
      | option       |
      | Sandboxed    |
      | Unrestricted |

  @welcome @pills
  Scenario Outline: Selecting a model option updates the pill and closes the menu
    Given the model menu is open
    When I select "<option>"
    Then the model pill reads "<option>"
    And the model menu is closed

    Examples:
      | option           |
      | gpt-5.5 Medium   |
      | gpt-5.5 High     |
      | claude-opus-4-7  |
      | local            |

  @welcome @projects
  Scenario: Projects are fetched on mount and the first becomes selected
    Given getProjects resolves with at least one project
    When the WelcomeScreen mounts
    Then the project list is populated
    And the first project is selected
    And the project context pill displays that project's name

  @welcome @projects
  Scenario: Offline project fetch keeps the list empty
    Given getProjects rejects
    When the WelcomeScreen mounts
    Then the project list stays empty
    And the project context pill displays "No project"

  @welcome @projects
  Scenario: Selecting a project from the menu updates the selection
    Given the project menu is open with multiple projects
    When I select a different project
    Then that project becomes selected
    And the project menu closes

  @welcome @projects @browse
  Scenario: Choosing Browse projects closes the menu and triggers browsing
    Given the project menu is open
    When I click "Browse projects…"
    Then the project menu closes
    And onBrowseProjects is called

---

### Feature: Welcome screen recent activity rail (TeamActivity)
  As a user on the welcome screen
  I want to see my recent coding sessions
  So that I can resume prior work

  @welcome @activity @loading
  Scenario: Activity rail shows a loading message initially
    When the TeamActivity panel mounts
    Then the panel header title reads "Recent Activity"
    And the body shows "Loading activity…"

  @welcome @activity
  Scenario: Recent sessions are listed when fetched
    Given getSessions resolves with sessions
    When the fetch completes
    Then each session is shown as an activity item with a forum icon
    And the activity text shows the session summary
    And a relative timestamp is shown for each session

  @welcome @activity
  Scenario: Activity text falls back to the session id when summary is empty
    Given a session has an empty summary
    Then its activity text shows the session id instead

  @welcome @activity
  Scenario: Only the first eight sessions are rendered
    Given getSessions resolves with more than eight sessions
    Then at most eight activity items are shown

  @welcome @activity @empty
  Scenario: Empty state when there is no recent activity
    Given getSessions resolves with an empty list
    Then the body shows "No recent activity yet."

  @welcome @activity @error
  Scenario: Error state when activity cannot be loaded
    Given getSessions rejects
    Then the body shows "Activity unavailable — start conduit serve."

  @welcome @activity
  Scenario Outline: Relative timestamps are formatted by age
    Given a session created "<ago>" ago
    Then its timestamp reads "<label>"

    Examples:
      | ago         | label    |
      | 30 seconds  | just now |
      | 5 minutes   | 5m ago   |
      | 3 hours     | 3h ago   |
      | 2 days      | 2d ago   |
      | 3 weeks     | 3w ago   |

  @welcome @activity
  Scenario: Unparseable timestamp renders an empty relative time
    Given a session createdAt cannot be parsed as a date
    Then its relative timestamp is an empty string

---

### Feature: Chat panel websocket connection lifecycle
  As the chat surface
  I want to manage a streaming websocket to /api/agent
  So that prompts and responses flow reliably and survive disconnects

  Background:
    Given the ChatPanel is rendered in live (non-demo) mode

  @chat @websocket @lifecycle
  Scenario: Connection starts in the connecting state
    When the ChatPanel mounts
    Then the connection state is "connecting"
    And the input placeholder reads "Reconnecting… (connecting)"
    And the input textarea is disabled

  @chat @websocket @lifecycle
  Scenario: Websocket opens and enables input
    When the websocket open event fires
    Then the connection state becomes "connected"
    And the input placeholder reads "Message Conduit…"
    And the input textarea is enabled

  @chat @websocket @options
  Scenario: Template id and project path are passed as query params on connect
    Given the ChatPanel has templateId "review" and projectPath "/home/u/app"
    When the agent websocket is opened
    Then the url is "/api/agent?template=review&projectPath=%2Fhome%2Fu%2Fapp"

  @chat @websocket @options
  Scenario: No query string is appended when no options are provided
    Given the ChatPanel has no templateId and no projectPath
    When the agent websocket is opened
    Then the url is exactly "/api/agent" with no query string

  @chat @websocket @reconnect
  Scenario: Unexpected close transitions to disconnected and schedules a reconnect
    Given the websocket is connected
    When the socket closes unexpectedly
    Then the connection state becomes "disconnected"
    And a reconnect is scheduled

  @chat @websocket @reconnect
  Scenario Outline: Reconnect backoff is capped exponential
    Given <retry> prior reconnect attempts have occurred
    When the socket closes
    Then the next reconnect is scheduled after <delay> ms

    Examples:
      | retry | delay |
      | 0     | 500   |
      | 1     | 1000  |
      | 2     | 2000  |
      | 3     | 4000  |
      | 4     | 8000  |
      | 5     | 8000  |

  @chat @websocket @reconnect
  Scenario: A successful open resets the backoff counter
    Given several reconnect attempts have elapsed
    When the websocket opens successfully
    Then the retry counter resets to zero so the next backoff starts at 500 ms

  @chat @websocket @error
  Scenario: A socket error defers to the close handler for reconnection
    When the websocket fires an error event
    Then no reconnect is scheduled by the error handler directly
    And reconnection is handled when the subsequent close event fires

  @chat @websocket @lifecycle
  Scenario: Closing the panel stops reconnection
    Given the ChatPanel websocket is connected
    When the ChatPanel unmounts
    Then the client close is invoked
    And any pending reconnect timer is cleared
    And no further reconnect is scheduled after close

  @chat @websocket @lifecycle
  Scenario: Reconnecting after a connect error re-disables the input
    Given the connection was connected then dropped
    When the state is "disconnected"
    Then the input placeholder reads "Reconnecting… (disconnected)"
    And the input textarea is disabled

  @chat @websocket
  Scenario: The connection is re-established when templateId or projectPath changes
    Given a live ChatPanel connection exists
    When the templateId or projectPath prop changes
    Then the existing client is closed
    And a new websocket is opened with the updated options

---

### Feature: Sending prompts in the chat panel
  As a user in an active chat
  I want to type and send prompts
  So that the agent responds

  Background:
    Given the ChatPanel is rendered in live mode
    And the websocket is connected

  @chat @send
  Scenario: Empty conversation shows a placeholder
    Given no turns exist
    Then the stream shows "No messages yet — say hello."

  @chat @send
  Scenario: Sending a prompt appends a user turn and transmits a prompt frame
    Given I type "list files" into the chat input
    When I click send
    Then a new turn is appended with a user block reading "list files"
    And a prompt frame {type:"prompt", text:"list files"} is sent over the websocket
    And the input draft is cleared

  @chat @send
  Scenario: The prompt text is trimmed before sending
    Given I type "  ship it  " into the chat input
    When I send
    Then the user block and prompt frame both read "ship it"

  @chat @send
  Scenario: Send button is disabled while the draft is empty
    Given the chat input draft is empty
    Then the send button is disabled

  @chat @send
  Scenario: Send button is disabled while disconnected
    Given the connection state is not "connected"
    And the draft contains text
    Then the send button is disabled
    And send is a no-op if invoked

  @chat @send
  Scenario: Enter without shift sends; shift+Enter inserts a newline
    Given the chat input contains "go"
    When I press Enter without shift
    Then the prompt is sent and default behavior is prevented
    When I instead press Enter with shift
    Then no prompt is sent and a newline is inserted

  @chat @send
  Scenario: Sending while a turn is in progress starts a new turn
    Given the last turn is not yet done
    When I send another prompt
    Then a new turn is appended rather than appending to the active one

  @chat @send @composer
  Scenario: The chat input auto-grows to fit its content
    When the draft text spans multiple lines
    Then the textarea height is recalculated to its scroll height

  @chat @send @scroll
  Scenario: The stream auto-scrolls to the bottom on new turns
    When the turns list changes
    Then the stream scrolls to its bottom

---

### Feature: Auto-sending the welcome prompt into the chat
  As a user who typed a prompt on the welcome screen
  I want it sent automatically once the chat connects
  So that I do not have to retype or resend

  Background:
    Given the ChatPanel is rendered in live mode with an initialPrompt

  @chat @initial-prompt @auto-send
  Scenario: The initial prompt is auto-sent once the websocket connects
    Given the connection state becomes "connected"
    And the initialPrompt is "scaffold a project"
    Then a user turn reading "scaffold a project" is appended
    And a prompt frame with that text is sent

  @chat @initial-prompt @auto-send
  Scenario: The initial prompt is trimmed before auto-send
    Given the initialPrompt is "  hi  "
    When the connection connects
    Then the auto-sent user turn and frame both read "hi"

  @chat @initial-prompt @auto-send
  Scenario: The initial prompt is sent only once
    Given the initial prompt was already auto-sent
    When the connection state changes again
    Then the initial prompt is not sent a second time

  @chat @initial-prompt @auto-send
  Scenario: A blank initial prompt is never auto-sent
    Given the initialPrompt is null or only whitespace
    When the connection connects
    Then no auto-send occurs

  @chat @initial-prompt @auto-send
  Scenario: The initial prompt is not sent before the connection is ready
    Given the connection state is "connecting"
    Then the initial prompt is not yet sent
    When the state later becomes "connected"
    Then the initial prompt is auto-sent

---

### Feature: Streaming server messages into chat turns
  As the chat surface
  I want to reduce each websocket frame into rendered blocks
  So that responses appear token-by-token with tools and finalization

  Background:
    Given the ChatPanel is rendered in live mode and connected

  @chat @streaming @session
  Scenario: A session frame records provider, model and session id
    When a {type:"session", id, provider:"openai", model:"gpt-5.5"} frame arrives
    Then the info is updated with that provider, model and sessionId
    And the version is preserved from any prior info
    And no new turn is created by the session frame

  @chat @streaming @text
  Scenario: First text_delta creates an assistant block
    Given the active turn has no trailing assistant block
    When a text_delta with text "Hel" arrives
    Then a new assistant block reading "Hel" is appended to the active turn

  @chat @streaming @text
  Scenario: Subsequent text_delta frames accumulate into the same assistant block
    Given the active turn ends with an assistant block reading "Hel"
    When text_delta "lo" then text_delta " world" arrive
    Then the assistant block accumulates to "Hello world"

  @chat @streaming @text
  Scenario: A frame arriving with no active turn (or a completed last turn) starts a fresh turn
    Given there is no active turn or the last turn is done
    When a streaming frame arrives
    Then a new active turn is created to hold the block

  @chat @streaming @tool-use
  Scenario: A tool_use frame appends a running tool block
    When a {type:"tool_use", id:"t1", name:"bash", input:"ls -la"} frame arrives
    Then a tool block with id "t1", name "bash" and input "ls -la" is appended
    And the block has null output and isError false
    And the tool pill shows "running" with the running style

  @chat @streaming @tool-result
  Scenario: A matching tool_result fills in the tool block output as success
    Given a running tool block with id "t1" exists
    When a {type:"tool_result", id:"t1", output:"file list", isError:false} frame arrives
    Then that tool block's output becomes "file list" and isError stays false
    And the tool pill shows "ok" with the ok style

  @chat @streaming @tool-result @error
  Scenario: A tool_result with isError true marks the tool block as an error
    Given a running tool block with id "t1" exists
    When a {type:"tool_result", id:"t1", output:"boom", isError:true} frame arrives
    Then that tool block's output becomes "boom" and isError becomes true
    And the tool pill shows "error" with the error style
    And the tool-call container carries the error class

  @chat @streaming @tool-result
  Scenario: A tool_result with no matching tool_use id is ignored
    Given no tool block with id "tX" exists
    When a tool_result for id "tX" arrives
    Then no block is modified

  @chat @streaming @tool-display
  Scenario: A tool block is collapsed by default and expands on click
    Given a tool block with output present
    Then the output pre is hidden by default
    When I click the tool head
    Then aria-expanded becomes true and the output pre is shown
    When I click the tool head again
    Then it collapses and the output pre is hidden

  @chat @streaming @tool-display
  Scenario: A running tool block shows no output even when expanded
    Given a tool block whose output is still null
    When I expand the tool head
    Then no output pre is rendered because output is null

  @chat @streaming @end-turn
  Scenario: end_turn finalizes the active turn
    Given an active in-progress turn
    When a {type:"end_turn"} frame arrives
    Then the active turn is marked done
    And the next streaming frame begins a new turn

  @chat @streaming @error
  Scenario: An error frame appends a bracketed assistant message
    When a {type:"error", message:"rate limited"} frame arrives
    Then an assistant block reading "[error] rate limited" is appended to the active turn

  @chat @streaming @malformed
  Scenario: A malformed websocket frame is dropped without tearing down the stream
    When a non-JSON message is received
    Then the parse failure is swallowed
    And the connection remains open and no turn changes occur

---

### Feature: Chat model badge
  As a user in chat
  I want to see which provider/model is serving the session
  So that I know what is responding

  Background:
    Given the ChatPanel is rendered

  @chat @model-badge
  Scenario: The model badge defaults before any info is known
    Given no info has been loaded
    Then the model badge reads "codex/gpt-5.5"
    And the badge tone class is the local tone

  @chat @model-badge
  Scenario: getInfo is fetched on mount in live mode
    When the live ChatPanel mounts
    Then getInfo is requested
    And a fetch failure is tolerated since the session frame also carries provider and model

  @chat @model-badge
  Scenario: getInfo is not fetched in demo mode
    Given the ChatPanel is in demo mode
    Then getInfo is not requested

  @chat @model-badge
  Scenario Outline: The badge tone maps from the provider string
    Given info.provider is "<provider>"
    Then the badge tone is "<tone>"

    Examples:
      | provider   | tone       |
      | anthropic  | claude     |
      | claude     | claude     |
      | openai     | openai     |
      | codex      | openai     |
      | litellm    | litellm    |
      | openrouter | openrouter |
      | something  | local      |
      |            | local      |

  @chat @model-badge
  Scenario: The badge label combines provider and model from info
    Given info has provider "openai" and model "gpt-5.5"
    Then the model badge reads "openai/gpt-5.5"

---

### Feature: Chat demo mode
  As a designer capturing the chat surface
  I want canned content without a backend
  So that I can screenshot the chat for QA

  @chat @demo
  Scenario: Demo mode seeds a canned exchange and skips all networking
    Given the ChatPanel is rendered with demo true
    Then the seed turn shows the user message "Give me a one-line hello for a screenshot."
    And the assistant reply "Hello — nice to see you!"
    And no websocket is opened
    And getInfo is not requested
    And the input is treated as connected so it is enabled

  @chat @demo
  Scenario: Demo mode does not auto-send an initial prompt
    Given the ChatPanel is in demo mode with an initialPrompt
    Then the initial prompt is not auto-sent

  @chat @demo
  Scenario: Demo mode turn id counter starts past the seed turns
    Given demo mode seeds one turn
    When I send a new prompt in demo context
    Then the new turn id continues after the seeded turn count

---

### Feature: SoulPanel memory display
  As a user in chat
  I want to view the agent's SOUL.md and USER.md memory
  So that I understand its persistent context

  @chat @soul-panel @loading
  Scenario: SoulPanel shows a loading message initially
    When the SoulPanel mounts
    Then the panel header title reads "soul.md" with a psychology icon
    And the body shows "Loading memory…"

  @chat @soul-panel
  Scenario: Loaded memory renders SOUL and USER sections
    Given getMemory resolves with non-empty soul and user markdown
    When the fetch completes
    Then the soul markdown is rendered in its section
    And the user markdown is rendered under a "User" heading
    And an edit (pencil) action appears in the header

  @chat @soul-panel @empty
  Scenario: Empty memory shows a seeding hint
    Given getMemory resolves with empty soul and empty user (after trimming)
    Then the body shows "No memory written yet. Use the pencil to seed SOUL.md."

  @chat @soul-panel @error
  Scenario: Offline memory fetch shows a calm hint
    Given getMemory rejects
    Then the body shows "Memory unavailable — start conduit serve to load SOUL.md."
    And no edit action is shown

  @chat @soul-panel
  Scenario: Fetch result is ignored after unmount
    Given the SoulPanel is unmounting
    When getMemory resolves after unmount
    Then no state update is applied (the cancelled flag guards it)

  @chat @soul-panel @markdown
  Scenario: Markdown headings render as soul headings
    Given memory text contains a line beginning with one to six "#" then text
    Then that line renders as a soul-heading with the "#" markers stripped

  @chat @soul-panel @markdown
  Scenario: Consecutive bullet lines collect into a single list
    Given memory text contains lines starting with "-" or "*"
    Then those lines render as one soul-list with a bullet glyph per item

  @chat @soul-panel @markdown
  Scenario: A blank line flushes the current bullet list
    Given a run of bullets is followed by a blank line
    Then the bullet list is closed before subsequent content

  @chat @soul-panel @markdown
  Scenario: Plain prose lines render as paragraphs
    Given a non-heading, non-bullet, non-empty line
    Then it renders as a soul-text paragraph

  @chat @soul-panel
  Scenario: The panel close button is shown only when onClose is provided
    Given an onClose handler is provided
    Then a close action appears in the header
    When I click it
    Then onClose is invoked
    And when no onClose is provided no close action is rendered

---

### Feature: SoulPanel memory editing
  As a user in chat
  I want to edit and save SOUL.md and USER.md
  So that I can shape the agent's persistent memory

  Background:
    Given the SoulPanel has loaded non-empty memory

  @chat @soul-panel @edit
  Scenario: Entering edit mode seeds the textareas from loaded memory
    When I click the edit (pencil) action
    Then the SOUL.md textarea is seeded with the loaded soul text
    And the USER.md textarea is seeded with the loaded user text
    And any prior save error is cleared
    And the edit action is hidden while editing

  @chat @soul-panel @edit
  Scenario: Editing while not loaded is a no-op
    Given memory has not loaded
    When startEdit is invoked
    Then edit mode is not entered

  @chat @soul-panel @edit @save
  Scenario: Saving posts both fields and returns to the loaded view
    Given I have edited the SOUL.md and USER.md textareas
    When I click Save
    Then saveMemory is called with both soul and user fields
    And while saving the Save button reads "Saving…" and both buttons are disabled
    And on success the loaded view shows the persisted memory and editing ends

  @chat @soul-panel @edit @save @error
  Scenario: A failed save shows an error and stays in edit mode
    Given saveMemory rejects
    When I click Save
    Then the message "Couldn't save — is conduit serve running?" is shown
    And the panel remains in edit mode
    And the saving flag is cleared so the buttons are re-enabled

  @chat @soul-panel @edit
  Scenario: Cancelling discards edits and exits edit mode
    Given I am editing memory
    When I click Cancel
    Then edit mode exits without calling saveMemory
    And the cancel button is disabled while a save is in flight

---

### Feature: Backend protocol and base URL
  As the GUI client
  I want a well-defined API surface
  So that the GUI and core server stay in sync

  @api @config
  Scenario: The API base URL defaults to localhost when unconfigured
    Given VITE_CONDUIT_API is not set
    Then the base URL is "http://localhost:9876"

  @api @config
  Scenario: The API base URL honors the VITE_CONDUIT_API override
    Given VITE_CONDUIT_API is "http://host:1234"
    Then the base URL is "http://host:1234"

  @api @config
  Scenario: The websocket base derives from the http base by scheme swap
    Given the base URL begins with "http"
    Then the websocket base swaps the leading "http" for "ws"

  @api @rest
  Scenario Outline: REST GET helpers hit the expected endpoints
    When "<helper>" is called
    Then it issues GET "<path>"

    Examples:
      | helper      | path          |
      | getInfo     | /api/info     |
      | getSessions | /api/sessions |
      | getMemory   | /api/memory   |
      | getProjects | /api/projects |

  @api @rest @error
  Scenario: A non-2xx REST GET response throws with the path and status
    Given the server responds with a non-ok status
    When a getJSON helper runs
    Then it throws an error including the path and HTTP status

  @api @rest
  Scenario: saveMemory POSTs both fields and returns the echoed memory
    When saveMemory is called with soul and user
    Then it issues POST /api/memory with a JSON body of both fields
    And returns the persisted memory echoed by the server
    And throws on a non-ok response

  @api @rest
  Scenario: patchSettings PATCHes provider and model
    When patchSettings is called with provider and model
    Then it issues PATCH /api/settings with that JSON body
    And on a non-ok response it throws using the server-provided error message when present
    And otherwise throws with the HTTP status

  @api @websocket
  Scenario: send only transmits when the socket is open
    Given the websocket readyState is OPEN
    Then send serializes and transmits the client message
    And when the socket is not open send is a silent no-op


---

## Projects, Workspace & Agent Sessions

These acceptance scenarios document the observable GUI behavior of the Conduit
Projects browser, the New Project form, the per-project Workspace Dashboard, the
Agent Sessions template picker, and the local-project persistence layer. Every
scenario is grounded in the current source:

- `spike/gui/src/components/ProjectsList.tsx`
- `spike/gui/src/components/NewProject.tsx`
- `spike/gui/src/components/ProjectDashboard.tsx`
- `spike/gui/src/components/AgentSessionsPage.tsx`
- `spike/gui/src/localProjects.ts`
- Wiring in `spike/gui/src/App.tsx` (`handleCreateProject`, `onEnter`,
  `activeProject`, navigation to `workspace`)

Data note: backend-backed projects, orphan chats, and dashboard data come from
`getProjects()` (`GET /api/projects` against `conduit serve`). Projects created
in the New Project form are *not* sent to the server; they are persisted only in
`localStorage` under `conduit.localProjects` and merged into the list labeled
"Local".

---

### Feature: Browsing the projects list

  Background:
    Given the user has navigated to the "projects" view
    And the Projects list mounts and immediately calls getProjects()

  @projects @loading
  Scenario: Loading state while fetching projects
    Given getProjects() has not yet resolved
    Then the status is "loading"
    And the notice "Loading projects…" is shown
    And the "Create New Project" affordance is still shown
    And no server-backed project cards are rendered yet

  @projects @ready @populated
  Scenario: Populated list of server-backed projects
    Given getProjects() resolves with one or more projects
    Then the status becomes "ready"
    And each project renders as a card showing:
      | element        | source                                            |
      | thumbnail icon | "account_tree" (filled)                           |
      | title          | project.name                                      |
      | description    | project.path                                      |
      | branch pill    | project.branch (only when branch is non-empty)    |
      | session pill   | "<sessionCount> session" or "<n> sessions"        |
      | updated note   | relativeTime(project.lastActivity)                |
    And each card shows a star (favorite) icon button and an "Enter Workspace" button

  @projects @ready @session-count-pluralization
  Scenario Outline: Session count pluralization on a project card
    Given getProjects() resolves with a project whose sessionCount is <count>
    Then the session pill reads "<text>"

    Examples:
      | count | text       |
      | 0     | 0 sessions |
      | 1     | 1 session  |
      | 2     | 2 sessions |

  @projects @ready @empty
  Scenario: Empty state when there is nothing to show
    Given getProjects() resolves with zero projects and zero orphans
    And there are zero local projects in localStorage
    Then the status is "ready"
    And the notice "No projects yet. Create one to get started." is shown
    And the "Create New Project" affordance is still shown

  @projects @error
  Scenario: Backend unreachable
    Given getProjects() rejects (e.g. conduit serve is not running)
    Then the status becomes "error"
    And the notice "Projects unavailable — start conduit serve." is shown
    And no project cards are rendered
    And the empty-state notice is NOT shown

  @projects @ready @orphans
  Scenario: Ungrouped (orphan) chats section
    Given getProjects() resolves with one or more orphan chats
    Then a "Ungrouped chats" section is shown below the project cards
    And each orphan renders as a button showing:
      | element   | source                                  |
      | icon      | "forum"                                 |
      | title     | orphan.title                            |
      | meta      | "<turnCount> turn" or "<n> turns"       |

  @projects @ready @orphans
  Scenario Outline: Orphan chat turn pluralization
    Given an orphan chat whose turnCount is <count>
    Then its meta reads "<text>"

    Examples:
      | count | text    |
      | 1     | 1 turn  |
      | 3     | 3 turns |

  @projects @ready @orphans
  Scenario: No orphans section when there are no orphan chats
    Given getProjects() resolves with zero orphan chats
    Then the "Ungrouped chats" section is NOT rendered

  @projects @helpers
  Scenario: Helper bento cards are always present
    Given the Projects view is rendered in any status
    Then three static helper cards are shown:
      | label             | description                                              |
      | Project Templates | Start from a curated blueprint and ship in minutes.      |
      | Quick Deploy      | Push a workspace live with a single guided flow.         |
      | Vault Integration | Connect encrypted secrets and credentials securely.      |
    And these helper cards are non-interactive (presentational only)

  @projects @relative-time
  Scenario Outline: Relative-time rendering for the updated note
    Given a project whose lastActivity corresponds to <elapsed> ago
    Then the updated note reads "<text>"

    Examples:
      | elapsed     | text               |
      | 30 seconds  | Updated just now   |
      | 5 minutes   | Updated 5m ago     |
      | 3 hours     | Updated 3h ago     |
      | 2 days      | Updated 2d ago     |
      | 3 weeks     | Updated 3w ago     |
      | 4 months    | Updated 4mo ago    |
      | 2 years     | Updated 2y ago     |

  @projects @relative-time @edge
  Scenario: Unparseable timestamp falls back gracefully
    Given a project whose lastActivity is not a valid ISO date
    Then the updated note reads "Updated recently"

---

### Feature: Filtering and sorting the projects list

  Background:
    Given the Projects view is in the "ready" status with several projects

  @projects @filter
  Scenario: Default filter is "All Projects"
    When the Projects view first renders
    Then the "All Projects" filter pill is active (aria-pressed=true)
    And the "Favorites" filter pill is inactive
    And all projects and all local projects are shown

  @projects @filter @favorites
  Scenario: Switching to the Favorites filter
    Given the user has starred at least one project
    When the user clicks the "Favorites" pill
    Then the "Favorites" pill becomes active
    And only projects whose id is in the favorites set are shown
    And only local projects whose id is in the favorites set are shown

  @projects @filter @favorites @empty
  Scenario: Favorites filter with server projects but no favorites
    Given there is at least one server-backed project
    And no project has been starred
    When the user selects the "Favorites" filter
    Then the notice "No favorites yet. Star a project to pin it here." is shown

  @projects @sort
  Scenario: Default sort is "Recently Updated"
    When the Projects view first renders
    Then the sort button label reads "Recently Updated"
    And projects are ordered by lastActivity descending (most recent first)

  @projects @sort
  Scenario: Opening and closing the sort menu
    When the user clicks the sort button
    Then the sort options listbox opens (aria-expanded=true)
    And it lists "Recently Updated" and "Name"
    When the user selects an option
    Then the listbox closes
    And the selected option becomes the active sort

  @projects @sort
  Scenario: Sorting by name
    When the user selects the "Name" sort option
    Then the sort button label reads "Name"
    And projects are ordered by name using a locale-aware comparison (ascending)

  @projects @sort @scope
  Scenario: Sort applies only to server-backed projects
    Given there are both local projects and server-backed projects
    When the user changes the sort order
    Then only the server-backed project order changes
    And local project order is unaffected by the sort control

---

### Feature: Favoriting projects (persisted in localStorage)

  Background:
    Given the Projects view is rendered
    And favorites are persisted in localStorage under the key "conduit.favorites"

  @projects @favorites @persistence
  Scenario: Starring a project
    Given a project that is not currently a favorite
    When the user clicks its star button
    Then the star icon becomes filled
    And the button's aria-pressed becomes true
    And its title/aria-label change from "Star project" to "Unstar project"
    And the project id is added to the stored favorites array

  @projects @favorites @persistence
  Scenario: Unstarring a project
    Given a project that is currently a favorite
    When the user clicks its star button
    Then the star icon becomes unfilled
    And the button's aria-pressed becomes false
    And the project id is removed from the stored favorites array

  @projects @favorites @persistence
  Scenario: Favorites survive a reload
    Given the user previously starred a project
    When the Projects view remounts
    Then loadFavorites() reads "conduit.favorites" from localStorage
    And the previously starred project still shows a filled star

  @projects @favorites @resilience
  Scenario: Corrupt or missing favorites storage is tolerated
    Given "conduit.favorites" is absent, or holds invalid JSON, or a non-array value
    When the Projects view loads favorites
    Then it falls back to an empty favorites set
    And the view does not crash

  @projects @favorites @resilience
  Scenario: localStorage write failure does not break the UI
    Given writing to localStorage throws (e.g. storage disabled or full)
    When the user toggles a favorite
    Then the in-memory favorite state still updates
    And no error is surfaced to the user

---

### Feature: Local projects in the list

  Background:
    Given the Projects view is rendered
    And local projects are loaded once at mount via loadLocalProjects()

  @projects @local
  Scenario: Local projects render as distinct cards
    Given there is at least one project in "conduit.localProjects"
    Then a local-project card is shown for each, displaying:
      | element     | source                                              |
      | thumb icon  | "folder" (filled)                                   |
      | title       | localProject.name                                   |
      | "Local" tag | the private tag label                               |
      | description | localProject.description or "Local project (not yet synced)" |
      | status pill | a lock icon plus localProject.visibility            |
      | created note| relativeTime(localProject.createdAt)                |
    And each local card has a star button and an "Enter Workspace" button

  @projects @local
  Scenario: Local project with empty description shows a placeholder
    Given a local project whose description is an empty string
    Then its card description reads "Local project (not yet synced)"

  @projects @local @section-visibility
  Scenario: Local projects section hidden when none visible
    Given there are no local projects (or none match the active filter)
    Then no local-project cards are rendered

  @projects @local @loaded-once
  Scenario: Local project list is read once at mount
    Given local projects are loaded into state at component mount
    Then adding a local project elsewhere does not update the already-mounted list
    And the new local project appears only after the Projects view remounts

---

### Feature: Entering a workspace from the list

  Background:
    Given the Projects view is rendered with at least one project or orphan

  @projects @workspace @navigation
  Scenario: Entering a server-backed project
    When the user clicks "Enter Workspace" on a server project card
    Then onEnter is called with { id: project.id, title: project.name }
    And App sets activeProject to that value
    And App navigates to the "workspace" view

  @projects @workspace @navigation
  Scenario: Entering a local project
    When the user clicks "Enter Workspace" on a local project card
    Then onEnter is called with { id: localProject.id, title: localProject.name }
    And App navigates to the "workspace" view

  @projects @workspace @navigation @orphan
  Scenario: Opening an ungrouped chat
    When the user clicks an orphan chat button
    Then onEnter is called with { id: chat.id, title: chat.title }
    And App navigates to the "workspace" view

  @projects @new-project @navigation
  Scenario: Starting a new project from the list
    When the user clicks "Create New Project"
    Then onCreateNew is called
    And App navigates to the "newProject" view

---

### Feature: Creating a new project — form structure and defaults

  Background:
    Given the user has navigated to the "newProject" view
    And the New Project form is rendered

  @new-project @structure
  Scenario: Form header and sections
    Then the header title reads "Create New Project"
    And the subtitle reads "Initialize a structured workspace for your next breakthrough."
    And the form contains these sections in order:
      | section                    |
      | Project Core               |
      | Infrastructure & Compute   |
      | Contextual Assets          |
      | Agent Sessions             |
      | Visibility & Governance    |
      | Footer (disclaimer + actions) |

  @new-project @defaults
  Scenario: Initial field defaults
    Then the Project Name input is empty
    And the Description textarea is empty
    And the Infrastructure choice defaults to "Local-First" (local)
    And the Contextual Assets tab defaults to "Designs"
    And the Visibility choice defaults to "Private"
    And exactly one agent ("Code Auditor" / code-auditor) is pre-selected

  @new-project @defaults @footer
  Scenario: Footer disclaimer is shown
    Then the footer shows the disclaimer about automated archival policies and compute quotas
    And the footer shows a "Cancel" button and a "Create Project" button

---

### Feature: New Project — Project Core fields

  Background:
    Given the New Project form is rendered

  @new-project @field @name
  Scenario: Editing the project name
    When the user types into the Project Name input
    Then the input reflects the typed value
    And the placeholder is "e.g. Project 'Aether' - Q4 Infrastructure"

  @new-project @field @description
  Scenario: Editing the description
    When the user types into the Description textarea
    Then the textarea reflects the typed value
    And the placeholder is "Define the objective and scope of this orchestration..."
    And the textarea is 3 rows tall

---

### Feature: New Project — Infrastructure & Compute selection

  Background:
    Given the New Project form is rendered

  @new-project @field @infra
  Scenario Outline: Selecting an infrastructure option
    When the user clicks the "<title>" infrastructure card
    Then that card becomes active (aria-pressed=true)
    And the other infrastructure card becomes inactive
    And the draft infra value becomes "<id>"

    Examples:
      | title        | id    |
      | Local-First  | local |
      | Cloud-Native | cloud |

  @new-project @field @infra @single-select
  Scenario: Infrastructure is single-select
    Given "Local-First" is currently active
    When the user clicks "Cloud-Native"
    Then "Cloud-Native" is active and "Local-First" is no longer active

---

### Feature: New Project — Contextual Assets (presentational spike)

  Background:
    Given the New Project form is rendered

  @new-project @assets @tabs
  Scenario Outline: Switching asset tabs
    When the user clicks the "<label>" asset tab
    Then that tab becomes active (aria-pressed=true)

    Examples:
      | label        |
      | Designs      |
      | Documents    |
      | Repositories |

  @new-project @assets @presentational
  Scenario: Asset controls are non-functional in the spike
    Then a drag-and-drop dropzone with a "browse local files" button is shown
    And a URL input with an "Add" button is shown
    And a "Staged Assets" list shows two hardcoded sample assets with a count chip "2"
    And none of these controls persist or affect the created project draft

---

### Feature: New Project — Agent Sessions selection (presentational)

  Background:
    Given the New Project form is rendered
    And six agent cards are shown:
      | title               |
      | Code Auditor        |
      | Content Strategist  |
      | System Architect    |
      | Product Manager     |
      | Security Researcher |
      | UI/UX Critic        |

  @new-project @agents
  Scenario: Code Auditor is pre-selected
    Then the "Code Auditor" agent card is active with a check indicator (aria-pressed=true)

  @new-project @agents
  Scenario: Selecting an additional agent
    When the user clicks an unselected agent card
    Then that card becomes active and shows a check
    And previously selected agents remain selected (multi-select)

  @new-project @agents
  Scenario: Deselecting an agent
    Given an agent card is currently selected
    When the user clicks it again
    Then it becomes inactive and the check is removed

  @new-project @agents @not-persisted
  Scenario: Selected agents are not part of the created draft
    Given the user has changed the agent selection
    When the project is created
    Then the agent selection is NOT included in the NewProjectDraft
    And it is not persisted (UI-spike state only)

  @new-project @agents @custom
  Scenario: Custom Agent and Automation buttons are presentational
    Then the section shows a "Custom Agent" button and an "Automation" chip
    And neither performs any action in this form

---

### Feature: New Project — Visibility & Governance selection

  Background:
    Given the New Project form is rendered

  @new-project @field @visibility
  Scenario Outline: Selecting a visibility option
    When the user clicks the "<title>" visibility card
    Then that card becomes active (aria-pressed=true)
    And the draft visibility value becomes "<id>"

    Examples:
      | title   | id      |
      | Private | private |
      | Team    | team    |
      | Public  | public  |

  @new-project @field @visibility @single-select
  Scenario: Visibility is single-select
    Given "Private" is currently active
    When the user clicks "Team"
    Then "Team" is active and "Private" is no longer active

---

### Feature: New Project — validation, create, and cancel

  Background:
    Given the New Project form is rendered

  @new-project @validation @required-name
  Scenario: Create is disabled when the name is empty
    Given the Project Name input is empty
    Then the "Create Project" button is disabled

  @new-project @validation @required-name
  Scenario: Create is disabled when the name is only whitespace
    Given the Project Name input contains only spaces
    Then the "Create Project" button is disabled

  @new-project @validation @required-name
  Scenario: Create becomes enabled with a non-empty name
    When the user enters a non-whitespace project name
    Then the "Create Project" button becomes enabled

  @new-project @create @draft-shape
  Scenario: Creating a project builds a trimmed draft
    Given the Project Name is "  Aether  " and the Description is "  goal  "
    And infra is "cloud" and visibility is "team"
    When the user clicks "Create Project"
    Then onCreate is called with a NewProjectDraft of:
      | field       | value  |
      | name        | Aether |
      | description | goal   |
      | infra       | cloud  |
      | visibility  | team   |
    And both name and description are trimmed of surrounding whitespace

  @new-project @create @persistence @navigation
  Scenario: Successful create persists locally and navigates to projects
    When onCreate fires with a valid draft
    Then App's handleCreateProject calls addLocalProject(draft)
    And a LocalProject is appended to "conduit.localProjects" in localStorage
    And App navigates to the "projects" view
    And the newly created project appears as a "Local" card after that view mounts

  @new-project @cancel @navigation
  Scenario: Cancelling the form
    When the user clicks "Cancel"
    Then onCancel is called
    And App navigates to the "projects" view
    And no project is persisted

---

### Feature: Local project persistence (localProjects.ts)

  @persistence @local-projects @load
  Scenario: Loading with no stored projects
    Given "conduit.localProjects" is absent from localStorage
    When loadLocalProjects() is called
    Then it returns an empty array

  @persistence @local-projects @load @resilience
  Scenario: Loading with corrupt or non-array storage
    Given "conduit.localProjects" holds invalid JSON or a non-array value
    When loadLocalProjects() is called
    Then it returns an empty array without throwing

  @persistence @local-projects @add
  Scenario: Adding a local project generates id and timestamp
    Given a draft of { name, description, infra, visibility }
    When addLocalProject(draft) is called
    Then the returned LocalProject includes all draft fields
    And it has an id of the form "local-<base36 timestamp>"
    And it has a createdAt set to the current time as an ISO string

  @persistence @local-projects @add @ordering
  Scenario: New local projects are prepended
    Given there are existing local projects in storage
    When addLocalProject(draft) is called
    Then the new project is stored at the front of the array (most recent first)
    And the existing projects are preserved after it

  @persistence @local-projects @add @resilience
  Scenario: Add tolerates a storage write failure
    Given writing to localStorage throws
    When addLocalProject(draft) is called
    Then it still returns the in-memory LocalProject
    And it does not throw

  @persistence @local-projects @survives-reload
  Scenario: Persisted local projects survive a reload
    Given a local project was added via the New Project form
    When the application is reloaded and the Projects view mounts
    Then loadLocalProjects() reads it back from "conduit.localProjects"
    And it is shown as a "Local" card

---

### Feature: Project Workspace Dashboard

  Background:
    Given the user has entered a project and the "workspace" view is rendered
    And ProjectDashboard mounts and calls getProjects()
    And it receives projectId and projectName from the entered project

  @workspace @loading
  Scenario: Loading state
    Given getProjects() has not yet resolved
    Then the load state is "loading"
    And the eyebrow shows the project title (projectName, else fetched name, else "Workspace")
    And the heading reads "Project Workspace"
    And the subtitle reads "Loading workspace…"
    And the dashboard grid is not yet rendered

  @workspace @error
  Scenario: Backend unavailable
    Given getProjects() rejects
    Then the load state is "error"
    And the subtitle reads "Workspace data unavailable — start conduit serve."
    And the dashboard grid is not rendered
    And the "Start New Session" button is still shown

  @workspace @ready @matching
  Scenario: Matching the entered project
    Given getProjects() resolves
    Then the dashboard selects the project whose id equals projectId
    And if no id matches it falls back to the first returned project
    And if there are no projects it resolves to null

  @workspace @ready @not-found
  Scenario: Project not found
    Given getProjects() resolves but yields no matching project and no projects at all
    Then the subtitle reads "Project not found."
    And the dashboard grid is not rendered

  @workspace @ready @subtitle
  Scenario Outline: Subtitle session summary
    Given a matched project with sessionCount <count> and path "<path>"
    Then the subtitle reads "<count> <word> at <path>"

    Examples:
      | count | word     | path        |
      | 1     | session  | ~/repo      |
      | 4     | sessions | ~/proj/api  |

  @workspace @ready @summary
  Scenario: Project Summary card
    Given a matched project is loaded
    Then the left column shows a "Project Summary" card with:
      | block    | value                                         |
      | Path     | project.path                                  |
      | Branch   | project.branch, or "—" when empty             |
      | Sessions | "<sessionCount> session" / "<n> sessions"     |

  @workspace @ready @agents-empty
  Scenario: Active Agents card shows an honest empty state
    Given a matched project is loaded
    Then an "Active Agents" card shows "No agents attached yet."

  @workspace @ready @sessions
  Scenario: Recent Sessions list with sessions
    Given the matched project has one or more chats
    Then a "Recent Sessions" list shows each chat with:
      | element | source                                       |
      | icon    | "forum"                                       |
      | title   | chat.title, or "Untitled session" when empty  |
      | time    | relativeTime(chat.createdAt)                  |
      | preview | "<turnCount> turn" / "<n> turns"              |

  @workspace @ready @sessions @empty
  Scenario: Recent Sessions empty state
    Given the matched project has zero chats
    Then the sessions card shows "No sessions yet in this project."

  @workspace @ready @pinned-empty
  Scenario: Pinned Assets card shows an honest empty state
    Given a matched project is loaded
    Then a "Pinned Assets" card shows "No pinned assets."

  @workspace @navigation @back
  Scenario: Back action
    Given the dashboard was opened with an onBack handler
    Then a back button (arrow_back, aria-label "Back") is shown in the header
    When the user clicks it
    Then onBack fires and App navigates to the "projects" view

  @workspace @navigation @back @absent
  Scenario: Back button omitted when no handler
    Given the dashboard is rendered without an onBack handler
    Then no back button is shown

  @workspace @navigation @start-session
  Scenario: Start New Session
    Given the dashboard is rendered in any load state
    Then a "Start New Session" button is always shown at the bottom
    When the user clicks it
    Then onStartSession fires
    And App calls go("welcome"), resetting chat state and navigating to "welcome"

  @workspace @refetch
  Scenario: Re-fetching when the project changes
    Given the dashboard is displaying one project
    When projectId changes
    Then the load state returns to "loading" and getProjects() is called again

---

### Feature: Agent Sessions template picker (AgentSessionsPage)

  Note: AgentSessionsPage is a spike component that imports getAgents,
  createAgent, and AgentTemplate from "../api" (these helpers are not currently
  exported by api.ts) and is not yet wired into App.tsx navigation. The
  scenarios below document its in-source behavior.

  Background:
    Given the AgentSessionsPage is rendered
    And it calls getAgents() on mount

  @sessions @agents @loading
  Scenario: Loading templates
    Given getAgents() has not yet resolved
    Then a "Loading templates…" message is shown
    And no template cards are rendered

  @sessions @agents @ready
  Scenario: Templates loaded from the backend
    Given getAgents() resolves with a list of templates
    Then a grid of template cards is shown
    And each card shows a mapped icon, the template name, and its description

  @sessions @agents @fallback
  Scenario: Backend unreachable falls back to built-ins
    Given getAgents() rejects
    Then the page falls back to the six built-in templates:
      | name                | id                  |
      | Code Auditor        | code-auditor        |
      | Content Strategist  | content-strategist  |
      | System Architect    | system-architect    |
      | Product Manager     | product-manager     |
      | Security Researcher | security-researcher |
      | UI/UX Critic        | ui-ux-critic        |
    And the loading state is cleared

  @sessions @agents @select
  Scenario: Selecting a template
    Given templates are shown and none is selected
    When the user clicks a template card
    Then that card becomes selected (aria-pressed=true)
    And a "Start Session" button appears

  @sessions @agents @toggle
  Scenario: Toggling a selected template off
    Given a template card is currently selected
    When the user clicks it again
    Then it becomes deselected
    And the "Start Session" button is hidden

  @sessions @agents @single-select
  Scenario: Only one template selected at a time
    Given template A is selected
    When the user clicks template B
    Then template B becomes selected and template A is deselected

  @sessions @agents @start
  Scenario: Starting a session with a selected template
    Given a template is selected
    When the user clicks "Start Session"
    Then onStartSession is called with the selected template id

  @sessions @agents @start-row-hidden
  Scenario: No start row when nothing is selected
    Given no template is selected
    Then the "Start Session" button is not shown

  @sessions @agents @automations
  Scenario: Opening automations
    When the user clicks the "Automation" button
    Then onOpenAutomations is called

---

### Feature: Creating a custom agent (Custom Agent modal)

  Background:
    Given the AgentSessionsPage is rendered
    And the user clicks the "Custom Agent" button so the modal opens

  @sessions @custom-agent @modal
  Scenario: Modal structure and focus
    Then a dialog titled "Custom Agent" opens (role=dialog, aria-modal)
    And it has Name, Description, and System Prompt fields
    And the Name input receives focus on open

  @sessions @custom-agent @validation
  Scenario: Name is required
    Given the Name field is empty
    When the user clicks "Save Agent"
    Then the error "Name is required." is shown
    And createAgent is not called

  @sessions @custom-agent @validation
  Scenario: System prompt is required
    Given the Name field is filled but the System Prompt is empty
    When the user clicks "Save Agent"
    Then the error "System prompt is required." is shown
    And createAgent is not called

  @sessions @custom-agent @save
  Scenario: Saving a valid custom agent
    Given Name and System Prompt are both filled
    When the user clicks "Save Agent"
    Then the button shows "Saving…" and is disabled while in flight
    And createAgent is called with trimmed name, description, and systemPrompt
    When createAgent resolves with the saved template
    Then onSave adds it to the template list, selects it, and closes the modal

  @sessions @custom-agent @save @error
  Scenario: Save failure surfaces an error
    Given Name and System Prompt are filled
    When createAgent rejects
    Then the error message from the rejection (or "Save failed.") is shown
    And the modal stays open
    And the "Save Agent" button is re-enabled

  @sessions @custom-agent @close
  Scenario: Closing the modal
    When the user clicks the close (X) button or "Cancel"
    Then onClose fires and the modal closes
    And no agent is created

  @sessions @custom-agent @close @keyboard
  Scenario: Closing the modal with Escape
    When the user presses Escape inside the dialog
    Then onClose fires and the modal closes


---

## Settings, Soul, Plugins, Automations & Integrations

These acceptance scenarios document the GUI behavior of the Conduit Settings
page (appearance + model provider configuration), the Soul memory editor, the
Plugins and Automations "coming soon" surfaces, and the presentational
integration cards. Every scenario is grounded in the current source:

- `spike/gui/src/components/SettingsPage.tsx`
- `spike/gui/src/components/SoulPage.tsx`
- `spike/gui/src/components/PluginsPage.tsx`
- `spike/gui/src/components/AutomationsPage.tsx`
- `spike/gui/src/components/IntegrationCard.tsx`
- `spike/gui/src/api.ts` (`getInfo`, `patchSettings`, `getMemory`, `saveMemory`)
- Backend: `internal/server/server.go` (`RuntimeConfig`), `internal/server/views.go`
  (`handleInfo`, `handleSettings`, `handleMemory`)

> Implementation notes captured from code (not invented):
> - The Settings page reads the active provider/model from `GET /api/info` and
>   saves via `PATCH /api/settings` (added in PR #296). Theme changes are local
>   (no network call).
> - The provider/model dropdown options live in a hard-coded `PROVIDER_MODELS`
>   map in the front-end. The backend only validates the **provider** name
>   (`anthropic`, `codex`, `echo`); it does **not** validate the model string.
> - The Soul page reads `GET /api/memory` and writes `POST /api/memory`; both
>   `soul` and `user` fields are always sent together.
> - Plugins and Automations are honest empty-state pages — no backend exists, so
>   their primary actions are permanently disabled.
> - `IntegrationCard` is purely presentational; clicking it is a documented no-op
>   (auth flows are not wired). There is no connected/disconnected state in code.

---

### Feature: Open Settings and view the page structure

```gherkin
@settings
### Feature: Settings page layout and entry
  As a Conduit user
  I want a Settings page that groups appearance, model, about, and account
  So that I can configure the app from one place

  Background:
    Given the Conduit desktop app is running
    And I navigate to the Settings page

  @settings @layout
  Scenario: Settings page renders its header and sections
    Then I see the page title "Settings"
    And I see the subtitle "Tune the look of Conduit and configure the model provider."
    And I see an "Appearance" section
    And I see a "Model provider" section
    And I see an "About" section
    And I see an "Account & team" section

  @settings @layout @a11y
  Scenario: Each section is labelled for assistive technology
    Then the "Appearance" section is labelled by "set-appearance-label"
    And the "Model provider" section is labelled by "set-model-label"
    And the "About" section is labelled by "set-about-label"
    And the "Account & team" section is labelled by "set-soon-label"

  @settings @loading
  Scenario: Settings page requests current info on mount
    When the Settings page mounts
    Then it calls "GET /api/info" exactly once to seed the current provider, model, and version
```

---

### Feature: Change the theme from Settings

```gherkin
@settings @theme @appearance
### Feature: Theme selection from the Appearance section
  As a user
  I want to switch between Light, Dark, and High-contrast themes
  So that Conduit matches my visual preference

  Background:
    Given I am on the Settings page
    And the Appearance card shows a "Theme" field
    And the field hint reads "Applies instantly and is remembered across launches."
    And the theme options are rendered as a radiogroup labelled "Theme"

  @settings @theme
  Scenario Outline: The three theme options are presented
    Then I see a theme option for "<label>"
    And the option shows its icon "<icon>"

    Examples:
      | theme | label         | icon  |
      | light | (light meta)  | (light meta icon) |
      | dark  | (dark meta)   | (dark meta icon)  |
      | hc    | (hc meta)     | (hc meta icon)    |

    # Labels and icons come from THEME_META in useTheme; the order is light, dark, hc.

  @settings @theme @happy-path
  Scenario: Selecting a theme applies it instantly
    Given the current theme is "dark"
    When I click the theme option that is not currently active
    Then "onSetTheme" is called with the chosen theme
    And the chosen theme is applied immediately without a save step
    And no network request is made for the theme change

  @settings @theme
  Scenario: The active theme is visually marked
    Given the current theme is "light"
    Then the "light" theme card has the active styling "set-theme-card-active"
    And the "light" theme card has aria-checked "true"
    And the "light" theme card shows a check_circle indicator
    And the other theme cards have aria-checked "false"
    And the other theme cards show no check indicator

  @settings @theme @persistence
  Scenario: Theme choice persists across launches
    Given I select the "hc" theme
    When I quit and relaunch the app
    Then the Settings page opens with "hc" still active
```

---

### Feature: View the active provider and model

```gherkin
@settings @model-config @about
### Feature: Display the current provider, model, and version
  As a user
  I want to see which provider and model are active
  So that I know what backend my agent runs against

  Background:
    Given I am on the Settings page

  @settings @model-config @happy-path
  Scenario: The About section shows live values when the backend is reachable
    Given "GET /api/info" returns provider "anthropic", model "claude-opus-4-5", and version "1.2.3"
    When the info loads
    Then the About "Version" row shows "1.2.3" in monospace
    And the About "Active provider" row shows "anthropic"
    And the About "Active model" row shows "claude-opus-4-5" in monospace

  @settings @model-config @loading
  Scenario: The About rows show placeholders while loading
    Given the info request has not yet resolved
    Then the About "Version" row shows "—"
    And the About "Active provider" row shows "—"
    And the About "Active model" row shows "—"

  @settings @model-config @error
  Scenario: The About rows show placeholders when the backend is unreachable
    Given "GET /api/info" rejects
    Then the info state is "error"
    And the About "Version", "Active provider", and "Active model" rows all show "—"
```

---

### Feature: Seed the provider/model selectors from current info

```gherkin
@settings @model-config
### Feature: Seeding the provider and model dropdowns
  As a user
  I want the dropdowns to start on my active configuration
  So that I do not accidentally reset them

  @settings @model-config @seed
  Scenario: Known provider seeds both dropdowns from info
    Given "GET /api/info" returns provider "codex" and model "gpt-4o"
    When the info loads for the first time
    Then the Provider dropdown is set to "codex"
    And the Model dropdown is set to "gpt-4o"

  @settings @model-config @seed
  Scenario: Unknown provider falls back to anthropic
    Given "GET /api/info" returns provider "some-future-provider" and model "x"
    When the info loads for the first time
    Then the Provider dropdown falls back to "anthropic"
    And the Model dropdown is set to the returned model "x"

  @settings @model-config @seed
  Scenario: Missing model uses the provider default
    Given "GET /api/info" returns provider "anthropic" and an empty model
    When the info loads for the first time
    Then the Provider dropdown is "anthropic"
    And the Model dropdown is the anthropic default "claude-opus-4-5"

  @settings @model-config @seed
  Scenario: Seeding happens only once
    Given the info has already seeded the dropdowns
    And I have changed the Provider dropdown to "echo"
    When a later "GET /api/info" resolution arrives
    Then my selected provider "echo" is not overwritten by the re-seed
    # seededRef guards against re-seeding on subsequent info updates.
```

---

### Feature: Select a provider and see the model list update

```gherkin
@settings @model-config @provider
### Feature: Provider selection drives the model dropdown
  As a user
  I want the model list to match the chosen provider
  So that I only pick models the provider supports

  Background:
    Given I am on the Settings page
    And the info has loaded
    And the Provider dropdown offers "Anthropic", "OpenAI Codex", and "Echo (no API calls)"

  @settings @model-config @provider
  Scenario Outline: Choosing a provider repopulates the model dropdown with its models
    When I select provider "<provider>"
    Then the Model dropdown is set to the default model "<default_model>"
    And the Model dropdown options are exactly "<options>"
    And the save feedback resets to idle

    Examples:
      | provider  | default_model        | options                                                                                  |
      | anthropic | claude-opus-4-5      | Claude Opus 4.7, Claude Opus 4.5, Claude Sonnet 4.6, Claude Haiku 4.5                     |
      | codex     | gpt-5.5              | GPT-5.5, GPT-4o, o3, o4-mini                                                              |
      | echo      | (none)               | (none — echo only)                                                                       |

  @settings @model-config @provider
  Scenario Outline: Model option values match the wire values sent on save
    When I select provider "<provider>"
    Then the Model dropdown option values are "<values>"

    Examples:
      | provider  | values                                                                          |
      | anthropic | claude-opus-4-7, claude-opus-4-5, claude-sonnet-4-6, claude-haiku-4-5-20251001   |
      | codex     | gpt-5.5, gpt-4o, o3, o4-mini                                                     |
      | echo      | (none)                                                                          |

  @settings @model-config @provider @echo
  Scenario: Selecting Echo disables the model dropdown
    When I select provider "echo"
    Then the Model dropdown is disabled
    And the Model dropdown shows only the single option "(none — echo only)"

  @settings @model-config @provider
  Scenario: Switching provider resets the model to that provider's default
    Given I have provider "anthropic" with model "claude-sonnet-4-6" selected
    When I select provider "codex"
    Then the Model dropdown resets to "gpt-5.5"
    And it does not retain the previous anthropic model
```

---

### Feature: Save (Apply) model configuration via PATCH /api/settings

```gherkin
@settings @model-config @save
### Feature: Applying provider/model changes
  As a user
  I want an Apply button that saves my provider and model
  So that new agent sessions use my chosen backend

  Background:
    Given I am on the Settings page
    And "GET /api/info" returned provider "anthropic" and model "claude-opus-4-5"
    And the dropdowns are seeded to that configuration

  @settings @model-config @save @dirty-state
  Scenario: Apply is disabled until a change is made
    Given the selected provider and model match the active info
    Then the "Apply" button is disabled

  @settings @model-config @save @dirty-state
  Scenario: Changing the model enables Apply
    When I select model "claude-sonnet-4-6"
    Then the configuration is dirty
    And the "Apply" button is enabled

  @settings @model-config @save @dirty-state
  Scenario: Changing the provider enables Apply
    When I select provider "codex"
    Then the configuration is dirty
    And the "Apply" button is enabled

  @settings @model-config @save @happy-path
  Scenario: Applying a valid change saves and confirms
    Given I have selected provider "codex" and model "gpt-5.5"
    When I click "Apply"
    Then the button shows "Saving…" and is disabled
    And "PATCH /api/settings" is called with body provider "codex" and model "gpt-5.5"
    And on success the About rows update to provider "codex" and model "gpt-5.5"
    And a "Saved" confirmation with a check_circle icon appears
    And the button shows the done styling "set-save-btn-done"
    And after about 2 seconds the save feedback returns to idle

  @settings @model-config @save @controls-disabled
  Scenario: Controls lock while saving
    Given a save is in progress
    Then the Provider dropdown is disabled
    And the Model dropdown is disabled
    And the "Apply" button is disabled and labelled "Saving…"

  @settings @model-config @save @after-save
  Scenario: After a successful save the configuration is no longer dirty
    Given I applied provider "codex" and model "gpt-5.5"
    And the info now reflects "codex" / "gpt-5.5"
    Then the "Apply" button becomes disabled again
    And no further save is required
```

---

### Feature: Save error feedback

```gherkin
@settings @model-config @save @error
### Feature: Error handling when applying settings fails
  As a user
  I want a clear error if my change cannot be saved
  So that I know the active provider was not changed

  Background:
    Given I am on the Settings page
    And the dropdowns are seeded and I have made a change

  @settings @model-config @save @error
  Scenario: Backend rejects with a structured error message
    Given "PATCH /api/settings" responds non-OK with body {"error": "unknown provider; must be anthropic, codex, or echo"}
    When I click "Apply"
    Then the save state becomes "error"
    And the error message "unknown provider; must be anthropic, codex, or echo" is shown next to the button
    And the About rows remain unchanged
    And the "Apply" button is re-enabled so I can retry

  @settings @model-config @save @error
  Scenario: Backend rejects without an error field
    Given "PATCH /api/settings" responds with HTTP 500 and no JSON error field
    When I click "Apply"
    Then the shown error falls back to "HTTP 500"

  @settings @model-config @save @error
  Scenario: Network failure during save
    Given "PATCH /api/settings" rejects before a response is received
    When I click "Apply"
    Then the save state becomes "error"
    And an error message describing the failure is displayed
    And the message defaults to "Unknown error" if no Error message is available
```

---

### Feature: Model provider section when the backend is offline

```gherkin
@settings @model-config @error @offline
### Feature: Offline notice in the Model provider card
  As a user
  I want a hint when the backend cannot be reached
  So that I know to start the server before changing the provider

  @settings @model-config @offline
  Scenario: Offline notice appears when info fails to load
    Given "GET /api/info" rejects
    When the Settings page is in the error state
    Then the Model provider card shows a notice telling me to start the backend with "conduit serve"
    And the dropdowns still render with their local default options
```

---

### Feature: Backend validation that the Settings save flow relies on

```gherkin
@settings @model-config @backend @validation
### Feature: PATCH /api/settings server-side contract
  As the GUI
  I depend on the server validating and persisting the runtime configuration
  So that the Apply button's success/error states are accurate

  # internal/server/views.go handleSettings + internal/server/server.go RuntimeConfig

  @backend @validation
  Scenario: Only PATCH is accepted
    When a non-PATCH request hits "/api/settings"
    Then the server responds 405 Method Not Allowed
    And sets an "Allow: PATCH" header
    And returns {"error": "method not allowed"}

  @backend @validation
  Scenario: Runtime config must be available
    Given the server was constructed without a RuntimeConfig
    When a PATCH request hits "/api/settings"
    Then the server responds 501 Not Implemented
    And returns {"error": "runtime settings not available"}

  @backend @validation
  Scenario: Malformed JSON body is rejected
    Given a RuntimeConfig is available
    When a PATCH with an invalid JSON body is received
    Then the server responds 400 Bad Request
    And returns {"error": "invalid JSON body"}

  @backend @validation
  Scenario Outline: Provider name is validated (model is not)
    Given a RuntimeConfig is available
    When a PATCH with provider "<provider>" is received
    Then the server responds "<status>"

    Examples:
      | provider   | status        |
      | anthropic  | 200 OK        |
      | codex      | 200 OK        |
      | echo       | 200 OK        |
      | bogus      | 400 Bad Request |
      | (empty)    | 400 Bad Request |

  @backend @validation
  Scenario: Invalid provider returns a descriptive error
    When a PATCH with an unknown provider is received
    Then the server returns {"error": "unknown provider; must be anthropic, codex, or echo"}

  @backend @validation @whitespace
  Scenario: Provider and model are trimmed before validation and storage
    When a PATCH with provider " anthropic " and model " claude-opus-4-5 " is received
    Then the leading and trailing whitespace is trimmed
    And the provider validates as "anthropic"

  @backend @happy-path
  Scenario: A valid PATCH hot-swaps the runtime config and echoes it
    When a PATCH with provider "codex" and model "gpt-5.5" is received
    Then the RuntimeConfig is updated to "codex" / "gpt-5.5"
    And the response echoes provider "codex", model "gpt-5.5", and the server version
    And new WebSocket connections pick up the change without a restart
    And in-flight sessions are unaffected
```

---

### Feature: Account & team (coming soon)

```gherkin
@settings @coming-soon
### Feature: Account & team placeholder
  As a user
  I want an honest placeholder for unfinished settings
  So that I am not misled about available features

  @settings @coming-soon
  Scenario: Account & team shows a coming-soon card
    Given I am on the Settings page
    Then the "Account & team" section shows a muted card with an hourglass icon
    And it reads "Coming soon"
    And it explains account, billing, and team settings are not available yet because there is no backend for them
    And there are no interactive controls in this card
```

---

### Feature: Soul memory page — load and structure

```gherkin
@soul @memory
### Feature: Soul page layout and loading
  As a user
  I want to view my agent's persistent memory
  So that I can inspect SOUL.md (identity) and USER.md (about me)

  Background:
    Given I navigate to the Soul page

  @soul @layout
  Scenario: Soul page header
    Then I see an eyebrow "Memory" with a psychology icon
    And I see the title "Soul"
    And I see the subtitle describing SOUL.md (identity) and USER.md (about you)

  @soul @loading
  Scenario: Loading state while fetching memory
    Given "GET /api/memory" has not yet resolved
    Then the page shows "Loading memory…"
    And no memory cards are rendered yet

  @soul @loading
  Scenario: Soul page requests memory on mount
    When the Soul page mounts
    Then it calls "GET /api/memory" once to load the raw SOUL.md and USER.md markdown

  @soul @error @offline
  Scenario: Error state when the backend is offline
    Given "GET /api/memory" rejects
    Then the page shows "Memory unavailable — start conduit serve."
    And no memory cards are rendered

  @soul @loaded
  Scenario: Loaded state renders both memory cards
    Given "GET /api/memory" returns soul and user markdown
    Then I see a "SOUL.md" card captioned "Identity — who your agent is."
    And I see a "USER.md" card captioned "About you — what your agent should know."
    And each card shows its icon (psychology for SOUL.md, person for USER.md)
```

---

### Feature: Soul memory page — read view rendering

```gherkin
@soul @memory @render
### Feature: Markdown read view for each memory file
  As a user
  I want my memory rendered as light markdown
  So that headings and bullets are readable without editing

  Background:
    Given the Soul page has loaded both memory files

  @soul @render
  Scenario: Non-empty content renders as markdown
    Given the SOUL.md content has a "# " heading, "## " subheadings, and "- " bullet lines
    Then the SOUL.md card renders the "# " line as a heading
    And renders "## " through "###### " lines as sub-headings
    And renders consecutive "-" or "*" lines as a bullet list
    And renders other non-blank lines as paragraphs
    And a blank line flushes the current bullet list

  @soul @render @empty
  Scenario: Empty file shows a seed prompt
    Given the USER.md content is empty or only whitespace
    Then the USER.md card shows "Nothing written yet. Use the pencil to seed USER.md."
    And no edit form is shown until I click the pencil

  @soul @render
  Scenario: The edit (pencil) button is offered only when loaded and not editing
    Given memory is loaded and I am not editing a card
    Then each card shows an Edit pencil button labelled "Edit SOUL.md" / "Edit USER.md"
    And while a card is in edit mode its pencil button is hidden
```

---

### Feature: Soul memory page — editing and saving

```gherkin
@soul @memory @edit @save
### Feature: Per-card memory editing
  As a user
  I want to edit and save each memory file independently
  So that I can update identity or personal context

  Background:
    Given the Soul page has loaded with SOUL.md "old soul" and USER.md "old user"

  @soul @edit
  Scenario: Entering edit mode seeds the textarea from the current value
    When I click the pencil on the SOUL.md card
    Then a textarea opens pre-filled with "old soul"
    And the textarea has a placeholder hint for that file
    And any previous save error is cleared
    And the textarea has spellCheck disabled

  @soul @edit
  Scenario: Only one card edits at a time
    Given I am editing the SOUL.md card
    When I click the pencil on the USER.md card
    Then the editor switches to the USER.md card
    And the draft is reset to the USER.md value

  @soul @edit @cancel
  Scenario: Cancelling discards the draft
    Given I am editing SOUL.md and have typed "new draft"
    When I click "Cancel"
    Then edit mode closes
    And the SOUL.md card still shows the original "old soul"
    And any save error is cleared

  @soul @save @happy-path
  Scenario: Saving a card persists both files together
    Given I am editing USER.md and changed it to "new user"
    When I click "Save"
    Then the button shows "Saving…" and the form controls are disabled
    And "POST /api/memory" is called with soul "old soul" and user "new user"
    And on success the card view updates to the saved content
    And edit mode closes

  @soul @save @consistency
  Scenario: Editing one card never drops the other file's content
    Given the loaded memory has both SOUL.md and USER.md populated
    When I edit and save only the SOUL.md card
    Then the POST body still includes the unchanged USER.md content
    # save() spreads the existing memory and overrides only the edited key.

  @soul @save @error
  Scenario: Save failure shows an inline error and keeps me in edit mode
    Given I am editing SOUL.md
    And "POST /api/memory" rejects
    When I click "Save"
    Then the inline error "Couldn't save — is conduit serve running?" is shown
    And the editor remains open with my draft intact
    And the form controls are re-enabled after the attempt
```

---

### Feature: Soul memory page — backend memory contract

```gherkin
@soul @memory @backend
### Feature: /api/memory server-side contract
  As the GUI
  I depend on the server reading and writing SOUL.md / USER.md
  So that the Soul page load and save flows behave correctly

  # internal/server/views.go handleMemory + saveMemory

  @backend
  Scenario: GET returns both files, empty strings when missing
    When the GUI requests "GET /api/memory"
    Then the server reads ~/.conduit/SOUL.md and ~/.conduit/USER.md
    And missing files return empty strings rather than errors

  @backend
  Scenario: POST/PUT writes both files atomically and echoes them
    When the GUI sends "POST /api/memory" with {soul, user}
    Then both SOUL.md and USER.md are written under ~/.conduit
    And writes are atomic (temp file then rename)
    And the response echoes the saved {soul, user}

  @backend @error
  Scenario: Save fails with a clear error when no home directory is configured
    Given the server has no home directory configured
    When a POST to "/api/memory" is received
    Then the server responds 500 with {"error": "no home directory configured"}

  @backend @error
  Scenario: Malformed save body is rejected
    When a POST to "/api/memory" has an invalid JSON body
    Then the server responds 400 with {"error": "invalid JSON body"}

  @backend @error
  Scenario: Unsupported method on /api/memory
    When a DELETE request hits "/api/memory"
    Then the server responds 405 with an "Allow: GET, POST, PUT" header
```

---

### Feature: Plugins page (coming soon)

```gherkin
@plugins @coming-soon
### Feature: Plugins empty-state surface
  As a user
  I want an honest plugins page
  So that I understand plugin support is not yet available

  Background:
    Given I navigate to the Plugins page

  @plugins @layout
  Scenario: Plugins header
    Then I see the title "Plugins"
    And I see the subtitle "Extend Conduit with tools and integrations."

  @plugins @empty
  Scenario: Plugins empty state
    Then I see an extension icon tile
    And I see the heading "No plugins installed"
    And I see text explaining plugin support is coming soon

  @plugins @disabled
  Scenario: Browse plugins call-to-action is permanently disabled
    Then a "Browse plugins" button is shown with a travel_explore icon
    And the button is disabled
    And clicking it does nothing

  @plugins @chips
  Scenario: Placeholder category chips are non-interactive
    Then I see category chips "Tools", "Data", and "Integrations"
    And the chips group is marked aria-hidden
    And the chips are not clickable
```

---

### Feature: Automations page (coming soon)

```gherkin
@automations @coming-soon
### Feature: Automations empty-state surface
  As a user
  I want an honest automations page
  So that I understand automation workflows are not yet available

  Background:
    Given I navigate to the Automations page

  @automations @layout
  Scenario: Automations header
    Then I see the title "Automations"
    And I see the subtitle "Schedule and chain agent workflows."

  @automations @empty
  Scenario: Automations empty state
    Then I see an auto_mode icon tile
    And I see the heading "No automations yet"
    And I see text explaining automation workflows are coming soon

  @automations @disabled
  Scenario: New automation call-to-action is permanently disabled
    Then a "New automation" button is shown with an add icon
    And the button is disabled
    And clicking it does nothing
```

---

### Feature: Integration cards (presentational)

```gherkin
@integrations
### Feature: Connect-X integration cards
  As a user
  I want to see available integrations
  So that I anticipate connecting tools like GitHub, Linear, and Slack

  # IntegrationCard is pure presentation. There is NO connected/disconnected
  # state machine in code; clicking a card is a documented no-op because
  # integration auth flows are not wired in this PR.

  Background:
    Given an IntegrationCard is rendered

  @integrations @render
  Scenario: A card renders its brand icon, title, and body
    Then the card shows the provided brand icon in a brand area
    And the card shows the provided title
    And the card shows the provided body text

  @integrations @render
  Scenario: The tile variant adjusts the brand area styling
    Given the card is rendered with tile set to true
    Then the brand area gets the additional "tile" class
    And without the tile flag the brand area has no "tile" class

  @integrations @brands
  Scenario Outline: Brand marks render with their real brand colors
    Given a "<brand>" mark is rendered
    Then it draws the "<brand>" logo as an inline SVG marked aria-hidden

    Examples:
      | brand  |
      | GitHub |
      | Linear |
      | Slack  |

  @integrations @hover
  Scenario: Hovering a card produces a visual affordance only
    When I hover over an integration card
    Then the card shows its hover styling
    But hovering does not trigger any connect action

  @integrations @no-op
  Scenario: Clicking a card is currently a no-op
    When I click an integration card
    Then no connect/disconnect flow runs
    And no network request is made
    # Auth flows aren't wired; the card has no onClick handler.
```


---

## Native GUI — Layout & Core Views

This document specifies the acceptance scenarios for the **native macOS GUI surface** modeled
in Go (package `gui`). This package holds the headless state/layout view-models for the
production GUI — the rendering layer (WKWebView / native AppKit) reads these models to decide
what to draw. The package boundary is finalized in Phase 5; this scaffold makes imports and
ownership explicit while the shared core evolves.

All scenarios are grounded in the source under `internal/gui/` (identical to
`internal/surface/gui/`) and the behavior pinned by the matching `_test.go` files. Exact
constant values, clamping ranges, fractions, and transitions are reproduced verbatim.

---

### Feature: Three-Column Window Layout

The GUI window is a three-column layout: a left **sidebar** (Sessions / Memory / Skills tabs),
a centre **main view** (Screenshot / Canvas / WorkflowDAG / MemoryBrowser / DiffEditor), and a
right **agent panel**. Column widths derive from user-resizable fractions of the total window
width, with pixel minimums that protect each column. `Compute()` is pure (no side effects) so
it can be called from render paths.

@native-gui @layout

Background:
  Given the default column proportions:
    | constant              | value | meaning                                          |
    | defaultSidebarFrac    | 0.18  | sidebar is ~18% of window width                  |
    | defaultAgentPanelFrac | 0.28  | agent panel is ~28% of window width              |
    | minSidebarWidth       | 180   | pixels; below this the sidebar collapses         |
    | minAgentPanelWidth    | 220   | pixels; the agent panel never collapses          |
    | minMainWidth          | 300   | pixels; the centre column minimum                |

  Scenario: A new Layout initializes with default tab and view
  Given a new Layout sized 1440 x 900
  Then the active sidebar tab is TabSessions
  And the active main view is ViewScreenshot
  And the sidebar fraction is 0.18
  And the agent panel fraction is 0.28

  Scenario: Computing a wide window yields three positive, summing columns
  Given a new Layout sized 1440 x 900
  When the layout dimensions are computed
  Then the sidebar is visible
  And the sidebar width is greater than 0
  And the agent panel width is greater than 0
  And the main width is at least 300 pixels
  And SidebarWidth + MainWidth + AgentPanelWidth equals the total width 1440

  Scenario: The agent panel is clamped up to its minimum width
  Given a new Layout
  When the computed agent panel raw width would be below 220 pixels
  Then the agent panel width is forced to 220 pixels
  And the agent panel never collapses to zero

  Scenario: Computed columns always sum exactly to the total width
  Given any Layout with computed dimensions
  Then SidebarWidth + MainWidth + AgentPanelWidth equals TotalWidth
  And TotalHeight equals the window height passed in

---

### Feature: Sidebar Collapse and Auto-Collapse

The sidebar can be collapsed explicitly via `ToggleSidebar()` or auto-collapse when the window
is too narrow for the sidebar to meet `minSidebarWidth` (180 px). When the main area would fall
below `minMainWidth` (300 px), the layout shrinks the sidebar first; if shrinking would push the
sidebar below its minimum, the sidebar is hidden entirely (width 0) and reclaimed by the main area.

@native-gui @layout @sidebar-collapse

  Scenario: Explicitly collapsing the sidebar hides it and zeroes its width
  Given a new Layout sized 1440 x 900
  When the sidebar is toggled once
  And the layout is computed
  Then the sidebar is not visible
  And the sidebar width is 0
  And SidebarWidth + MainWidth + AgentPanelWidth still equals TotalWidth

  Scenario: A narrow window auto-collapses the sidebar
  Given a new Layout sized 700 x 600
  # At 700 px the raw sidebar width is ~126 px, below minSidebarWidth=180
  When the layout is computed
  Then the sidebar is auto-collapsed and not visible
  And the main width is still at least 300 pixels

  Scenario: Toggling the sidebar twice on a wide window restores it
  Given a new Layout sized 1440 x 900
  When the sidebar is toggled to collapse
  And the sidebar is toggled again to expand
  And the layout is computed
  Then the sidebar is visible again

  Scenario: The main area is preserved by shrinking the sidebar first
  Given a Layout where the main width would fall below 300 pixels
  When the layout is computed
  Then the sidebar width is reduced by the excess needed to restore the main area
  But if the reduced sidebar would fall below 180 pixels then the sidebar is hidden
  And its width is set to 0 and the main area absorbs the freed space

---

### Feature: Window Resize Recomputes Columns

`Resize(width, height)` updates the stored window dimensions; `Compute()` then re-derives all
column widths from the (preserved) fractions.

@native-gui @layout @resize

  Scenario: Resizing the window recomputes column widths to the new total
  Given a new Layout sized 1440 x 900
  When the window is resized to 1000 x 800
  And the layout is computed
  Then the total width is 1000
  And SidebarWidth + MainWidth + AgentPanelWidth equals 1000

---

### Feature: Sidebar Tabs and Main View Selection

The sidebar exposes three navigation tabs; the centre column exposes five main views. Selecting
either updates the active state read by the renderer.

@native-gui @layout @navigation

Background:
  Given the sidebar tabs:
    | tab         | ordinal | purpose                                |
    | TabSessions | 0       | session history list                   |
    | TabMemory   | 1       | SOUL.md / USER.md / memory inspector   |
    | TabSkills   | 2       | skills registry                        |
  And the main views:
    | view              | ordinal | purpose                                  |
    | ViewScreenshot    | 0       | live computer-use screenshot stream      |
    | ViewCanvas        | 1       | WKWebView / HTML canvas panel            |
    | ViewWorkflowDAG   | 2       | workflow step graph                      |
    | ViewMemoryBrowser | 3       | memory / SOUL / USER entries             |
    | ViewDiffEditor    | 4       | GitHub-style diff view (coding mode)     |

  Scenario Outline: Selecting a sidebar tab updates the active tab
  Given a new Layout sized 1440 x 900
  When sidebar tab <tab> is selected
  Then the active sidebar tab is <tab>

  Examples:
    | tab         |
    | TabSessions |
    | TabMemory   |
    | TabSkills   |

  Scenario Outline: Selecting a main view updates the active view
  Given a new Layout sized 1440 x 900
  When main view <view> is selected
  Then the active main view is <view>

  Examples:
    | view              |
    | ViewScreenshot    |
    | ViewCanvas        |
    | ViewWorkflowDAG   |
    | ViewMemoryBrowser |
    | ViewDiffEditor    |

---

### Feature: Drag-Handle Fraction Clamping

Drag handles let the user resize the sidebar and agent panel. The incoming fraction is clamped
into a sensible range so columns can never become unusable.

@native-gui @layout @resize

  Scenario Outline: The sidebar fraction is clamped to [0.10, 0.30]
  Given a new Layout sized 1440 x 900
  When the sidebar fraction is set to <input>
  Then the stored sidebar fraction is <clamped>

  Examples:
    | input | clamped | note            |
    | 0.01  | 0.10    | below minimum   |
    | 0.10  | 0.10    | at minimum      |
    | 0.18  | 0.18    | in range        |
    | 0.30  | 0.30    | at maximum      |
    | 0.99  | 0.30    | above maximum   |

  Scenario Outline: The agent-panel fraction is clamped to [0.20, 0.40]
  Given a new Layout sized 1440 x 900
  When the agent-panel fraction is set to <input>
  Then the stored agent-panel fraction is <clamped>

  Examples:
    | input | clamped | note            |
    | 0.01  | 0.20    | below minimum   |
    | 0.20  | 0.20    | at minimum      |
    | 0.28  | 0.28    | in range        |
    | 0.40  | 0.40    | at maximum      |
    | 0.99  | 0.40    | above maximum   |

---

### Feature: Canvas Panel — HTML Frame History

`CanvasPanel` is the view-model for the Canvas HTML panel. Agents push HTML frames via
`[[canvas: html]]` reply tags; the panel keeps a browser-style navigation history so users can
step back and forward through renders. The WKWebView reads `HTML()` and `Visible()` to decide
what to inject. The panel is concurrency-safe (the replytag dispatcher and the UI event loop run
on different goroutines). A new panel is empty (cursor -1) and hidden.

@native-gui @canvas

Background:
  Given a new empty CanvasPanel
  Then it has 0 frames
  And it is not visible
  And HTML() returns the empty string

  Scenario: Pushing a frame stores it and returns it as the active HTML
  Given a new CanvasPanel
  When the frame "<h1>Hello</h1>" is pushed
  Then HTML() returns "<h1>Hello</h1>"

  Scenario: The first push makes the panel visible automatically
  Given a new CanvasPanel that is not visible
  When the frame "<p>hi</p>" is pushed
  Then the panel becomes visible

  Scenario: Show and Hide toggle visibility without affecting frames
  Given a CanvasPanel with one pushed frame "<p>x</p>"
  When Hide() is called
  Then the panel is not visible
  When Show() is called
  Then the panel is visible

  Scenario: Back and Forward navigate the frame history
  Given a CanvasPanel with frames pushed in order "frame1", "frame2", "frame3"
  Then HTML() returns "<p>frame3</p>" (the newest)
  When Back() is called
  Then HTML() returns "<p>frame2</p>"
  When Back() is called again
  Then HTML() returns "<p>frame1</p>"
  When Back() is called at the oldest frame
  Then HTML() still returns "<p>frame1</p>" (no-op)
  When Forward() is called
  Then HTML() returns "<p>frame2</p>"

  Scenario Outline: CanGoBack / CanGoForward reflect cursor position
  Given a CanvasPanel in state "<state>"
  Then CanGoBack() is <can_back>
  And CanGoForward() is <can_forward>

  Examples:
    | state                                          | can_back | can_forward |
    | empty                                          | false    | false       |
    | single frame at newest                         | false    | false       |
    | two frames, cursor at newest                   | true     | false       |
    | two frames, navigated back to oldest           | false    | true        |

  Scenario: Reload returns the active frame for re-injection
  Given a new CanvasPanel
  Then Reload() returns the empty string
  When the frame "<div>reload me</div>" is pushed
  Then Reload() returns "<div>reload me</div>"

  Scenario: Pushing mid-history truncates forward frames (browser-style)
  Given a CanvasPanel with frames "frame1", "frame2", "frame3"
  And the cursor is navigated back twice to "frame1"
  When the frame "<p>new</p>" is pushed
  Then the history holds exactly 2 frames (frame1 + new)
  And HTML() returns "<p>new</p>"
  And CanGoForward() is false
  And CanGoBack() is true
  When Back() is called
  Then HTML() returns "<p>frame1</p>"
  And the panel remains visible throughout

  Scenario: Clear empties the panel and hides it
  Given a CanvasPanel with frames "<p>a</p>" and "<p>b</p>"
  When Clear() is called
  Then the panel holds 0 frames
  And the panel is not visible
  And HTML() returns the empty string

  Scenario: Navigation on an empty panel is a safe no-op
  Given a new empty CanvasPanel
  When Back() and Forward() are called
  Then no panic occurs
  And HTML() returns the empty string

  Scenario: Len reflects the number of pushed frames
  Given a new CanvasPanel
  When 5 frames "frame0".."frame4" are pushed
  Then Len() returns 5

  Scenario: Concurrent pushes are safe
  Given a new CanvasPanel
  When two goroutines each push 50 frames concurrently
  Then no panic occurs
  And the panel holds at least one frame afterward

---

### Feature: Screenshot Stream — Live Computer-Use Frames

`ScreenshotStream` is the view-model for the live computer-use screenshot stream shown when
`ViewScreenshot` is active. Each `ScreenshotStep` carries a StepID, timestamp, pre/post PNG
paths, capture flag, duration, status, and a pin flag. Steps are ordered oldest → newest. The
stream is concurrency-safe (capture backend vs. UI event loop). The default displayed phase is
**PhasePost** (the result screenshot). `maxSteps` caps in-memory history; 0 means unlimited.

@native-gui @screenshot-stream

Background:
  Given the active phases:
    | phase     | ordinal | meaning                       |
    | PhasePre  | 0       | show the pre-action screenshot  |
    | PhasePost | 1       | show the post-action screenshot |
  And a new stream defaults to displaying PhasePost

  Scenario: Pushing steps appends them oldest-first
  Given a new ScreenshotStream with unlimited history
  When step "step-1" is pushed
  And step "step-2" is pushed
  Then the stream holds 2 steps
  And the first step is "step-1"
  And the second step is "step-2"

  Scenario: Re-pushing the same StepID merges in place (pre then post arrives)
  Given a new ScreenshotStream
  And step "step-1" is pushed with PostPath empty and Captured false
  When step "step-1" is pushed again carrying only the PostPath
  Then the stream holds exactly 1 step
  And the original PrePath is preserved
  And the PostPath is updated to the new value

  Scenario: The most-recently pushed step becomes active automatically
  Given a new ScreenshotStream
  When step "step-1" is pushed
  Then the active step is "step-1"
  When step "step-2" is pushed
  Then the active step auto-advances to "step-2"

  Scenario Outline: ActiveImagePath resolves by phase with pre fallback
  Given a stream with a single step having PrePath "<pre>" and PostPath "<post>"
  And the selected phase is <phase>
  Then ActiveImagePath() returns "<expected>"

  Examples:
    | phase     | pre              | post              | expected          |
    | PhasePost | /tmp/s1-pre.png  | /tmp/s1-post.png  | /tmp/s1-post.png  |
    | PhasePre  | /tmp/s1-pre.png  | /tmp/s1-post.png  | /tmp/s1-pre.png   |
    | PhasePost | /tmp/s1-pre.png  | (empty)           | /tmp/s1-pre.png   |

  Scenario: Pinning a step exposes it in PinnedSteps and PinnedPaths
  Given a stream with steps "s1" and "s2"
  When step "s1" is pinned
  Then PinnedSteps() returns exactly "s1"
  And PinnedPaths() returns exactly "/tmp/s1-post.png"

  Scenario: Unpinning removes a step from the pinned set
  Given a stream with step "s1" pinned
  When step "s1" is unpinned
  Then PinnedSteps() returns no steps

  Scenario: NavigatePrev and NavigateNext walk the step list
  Given a stream with steps "s1", "s2", "s3" (active = newest "s3")
  When NavigatePrev() is called
  Then the active step is "s2"
  When NavigatePrev() is called again
  Then the active step is "s1"
  When NavigatePrev() is called at the oldest step
  Then the active step stays "s1" (no-op)
  When NavigateNext() is called
  Then the active step is "s2"

  Scenario: Reaching the max step cap evicts the oldest unpinned step
  Given a new ScreenshotStream with maxSteps = 3
  When steps "s1", "s2", "s3", "s4" are pushed
  Then the stream holds 3 steps
  And "s1" (oldest unpinned) was evicted
  And the oldest remaining step is "s2"

  Scenario: Pinned steps are never evicted by the cap
  Given a new ScreenshotStream with maxSteps = 2
  And step "s1" is pushed and pinned
  When steps "s2" and "s3" are pushed
  Then the stream holds 2 steps
  And "s2" was evicted instead of the pinned "s1"
  And the pinned "s1" is still present

  Scenario: Clear keeps pinned steps and drops the rest
  Given a stream with steps "s1" and "s2" where "s2" is pinned
  When Clear() is called
  Then the stream holds 1 step
  And the surviving step is the pinned "s2"

  Scenario: An empty stream returns nil and empty values
  Given a new empty ScreenshotStream
  Then ActiveStep() returns nil
  And ActiveImagePath() returns the empty string
  And Len() returns 0

  Scenario: Concurrent pushes are safe
  Given a new ScreenshotStream with maxSteps = 100
  When two goroutines each push 50 steps concurrently
  Then no panic occurs
  And the stream holds at least one step afterward

  Scenario: StepsByTimestamp returns a stable chronological order
  Given a stream where steps "s3", "s1", "s2" are pushed out of timestamp order
  When StepsByTimestamp() is called
  Then the returned steps are ordered ascending by timestamp

  Scenario Outline: ThumbnailPath prefers post, falling back to pre
  Given a step with PostPath "<post>" and PrePath "<pre>"
  Then ThumbnailPath returns "<expected>"

  Examples:
    | post              | pre              | expected          |
    | /tmp/s1-post.png  | /tmp/s1-pre.png  | /tmp/s1-post.png  |
    | (empty)           | /tmp/s1-pre.png  | /tmp/s1-pre.png   |

  Scenario: StepBasename returns the file base name for strip labels
  Given the path "/some/long/dir/step-1-post.png"
  Then StepBasename returns "step-1-post.png"

---

### Feature: Diff View — GitHub-Style Coding Diff (issue #69)

Every file write/edit by the agent emits a `DiffEntry`. `DiffView` holds the live list, supports
unified and side-by-side rendering, hunk-level approve/reject, and per-line annotations that flow
back to the agent. The view is concurrency-safe (tool execution writes entries while the UI reads
and approves them). A new view starts empty in **unified** mode.

@native-gui @diff-view

Background:
  Given the diff modes:
    | mode               | ordinal | rendering                          |
    | DiffModeUnified    | 0       | single column, +/- prefixes        |
    | DiffModeSideBySide | 1       | two columns: before | after        |
  And the line kinds:
    | kind        | ordinal | meaning                          |
    | LineContext | 0       | unchanged context line             |
    | LineAdded   | 1       | present only in the new file       |
    | LineRemoved | 2       | present only in the old file       |
  And the hunk statuses:
    | status       | ordinal | disposition           |
    | HunkPending  | 0       | awaiting user review    |
    | HunkApproved | 1       | accepted                |
    | HunkRejected | 2       | discarded               |

  Scenario: A new view is empty and has no active entry
  Given a new DiffView
  Then Active() returns nil
  And the default mode is DiffModeUnified

  Scenario: Pushing an entry makes it active and listed
  Given a new DiffView
  When a DiffEntry "e1" for "main.go" (tracked, base "abc") is pushed
  Then Active() is the entry "e1"
  And the view holds 1 entry

  Scenario: Re-pushing the same entry ID replaces it in place
  Given a DiffView holding entry "e1" with base "abc"
  When a new entry "e1" for "main.go" with base "def" is pushed
  Then the view still holds 1 entry
  And the active entry's BaseSHA is "def"

  Scenario: Mode toggles between unified and side-by-side
  Given a new DiffView in DiffModeUnified
  When the mode is set to DiffModeSideBySide
  Then Mode() returns DiffModeSideBySide

  Scenario: Approving and rejecting hunks records their disposition
  Given an entry "e1" with hunks "h1" and "h2" pushed into the view
  When hunk "h1" is approved
  And hunk "h2" is rejected
  Then hunk "h1" status is HunkApproved
  And hunk "h2" status is HunkRejected

  Scenario: PendingHunks lists only undisposed hunks across all entries
  Given entry "e1" for "a.go" with hunks "h1" and "h2"
  And entry "e2" for "b.go" with hunk "h3"
  When hunk "h1" is approved
  Then PendingHunks contains exactly 2 hunks: "h2" and "h3"
  And each HunkRef carries its EntryID, HunkID, and Path

  Scenario: Annotations are recorded and surfaced chronologically to the agent
  Given an entry "e1" for "x.go" with hunk "h1"
  When line 0 is annotated "use a constant here"
  And line 1 is annotated "extract this into a helper"
  Then AnnotationsForAgent returns 2 notes
  And the first note body is "use a constant here"

  Scenario: Selecting an entry by ID makes it active
  Given a DiffView holding entries "e1" and "e2"
  When entry "e1" is selected
  Then Active().ID is "e1"

  Scenario: Clear empties the view
  Given a DiffView holding entries "e1" and "e2"
  When Clear() is called
  Then the view holds 0 entries
  And Active() returns nil

  Scenario: Concurrent push and read are safe
  Given a new DiffView
  When one goroutine pushes 100 entries while another reads Entries() and PendingHunks() 100 times
  Then no panic occurs

---

### Feature: Unified Diff Parsing

`ParseUnifiedDiff` is a conservative parser for a single-file unified diff. It understands
`@@ -a,b +c,d @@` hunk headers and ` `, `+`, `-` line prefixes (excluding `+++` / `---` file
headers). Other markers (e.g. `\ No newline at end of file`) are skipped silently. Old/new line
numbers track context and changed lines; missing header values default to 1. Hunk IDs are
generated as `<entryID>#h<seq>`.

@native-gui @diff-view

  Scenario: A simple single hunk parses lines and tracks line numbers
  Given the unified diff text:
    """
    @@ -1,3 +1,3 @@
     line1
    -line2
    +line2-modified
     line3
    """
  When parsed as entry "e1" for "x.go" (tracked, base "abc")
  Then the entry has 1 hunk
  And the hunk's OldStart and NewStart are both 1
  And the hunk has 4 lines with kinds [LineContext, LineRemoved, LineAdded, LineContext]
  And the added line text is "line2-modified"
  And the added line's NewLine number is 2

  Scenario: Multiple hunks each parse their own header start lines
  Given the unified diff text:
    """
    @@ -1,2 +1,2 @@
     a
    +b
    @@ -10,1 +11,1 @@
    -old
    +new
    """
  When parsed into an entry
  Then the entry has 2 hunks
  And the second hunk's OldStart is 10 and NewStart is 11

  Scenario Outline: Hunk header parsing defaults missing values to 1
  Given a hunk header "<header>"
  Then the parsed OldStart is <old> and NewStart is <new>

  Examples:
    | header           | old | new |
    | @@ -1,3 +1,3 @@   | 1   | 1   |
    | @@ -10,1 +11,1 @@ | 10  | 11  |

---

### Feature: Workflow DAG — Step Graph View-Model

`WorkflowDAG` is the view-model for the workflow DAG panel. It stores an ordered set of nodes and
their directed edges, tracks per-node execution status for the renderer, and records the
inspected node so a panel can show its inputs/outputs. The DAG is concurrency-safe (workflow
engine vs. UI event loop). A new DAG is empty and hidden.

@native-gui @workflow-dag

Background:
  Given the node statuses and their render appearance:
    | status              | ordinal | appearance        |
    | NodeStatusPending   | 0       | dimmed outline    |
    | NodeStatusRunning   | 1       | pulsing rounded   |
    | NodeStatusCompleted | 2       | solid             |
    | NodeStatusFailed    | 3       | solid red         |
    | NodeStatusSkipped   | 4       | muted             |

  Scenario: Adding a node stores its fields and defaults to pending
  Given a new WorkflowDAG
  When node "step-1" labeled "Fetch data" is added
  Then Node("step-1") returns that node
  And its Label is "Fetch data"
  And its Status is NodeStatusPending

  Scenario: Looking up a missing node returns nil
  Given a new WorkflowDAG
  Then Node("nonexistent") returns nil

  Scenario: Len counts the nodes in the DAG
  Given a new WorkflowDAG
  Then Len() is 0
  When nodes "a" and "b" are added
  Then Len() is 2

  Scenario: Nodes are returned in insertion order
  Given nodes "step-1", "step-2", "step-3" added in that order
  When Nodes() is called
  Then the returned nodes are ordered "step-1", "step-2", "step-3"

  Scenario: Adding an existing node ID replaces it but preserves order
  Given a WorkflowDAG with node "step-1" labeled "Old label"
  When node "step-1" is re-added labeled "New label" with status NodeStatusRunning
  Then Len() is still 1
  And Node("step-1").Label is "New label"
  And Node("step-1").Status is NodeStatusRunning

  Scenario Outline: UpdateStatus sets the render state of an existing node
  Given a WorkflowDAG with node "step-1"
  When the status is updated to <status>
  Then Node("step-1").Status is <status>

  Examples:
    | status              |
    | NodeStatusRunning   |
    | NodeStatusCompleted |
    | NodeStatusSkipped   |

  Scenario: UpdateStatus on a missing node is a no-op
  Given a new WorkflowDAG
  When UpdateStatus("nonexistent", NodeStatusRunning) is called
  Then no panic occurs

  Scenario: SetOutput on success records output and marks completed
  Given a WorkflowDAG with running node "step-1"
  When SetOutput("step-1", {"result":"ok"}, "") is called
  Then the node status is NodeStatusCompleted
  And the node Error is empty
  And the node Output map's "result" is "ok"

  Scenario: SetOutput on failure records the error and marks failed
  Given a WorkflowDAG with running node "step-1"
  When SetOutput("step-1", nil, "timeout after 30s") is called
  Then the node status is NodeStatusFailed
  And the node Error is "timeout after 30s"

  Scenario: SetOutput on a missing node is a no-op
  Given a new WorkflowDAG
  When SetOutput("nonexistent", "value", "") is called
  Then no panic occurs

  Scenario: Selecting a node opens it for inspection with its output
  Given a WorkflowDAG with node "step-1" whose output was set to "hello"
  Then SelectedNode() is nil before any selection
  When node "step-1" is selected
  Then SelectedNode().ID is "step-1"
  And SelectedNode().Output is "hello"

  Scenario: Selecting a missing node does not clear the current selection
  Given a WorkflowDAG with node "step-1" selected
  When SelectNode("nonexistent") is called
  Then the selection remains "step-1"

  Scenario: DeselectNode clears the inspection selection
  Given a WorkflowDAG with node "step-1" selected
  When DeselectNode() is called
  Then SelectedNode() is nil

  Scenario: ActiveNode returns the running node, or nil when none run
  Given a WorkflowDAG with nodes "step-1" and "step-2"
  Then ActiveNode() is nil when no node is running
  When "step-2" is set to NodeStatusRunning
  Then ActiveNode().ID is "step-2"

  Scenario: With parallel running nodes, ActiveNode returns the first in insertion order
  Given a WorkflowDAG with nodes "a" then "b"
  When both "a" and "b" are set to NodeStatusRunning
  Then ActiveNode().ID is "a"

  Scenario: Show, Hide, and Visible control panel activation
  Given a new WorkflowDAG that is not visible
  When Show() is called
  Then the panel is visible
  When Hide() is called
  Then the panel is not visible

  Scenario: Clear removes nodes, selection, and hides the panel
  Given a visible WorkflowDAG with nodes "step-1", "step-2" and "step-1" selected
  When Clear() is called
  Then Len() is 0
  And the panel is not visible
  And SelectedNode() is nil
  And ActiveNode() is nil

  Scenario: Nodes returns copies that cannot mutate internal state
  Given a WorkflowDAG with node "step-1" labeled "Step 1"
  When the caller mutates the Label of a node from Nodes()
  Then Node("step-1").Label is still "Step 1"

  Scenario: Edges are stored on the node for DAG edge rendering
  Given a node "step-1" with edges ["step-2", "step-3"]
  When the node is added
  Then Node("step-1").Edges is ["step-2", "step-3"]

  Scenario: An empty DAG returns a non-nil, zero-length node snapshot
  Given a new WorkflowDAG
  When Nodes() is called
  Then the result is non-nil
  And its length is 0

  Scenario: Full happy-path lifecycle transitions pending → running → completed
  Given a WorkflowDAG with node "step-1" labeled "Process"
  When the status is updated to NodeStatusRunning
  Then the node status is NodeStatusRunning
  When SetOutput("step-1", "done", "") is called
  Then the node status is NodeStatusCompleted

  Scenario: A conditional step can be marked skipped
  Given a WorkflowDAG with node "step-1" labeled "Conditional step"
  When the status is updated to NodeStatusSkipped
  Then the node status is NodeStatusSkipped

  Scenario: Concurrent mutation and reads are safe
  Given a new WorkflowDAG
  When one goroutine repeatedly adds, updates, and sets output for node "a"
  And another goroutine repeatedly reads Node, ActiveNode, and Nodes
  Then no panic occurs

---

### Feature: Session Tree Browser

`SessionTree` is the view-model for the session-history tree browser. It wraps an authoritative
`sessions.Tree` with the UI state the browser needs: per-session expand/collapse, a selected
turn, and a pending fork intent. It never mutates the underlying tree. Selection and expand
state are preserved by id across tree refreshes (the WebSocket push pump and the input handler
run on different goroutines).

@native-gui @session-tree

Background:
  Given the fork modes:
    | mode                | ordinal | semantics                                              |
    | ForkModeUnspecified | 0       | placeholder; substituted with the default at stage time |
    | ForkSnapshot        | 1       | copy parent state at the source turn; no later coupling |
    | ForkReplay          | 2       | re-execute turns 1..N to rebuild state deterministically |
    | ForkReference       | 3       | child references parent turns up to N (copy-on-write)  |
  And the default fork mode is ForkSnapshot (isolation by default)

  Scenario: A new session tree is empty by default
  Given a new SessionTree wrapping a nil tree
  Then Tree() returns nil
  And Selected() is the empty string
  And PendingFork() is nil

  Scenario: Sessions default to collapsed and toggle on each press
  Given a new SessionTree
  Then session "s1" is not expanded
  When session "s1" is toggled
  Then session "s1" is expanded
  When session "s1" is toggled again
  Then session "s1" is collapsed

  Scenario: Selecting a turn resolves to the underlying sessions.Turn
  Given a SessionTree wrapping a tree with one session and turn "turnA"
  Then SelectedTurn() is nil before any selection
  When turn "turnA" is selected
  Then Selected() returns "turnA"
  And SelectedTurn().ID is "turnA"
  When the selection is cleared with ""
  Then SelectedTurn() is nil

  Scenario: Staging a fork without a selected turn is refused
  Given a new SessionTree with no selected turn
  When StageFork(ForkSnapshot, true, "new-session") is called
  Then it returns false
  And PendingFork() is nil

  Scenario: Staging and committing a fork from the selected turn
  Given a SessionTree with turn "turnA" selected
  When StageFork(ForkSnapshot, true, "child-session") is called
  Then it returns true
  And the pending fork's SourceTurnID is "turnA"
  And its Mode is ForkSnapshot
  And its CopyMemory is true
  And its NewSessionID is "child-session"
  When CommitFork() is called
  Then PendingFork() is nil

  Scenario: Cancelling a fork drops the staged plan
  Given a SessionTree with turn "turnA" selected and a staged ForkReplay plan
  When CancelFork() is called
  Then PendingFork() is nil

  Scenario: Staging with ForkModeUnspecified falls back to the default mode
  Given a SessionTree with turn "turnA" selected
  When StageFork(ForkModeUnspecified, false, "x") is called
  Then the staged plan's Mode equals DefaultForkMode()

  Scenario: SetTree swaps the forest while preserving selection and expand state
  Given a SessionTree with a selection and expanded sessions
  When SetTree replaces the underlying forest with a refreshed tree
  Then the selection and expand state are preserved by id

---

### Feature: Memory Editor — SOUL.md / USER.md

`MemoryEditor` is the view-model for the SOUL.md / USER.md editor in the Memory tab. It buffers
an in-progress edit so the user can revise without writing to disk per keystroke, tracks a dirty
flag against the last-loaded baseline, and exposes Save/Revert hooks. The editor knows nothing
about disk layout — a `Persister` (owned by the memory package) handles Load/Save. One editor is
mounted per document. Reads are concurrency-safe; writes serialise on one mutex.

@native-gui @memory-editor

Background:
  Given the memory documents:
    | doc     | ordinal | path                |
    | DocSoul | 0       | ~/.conduit/SOUL.md    |
    | DocUser | 1       | ~/.conduit/USER.md    |

  Scenario: Reload hydrates the buffer from the persister and is clean
  Given a SOUL editor over a persister holding "# SOUL\n\ncuriosity > certainty\n"
  When Reload() is called
  Then the buffer equals the persister contents
  And the editor is not dirty

  Scenario: Editing the buffer marks the editor dirty
  Given a USER editor reloaded with baseline "alpha"
  When the buffer is set to "beta"
  Then the editor is dirty

  Scenario: Saving persists the buffer and adopts it as the new baseline
  Given a USER editor reloaded with baseline "alpha" and buffer set to "beta"
  When Save() is called
  Then the persister received exactly 1 save call
  And the persisted data is "beta"
  And the editor is no longer dirty

  Scenario: Saving a clean buffer is a no-op (no disk write)
  Given a USER editor reloaded with baseline "alpha" and no edits
  When Save() is called
  Then the persister received 0 save calls

  Scenario: Reverting drops the in-progress edit and restores the baseline
  Given a USER editor reloaded with baseline "alpha" and buffer set to "beta"
  When Revert() is called
  Then the buffer is "alpha"
  And the editor is not dirty

  Scenario: A load failure is reported and remembered
  Given a SOUL editor over a persister whose Load fails with "disk on fire"
  When Reload() is called
  Then Reload returns the "disk on fire" error
  And LoadError() remembers the failure

  Scenario: A save failure keeps the buffer dirty
  Given a USER editor reloaded with "alpha", buffer set to "beta", whose Save fails with "read-only fs"
  When Save() is called
  Then Save returns the "read-only fs" error
  And the editor remains dirty

  Scenario Outline: The cursor is clamped into the buffer range
  Given a SOUL editor reloaded with buffer "abcde"
  When the cursor is set to <input>
  Then the cursor is <clamped>

  Examples:
    | input | clamped | note                       |
    | -5    | 0       | negative clamps to 0         |
    | 99    | 5       | over-range clamps to len     |

  Scenario: Shrinking the buffer reclamps an out-of-range cursor
  Given a SOUL editor reloaded with buffer "abcde" and cursor at 5
  When the buffer is set to "ab"
  Then the cursor is reclamped to 2 (the new length)

  Scenario: Operating without a persister errors on Reload and Save
  Given a SOUL editor constructed with a nil persister
  When Reload() is called
  Then it returns an error
  When Save() is called
  Then it returns an error

---

### Feature: Native Spotlight Command Palette

`Spotlight` is the view-model for the Conduit Spotlight overlay, summoned with a global hotkey
(default ⌥Space). It renders a centered floating panel with a single input field; as the user
types, ranked results from sources (commands, workflows, memory, recents, sessions) appear
inline. Selecting runs inline or hands off. It is concurrency-safe. Defaults: hidden,
`maxRes = 10` results, `maxRec = 25` recents, cursor -1. Sources are polled synchronously on
every keystroke. The query is preserved across summons.

@native-gui @native-spotlight

Background:
  Given the spotlight result kinds:
    | kind         | ordinal | dispatch                              |
    | KindCommand  | 0       | built-in slash command (run inline)     |
    | KindWorkflow | 1       | named workflow (hand off to GUI/TUI)    |
    | KindMemory   | 2       | memory entry (hand off to browser)      |
    | KindRecent   | 3       | recent command from history             |
    | KindSession  | 4       | resume a prior session                  |
  And the Score helper rules:
    | match type      | score |
    | empty query     | 1.0   |
    | exact match     | 100   |
    | prefix match    | 50    |
    | substring match | 10    |
    | no match        | 0     |

  Scenario: Spotlight is hidden by default
  Given a new Spotlight
  Then it is not visible

  Scenario: Toggle, Show, and Hide control visibility
  Given a new Spotlight
  When Toggle() is called
  Then it is visible
  When Toggle() is called again
  Then it is hidden
  When Show() is called
  Then it is visible
  When Hide() is called
  Then it is hidden

  Scenario: A query ranks matching results from registered sources
  Given a Spotlight with a workflow source containing "deploy", "test suite", "release notes"
  When the query is set to "test"
  Then exactly 1 result is returned
  And the result ID is "test"

  Scenario: A prefix match outranks a substring match
  Given a Spotlight with results "do a thing" (substring) and "thingy command" (prefix)
  When the query is set to "thing"
  Then 2 results are returned
  And the prefix-matching "thingy command" ranks first

  Scenario: The cursor moves with arrow keys and wraps at the ends
  Given a Spotlight whose query "alph" yields 3 results
  Then the cursor starts at 0
  When the cursor moves by +1
  Then the cursor is 1
  When the cursor moves by +5
  Then the cursor wraps to (1+5) mod 3 = 0
  When the cursor moves by -10
  Then the cursor remains within [0, 3)

  Scenario: Activating a result records it as recent and hides the overlay
  Given a visible Spotlight whose query "deploy" yields the result "deploy prod"
  When Activate() is called
  Then it returns the "deploy" result
  And the overlay is hidden
  And the recents list contains one entry for "deploy"

  Scenario: Activating with no selection returns nil
  Given a Spotlight with no results
  When Activate() is called
  Then it returns nil

  Scenario: Recents deduplicate by ID and an empty query surfaces them on top
  Given a Spotlight with sources "deploy" and "build"
  When "deploy" is activated, then "build", then "deploy" again
  Then the recents list has 2 entries (deduped)
  And the most-recent entry is "deploy"
  When the query is cleared to ""
  Then the first result is of kind KindRecent

  Scenario: Clearing the query preserves the recents list
  Given a Spotlight where "alpha" was activated, leaving 1 recent
  When the query is set to ""
  Then the recents list still has 1 entry

  Scenario: Results are capped at maxRes (10)
  Given a Spotlight whose source returns 30 rows all matching "alpha"
  When the query is set to "alpha"
  Then at most 10 results are returned

  Scenario: Concurrent query updates and reads are safe
  Given a Spotlight with one source
  When one goroutine repeatedly sets the query and moves the cursor
  And another repeatedly reads Results() and Visible()
  Then no panic occurs

  Scenario Outline: The Score helper ranks query/title matches
  Given a query "<query>" and a title "<title>"
  Then Score returns a value that is <relation>

  Examples:
    | query | title    | relation                                  |
    | (empty) | anything | greater than 0 (matches everything)      |
    | foo   | bar      | 0 (no match)                              |
    | foo   | foobar   | greater than Score("foo","barfoo")        |
    | foo   | foo      | greater than Score("foo","foobar")        |
```


---

## Native GUI — Developer Tooling Views

This document captures the acceptance scenarios for the native macOS GUI surface
modeled in Go (`package gui`). These view-models hold headless state and logic
for the production GUI's developer-tooling views. Every scenario below is grounded
in `internal/gui/*.go` and its matching `*_test.go` file. Exact constants, values,
sort orders, and edge cases are reproduced from the source and tests.

---

@native-gui @coding-tabs
### Feature: Coding-agent tab strip

  The coding-agent GUI panel renders a strip of workspace tabs in a fixed
  canonical order. The view-model (`CodingTabState`) tracks the active tab,
  per-tab badges (unread/pending counts), error flags, and user-hidden tabs.
  It does not touch backend services; the renderer decorates the strip from
  this state. The model is safe for concurrent use.

  Background:
    Given a new coding tab state

  Scenario: Default active tab is Tasks
    When the coding tab state is created
    Then the active tab is "TabTasks"

  Scenario Outline: Canonical render order and display titles
    Given the canonical tab order
    Then position <index> is "<tab>" with title "<title>"

    Examples:
      | index | tab                | title      |
      | 0     | TabTasks           | Tasks      |
      | 1     | TabPlan            | Plan       |
      | 2     | TabCodingMemory    | Memory     |
      | 3     | TabHistory         | History    |
      | 4     | TabBackground      | Background |
      | 5     | TabWorktree        | Worktree   |
      | 6     | TabCodingSkills    | Skills     |
      | 7     | TabAccounts        | Accounts   |
      | 8     | TabRemote          | Remote     |
      | 9     | TabMCP             | MCP        |
      | 10    | TabPlugins         | Plugins    |
      | 11    | TabAskQueue        | Ask queue  |
      | 12    | TabCodingWorkflows | Workflows  |
      | 13    | TabSearch          | Search     |
      | 14    | TabTriggers        | Triggers   |
      | 15    | TabTeams           | Teams      |
      | 16    | TabDiagnostics     | Diagnostics|

  Scenario: Every tab in the canonical order has a non-placeholder title
    Given the canonical tab order
    Then no tab title equals "?"

  Scenario: An unknown tab value yields a placeholder title
    Given a tab value that is not registered
    When its title is requested
    Then the title is "?"

  Scenario: AllCodingTabs returns a safe-to-mutate copy
    When the canonical tab order is requested
    Then the returned slice is a copy that can be mutated without affecting the registry

  Scenario: Activating a tab makes it active
    When the tab "TabPlan" is activated
    Then the active tab is "TabPlan"

  Scenario: Next from the last tab wraps to the first
    Given the active tab is the last tab in the canonical order
    When the next tab is selected
    Then the active tab is the first tab in the canonical order

  Scenario: Prev from the first tab wraps to the last
    Given the active tab is the first tab in the canonical order
    When the previous tab is selected
    Then the active tab is the last tab in the canonical order

  Scenario: Next navigation skips hidden tabs
    Given the active tab is "TabTasks"
    And the tab "TabPlan" is hidden
    When the next tab is selected
    Then the active tab is "TabCodingMemory"

  Scenario: Hiding the active tab moves focus away from it
    Given the active tab is "TabPlan"
    When the tab "TabPlan" is hidden
    Then the active tab is no longer "TabPlan"

  Scenario: Activating a hidden tab is a no-op
    Given the tab "TabPlan" is hidden
    When the tab "TabPlan" is activated
    Then the active tab is not "TabPlan"

  Scenario: Unhiding restores a previously hidden tab to the strip
    Given the tab "TabPlan" is hidden
    When the tab "TabPlan" is unhidden
    Then "TabPlan" is no longer reported as hidden
    And "TabPlan" appears in the visible tabs

  Scenario Outline: Setting and clearing a badge count
    When the badge for "TabAskQueue" is set to <input>
    Then the badge for "TabAskQueue" reads <result>

    Examples:
      | input | result |
      | 3     | 3      |
      | 0     | 0      |
      | -5    | 0      |

  Scenario: A non-positive badge value clears the badge
    Given the badge for "TabAskQueue" is set to 3
    When the badge for "TabAskQueue" is set to 0
    Then the badge for "TabAskQueue" reads 0

  Scenario Outline: Setting and clearing the error flag
    When the error flag for "TabMCP" is set to <flag>
    Then the tab "TabMCP" errored state is <errored>

    Examples:
      | flag  | errored |
      | true  | true    |
      | false | false   |

  Scenario: Visible tabs exclude hidden tabs
    Given the tab "TabRemote" is hidden
    And the tab "TabTeams" is hidden
    When the visible tabs are listed
    Then neither "TabRemote" nor "TabTeams" appears
    And the visible tab count equals the canonical count minus 2

  Scenario: Visible tabs preserve canonical order
    Given some tabs are hidden
    When the visible tabs are listed
    Then the remaining tabs appear in canonical render order

  Scenario: Concurrent badge updates and navigation are race-free
    Given a goroutine sets the badge for "TabHistory" 200 times
    And the UI thread advances and reads the active tab 200 times
    Then no data race occurs and the operations complete

---

@native-gui @mini-ide
### Feature: Embedded Mini IDE

  The Mini IDE view-model (`MiniIDE`, PRD §11.3) owns the state the native
  frontend needs for file browsing, editor tabs, inline AI suggestions,
  terminal splits, and external-editor launch commands. It is scoped to a
  project root and rejects paths that escape that root. It does not run shell
  or editor processes; platform adapters consume the launch specs. The model
  is safe for concurrent use.

  Background:
    Given a Mini IDE rooted at a temporary project directory

  Scenario: Creating a Mini IDE requires an existing directory root
    When a Mini IDE is created with a root that is not a directory
    Then construction returns an error "mini ide root ... is not a directory"

  Scenario: A new Mini IDE seeds a default terminal pane
    When the Mini IDE is created
    Then a terminal with id "terminal-1" titled "Shell" exists
    And its working directory is the project root
    And its shell is taken from $SHELL or "/bin/sh" when $SHELL is empty
    And the terminal is marked active

  Scenario: A new Mini IDE seeds the default external editors
    When the Mini IDE is created
    Then the configured external editors are "VS Code", "Obsidian", and "Xcode"

  Scenario Outline: Default external editor definitions
    Given the default external editors
    Then editor "<name>" runs command "<command>" for file types "<types>"

    Examples:
      | name     | command | types                            |
      | VS Code  | code    | *                                |
      | Obsidian | open    | .md, .markdown                   |
      | Xcode    | open    | .swift, .xcodeproj, .xcworkspace |

  Scenario: Opening a file creates an active tab and shows the IDE
    Given a file "internal/app.go" containing Go source
    When the file is opened
    Then a tab exists with the absolute path of the file
    And the tab language is "go"
    And the tab SoftWrap defaults to true
    And the tab FontSize equals the default editor font size of 13
    And the tab CursorLine is 1 and CursorColumn is 1
    And the Mini IDE becomes visible
    And the active tab is the opened file

  Scenario: Opening a large file enables the minimap by default
    Given a file whose size is at least 65536 bytes
    When the file is opened
    Then the tab MinimapEnabled defaults to true

  Scenario: Opening a small file leaves the minimap disabled by default
    Given a file whose size is less than 65536 bytes
    When the file is opened
    Then the tab MinimapEnabled defaults to false

  Scenario: Re-opening an already-open file preserves per-tab options
    Given a file is open with adjusted cursor, soft-wrap, font size, and minimap settings
    When the same file is opened again
    Then the existing cursor, soft-wrap, font size, and minimap settings are preserved
    And no duplicate tab is appended to the tab order

  Scenario: Opening a directory as a file is rejected
    When a directory path is opened as a file
    Then an error "cannot open directory ... as file" is returned

  Scenario: Updating the buffer marks the tab dirty
    Given a file "README.md" is open
    When the buffer is updated to new content
    Then the active tab is dirty
    And the tab content equals the new content

  Scenario: Updating a file that is not open returns an error
    When the buffer is updated for a path that has no open tab
    Then an error "file ... is not open" is returned

  Scenario: Saving writes the buffer to disk and clears the dirty bit
    Given a file is open and its buffer has been changed to "# New\n"
    When the file is saved
    Then the on-disk content equals "# New\n"
    And the active tab is no longer dirty

  Scenario: Saving a file that is not open returns an error
    When a path with no open tab is saved
    Then an error "file ... is not open" is returned

  Scenario Outline: Moving the cursor enforces 1-based coordinates
    Given a file is open
    When the cursor is moved to line <line> column <col>
    Then the result is <outcome>

    Examples:
      | line | col | outcome                                        |
      | 5    | 3   | the cursor updates to line 5 column 3          |
      | 0    | 1   | an error "cursor line and column are 1-based"  |
      | 1    | 0   | an error "cursor line and column are 1-based"  |

  Scenario: Moving the cursor in a file that is not open returns an error
    When the cursor is moved in a path with no open tab
    Then an error "file ... is not open" is returned

  Scenario Outline: Font size must be within the supported range 9..32
    Given a file is open
    When the font size is set to <size>
    Then the result is <outcome>

    Examples:
      | size | outcome                                             |
      | 16   | the tab font size updates to 16                     |
      | 9    | the tab font size updates to 9                      |
      | 32   | the tab font size updates to 32                     |
      | 8    | an error "font size 8 outside supported range 9..32"|
      | 33   | an error "font size 33 outside supported range 9..32"|

  Scenario: Toggling soft wrap updates the open tab
    Given a file is open with SoftWrap true
    When soft wrap is set to false
    Then the tab SoftWrap is false

  Scenario: Toggling the minimap updates the open tab
    Given a file is open
    When the minimap is set to true
    Then the tab MinimapEnabled is true

  Scenario: Tab option changes on a file that is not open return an error
    When soft wrap, font size, or minimap is set for a path with no open tab
    Then an error "file ... is not open" is returned

  Scenario: ActiveTab returns nil when no file is open
    Given no file has been opened
    When the active tab is requested
    Then the result is nil

  Scenario: Closing a tab that is not open returns an error
    When a path with no open tab is closed
    Then an error "file ... is not open" is returned

  Scenario: Closing the active tab moves focus to the last remaining tab
    Given two files are open and the second is active
    When the active tab is closed
    Then the active tab becomes the last tab remaining in tab order

  Scenario: Closing the last open tab leaves no active tab
    Given exactly one file is open and active
    When that tab is closed
    Then there is no active tab

  Scenario: Paths outside the project root are rejected
    Given a file outside the project root
    When that absolute path is opened
    Then an error is returned
    And updating a buffer for a "../outside.go" relative path also errors

  Scenario: Relative paths resolve against the project root
    Given a file "README.md" inside the root
    When the file is opened by its relative path "README.md"
    Then the tab is created for the absolute path under the root

  Scenario Outline: Language detection by file extension
    When the language is detected for a "<ext>" file
    Then the language id is "<lang>"

    Examples:
      | ext       | lang       |
      | .go       | go         |
      | .js       | javascript |
      | .jsx      | javascript |
      | .mjs      | javascript |
      | .cjs      | javascript |
      | .ts       | typescript |
      | .tsx      | typescript |
      | .py       | python     |
      | .rs       | rust       |
      | .swift    | swift      |
      | .java     | java       |
      | .c        | c          |
      | .h        | c          |
      | .cc       | cpp        |
      | .cpp      | cpp        |
      | .cxx      | cpp        |
      | .hpp      | cpp        |
      | .rb       | ruby       |
      | .md       | markdown   |
      | .markdown | markdown   |
      | .json     | json       |
      | .yaml     | yaml       |
      | .yml      | yaml       |
      | .html     | html       |
      | .htm      | html       |
      | .css      | css        |
      | .sh       | shell      |
      | .bash     | shell      |
      | .zsh      | shell      |
      | .unknown  | plaintext  |

  Scenario: File tree is sorted directories-first then alphabetically and is language-aware
    Given files "zeta.go", "cmd/main.ts", "alpha.md", and ".hidden/secret.go"
    When the file tree is built with maxDepth 3 and maxEntries 20
    Then the top-level children are ["cmd", "alpha.md", "zeta.go"] in that order
    And the dotfile-prefixed ".hidden" directory is excluded
    And the language of "cmd/main.ts" is "typescript"

  Scenario: File tree hides dotfiles and dot-directories
    Given a hidden entry whose name begins with "."
    When the file tree is built
    Then the hidden entry does not appear in the tree

  Scenario: File tree depth and entry caps protect against huge worktrees
    Given maxDepth and maxEntries are positive
    When the file tree is built
    Then traversal stops once maxEntries is reached
    And directories beyond maxDepth are not expanded

  Scenario: Completions merge language keywords and in-buffer symbols, deduped and sorted
    Given an open Go file containing "func renderPanel()" and "var retryCount int"
    When completions are requested for prefix "re" with limit 10
    Then the completions are ["renderPanel", "retryCount", "return"] in sorted order

  Scenario: Completions limit caps the result count
    Given many candidates match a prefix
    When completions are requested with a positive limit
    Then no more than that many completions are returned

  Scenario: Completions for a file that is not open returns an error
    When completions are requested for a path with no open tab
    Then an error "file ... is not open" is returned

  Scenario: Adding a suggestion requires an ID
    When a suggestion with an empty ID is added
    Then an error "suggestion ID is required" is returned

  Scenario Outline: Suggestion line range validation
    When a suggestion is added with StartLine <start> and EndLine <end>
    Then the result is <outcome>

    Examples:
      | start | end | outcome                                       |
      | 1     | 1   | the suggestion is stored as pending           |
      | 1     | 3   | the suggestion is stored as pending           |
      | 0     | 1   | an error "suggestion line range is invalid"   |
      | 2     | 1   | an error "suggestion line range is invalid"   |

  Scenario: A newly added suggestion starts in the pending state
    When a valid suggestion is added
    Then its status is "SuggestionPending"

  Scenario Outline: Resolving a suggestion sets its terminal status
    Given a pending suggestion "s1" exists
    When the suggestion is resolved with status "<status>"
    Then the result is <outcome>

    Examples:
      | status             | outcome                                                    |
      | SuggestionAccepted | the suggestion status becomes accepted                     |
      | SuggestionRejected | the suggestion status becomes rejected                     |
      | SuggestionPending  | an error "resolved suggestion status must be accepted or rejected" |

  Scenario: Resolving an unknown suggestion returns an error
    When a suggestion id that does not exist is resolved
    Then an error "suggestion ... not found" is returned

  Scenario: Suggestions for a file are returned sorted by ID
    Given multiple suggestions exist for one file
    When suggestions for that file are requested
    Then they are returned sorted ascending by ID

  Scenario: Adding a terminal makes it active and deactivates others
    Given the default terminal exists
    When a terminal "test" titled "Tests" with shell "/bin/zsh" is added
    Then two terminals exist
    And the new terminal "test" is active
    And the previously active terminals are deactivated

  Scenario: Adding a terminal requires a non-empty id
    When a terminal with an empty id is added
    Then an error "terminal ID is required" is returned

  Scenario: Adding a terminal whose cwd is not a directory is rejected
    When a terminal is added with a cwd that is not a directory
    Then an error "terminal cwd ... is not a directory" is returned

  Scenario: A terminal with an empty shell falls back to $SHELL then /bin/sh
    When a terminal is added with an empty shell
    Then its shell is $SHELL, or "/bin/sh" when $SHELL is empty

  Scenario: Appending terminal output accumulates lines in order
    Given a terminal "test" exists
    When the lines "$ go test ./..." and "ok ./internal/gui" are appended
    Then the terminal lines are ["$ go test ./...", "ok ./internal/gui"] in order

  Scenario: Appending output to an unknown terminal returns an error
    When output is appended to a terminal id that does not exist
    Then an error "terminal ... not found" is returned

  Scenario: Collapsing a terminal updates its collapsed flag
    Given the default terminal "terminal-1" exists
    When it is set collapsed
    Then the default terminal is collapsed

  Scenario: Collapsing an unknown terminal returns an error
    When an unknown terminal id is set collapsed
    Then an error "terminal ... not found" is returned

  Scenario: Terminals are returned in insertion order with copied line slices
    Given the default terminal is collapsed and a second active terminal exists with output
    When the terminals are listed
    Then the default terminal is first and collapsed
    And the second terminal is active and carries its appended lines

  Scenario: External launches filter by file type and substitute the path
    Given a markdown file "notes/todo.md" and a Go file "main.go"
    When external launches are computed for the markdown file
    Then the launch names are ["VS Code", "Obsidian"]
    And the Obsidian arguments contain the substituted absolute file path
    When external launches are computed for the Go file
    Then the only launch is "VS Code"

  Scenario: A wildcard file type matches every file
    Given an editor configured with file type "*"
    When external launches are computed for any file
    Then that editor is included

  Scenario: SetExternalEditors replaces the configured editors
    When a custom list of external editors is set
    Then external launches are computed from the custom list

  Scenario: Show and Hide toggle visibility
    Given the Mini IDE is hidden
    When the Mini IDE is shown
    Then it reports visible
    When the Mini IDE is hidden
    Then it reports not visible

---

@native-gui @usage-dashboard
### Feature: Usage dashboard metrics

  The usage dashboard view-model (`UsageDashboard`, issue #116, PRD §14.3)
  ingests per-request `UsageSample` records and aggregates them on demand into
  panels: cost overview, cost by model/feature, request volume, latency
  percentiles, error rate, token usage, model comparison, plugin usage, and
  budget status. A preset time range and multi-dimensional filter scope each
  query. It is safe for concurrent use.

  Background:
    Given a new usage dashboard

  Scenario: A new dashboard defaults to the Last 7 days range
    When the dashboard is created
    Then the active range is "RangeLast7d"
    And no budget is set

  Scenario: Cost overview sums only filtered samples
    Given a sample at now costing 0.50 for "opus"
    And a sample 2 days ago costing 0.10 for "sonnet"
    When the filter From is set to 24 hours ago
    And the Cost Overview panel is computed
    Then the panel Total is 0.50

  Scenario: Cost by model is grouped as provider/model and sorted by value descending
    Given a sample for "sonnet" costing 0.10
    And a sample for "opus" costing 1.00
    And another sample for "opus" costing 0.50
    When the Cost By Model panel is computed
    Then there are 2 series points
    And the top series label is "anthropic/opus" with value 1.50

  Scenario: Cost by feature groups samples by feature label
    Given samples carrying feature labels
    When the Cost By Feature panel is computed
    Then series are grouped by feature and sorted by value descending

  Scenario: Latency percentiles compute p50, p95, and p99
    Given 100 samples with latencies 1..100 ms
    When the Latency Percentiles panel is computed
    Then P50 is between 40 and 60
    And P95 is between 90 and 100
    And P99 is at least 95

  Scenario: Error rate is errored calls over total calls
    Given 9 successful samples and 1 errored sample, each with 1 call
    When the Error Rate panel is computed
    Then the panel Total is approximately 0.10

  Scenario: Error rate is zero when there are no calls
    Given no samples match the filter
    When the Error Rate panel is computed
    Then the panel Total is 0

  Scenario: Request volume buckets calls into UTC days
    Given two samples on 2026-01-01 and one sample on 2026-01-02
    When the Request Volume panel is computed
    Then there are 2 time buckets sorted ascending by day
    And the first day's value is 2 calls

  Scenario: Token usage buckets tokens into UTC days
    Given samples spanning multiple UTC days
    When the Token Usage panel is computed
    Then tokens are summed per UTC day and sorted ascending

  Scenario: Model comparison aggregates per provider and model and sorts by cost descending
    Given a sample for "opus" costing 1.00 at 200 ms
    And a sample for "sonnet" costing 0.20 at 80 ms
    And an errored sample for "opus" costing 0.50 at 100 ms
    When the Model Comparison panel is computed
    Then there are 2 rows
    And the top row model is "opus" with 2 calls
    And the opus row error percentage is approximately 0.50

  Scenario: Model comparison computes call-weighted average latency
    Given several samples for one model with differing latencies and call counts
    When the Model Comparison panel is computed
    Then the AvgMS is the call-weighted mean latency

  Scenario: Plugin usage groups unattributed cost under "(none)" and sorts by cost descending
    Given a sample with no plugin costing 1.00
    And a sample with plugin "github" costing 2.00
    When the Plugin Usage panel is computed
    Then there are 2 series points
    And the top plugin label is "github"
    And the unattributed cost is labeled "(none)"

  Scenario: Budget status reports spend and carries the configured budget
    Given the monthly budget is set to 100.0
    And a sample costing 30.0 and a sample costing 15.0
    When the Budget Status panel is computed
    Then the panel Total spent is 45.0
    And the panel carries the budget 100.0 in the P50 field

  Scenario: Setting a budget of zero disables the budget panel
    When the budget is set to 0
    Then the budget panel reports no budget

  Scenario Outline: Preset ranges populate or clear the filter bounds
    When the range is set to "<range>"
    Then the filter From is <from> and To is <to>

    Examples:
      | range            | from        | to          |
      | RangeLast24h     | now-24h     | now         |
      | RangeLast7d      | now-7d      | now         |
      | RangeLast30d     | now-30d     | now         |
      | RangeMonthToDate | month start | now         |
      | RangeAllTime     | zero        | zero        |

  Scenario: Custom range does not auto-populate the filter bounds
    When the range is set to "RangeCustom"
    Then the filter From and To are left for the caller to set directly

  Scenario: Drill-down returns the raw samples matching the active filter
    Given a sample for "opus" and a sample for "sonnet"
    When the filter Model is set to "opus"
    And drill-down is requested
    Then exactly one row is returned and its model is "opus"

  Scenario Outline: Filter dimensions exclude non-matching samples
    Given samples differing by provider, model, feature, and plugin
    When the filter "<dimension>" is set to "<value>"
    Then only samples matching that dimension are aggregated

    Examples:
      | dimension | value     |
      | Provider  | anthropic |
      | Model     | opus      |
      | Feature   | chat      |
      | Plugin    | github    |

  Scenario: Empty filter fields match anything
    Given a filter with empty Provider, Model, Feature, and Plugin
    When any panel is computed
    Then all samples within the time bounds are included

  Scenario: Concurrent ingest and panel queries are race-free
    Given a goroutine ingests 200 samples
    And the UI thread computes the Cost Overview panel 200 times
    Then no data race occurs and the operations complete

---

@native-gui @evals
### Feature: Evals results view

  The Evals view-model (`EvalsView`, PRD §6.23 + §11.2) accepts a stream of
  `eval.CaseResult` records and produces per-model scorecards (pass rate,
  average cost, average latency, total cost), per-model per-day trend points,
  and the observed suite/model lists. Suite and model filters scope the
  rollups. Only derived rollups are retained, keeping memory bounded.

  Background:
    Given a new evals view

  Scenario: A new evals view is empty
    When the view is created
    Then there are no scorecards
    And there are no observed models

  Scenario: Scorecards aggregate cases by model and sort by model name
    Given results for "claude-opus-4-7" passing, passing, and failing
    And one passing result for "claude-sonnet-4-6"
    When the scorecards are read
    Then there are 2 scorecards
    And the first scorecard model is "claude-opus-4-7" (sorted ascending)
    And the opus card shows 3 cases and 2 passed
    And the opus pass rate is 2/3
    And the opus average cost is (0.10 + 0.20 + 0.05) / 3
    And the opus total cost is approximately 0.35

  Scenario Outline: NeedsAttention flags any model below the 0.7 pass-rate threshold
    Given a model with <passed> of <total> cases passing
    When the scorecards are read
    Then the model NeedsAttention flag is <flag>

    Examples:
      | passed | total | flag  |
      | 1      | 3     | true  |
      | 4      | 4     | false |

  Scenario: Pass rate, average cost, and average latency are zero for zero cases
    Given a model bucket with no cases
    When its scorecard is computed
    Then PassRate, AvgCostUSD, and AvgLatencySecs are all 0

  Scenario: Suite filter restricts the aggregation
    Given a "smoke" passing result and a "regression" failing result for model "m"
    When the suite filter is set to "smoke" and results are re-set
    Then there is 1 scorecard
    And it shows 1 case and 1 passed

  Scenario: An empty suite filter means all suites
    Given results across multiple suites
    When the suite filter is empty
    Then all suites contribute to the rollups

  Scenario: Trend points bucket results per day and carry pass percentage
    Given a passing result on 2026-05-04
    And a failing result on 2026-05-05 at 09:00
    And a passing result on 2026-05-05 at 18:00
    When the trend for model "m" is read
    Then there are 2 day buckets sorted ascending by timestamp
    And the first day's score is 100
    And the second day's score is 50
    And each trend point bucket duration is 24 hours

  Scenario: AllSuites and AllModels are sorted and de-duplicated
    Given results for suites "zeta" and "alpha" and models "model-z" and "model-a" with a duplicate model
    When the suites and models lists are read
    Then AllSuites is ["alpha", "zeta"]
    And AllModels is ["model-a", "model-z"] with duplicates removed

  Scenario: The model filter restricts the scorecards returned
    Given scorecards for "model-a" and "model-b"
    When the model filter is set to "model-a"
    Then only the "model-a" scorecard is returned
    When the model filter is cleared
    Then both scorecards are returned again

  Scenario: Filters are preserved across SetResults
    Given a suite filter is active
    When new results are set
    Then the suite and model filters still apply to the recomputed rollups

---

@native-gui @perf-comparison
### Feature: Performance comparison between models

  The performance comparison view-model (`PerfComparison`, issue #118,
  PRD §14.5) holds raw `PerfSample` observations and derives a side-by-side
  metric table plus a latency-vs-cost scatter plot. Metrics include median
  TTFT and total latency, tokens/sec, error rate, fallback rate, cost per 1K
  tokens, and the quality proxies (edit/retry/completion rate). Include/Exclude
  filters scope the comparison. It is safe for concurrent use.

  Background:
    Given a new performance comparison view

  Scenario: Rows group by provider and model, sorted by total latency ascending
    Given an "opus" sample at 500 ms and an "opus" sample at 1000 ms
    And a "sonnet" sample at 200 ms
    When the rows are read
    Then there are 2 rows
    And the first row is "sonnet" (lowest median total latency 200 vs opus 750)

  Scenario: Tokens per second is output tokens over total seconds
    Given a sample with 1000 output tokens in 1000 ms
    When the row is computed
    Then TokensPerSec is approximately 1000

  Scenario: Error rate and fallback rate are counts over sample count
    Given 8 clean samples, 1 errored sample, and 1 fallback sample for one model
    When the row is computed
    Then ErrorRate is approximately 0.10
    And FallbackRate is approximately 0.10

  Scenario: Cost per 1K tokens divides total cost by total tokens in thousands
    Given a sample of 2000 tokens at $0.04 and a sample of 3000 tokens at $0.06
    When the row is computed
    Then CostPer1KTokens is approximately 0.02

  Scenario: Quality proxies are averaged across samples
    Given a sample with EditRate 0.4, RetryRate 0.0, CompletionRate 1.0
    And a sample with EditRate 0.6, RetryRate 0.2, CompletionRate 0.8
    When the row is computed
    Then EditRate is approximately 0.5
    And RetryRate is approximately 0.1
    And CompletionRate is approximately 0.9

  Scenario: An include allowlist restricts the comparison to listed models
    Given samples for "a/x", "a/y", and "b/z"
    When "a/x" and "b/z" are included
    Then there are 2 rows
    And "y" is excluded by the allowlist

  Scenario: An empty allowlist means all models are compared
    Given samples for several models and no include entries
    When the rows are read
    Then every non-excluded model appears

  Scenario: Excluding a model hides it from the comparison
    Given samples for "a/x" and "a/y"
    When "a/y" is excluded
    Then there is 1 row and it is "x"

  Scenario: ClearFilters removes all include and exclude entries
    Given some include and exclude entries are set
    When the filters are cleared
    Then all models reappear in the comparison

  Scenario: Scatter returns one point per model ordered by latency ascending
    Given a "fast" sample at 100 ms costing 0.005 and a "slow" sample at 5000 ms costing 0.10
    When the scatter points are read
    Then there are 2 points
    And the first point is "fast" (lower latency)
    And the latencies are 100 and 5000 respectively

  Scenario Outline: Median handles empty, odd, and even inputs
    When the median of <input> is computed
    Then the result is <result>

    Examples:
      | input        | result |
      | (nil)        | 0      |
      | [5]          | 5      |
      | [1, 2, 3]    | 2      |
      | [1, 2, 3, 4] | 2.5    |

  Scenario: An empty sample group yields a zero-valued metric row
    Given a model group with no samples
    When the row is computed
    Then a zero-valued ModelMetrics row is returned

  Scenario: The quality-proxy disclaimer constant is non-empty
    Then the disclaimer reads "Quality signals are proxies derived from user behaviour, not ground-truth ratings."

  Scenario: Concurrent ingest and row queries are race-free
    Given a goroutine ingests 200 samples
    And the UI thread reads rows 200 times
    Then no data race occurs and the operations complete

---

@native-gui @worktree
### Feature: Git worktree tab

  The Worktree tab view-model (`WorktreeTab`) stores the latest coding-session
  worktree snapshot (`contracts.CodingWorktreeState`) and renders a compact
  status/history text body that native frontends can mirror without knowing
  git internals. State is copied defensively in and out. It is safe for
  concurrent use.

  Background:
    Given a new worktree tab

  Scenario: With no session attached the view shows an empty notice
    Given no coding session has been attached (empty SessionID)
    When the view is rendered
    Then the view reads "Worktree" then "No coding session attached."

  Scenario: An active worktree view includes status, path, cwd, and history
    Given a session "code-1" with cwd "/repo-wt", branch "main"
    And worktree active on branch "feature/x" at path "/repo-wt"
    And a history event at 12:00:00 UTC with action "enter" on branch "feature/x"
    When the view is rendered
    Then it contains "Worktree"
    And it contains "Session: code-1"
    And it contains "Status: active on feature/x"
    And it contains "Path: /repo-wt"
    And it contains "enter feature/x"

  Scenario: An inactive worktree reports the repository branch instead of a path
    Given a session whose worktree is not active
    When the view is rendered
    Then it shows "Status: repository on <branch>"
    And no worktree Path line is rendered

  Scenario Outline: Missing fields fall back to a dash
    Given an active worktree session with an empty "<field>"
    When the view is rendered
    Then that field is rendered as "-"

    Examples:
      | field          |
      | WorktreeBranch |
      | WorktreePath   |
      | Branch         |
      | CWD            |

  Scenario: An empty history renders "History: none"
    Given a session with no history events
    When the view is rendered
    Then the body ends with "History: none"

  Scenario: History events render with HH:MM:SS time, action, and branch-or-path
    Given a session with one or more history events
    When the view is rendered
    Then each event renders as "- <HH:MM:SS> <action> <branch-or-path>"
    And the branch is preferred, falling back to the path when the branch is empty

  Scenario: State returns a defensive copy of the history slice
    Given a session snapshot with history
    When the state is read
    Then the returned history is a copy that does not alias the stored slice

  Scenario: Update stores a defensive copy of the incoming history
    When a snapshot is stored via Update
    Then mutating the caller's original history slice does not affect the stored state

---

@native-gui @remote
### Feature: Remote connection lifecycle

  The remote connection view-model (`RemoteConnection`, PRD §11.2 + §17) owns
  the GUI ↔ core WebSocket lifecycle state and the reconnect timer math, but
  not the socket itself. The shell drives the state machine via Mark* methods.
  Backoff is computed by a pluggable `BackoffPolicy`; the default is jittered
  exponential capped at 30s. It is safe for concurrent use.

  Background:
    Given a remote connection bound to a URL with the default backoff policy

  Scenario: A new connection starts disconnected with zero attempts
    When the connection is created with url "ws://localhost:7777"
    Then the state is "disconnected"
    And the attempt count is 0
    And the URL is "ws://localhost:7777"

  Scenario: Happy-path lifecycle connecting then connected
    When MarkConnecting is called
    Then the state is "connecting"
    When MarkConnected is called at 2026-05-06 12:00:00 UTC
    Then the state is "connected"
    And LastUp equals that timestamp
    And the attempt count resets to 0

  Scenario: MarkConnecting clears the last error and pending attempt time
    Given a previous error and a scheduled next attempt
    When MarkConnecting is called
    Then the last error is cleared
    And the next-attempt time is zeroed

  Scenario: A dropped connection schedules a reconnect
    Given a connected connection using a no-jitter exponential backoff (base 100ms, cap 1s)
    When MarkDisconnected is called with error "conn reset" at 2026-05-06 12:00:00 UTC
    Then the state is "reconnecting"
    And the attempt count is 1
    And the returned delay is positive
    And the last error records the disconnect cause
    And NextAttemptAt equals the timestamp plus the delay

  Scenario: The state machine gives up after MaxAttempts
    Given an exponential backoff with MaxAttempts 2
    When MarkDisconnected is called the first time
    Then a positive retry delay is returned and the state is "reconnecting"
    When MarkDisconnected is called the second time
    Then the returned delay is 0
    And the state is "failed"

  Scenario: A user-initiated disconnect clears the reconnect timer
    Given a reconnect has been scheduled after a drop
    When Disconnect is called
    Then the state is "disconnected"
    And NextAttemptAt is zero
    And the attempt count resets to 0

  Scenario: Changing the URL resets the connection to disconnected
    Given the connection has begun connecting to "ws://a"
    When the URL is changed to "ws://b"
    Then the URL is "ws://b"
    And the state drops to "disconnected"
    And the attempt count and next-attempt time are reset

  Scenario: SetPing records the latest ping latency
    When a ping of N milliseconds is recorded
    Then PingMs returns N

  Scenario Outline: Connection state names render for the status bar
    When the state "<state>" is stringified
    Then the label is "<label>"

    Examples:
      | state             | label         |
      | StateDisconnected | disconnected  |
      | StateConnecting   | connecting    |
      | StateConnected    | connected     |
      | StateReconnecting | reconnecting  |
      | StateFailed       | failed        |

  Scenario: An unknown state renders as "unknown"
    When an unrecognized state value is stringified
    Then the label is "unknown"

  Scenario Outline: Exponential backoff grows by powers of two and caps
    Given a backoff with base 100ms and cap 800ms and no jitter
    When the delay for attempt <attempt> is computed
    Then the delay is <delay>

    Examples:
      | attempt | delay |
      | 0       | 100ms |
      | 1       | 200ms |
      | 2       | 400ms |
      | 3       | 800ms |
      | 10      | 800ms |

  Scenario: Jitter stays within bounds
    Given a backoff with base 100ms, cap 1s, and jitter 50ms
    When the delay for attempt 0 is computed repeatedly with a deterministic source
    Then every delay is within [100ms, 150ms)

  Scenario: A negative attempt is treated as attempt zero
    When the delay for a negative attempt is computed
    Then it equals the base-attempt-zero delay

  Scenario Outline: GiveUp respects MaxAttempts
    Given a backoff with MaxAttempts <max>
    When GiveUp is checked at attempt <attempt>
    Then the result is <result>

    Examples:
      | max | attempt | result |
      | 0   | 1000    | false  |
      | 3   | 2       | false  |
      | 3   | 3       | true   |

  Scenario: The default production backoff uses sensible values
    When a new exponential backoff is created
    Then the base is 500ms, the cap is 30s, the jitter is 250ms, and there is no attempt cap

  Scenario: ErrNoURL is defined for dial transitions requested before a URL is set
    Then the package exposes ErrNoURL with message "gui: no remote URL configured"


---
