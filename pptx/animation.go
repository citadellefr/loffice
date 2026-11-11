package pptx

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/citadellefr/loffice/internal/xmldom"
	"github.com/citadellefr/trame/ot"
)

// Effect is an animation of a slide, in the order they play.
type Effect struct {
	// Target is the shape the effect is on, a node of the slide, and Para
	// the paragraphs of its text, when it is not all of it.
	Target string `json:"target,omitempty"`
	Para   *Span  `json:"para,omitempty"`
	// Class is "entr" for an entrance, "emph" for an emphasis, "exit" or
	// "path"; Preset and Subtype the presetID and presetSubtype of the
	// effect in PowerPoint's gallery.
	Class   string `json:"class"`
	Preset  int    `json:"preset"`
	Subtype int    `json:"subtype,omitempty"`
	// Trigger is "click" for an effect that starts at a click, "with" for
	// one that starts with the effect before it, "after" for one that
	// starts when it ends.
	Trigger string `json:"trigger"`
	// Delay and Dur are the delay and the duration of the effect, in
	// milliseconds.
	Delay int `json:"delay,omitempty"`
	Dur   int `json:"dur,omitempty"`
	// XML is the effect as read, set when the writer cannot make it: it is
	// written as it is.
	XML string `json:"xml,omitempty"`
}

// Span is a range of paragraphs, the first and the last, from 0.
type Span struct {
	From int `json:"from"`
	To   int `json:"to"`
}

const (
	maxEffects = 1000
	maxTime    = 3600000
	maxPara    = 100000
)

var (
	classes   = []string{"entr", "emph", "exit", "path"}
	nodeTypes = map[string]string{"click": "clickEffect", "with": "withEffect", "after": "afterEffect"}
	triggers  = map[string]string{"clickEffect": "click", "withEffect": "with", "afterEffect": "after"}
)

type preset struct {
	class       string
	id, subtype int
}

// flyFrom are the starts of the x and y of a fly in, by side of the slide
// it comes from.
var flyFrom = map[int][2]string{
	1: {"#ppt_x", "0-#ppt_h/2"},
	2: {"1+#ppt_w/2", "#ppt_y"},
	4: {"#ppt_x", "1+#ppt_h/2"},
	8: {"0-#ppt_w/2", "#ppt_y"},
}

// body is the behaviors of an effect of the gallery the writer makes, on
// the target tgt; false for another.
func body(p preset, tgt string, dur int) (string, bool) {
	switch p {
	case preset{"entr", 1, 0}:
		return visibility(tgt, "visible", 0), true
	case preset{"exit", 1, 0}:
		return visibility(tgt, "hidden", 0), true
	case preset{"entr", 10, 0}:
		return visibility(tgt, "visible", 0) + fade(tgt, "in", dur), true
	case preset{"exit", 10, 0}:
		return visibility(tgt, "hidden", max(dur-1, 0)) + fade(tgt, "out", dur), true
	case preset{"entr", 53, 16}:
		return visibility(tgt, "visible", 0) +
			anim(tgt, "ppt_w", "", `<p:fltVal val="0"/>`, `<p:strVal val="#ppt_w"/>`, dur) +
			anim(tgt, "ppt_h", "", `<p:fltVal val="0"/>`, `<p:strVal val="#ppt_h"/>`, dur) +
			fade(tgt, "in", dur), true
	}
	if from, ok := flyFrom[p.subtype]; ok && p.class == "entr" && p.id == 2 {
		return visibility(tgt, "visible", 0) +
			anim(tgt, "ppt_x", ` additive="base"`, `<p:strVal val="`+from[0]+`"/>`, `<p:strVal val="#ppt_x"/>`, dur) +
			anim(tgt, "ppt_y", ` additive="base"`, `<p:strVal val="`+from[1]+`"/>`, `<p:strVal val="#ppt_y"/>`, dur), true
	}
	return "", false
}

func visibility(tgt, value string, delay int) string {
	return fmt.Sprintf(`<p:set><p:cBhvr><p:cTn id="0" dur="1" fill="hold"><p:stCondLst><p:cond delay="%d"/></p:stCondLst></p:cTn><p:tgtEl>%s</p:tgtEl><p:attrNameLst><p:attrName>style.visibility</p:attrName></p:attrNameLst></p:cBhvr><p:to><p:strVal val="%s"/></p:to></p:set>`,
		delay, tgt, value)
}

