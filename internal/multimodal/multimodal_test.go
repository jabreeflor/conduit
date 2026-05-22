package multimodal

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jabreeflor/conduit/internal/router"
)

func TestParseAndLoadImageDirective(t *testing.T) {
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "shot.png")
	content := []byte("fake-png-bytes")
	if err := os.WriteFile(imgPath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	clean, atts, err := ParseAndLoad("@image " + imgPath + " describe this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean != "describe this" {
		t.Errorf("cleanText = %q, want %q", clean, "describe this")
	}
	if len(atts) != 1 {
		t.Fatalf("attachments = %d, want 1", len(atts))
	}
	if atts[0].MediaType != "image/png" {
		t.Errorf("MediaType = %q, want image/png", atts[0].MediaType)
	}
	want := base64.StdEncoding.EncodeToString(content)
	if atts[0].Data != want {
		t.Errorf("Data mismatch")
	}
}

func TestParseAndLoadPDFDirective(t *testing.T) {
	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "report.pdf")
	if err := os.WriteFile(pdfPath, []byte("%PDF-1.4"), 0o600); err != nil {
		t.Fatal(err)
	}

	clean, atts, err := ParseAndLoad("@pdf " + pdfPath + " summarize")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean != "summarize" {
		t.Errorf("cleanText = %q, want summarize", clean)
	}
	if len(atts) != 1 {
		t.Fatalf("attachments = %d, want 1", len(atts))
	}
	if atts[0].MediaType != "application/pdf" {
		t.Errorf("MediaType = %q, want application/pdf", atts[0].MediaType)
	}
}

func TestParseAndLoadMultipleImages(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.png", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	text := "@image " + filepath.Join(dir, "a.png") + " @image " + filepath.Join(dir, "b.jpg") + " describe differences"
	clean, atts, err := ParseAndLoad(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean != "describe differences" {
		t.Errorf("cleanText = %q, want %q", clean, "describe differences")
	}
	if len(atts) != 2 {
		t.Fatalf("attachments = %d, want 2", len(atts))
	}
	if atts[0].MediaType != "image/png" {
		t.Errorf("first MediaType = %q, want image/png", atts[0].MediaType)
	}
	if atts[1].MediaType != "image/jpeg" {
		t.Errorf("second MediaType = %q, want image/jpeg", atts[1].MediaType)
	}
}

func TestParseAndLoadNoDirectives(t *testing.T) {
	clean, atts, err := ParseAndLoad("just a plain message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean != "just a plain message" {
		t.Errorf("cleanText = %q, want original text", clean)
	}
	if len(atts) != 0 {
		t.Errorf("attachments = %d, want 0", len(atts))
	}
}

func TestParseAndLoadMissingPath(t *testing.T) {
	_, _, err := ParseAndLoad("@image")
	if err == nil {
		t.Fatal("expected error for missing path, got nil")
	}
}

func TestParseAndLoadFileNotFound(t *testing.T) {
	_, _, err := ParseAndLoad("@image /nonexistent/path.png")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestParseAndLoadUnsupportedImageExt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.bmp")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := ParseAndLoad("@image " + p)
	if err == nil {
		t.Fatal("expected error for unsupported extension, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported extension") {
		t.Errorf("error = %q, want 'unsupported extension'", err.Error())
	}
}

func TestParseAndLoadPDFWrongExt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := ParseAndLoad("@pdf " + p)
	if err == nil {
		t.Fatal("expected error for wrong extension, got nil")
	}
}

func TestAttachmentToInput(t *testing.T) {
	a := Attachment{Path: "/tmp/shot.png", MediaType: "image/png", Data: "abc123"}
	inp := a.ToInput()
	if inp.Type != router.InputImage {
		t.Errorf("Type = %q, want InputImage", inp.Type)
	}
	if inp.Ref != "/tmp/shot.png" {
		t.Errorf("Ref = %q, want /tmp/shot.png", inp.Ref)
	}
	if inp.Data != "abc123" {
		t.Errorf("Data = %q, want abc123", inp.Data)
	}
	if inp.MediaType != "image/png" {
		t.Errorf("MediaType = %q, want image/png", inp.MediaType)
	}
}

func TestPDFAttachmentToInput(t *testing.T) {
	a := Attachment{Path: "/tmp/doc.pdf", MediaType: "application/pdf", Data: "pdfdata"}
	inp := a.ToInput()
	if inp.Type != router.InputPDF {
		t.Errorf("Type = %q, want InputPDF", inp.Type)
	}
}

func TestAttachmentLabel(t *testing.T) {
	cases := []struct {
		a    Attachment
		want string
	}{
		{Attachment{Path: "/tmp/shot.png", MediaType: "image/png"}, "[image: shot.png]"},
		{Attachment{Path: "/home/user/docs/report.pdf", MediaType: "application/pdf"}, "[pdf: report.pdf]"},
		{Attachment{Path: "/tmp/photo.webp", MediaType: "image/webp"}, "[image: photo.webp]"},
	}
	for _, tc := range cases {
		got := AttachmentLabel(tc.a)
		if got != tc.want {
			t.Errorf("AttachmentLabel(%q) = %q, want %q", tc.a.Path, got, tc.want)
		}
	}
}
