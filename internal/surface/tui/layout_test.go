package tui

import "testing"

func TestPanelWidths_contextHidden(t *testing.T) {
	convW, ctxW := panelWidths(120, false)
	if ctxW != 0 {
		t.Fatalf("expected ctxW=0 when context hidden, got %d", ctxW)
	}
	if convW != 118 { // 120 - 2 border chars
		t.Fatalf("expected convW=118, got %d", convW)
	}
}

func TestPanelWidths_contextVisible(t *testing.T) {
	convW, ctxW := panelWidths(120, true)
	if ctxW == 0 {
		t.Fatal("expected non-zero ctxW when context visible at 120 cols")
	}
	if convW+ctxW > 120 {
		t.Fatalf("panel widths %d+%d exceed terminal width 120", convW, ctxW)
	}
}

func TestPanelWidths_tooNarrow(t *testing.T) {
	// Terminal narrower than minContextWidth — context should auto-hide.
	convW, ctxW := panelWidths(40, true)
	if ctxW != 0 {
		t.Fatalf("expected ctxW=0 on narrow terminal (40 cols), got %d", ctxW)
	}
	if convW != 38 {
		t.Fatalf("expected convW=38 on narrow terminal, got %d", convW)
	}
}