func fade(tgt, transition string, dur int) string {
	return fmt.Sprintf(`<p:animEffect transition="%s" filter="fade"><p:cBhvr><p:cTn id="0" dur="%d"/><p:tgtEl>%s</p:tgtEl></p:cBhvr></p:animEffect>`,
		transition, dur, tgt)
}

func anim(tgt, attr, additive, from, to string, dur int) string {
	return fmt.Sprintf(`<p:anim calcmode="lin" valueType="num"><p:cBhvr%s><p:cTn id="0" dur="%d" fill="hold"/><p:tgtEl>%s</p:tgtEl><p:attrNameLst><p:attrName>%s</p:attrName></p:attrNameLst></p:cBhvr><p:tavLst><p:tav tm="0"><p:val>%s</p:val></p:tav><p:tav tm="100000"><p:val>%s</p:val></p:tav></p:tavLst></p:anim>`,
		additive, dur, tgt, attr, from, to)
}

func target(spid string, para *Span) string {
	if para == nil {
		return `<p:spTgt spid="` + spid + `"/>`
	}
	return fmt.Sprintf(`<p:spTgt spid="%s"><p:txEl><p:pRg st="%d" end="%d"/></p:txEl></p:spTgt>`, spid, para.From, para.To)
}

// buildEffect is the time node of an effect of the gallery, aimed at the
// shape of id spid; nil for another.
func buildEffect(e Effect, spid string) *xmldom.Element {
	b, ok := body(preset{e.Class, e.Preset, e.Subtype}, target(spid, e.Para), e.Dur)
	if !ok || nodeTypes[e.Trigger] == "" {
		return nil
	}
	el, err := xmldom.ParseFragment([]byte(fmt.Sprintf(
		`<p:par><p:cTn id="0" presetID="%d" presetClass="%s" presetSubtype="%d" fill="hold" grpId="0" nodeType="%s"><p:stCondLst><p:cond delay="%d"/></p:stCondLst><p:childTnLst>%s</p:childTnLst></p:cTn></p:par>`,
		e.Preset, e.Class, e.Subtype, nodeTypes[e.Trigger], e.Delay, b)), standardSpaces())
	if err != nil {
		return nil
	}
	return el
}

// canonical is a time node without what does not tell effects apart: the
// ids of the nodes.
func canonical(e *xmldom.Element) string {
	c := e.Clone()
	walkElements(c, func(n *xmldom.Element) bool {
		if n.Space == pNS && n.Local == "cTn" {
			n.Set("id", "0")
			n.Unset("grpId")
		}
		return true
	})
	return string(c.Bytes())
}

func descend(e *xmldom.Element, path ...string) *xmldom.Element {
	for _, local := range path {
		if e == nil {
			return nil
		}
		e = e.Child(pNS, local)
	}
	return e
}

// mainSeq is the time node of the main sequence of a p:timing, the effects
// of the slide in it.
func mainSeq(timing *xmldom.Element) *xmldom.Element {
	for _, par := range kids(descend(timing, "tnLst")) {
		root := descend(par, "cTn")
		if root == nil || root.Get("nodeType") != "tmRoot" {
			continue
		}
		for _, seq := range kids(descend(root, "childTnLst")) {
			if ctn := descend(seq, "cTn"); ctn != nil && ctn.Get("nodeType") == "mainSeq" {
				return ctn
			}
		}
	}
	return nil
}

// childrenOf are the time nodes under a par.
func childrenOf(par *xmldom.Element) []*xmldom.Element {
	if par.Space != pNS || par.Local != "par" {
		return nil
	}
	return kids(descend(par, "cTn", "childTnLst"))
}

func kids(e *xmldom.Element) []*xmldom.Element {
	if e == nil {
		return nil
	}
	return e.Elements()
}

// readAnimations reads the effects of a slide, in order: those of the
// groups of the main sequence, a group at each click, in the steps of
// which the effects start together. exists tells whether a shape is a node.
func readAnimations(sld *xmldom.Element, slideID string, exists func(string) bool) []Effect {
	seq := descend(mainSeq(sld.Child(pNS, "timing")), "childTnLst")
	var out []Effect
	for _, group := range kids(seq) {
		steps := childrenOf(group)
		if steps == nil {
			out = append(out, opaque(group, "click", slideID, exists))
		}
		for _, step := range steps {
			effects := childrenOf(step)
			if effects == nil {
				out = append(out, opaque(step, "with", slideID, exists))
			}
			for _, el := range effects {
				out = append(out, readEffect(el, slideID, exists))
			}
		}
	}
	return out
}

