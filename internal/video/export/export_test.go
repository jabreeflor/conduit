package export

import (
	"strings"
	"testing"
)

func TestLookup_Known(t *testing.T) {
	for _, name := range PresetNames() {
		p, err := Lookup(name)
		if err != nil {
			t.Fatalf("preset %q: %v", name, err)
		}
		if err := Validate(p.Settings); err != nil {
			t.Errorf("preset %q invalid: %v", name, err)
		}
	}
}

func TestLookup_Unknown(t *testing.T) {
	if _, err := Lookup("xyz"); err == nil {
		t.Error("expected error")
	}
}

func TestPresetNamesIncludesAll(t *testing.T) {
	names := PresetNames()
	want := []string{"twitter", "youtube", "youtube-4k", "shorts", "slack", "linkedin", "docs", "prores", "gif"}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing preset %q", w)
		}
	}
}

func TestValidate_RejectsMismatch(t *testing.T) {
	bad := Settings{
		Container: ContainerWebM, Codec: CodecH264,
		Resolution: Res1080p, FPS: 30,
	}
	if err := Validate(bad); err == nil {
		t.Error("expected codec/container mismatch error")
	}
}

func TestValidate_FieldChecks(t *testing.T) {
	cases := map[string]Settings{
		"missing container":  {Codec: CodecH264, Resolution: Res1080p, FPS: 30},
		"missing codec":      {Container: ContainerMP4, Resolution: Res1080p, FPS: 30},
		"invalid resolution": {Container: ContainerMP4, Codec: CodecH264, FPS: 30},
		"invalid fps":        {Container: ContainerMP4, Codec: CodecH264, Resolution: Res1080p},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if err := Validate(s); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestFFmpegArgs_IncludesCoreFlags(t *testing.T) {
	args := FFmpegArgs("in.mov", "out.mp4", Settings{
		Container: ContainerMP4, Codec: CodecH264, Resolution: Res1080p,
		FPS: 60, BitrateK: 8000, CRF: 23, Preset: "fast",
		AudioKHz: 48000, AudioKBps: 192,
	})
	joined := strings.Join(args, " ")
	for _, want := range []string{"-i in.mov", "libx264", "-r 60", "scale=1920:1080", "8000k", "-crf 23", "-preset fast", "-b:a 192k", "-ar 48000", "out.mp4"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args missing %q: %s", want, joined)
		}
	}
}

func TestPrepareFromPreset(t *testing.T) {
	plan, err := PrepareFromPreset("youtube", "in.mov", "out.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Preset.Name != "youtube" {
		t.Errorf("preset name = %q", plan.Preset.Name)
	}
	if len(plan.Argv) == 0 {
		t.Error("argv empty")
	}
}

func TestPrepareFromPreset_Errors(t *testing.T) {
	if _, err := PrepareFromPreset("xyz", "i", "o"); err == nil {
		t.Error("expected unknown-preset error")
	}
	if _, err := PrepareFromPreset("youtube", "", "o"); err == nil {
		t.Error("expected missing-paths error")
	}
}

func TestCodecToFFmpegMappings(t *testing.T) {
	cases := map[Codec]string{
		CodecH264: "libx264", CodecH265: "libx265", CodecVP9: "libvpx-vp9",
		CodecAV1: "libaom-av1", CodecProRes: "prores_ks", CodecGIF: "gif",
		CodecAPNG: "apng", CodecWebP: "libwebp",
	}
	for c, want := range cases {
		if got := codecToFFmpeg(c); got != want {
			t.Errorf("%s -> %s, want %s", c, got, want)
		}
	}
}
