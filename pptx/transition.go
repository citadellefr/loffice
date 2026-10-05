package pptx

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/citadellefr/loffice/internal/xmldom"
)

// Transition is how a slide comes on in a slide show, and when the show
// goes on from it.
type Transition struct {
	// Effect is the local name of the effect's element: "fade", "push",
	// "wipe", "split", "zoom"…, or one of PowerPoint 2010's as "vortex";
	// "" for none.
	Effect string `json:"effect,omitempty"`
	// Dir, Orient and ThruBlk are the options of the effect: a side "l"
	// "u" "r" "d", a corner "lu"…, "in" or "out", "horz" or "vert";
	// through black.
	Dir     string `json:"dir,omitempty"`
	Orient  string `json:"orient,omitempty"`
	ThruBlk bool   `json:"thruBlk,omitempty"`
	// Dur is how long the effect lasts, in milliseconds.
	Dur int `json:"dur"`
	// NoClick is set when a click does not go on to the next slide, After
	// when the show goes on by itself, in milliseconds.
	NoClick bool `json:"noClick,omitempty"`
	After   *int `json:"after,omitempty"`
}

var (
	sides   = []string{"l", "u", "r", "d"}
	corners = []string{"lu", "ru", "ld", "rd"}
	eight   = []string{"l", "u", "r", "d", "lu", "ru", "ld", "rd"}
	axes    = []string{"horz", "vert"}
	inOut   = []string{"in", "out"}
)

// effects are the effects of PresentationML, which the writer writes, and
// the values of their options.
var effects = map[string]map[string][]string{
	"blinds": {"dir": axes}, "checker": {"dir": axes}, "comb": {"dir": axes}, "randomBar": {"dir": axes},
	"circle": nil, "diamond": nil, "dissolve": nil, "newsflash": nil, "plus": nil, "random": nil, "wedge": nil, "wheel": nil,
	"cover": {"dir": eight}, "pull": {"dir": eight},
	"cut": nil, "fade": nil,
	"push": {"dir": sides}, "wipe": {"dir": sides},
	"split":  {"orient": axes, "dir": inOut},
	"strips": {"dir": corners},
	"zoom":   {"dir": inOut},
}

// speeds are the durations of the speeds of PresentationML, in
// milliseconds.
var speeds = map[string]int{"fast": 500, "med": 750, "slow": 1000}

var sldOrder = []string{"cSld", "clrMapOvr", "AlternateContent", "transition", "timing", "extLst"}

// transitionOf is the p:transition of a slide, the richer one of an
// mc:AlternateContent, and the element to remove to replace it.
func transitionOf(sld *xmldom.Element) (t, outer *xmldom.Element) {
	for _, c := range sld.Elements() {
		if c.Space == pNS && c.Local == "transition" {
			return c, c
		}
		if c.Space != mcNS || c.Local != "AlternateContent" {
			continue
		}
		for _, branch := range c.Elements() {
			if t := branch.Child(pNS, "transition"); t != nil {
				return t, c
			}
		}
	}
	return nil, nil
}

func readTransition(sld *xmldom.Element) *Transition {
	e, _ := transitionOf(sld)
	if e == nil {
		return nil
	}
	t := &Transition{Dur: speeds["fast"]}
	if d, ok := speeds[e.Get("spd")]; ok {
		t.Dur = d
	}
	if d, err := strconv.Atoi(e.Get("p14:dur")); err == nil && d >= 0 {
		t.Dur = d
	}
	if v := e.Get("advClick"); v == "0" || v == "false" {
		t.NoClick = true
	}
	if ms, err := strconv.Atoi(e.Get("advTm")); err == nil && ms >= 0 {
		t.After = &ms
	}
	if effect := effectOf(e); effect != nil {
		t.Effect = effect.Local
		t.Dir, t.Orient = effect.Get("dir"), effect.Get("orient")
		t.ThruBlk = effect.Get("thruBlk") == "1" || effect.Get("thruBlk") == "true"
	}
	return t
}

func effectOf(transition *xmldom.Element) *xmldom.Element {
	for _, c := range transition.Elements() {
		if c.Local != "sndAc" && c.Local != "extLst" {
			return c
		}
	}
	return nil
}

// setTransition replaces the transition of a slide. An effect the writer
// does not know is kept from the transition it replaces, under the
// mc:AlternateContent PowerPoint writes, its fallback a fade.
func setTransition(sld *xmldom.Element, value json.RawMessage) {
	var kept *xmldom.Element
	if old, outer := transitionOf(sld); old != nil {
		kept = effectOf(old)
		sld.Remove(outer)
	}
	var t Transition
	if value == nil || json.Unmarshal(value, &t) != nil {
		return
	}
	base := xmldom.New(pNS, "p:transition")
	spd := "slow"
	for _, s := range []string{"fast", "med"} {
		if t.Dur <= speeds[s] {
			spd = s
			break
		}
	}
	if spd != "fast" {
		base.Set("spd", spd)
	}
	if t.NoClick {
		base.Set("advClick", "0")
	}
	if t.After != nil && *t.After >= 0 {
		base.Set("advTm", strconv.Itoa(*t.After))
	}

	effect := newEffect(t)
	if effect == nil && kept != nil && kept.Local == t.Effect && t.Effect != "" {
		effect = kept.Clone()
	}
	_, known := effects[t.Effect]
	if t.Dur == speeds[spd] && (known || effect == nil) {
		if effect != nil {
			base.Append(effect)
		}
		sld.Insert(base, sldOrder)
		return
	}

	// p14:dur, and effects of later versions, need PowerPoint 2010
	requires, space := "p14", standardSpaces()["p14"]
	if effect != nil && !known {
		if prefix, _, ok := strings.Cut(effect.Name, ":"); ok {
			requires, space = prefix, effect.Space
		}
	}
	choice := xmldom.New(mcNS, "mc:Choice", "Requires", requires, "xmlns:"+requires, space)
	if requires != "p14" {
		choice.Set("xmlns:p14", standardSpaces()["p14"])
	}
	rich := base.Clone()
	rich.Set("p14:dur", strconv.Itoa(max(t.Dur, 0)))
	if effect != nil {
		rich.Append(effect)
	}
	choice.Append(rich)
	fallback := xmldom.New(mcNS, "mc:Fallback")
	if known && effect != nil {
		base.Append(effect.Clone())
	} else if effect != nil {
		base.Append(xmldom.New(pNS, "p:fade"))
	}
	fallback.Append(base)
	ac := xmldom.New(mcNS, "mc:AlternateContent", "xmlns:mc", mcNS)
	ac.Append(choice)
	ac.Append(fallback)
	sld.Insert(ac, sldOrder)
}

// newEffect is the element of an effect of PresentationML, with the
// options it has; nil for none or another effect.
func newEffect(t Transition) *xmldom.Element {
	options, ok := effects[t.Effect]
	if !ok {
		return nil
	}
	e := xmldom.New(pNS, "p:"+t.Effect)
	if t.ThruBlk && (t.Effect == "fade" || t.Effect == "cut") {
		e.Set("thruBlk", "1")
	}
	if slices.Contains(options["orient"], t.Orient) {
		e.Set("orient", t.Orient)
	}
	if slices.Contains(options["dir"], t.Dir) {
		e.Set("dir", t.Dir)
	}
	return e
}
