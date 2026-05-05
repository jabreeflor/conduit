// Package importurl downloads videos from external platforms
// (YouTube, Twitch, Vimeo, Twitter/X) and surfaces them as first-class
// EDL clips with metadata, thumbnails, captions, and chapters.
//
// PRD §13.6. Imported videos integrate with the LLM Wiki for transcript
// filing — the host wires that side via a TranscriptSink.
//
// The actual download backend (yt-dlp / a Go yt-dlp port) is injected
// via the Downloader interface so the package stays unit-testable.
package importurl

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Source identifies the platform a URL belongs to.
type Source string

const (
	SourceYouTube Source = "youtube"
	SourceTwitch  Source = "twitch"
	SourceVimeo   Source = "vimeo"
	SourceTwitter Source = "twitter"
	SourceUnknown Source = "unknown"
)

// Quality is a target resolution selector.
type Quality string

const (
	QualityBest  Quality = "best"
	Quality4K    Quality = "2160p"
	Quality1440p Quality = "1440p"
	Quality1080p Quality = "1080p"
	Quality720p  Quality = "720p"
	Quality480p  Quality = "480p"
	QualityAudio Quality = "audio-only"
)

// Chapter is a named time segment in the source.
type Chapter struct {
	Title string
	Start time.Duration
	End   time.Duration
}

// Caption is a single timed line.
type Caption struct {
	Start, End time.Duration
	Text       string
}

// Metadata is everything we know about an imported video.
type Metadata struct {
	Source            Source
	URL               string
	ID                string
	Title             string
	Description       string
	Author            string
	Duration          time.Duration
	ThumbnailURL      string
	Chapters          []Chapter
	Captions          []Caption
	HasNativeCaptions bool
}

// DetectSource maps a URL to its platform.
func DetectSource(raw string) Source {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return SourceUnknown
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	switch {
	case host == "youtube.com" || host == "m.youtube.com" || host == "youtu.be" || strings.HasSuffix(host, ".youtube.com"):
		return SourceYouTube
	case host == "twitch.tv" || strings.HasSuffix(host, ".twitch.tv"):
		return SourceTwitch
	case host == "vimeo.com" || strings.HasSuffix(host, ".vimeo.com"):
		return SourceVimeo
	case host == "twitter.com" || host == "x.com" || strings.HasSuffix(host, ".twitter.com") || strings.HasSuffix(host, ".x.com"):
		return SourceTwitter
	}
	return SourceUnknown
}

// Downloader is the swappable backend that fetches metadata and media
// for a URL. Production wires yt-dlp; tests substitute a fake.
type Downloader interface {
	Probe(ctx context.Context, url string) (*Metadata, error)
	Download(ctx context.Context, url string, quality Quality, dest string) error
	FetchCaptions(ctx context.Context, url string, lang string) ([]Caption, error)
}

// Transcriber is invoked when no native captions exist. The host wires
// this to the existing Whisper.cpp integration.
type Transcriber interface {
	Transcribe(ctx context.Context, mediaPath string) ([]Caption, error)
}

// TranscriptSink lets the importer file the captions into the LLM Wiki.
// Implementations may be no-ops in tests.
type TranscriptSink interface {
	File(ctx context.Context, meta Metadata, captions []Caption) error
}

// ImportOptions controls a single import.
type ImportOptions struct {
	URL         string
	Quality     Quality
	Dest        string // local file destination
	CaptionLang string // empty -> autodetect
	FileToWiki  bool
}

// Result is what Import produces.
type Result struct {
	Metadata        Metadata
	MediaPath       string
	Captions        []Caption
	UsedTranscriber bool
}

// Importer composes a Downloader + Transcriber + TranscriptSink.
type Importer struct {
	dl   Downloader
	tx   Transcriber
	sink TranscriptSink
}

// New returns an Importer. The Downloader is required; transcriber and
// sink may be nil to skip those phases.
func New(dl Downloader, tx Transcriber, sink TranscriptSink) (*Importer, error) {
	if dl == nil {
		return nil, errors.New("importurl: nil downloader")
	}
	return &Importer{dl: dl, tx: tx, sink: sink}, nil
}

// Import runs probe -> download -> captions (native or whisper) -> wiki.
func (i *Importer) Import(ctx context.Context, opts ImportOptions) (*Result, error) {
	if strings.TrimSpace(opts.URL) == "" {
		return nil, errors.New("importurl: empty url")
	}
	if opts.Dest == "" {
		return nil, errors.New("importurl: missing dest")
	}
	q := opts.Quality
	if q == "" {
		q = QualityBest
	}
	meta, err := i.dl.Probe(ctx, opts.URL)
	if err != nil {
		return nil, fmt.Errorf("importurl: probe: %w", err)
	}
	if meta == nil {
		return nil, errors.New("importurl: probe returned nil metadata")
	}
	if meta.Source == "" {
		meta.Source = DetectSource(opts.URL)
	}
	if err := i.dl.Download(ctx, opts.URL, q, opts.Dest); err != nil {
		return nil, fmt.Errorf("importurl: download: %w", err)
	}

	captions := meta.Captions
	usedTx := false
	if len(captions) == 0 {
		c, err := i.dl.FetchCaptions(ctx, opts.URL, opts.CaptionLang)
		if err == nil && len(c) > 0 {
			captions = c
			meta.HasNativeCaptions = true
		} else if i.tx != nil {
			tc, err := i.tx.Transcribe(ctx, opts.Dest)
			if err != nil {
				return nil, fmt.Errorf("importurl: transcribe: %w", err)
			}
			captions = tc
			usedTx = true
		}
	}

	if opts.FileToWiki && i.sink != nil {
		if err := i.sink.File(ctx, *meta, captions); err != nil {
			return nil, fmt.Errorf("importurl: wiki file: %w", err)
		}
	}

	return &Result{
		Metadata:        *meta,
		MediaPath:       opts.Dest,
		Captions:        captions,
		UsedTranscriber: usedTx,
	}, nil
}
