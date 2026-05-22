// Package multimodal handles @image and @pdf directive parsing, file loading,
// and base64 encoding for the TUI input pipeline (PRD §6.22).
package multimodal

import (
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jabreeflor/conduit/internal/router"
)

// supportedImageExts is the set of image extensions accepted by @image.
var supportedImageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
}

// Attachment represents a file that has been parsed from a TUI directive,
// loaded from disk, and base64-encoded for transmission.
type Attachment struct {
	Path      string
	MediaType string
	Data      string // base64-encoded file content
}

// ToInput converts an Attachment to a router.Input for vision-aware routing.
func (a Attachment) ToInput() router.Input {
	inputType := router.InputImage
	if a.MediaType == "application/pdf" {
		inputType = router.InputPDF
	}
	return router.Input{
		Type:      inputType,
		Ref:       a.Path,
		Data:      a.Data,
		MediaType: a.MediaType,
	}
}

// ParseAndLoad extracts @image and @pdf directives from text, loads each
// referenced file, and returns the cleaned text with directives removed plus
// the loaded attachments.
//
// Directive syntax: @image <path> or @pdf <path> anywhere in the input.
// Multiple directives in one message are supported.
func ParseAndLoad(text string) (cleanText string, attachments []Attachment, err error) {
	tokens := strings.Fields(text)
	var kept []string

	for i := 0; i < len(tokens); i++ {
		switch tokens[i] {
		case "@image", "@pdf":
			if i+1 >= len(tokens) {
				return "", nil, fmt.Errorf("%s requires a file path argument", tokens[i])
			}
			i++ // consume path token
			path := tokens[i]
			att, loadErr := loadFile(tokens[i-1], path)
			if loadErr != nil {
				return "", nil, loadErr
			}
			attachments = append(attachments, att)
		default:
			kept = append(kept, tokens[i])
		}
	}

	return strings.Join(kept, " "), attachments, nil
}

// loadFile reads the file at path, validates its extension against the
// directive kind, and returns a base64-encoded Attachment.
func loadFile(directive, path string) (Attachment, error) {
	ext := strings.ToLower(filepath.Ext(path))

	var mediaType string
	switch directive {
	case "@image":
		mt, ok := supportedImageExts[ext]
		if !ok {
			return Attachment{}, fmt.Errorf(
				"@image: unsupported extension %q (supported: png, jpg, jpeg, gif, webp)", ext,
			)
		}
		mediaType = mt
	case "@pdf":
		if ext != ".pdf" {
			return Attachment{}, fmt.Errorf("@pdf: expected .pdf extension, got %q", ext)
		}
		mediaType = "application/pdf"
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Attachment{}, fmt.Errorf("%s: cannot read %q: %w", directive, path, err)
	}

	return Attachment{
		Path:      path,
		MediaType: mediaType,
		Data:      base64.StdEncoding.EncodeToString(data),
	}, nil
}

// AttachmentLabel returns a short display string for use in TUI chips,
// e.g. "[image: screenshot.png]" or "[pdf: report.pdf]".
func AttachmentLabel(a Attachment) string {
	base := filepath.Base(a.Path)
	if a.MediaType == "application/pdf" {
		return fmt.Sprintf("[pdf: %s]", base)
	}
	return fmt.Sprintf("[image: %s]", base)
}
