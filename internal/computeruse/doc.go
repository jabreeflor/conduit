// Package computeruse holds the runtime hooks that gate Conduit's macOS
// computer-use capability (PRD §6.8).
//
// The package is self-contained: callers depend only on the exported types
// and functions here. Sibling computer-use modules (MCP runtime in #37,
// capability adapters in #40) integrate via the EnsureApproved hook below.
//
// Approvals are persisted to ~/.conduit/approved-apps.json so that revocations
// and grants survive across processes. The on-disk format is documented on
// ApprovalRecord and is intentionally human-readable: users may audit or
// hand-edit it just like the credentials/usage files.
package computeruse
