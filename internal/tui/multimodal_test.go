package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/contracts"
	"github.com/jabreeflor/conduit/internal/multimodal"
)

// newTestModel returns a minimal Model suitable for unit tests. It mirrors
// the baseline used by other tui package tests.
func newTestModel() Model {
	return newModel("claude-haiku-4-5", contracts.FirstRunSetupSnapshot{}, nil)
}

func TestConversationContentRendersAttachmentChips(t *testing.T) {
	m := newTestModel()
	m.messages = append(m.messages, message{
		role: roleUser,
		text: "describe this",
		attachments: []multimodal.Attachment{
			{Path: "/tmp/shot.png", MediaType: "image/png"},
		},
	})

	content := m.conversationContent()

	if !strings.Contains(content, "describe this") {
		t.Errorf("conversationContent missing user text: %q", content)
	}
	if !strings.Contains(content, "[image: shot.png]") {
		t.Errorf("conversationContent missing image chip: %q", content)
	}
}

func TestConversationContentRendersPDFChip(t *testing.T) {
	m := newTestModel()
	m.messages = append(m.messages, message{
		role: roleUser,
		text: "summarize",
		attachments: []multimodal.Attachment{
			{Path: "/home/user/report.pdf", MediaType: "application/pdf"},
		},
	})

	content := m.conversationContent()

	if !strings.Contains(content, "[pdf: report.pdf]") {
		t.Errorf("conversationContent missing pdf chip: %q", content)
	}
}

func TestConversationContentNoChipsOnPlainMessage(t *testing.T) {
	m := newTestModel()
	m.messages = append(m.messages, message{role: roleUser, text: "hello"})

	content := m.conversationContent()

	if strings.Contains(content, "[image:") || strings.Contains(content, "[pdf:") {
		t.Errorf("conversationContent has unexpected chips for plain message: %q", content)
	}
}

func TestSubmitHandlerParsesImageDirective(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(imgPath, []byte("fake-png"), 0o600); err != nil {
		t.Fatal(err)
	}

	m := newTestModel()
	// Plant the directive in the input buffer via direct field access (no tea.Program).
	m.input.SetValue("@image " + imgPath + " what is this?")

	// Simulate Submit by calling the same logic the Update handler runs.
	text := strings.TrimSpace(m.input.Value())
	cleanText, atts, err := parseSubmit(text)

	if err != nil {
		t.Fatalf("parseSubmit error: %v", err)
	}
	if cleanText != "what is this?" {
		t.Errorf("cleanText = %q, want %q", cleanText, "what is this?")
	}
	if len(atts) != 1 {
		t.Fatalf("attachments = %d, want 1", len(atts))
	}
	if atts[0].MediaType != "image/png" {
		t.Errorf("MediaType = %q, want image/png", atts[0].MediaType)
	}
}

func TestSubmitHandlerParsesPDFDirective(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}

	cleanText, atts, err := parseSubmit("@pdf " + pdfPath + " summarize key findings")
	if err != nil {
		t.Fatalf("parseSubmit error: %v", err)
	}
	if cleanText != "summarize key findings" {
		t.Errorf("cleanText = %q, want %q", cleanText, "summarize key findings")
	}
	if len(atts) != 1 || atts[0].MediaType != "application/pdf" {
		t.Errorf("unexpected attachments: %+v", atts)
	}
}

func TestSubmitHandlerReturnsErrorForMissingFile(t *testing.T) {
	_, _, err := parseSubmit("@image /nonexistent/missing.png describe")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestSubmitHandlerPlainTextNoAttachments(t *testing.T) {
	cleanText, atts, err := parseSubmit("just a normal message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cleanText != "just a normal message" {
		t.Errorf("cleanText = %q, want original", cleanText)
	}
	if len(atts) != 0 {
		t.Errorf("expected no attachments, got %d", len(atts))
	}
}
