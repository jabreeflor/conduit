// Package export turns a video render request into a concrete pipeline
// invocation. Codec/container/resolution/bitrate/preset selection lives
// here so the editor stays renderer-agnostic.
//
// PRD §13.3. Formats: MP4 (H.264/H.265), WebM, MOV (ProRes), GIF, APNG,
// WebP. Resolutions 4K → square → vertical. Platform presets for
// Twitter, YouTube, Slack, LinkedIn, internal docs.
//
// Concrete renderer integration (FFmpeg, AVFoundation) is the
// integration TODO called out at the bottom of this file.
package export

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Container names a wrapper format.
type Container string

const (
	ContainerMP4  Container = "mp4"
	ContainerWebM Container = "webm"
	ContainerMOV  Container = "mov"
	ContainerGIF  Container = "gif"
	ContainerAPNG Container = "apng"
	ContainerWebP Container = "webp"
)

// Codec names a video codec.
type Codec string

const (
	CodecH264   Codec = "h264"
	CodecH265   Codec = "h265"
	CodecVP9    Codec = "vp9"
	CodecAV1    Codec = "av1"
	CodecProRes Codec = "prores"
	CodecGIF    Codec = "gif"
	CodecAPNG   Codec = "apng"
	CodecWebP   Codec = "webp"
)

// Resolution is a target output size.
type Resolution struct {
	Width, Height int
}

// Common resolutions.
var (
	Res4K       = Resolution{3840, 2160}
	Res1440p    = Resolution{2560, 1440}
	Res1080p    = Resolution{1920, 1080}
	Res720p     = Resolution{1280, 720}
	ResSquare   = Resolution{1080, 1080}
	ResVertical = Resolution{1080, 1920}
)

// Settings is the resolved render configuration.
type Settings struct {
	Container  Container
	Codec      Codec
	Resolution Resolution
	FPS        int
	BitrateK   int    // kilobits per second; 0 means use codec default
	CRF        int    // constant rate factor; 0 means use codec default
	Preset     string // ffmpeg preset name when applicable
	AudioKHz   int    // audio sample rate
	AudioKBps  int    // audio bitrate kilobits/s
}

// Preset is a named curated Settings tuned for a destination.
type Preset struct {
	Name        string
	Description string
	Settings    Settings
	Aux         AuxOutputs
}

// AuxOutputs flags companion deliverables a preset wants alongside the
// video itself.
type AuxOutputs struct {
	Thumbnail bool
	Chapters  bool
	SRT       bool
	VTT       bool
	BlogPost  bool
}

// Presets indexed by name.
var presets = map[string]Preset{
	"twitter": {
		Name:        "twitter",
		Description: "Twitter/X feed video, 1080x1080, 30fps, h264",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH264,
			Resolution: ResSquare, FPS: 30, BitrateK: 5000,
			AudioKHz: 48000, AudioKBps: 128,
		},
		Aux: AuxOutputs{Thumbnail: true, SRT: true},
	},
	"youtube": {
		Name:        "youtube",
		Description: "YouTube 1080p60 h264 8 Mbps with chapters & captions",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH264,
			Resolution: Res1080p, FPS: 60, BitrateK: 8000,
			AudioKHz: 48000, AudioKBps: 192,
		},
		Aux: AuxOutputs{Thumbnail: true, Chapters: true, SRT: true, VTT: true, BlogPost: true},
	},
	"youtube-4k": {
		Name:        "youtube-4k",
		Description: "YouTube 4K60 h265 35 Mbps",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH265,
			Resolution: Res4K, FPS: 60, BitrateK: 35000,
			AudioKHz: 48000, AudioKBps: 256,
		},
		Aux: AuxOutputs{Thumbnail: true, Chapters: true, SRT: true, VTT: true, BlogPost: true},
	},
	"shorts": {
		Name:        "shorts",
		Description: "Vertical 1080x1920 60fps for Shorts/TikTok/Reels",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH264,
			Resolution: ResVertical, FPS: 60, BitrateK: 8000,
			AudioKHz: 48000, AudioKBps: 192,
		},
		Aux: AuxOutputs{Thumbnail: true, SRT: true},
	},
	"slack": {
		Name:        "slack",
		Description: "Slack-friendly mp4, 720p, h264, small file",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH264,
			Resolution: Res720p, FPS: 30, BitrateK: 2500,
			AudioKHz: 44100, AudioKBps: 96,
		},
		Aux: AuxOutputs{Thumbnail: true},
	},
	"linkedin": {
		Name:        "linkedin",
		Description: "LinkedIn 1080p30 h264 with captions",
		Settings: Settings{
			Container: ContainerMP4, Codec: CodecH264,
			Resolution: Res1080p, FPS: 30, BitrateK: 5000,
			AudioKHz: 48000, AudioKBps: 128,
		},
		Aux: AuxOutputs{Thumbnail: true, SRT: true},
	},
	"docs": {
		Name:        "docs",
		Description: "Internal docs: WebM/VP9, 1080p, modest bitrate",
		Settings: Settings{
			Container: ContainerWebM, Codec: CodecVP9,
			Resolution: Res1080p, FPS: 30, BitrateK: 3500,
			AudioKHz: 48000, AudioKBps: 128,
		},
		Aux: AuxOutputs{VTT: true, Chapters: true},
	},
	"prores": {
		Name:        "prores",
		Description: "ProRes 422 master in MOV",
		Settings: Settings{
			Container: ContainerMOV, Codec: CodecProRes,
			Resolution: Res1080p, FPS: 30,
			AudioKHz: 48000, AudioKBps: 256,
		},
	},
	"gif": {
		Name:        "gif",
		Description: "GIF, 720p15 — for inline previews",
		Settings: Settings{
			Container: ContainerGIF, Codec: CodecGIF,
			Resolution: Res720p, FPS: 15,
		},
	},
}

