// extension.ts — wires the Conduit commands into VS Code (and any
// VS Code-compatible editor: Cursor, Windsurf).
//
// What this scaffold provides:
//   * sidebar webview placeholder
//   * status-bar item showing current connection state
//   * `conduit.connect` command that probes /v1/healthz
//   * `conduit.shareFile` / `conduit.shareSelection` — POST file-path-only
//     context to the running daemon (no file *contents* leave the editor in
//     this scaffold; that's an intentional follow-up choice)
//
// Rich UI (chat panel, diff review, inline ghost text) is intentionally out
// of scope per #143's "scaffold only" guidance.

import * as vscode from 'vscode';
import { probe, ConnectionState } from './connect';

let connection: ConnectionState = { kind: 'disconnected' };
let statusItem: vscode.StatusBarItem;

export function activate(context: vscode.ExtensionContext): void {
  statusItem = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Right, 100);
  statusItem.command = 'conduit.connect';
  refreshStatus();
  statusItem.show();
  context.subscriptions.push(statusItem);

  context.subscriptions.push(
    vscode.commands.registerCommand('conduit.connect', () => connectCommand()),
    vscode.commands.registerCommand('conduit.openSidebar', () =>
      vscode.commands.executeCommand('workbench.view.extension.conduit'),
    ),
    vscode.commands.registerCommand('conduit.shareFile', () => shareFileCommand()),
    vscode.commands.registerCommand('conduit.shareSelection', () => shareSelectionCommand()),
  );

  context.subscriptions.push(
    vscode.window.registerWebviewViewProvider('conduit.sidebar', new SidebarProvider()),
  );

  // Auto-probe on activation, but don't block the editor.
  void connectCommand({ silent: true });
}

export function deactivate(): void {
  statusItem?.dispose();
}

// --- commands ---

interface ConnectOpts {
  silent?: boolean;
}

async function connectCommand(opts: ConnectOpts = {}): Promise<void> {
  const cfg = vscode.workspace.getConfiguration('conduit');
  const endpoint = cfg.get<string>('endpoint', 'http://127.0.0.1:8923');
  const timeoutMs = cfg.get<number>('timeoutMs', 2000);

  setConnection({ kind: 'connecting', endpoint });
  const result = await probe({ endpoint, timeoutMs });
  setConnection(result);

  if (opts.silent) return;
  switch (result.kind) {
    case 'connected':
      vscode.window.showInformationMessage(`Conduit connected (v${result.version}) at ${result.endpoint}`);
      break;
    case 'error':
      vscode.window.showWarningMessage(`Could not reach Conduit at ${result.endpoint}: ${result.message}`);
      break;
  }
}

async function shareFileCommand(): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  if (!editor) {
    vscode.window.showInformationMessage('Open a file first.');
    return;
  }
  await postContext({ kind: 'file', uri: editor.document.uri.toString() });
}

async function shareSelectionCommand(): Promise<void> {
  const editor = vscode.window.activeTextEditor;
  if (!editor || editor.selection.isEmpty) {
    vscode.window.showInformationMessage('Make a selection first.');
    return;
  }
  const sel = editor.selection;
  await postContext({
    kind: 'selection',
    uri: editor.document.uri.toString(),
    startLine: sel.start.line,
    endLine: sel.end.line,
  });
}

interface ContextRef {
  kind: 'file' | 'selection';
  uri: string;
  startLine?: number;
  endLine?: number;
}

async function postContext(ref: ContextRef): Promise<void> {
  if (connection.kind !== 'connected') {
    vscode.window.showWarningMessage('Conduit is not connected. Run "Conduit: Connect to running instance" first.');
    return;
  }
  try {
    const res = await fetch(`${connection.endpoint}/v1/context`, {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify(ref),
    });
    if (!res.ok) {
      vscode.window.showWarningMessage(`Conduit rejected context: HTTP ${res.status}`);
      return;
    }
    vscode.window.setStatusBarMessage(`Conduit: shared ${ref.kind}`, 2000);
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    vscode.window.showWarningMessage(`Conduit context share failed: ${message}`);
  }
}

// --- UI plumbing ---

function setConnection(state: ConnectionState): void {
  connection = state;
  refreshStatus();
}

function refreshStatus(): void {
  switch (connection.kind) {
    case 'disconnected':
      statusItem.text = '$(circle-slash) Conduit';
      statusItem.tooltip = 'Click to connect';
      break;
    case 'connecting':
      statusItem.text = '$(sync~spin) Conduit';
      statusItem.tooltip = `Connecting to ${connection.endpoint}…`;
      break;
    case 'connected':
      statusItem.text = `$(check) Conduit ${connection.version}`;
      statusItem.tooltip = `Connected to ${connection.endpoint}`;
      break;
    case 'error':
      statusItem.text = '$(warning) Conduit';
      statusItem.tooltip = `Error: ${connection.message}`;
      break;
  }
}

class SidebarProvider implements vscode.WebviewViewProvider {
  resolveWebviewView(view: vscode.WebviewView): void {
    view.webview.options = { enableScripts: false };
    view.webview.html = `<!doctype html>
<html><head><meta charset="utf-8"><title>Conduit</title></head>
<body style="font-family: system-ui; padding: 1rem;">
  <h2>Conduit</h2>
  <p>This panel will host the chat surface in a follow-up release.</p>
  <p>Use the command palette (Ctrl/Cmd+Shift+P → "Conduit: …") in the meantime.</p>
</body></html>`;
  }
}