func readEffect(el *xmldom.Element, slideID string, exists func(string) bool) Effect {
	ctn := descend(el, "cTn")
	if ctn == nil {
		return opaque(el, "with", slideID, exists)
	}
	trigger, ok := triggers[ctn.Get("nodeType")]
	if !ok {
		return opaque(el, "with", slideID, exists)
	}
	e, spid := describe(el, ctn, trigger, slideID, exists)
	if known := buildEffect(e, spid); known == nil || canonical(known) != canonical(el) {
		e.XML = string(el.Bytes())
	}
	return e
}

func opaque(el *xmldom.Element, trigger, slideID string, exists func(string) bool) Effect {
	e, _ := describe(el, descend(el, "cTn"), trigger, slideID, exists)
	e.XML = string(el.Bytes())
	return e
}

// describe is what an effect says of itself, and the id of the shape it is
// on.
func describe(el, ctn *xmldom.Element, trigger, slideID string, exists func(string) bool) (e Effect, spid string) {
	e.Trigger = trigger
	if ctn == nil {
		return e, ""
	}
	e.Class = ctn.Get("presetClass")
	e.Preset, _ = strconv.Atoi(ctn.Get("presetID"))
	e.Subtype, _ = strconv.Atoi(ctn.Get("presetSubtype"))
	if cond := descend(ctn, "stCondLst", "cond"); cond != nil {
		e.Delay, _ = strconv.Atoi(cond.Get("delay"))
	}
	walkElements(el, func(c *xmldom.Element) bool {
		if c.Space == pNS && c.Local == "set" {
			return false
		}
		if c.Space == pNS && c.Local == "cTn" && c != ctn {
			if d, err := strconv.Atoi(c.Get("dur")); err == nil {
				e.Dur = max(e.Dur, d)
			}
		}
		return true
	})
	if tgt := firstTarget(el); tgt != nil {
		spid = tgt.Get("spid")
		if id := slideID + "-" + spid; exists(id) {
			e.Target = id
		}
		if rg := descend(tgt, "txEl", "pRg"); rg != nil {
			from, err1 := strconv.Atoi(rg.Get("st"))
			to, err2 := strconv.Atoi(rg.Get("end"))
			if err1 == nil && err2 == nil {
				e.Para = &Span{from, to}
			}
		}
	}
	return e, spid
}

func firstTarget(el *xmldom.Element) *xmldom.Element {
	var found *xmldom.Element
	walkElements(el, func(c *xmldom.Element) bool {
		if found == nil && c.Space == pNS && c.Local == "spTgt" {
			found = c
		}
		return found == nil
	})
	return found
}

// without gives the effects that stay when those that are gone leave. The
// first of those that follow an effect that started a group and went takes
// its trigger, so that a click or a chain still starts.
func without(list []Effect, gone func(Effect) bool) []Effect {
	var out []Effect
	lost := false
	for _, e := range list {
		if gone(e) {
			lost = lost || e.Trigger == "click"
			continue
		}
		if lost {
			e.Trigger = "click"
		}
		lost = false
		out = append(out, e)
	}
	return out
}

const timingSkeleton = `<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst>` +
	`<p:seq concurrent="1" nextAc="seek"><p:cTn id="2" dur="indefinite" nodeType="mainSeq"><p:childTnLst/></p:cTn>` +
	`<p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst>` +
	`<p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst></p:seq>` +
	`</p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`

// setAnimations replaces the effects of a slide with those of value, the
// shapes by node in spids, the rest of the timing as it is.
func (w *writer) setAnimations(sld *xmldom.Element, value json.RawMessage) {
	var list []Effect
	if value != nil {
		_ = json.Unmarshal(value, &list)
	}
	timing := sld.Child(pNS, "timing")
	seq := mainSeq(timing)
	if seq == nil {
		if timing != nil {
			sld.Remove(timing)
		}
		if len(list) == 0 {
			return
		}
		timing, _ = xmldom.ParseFragment([]byte(timingSkeleton), standardSpaces())
		sld.Insert(timing, sldOrder)
		seq = mainSeq(timing)
	}
	next := 1
	walkElements(timing, func(c *xmldom.Element) bool {
		if n, err := strconv.Atoi(c.Get("id")); err == nil && c.Local == "cTn" {
			next = max(next, n+1)
		}
		return true
	})

	groups := w.groups(list, seq.Get("id"))
	for _, g := range groups {
		walkElements(g, func(c *xmldom.Element) bool {
			if c.Space == pNS && c.Local == "cTn" && c.Get("id") == "0" {
				c.Set("id", strconv.Itoa(next))
				next++
			}
			return true
		})
	}
	childList := xmldom.New(pNS, "p:childTnLst")
	for _, g := range groups {
		childList.Append(g)
	}
	seq.Replace(seq.Child(pNS, "childTnLst"), childList)

	w.syncBuilds(timing, list)
	if len(effectNodes(timing)) == 0 {
		sld.Remove(timing)
	}
}

