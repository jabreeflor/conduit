package importurl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDetectSource(t *testing.T) {
	cases := map[string]Source{
		"https://www.youtube.com/watch?v=abc": SourceYouTube,
		"https://youtu.be/abc":                SourceYouTube,
		"https://m.youtube.com/watch?v=abc":   SourceYouTube,
		"https://twitch.tv/some_channel":      SourceTwitch,
		"https://www.vimeo.com/12345":         SourceVimeo,
		"https://x.com/jack/status/1":         SourceTwitter,
		"https://twitter.com/jack/status/1":   SourceTwitter,
		"https://example.com/video":           SourceUnknown,
		"not a url":                           SourceUnknown,
	}
	for url, want := range cases {
		t.Run(url, func(t *testing.T) {
			if got := DetectSource(url); got != want {
				t.Errorf("DetectSource(%q) = %q, want %q", url, got, want)
			}
		})
	}
}

type fakeDL struct {
	meta        *Metadata
	probeErr    error
	downloadErr error
	captions    []Caption
	captionsErr error
	dlCalled    bool
	cpCalled    bool
}

func (f *fakeDL) Probe(_ context.Context, _ string) (*Metadata, error) {
	return f.meta, f.probeErr
}
func (f *fakeDL) Download(_ context.Context, _ string, _ Quality, _ string) error {
	f.dlCalled = true
	return f.downloadErr
}
func (f *fakeDL) FetchCaptions(_ context.Context, _ string, _ string) ([]Caption, error) {
	f.cpCalled = true
	return f.captions, f.captionsErr
}

type fakeTx struct {
	captions []Caption
	err      error
	called   bool
}

func (f *fakeTx) Transcribe(_ context.Context, _ string) ([]Caption, error) {
	f.called = true
	return f.captions, f.err
}

type fakeSink struct {
	called bool
	err    error
}

func (f *fakeSink) File(_ context.Context, _ Metadata, _ []Caption) error {
	f.called = true
	return f.err
}

func newImporter(t *testing.T, dl Downloader, tx Transcriber, sink TranscriptSink) *Importer {
	t.Helper()
	i, err := New(dl, tx, sink)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

func TestNew_NilDownloader(t *testing.T) {
	if _, err := New(nil, nil, nil); err == nil {
		t.Error("expected nil-downloader error")
	}
}

func TestImport_HappyPath_NativeCaptions(t *testing.T) {
	dl := &fakeDL{
		meta: &Metadata{Title: "X", Duration: time.Minute},
		captions: []Caption{
			{Start: 0, End: time.Second, Text: "hi"},
		},
	}
	imp := newImporter(t, dl, nil, nil)
	res, err := imp.Import(context.Background(), ImportOptions{URL: "https://youtu.be/abc", Dest: "/tmp/x.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if !dl.dlCalled || !dl.cpCalled {
		t.Errorf("download/captions not called: %+v", dl)
	}
	if !res.Metadata.HasNativeCaptions {
		t.Error("HasNativeCaptions should be true")
	}
	if res.UsedTranscriber {
		t.Error("transcriber should not be used")
	}
	if res.Metadata.Source != SourceYouTube {
		t.Errorf("source = %q, want youtube", res.Metadata.Source)
	}
}

func TestImport_FallbackToTranscriber(t *testing.T) {
	dl := &fakeDL{
		meta:        &Metadata{Title: "X"},
		captionsErr: errors.New("no captions"),
	}
	tx := &fakeTx{captions: []Caption{{Text: "whisper"}}}
	imp := newImporter(t, dl, tx, nil)
	res, err := imp.Import(context.Background(), ImportOptions{URL: "https://x.com/a/b", Dest: "/tmp/x.mp4"})
	if err != nil {
		t.Fatal(err)
	}
	if !tx.called {
		t.Error("transcriber not called")
	}
	if !res.UsedTranscriber || len(res.Captions) != 1 {
		t.Errorf("expected whisper captions: %+v", res)
	}
}

func TestImport_FilesToWiki(t *testing.T) {
	dl := &fakeDL{meta: &Metadata{}, captions: []Caption{{Text: "x"}}}
	sink := &fakeSink{}
	imp := newImporter(t, dl, nil, sink)
	_, err := imp.Import(context.Background(), ImportOptions{
		URL: "https://youtu.be/abc", Dest: "/tmp/x.mp4", FileToWiki: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sink.called {
		t.Error("sink not called")
	}
}

func TestImport_SkipsWikiWhenDisabled(t *testing.T) {
	dl := &fakeDL{meta: &Metadata{}, captions: []Caption{{Text: "x"}}}
	sink := &fakeSink{}
	imp := newImporter(t, dl, nil, sink)
	_, err := imp.Import(context.Background(), ImportOptions{
		URL: "https://youtu.be/abc", Dest: "/tmp/x.mp4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if sink.called {
		t.Error("sink should not have been called")
	}
}

func TestImport_Validations(t *testing.T) {
	imp := newImporter(t, &fakeDL{meta: &Metadata{}}, nil, nil)
	if _, err := imp.Import(context.Background(), ImportOptions{Dest: "/tmp/x"}); err == nil {
		t.Error("expected empty-url error")
	}
	if _, err := imp.Import(context.Background(), ImportOptions{URL: "https://x"}); err == nil {
		t.Error("expected missing-dest error")
	}
}

func TestImport_PropagatesErrors(t *testing.T) {
	cases := []struct {
		name string
		dl   *fakeDL
		want string
	}{
		{"probe", &fakeDL{probeErr: errors.New("boom")}, "probe"},
		{"download", &fakeDL{meta: &Metadata{}, downloadErr: errors.New("boom")}, "download"},
		{"nil meta", &fakeDL{}, "nil metadata"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			imp := newImporter(t, tc.dl, nil, nil)
			_, err := imp.Import(context.Background(), ImportOptions{URL: "https://x", Dest: "/tmp/x"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestImport_TranscribeError(t *testing.T) {
	dl := &fakeDL{meta: &Metadata{}, captionsErr: errors.New("none")}
	tx := &fakeTx{err: errors.New("whisper-broke")}
	imp := newImporter(t, dl, tx, nil)
	_, err := imp.Import(context.Background(), ImportOptions{URL: "https://x", Dest: "/tmp/x"})
	if err == nil || !strings.Contains(err.Error(), "transcribe") {
		t.Errorf("err = %v", err)
	}
}

func TestImport_WikiError(t *testing.T) {
	dl := &fakeDL{meta: &Metadata{}, captions: []Caption{{Text: "x"}}}
	sink := &fakeSink{err: errors.New("wiki-broke")}
	imp := newImporter(t, dl, nil, sink)
	_, err := imp.Import(context.Background(), ImportOptions{URL: "https://x", Dest: "/tmp/x", FileToWiki: true})
	if err == nil || !strings.Contains(err.Error(), "wiki") {
		t.Errorf("err = %v", err)
	}
}
