package video

// Template is a reusable composition the planner can apply wholesale —
// e.g. "podcast", "feature-launch", "tutorial". Templates are pure data
// so plugins can ship their own.
type Template struct {
	Name  string
	Intro *Effect
	Outro *Effect
	Music *MusicTrack
	Speed float64 // global speed scalar; 0 = leave untouched
}

// ApplyTemplate wires the template's intro/outro/music/speed into an EDL.
func (e *EDL) ApplyTemplate(t Template) {
	if t.Intro != nil && len(e.Clips) > 0 {
		e.Clips[0].Effects = append(e.Clips[0].Effects, *t.Intro)
	}
	if t.Outro != nil && len(e.Clips) > 0 {
		last := len(e.Clips) - 1
		e.Clips[last].Effects = append(e.Clips[last].Effects, *t.Outro)
	}
	if t.Music != nil {
		e.SetMusic(*t.Music)
	}
	if t.Speed > 0 {
		for i := range e.Clips {
			e.Clips[i].Speed = t.Speed
		}
	}
}