// groups are the groups of the main sequence of effects, whose first node
// has the id seqID.
func (w *writer) groups(list []Effect, seqID string) []*xmldom.Element {
	var groups []*xmldom.Element
	var steps, effects *xmldom.Element
	var groupEnd, stepStart int
	for _, e := range list {
		el := w.effectElement(e)
		if el == nil {
			continue
		}
		if steps == nil || e.Trigger == "click" {
			var group *xmldom.Element
			group, steps = newPar("indefinite")
			if e.Trigger != "click" {
				onBegin := xmldom.New(pNS, "p:cond", "evt", "onBegin", "delay", "0")
				onBegin.Append(xmldom.New(pNS, "p:tn", "val", seqID))
				descend(group, "cTn", "stCondLst").Append(onBegin)
			}
			groups = append(groups, group)
			groupEnd, effects = 0, nil
		}
		if effects == nil || e.Trigger != "with" {
			stepStart = 0
			if e.Trigger == "after" {
				stepStart = groupEnd
			}
			var step *xmldom.Element
			step, effects = newPar(strconv.Itoa(stepStart))
			steps.Append(step)
		}
		groupEnd = max(groupEnd, stepStart+e.Delay+e.Dur)
		effects.Append(el)
	}
	return groups
}

// newPar is a par of the time nodes under it, and their list.
func newPar(delay string) (par, list *xmldom.Element) {
	par = xmldom.New(pNS, "p:par")
	ctn := xmldom.New(pNS, "p:cTn", "id", "0", "fill", "hold")
	cond := xmldom.New(pNS, "p:stCondLst")
	cond.Append(xmldom.New(pNS, "p:cond", "delay", delay))
	list = xmldom.New(pNS, "p:childTnLst")
	ctn.Append(cond)
	ctn.Append(list)
	par.Append(ctn)
	return par, list
}

// effectElement is the time node of an effect, nil when the writer cannot
// have it: a shape that is gone, an effect of the gallery it does not make.
func (w *writer) effectElement(e Effect) *xmldom.Element {
	if e.XML != "" {
		el := w.d.fragment(e.XML)
		if ctn := descend(el, "cTn"); ctn != nil && triggers[ctn.Get("nodeType")] != "" {
			ctn.Set("nodeType", nodeTypes[e.Trigger])
		}
		return el
	}
	c := w.spids[e.Target]
	if c == nil {
		return nil
	}
	return buildEffect(e, c.Get("id"))
}

// syncBuilds makes the build list of a timing agree with the effects: the
// shapes without effects leave it, the shapes with text that have one come
// in.
func (w *writer) syncBuilds(timing *xmldom.Element, list []Effect) {
	used := map[string]bool{}
	walkElements(descend(timing, "tnLst"), func(c *xmldom.Element) bool {
		if v, ok := c.Attr("spid"); ok {
			used[v] = true
		}
		return true
	})
	bld := descend(timing, "bldLst")
	added := bld == nil
	if added {
		bld = xmldom.New(pNS, "p:bldLst")
	}
	has := map[string]*xmldom.Element{}
	for _, b := range bld.Elements() {
		if spid := b.Get("spid"); used[spid] {
			has[spid] = b
		} else {
			bld.Remove(b)
		}
	}
	for _, e := range list {
		c := w.spids[e.Target]
		if c == nil || e.XML != "" || w.tree.Node(e.Target).Type != "sp" {
			continue
		}
		spid := c.Get("id")
		b := has[spid]
		if b == nil {
			b = xmldom.New(pNS, "p:bldP", "spid", spid, "grpId", "0", "animBg", "1")
			bld.Append(b)
			has[spid] = b
		}
		if e.Para != nil && b.Local == "bldP" && b.Get("build") != "p" {
			b.Unset("animBg")
			b.Set("build", "p")
		}
	}
	switch {
	case len(bld.Elements()) == 0:
		timing.Remove(bld)
	case added:
		timing.Insert(bld, []string{"tnLst", "bldLst", "extLst"})
	}
}

// slideAnimations are the effects of a slide node.
func slideAnimations(n *ot.Node) (list []Effect) {
	_ = json.Unmarshal(n.Attrs["animations"], &list)
	return list
}