// Lookup returns a preset by name.
func Lookup(name string) (Preset, error) {
	p, ok := presets[name]
	if !ok {
		return Preset{}, fmt.Errorf("export: unknown preset %q", name)
	}
	return p, nil
}

// PresetNames returns the registered preset names, sorted.
func PresetNames() []string {
	out := make([]string, 0, len(presets))
	for k := range presets {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Validate sanity-checks Settings before handing them to the renderer.
func Validate(s Settings) error {
	if s.Container == "" {
		return errors.New("export: missing container")
	}
	if s.Codec == "" {
		return errors.New("export: missing codec")
	}
	if s.Resolution.Width <= 0 || s.Resolution.Height <= 0 {
		return errors.New("export: invalid resolution")
	}
	if s.FPS <= 0 {
		return errors.New("export: invalid fps")
	}
	if !codecMatchesContainer(s.Codec, s.Container) {
		return fmt.Errorf("export: codec %s incompatible with container %s", s.Codec, s.Container)
	}
	return nil
}

func codecMatchesContainer(c Codec, k Container) bool {
	switch k {
	case ContainerMP4:
		return c == CodecH264 || c == CodecH265 || c == CodecAV1
	case ContainerWebM:
		return c == CodecVP9 || c == CodecAV1
	case ContainerMOV:
		return c == CodecProRes || c == CodecH264 || c == CodecH265
	case ContainerGIF:
		return c == CodecGIF
	case ContainerAPNG:
		return c == CodecAPNG
	case ContainerWebP:
		return c == CodecWebP
	}
	return false
}

// FFmpegArgs renders Settings to an ffmpeg argv slice. The actual
// invocation lives behind a Renderer interface so the host can swap in
// AVFoundation on macOS.
func FFmpegArgs(in, out string, s Settings) []string {
	args := []string{"-y", "-i", in,
		"-c:v", string(codecToFFmpeg(s.Codec)),
		"-r", fmt.Sprintf("%d", s.FPS),
		"-vf", fmt.Sprintf("scale=%d:%d", s.Resolution.Width, s.Resolution.Height),
	}
	if s.BitrateK > 0 {
		args = append(args, "-b:v", fmt.Sprintf("%dk", s.BitrateK))
	}
	if s.CRF > 0 {
		args = append(args, "-crf", fmt.Sprintf("%d", s.CRF))
	}
	if s.Preset != "" {
		args = append(args, "-preset", s.Preset)
	}
	if s.AudioKBps > 0 {
		args = append(args, "-c:a", "aac", "-b:a", fmt.Sprintf("%dk", s.AudioKBps))
	}
	if s.AudioKHz > 0 {
		args = append(args, "-ar", fmt.Sprintf("%d", s.AudioKHz))
	}
	args = append(args, out)
	return args
}

func codecToFFmpeg(c Codec) string {
	switch c {
	case CodecH264:
		return "libx264"
	case CodecH265:
		return "libx265"
	case CodecVP9:
		return "libvpx-vp9"
	case CodecAV1:
		return "libaom-av1"
	case CodecProRes:
		return "prores_ks"
	case CodecGIF:
		return "gif"
	case CodecAPNG:
		return "apng"
	case CodecWebP:
		return "libwebp"
	}
	return string(c)
}

// Renderer is implemented by concrete encoders (FFmpeg, AVFoundation, …).
//
// Render reads the source EDL render at `in`, encodes it according to
// `settings`, and writes the result to `out`. Returning a non-nil error
// must leave any partial output cleaned up.
type Renderer interface {
	Render(in, out string, settings Settings) error
}

// Plan is the user-facing summary of an export — preset name, settings,
// and aux deliverables — that callers can confirm before kicking off a
// render.
type Plan struct {
	Preset Preset
	In     string
	Out    string
	Argv   []string
}

// PrepareFromPreset builds a Plan from a preset name and IO paths.
func PrepareFromPreset(name, in, out string) (*Plan, error) {
	p, err := Lookup(name)
	if err != nil {
		return nil, err
	}
	if err := Validate(p.Settings); err != nil {
		return nil, err
	}
	if strings.TrimSpace(in) == "" || strings.TrimSpace(out) == "" {
		return nil, errors.New("export: in and out paths required")
	}
	return &Plan{
		Preset: p,
		In:     in,
		Out:    out,
		Argv:   FFmpegArgs(in, out, p.Settings),
	}, nil
}

// --- Integration TODO --------------------------------------------------------
//
// A real Renderer needs to:
//   1. Spawn ffmpeg (or AVAssetExportSession) with the produced argv
//   2. Pipe progress back to the host (frame count → percentage)
//   3. Generate AuxOutputs (thumbnail via -vframes 1, chapters from EDL
//      markers, SRT/VTT from the EDL caption track, blog post via the
//      LLM)
//
// None of those are wired up in this PR; the data model + preset
// catalog + arg builder is enough to unblock the surfaces.
