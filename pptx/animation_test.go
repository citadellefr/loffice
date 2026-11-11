package pptx

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/citadellefr/trame/ot"
)

// mainTiming is a p:timing of the effects given, one group per par.
func mainTiming(groups ...string) string {
	return number(`<p:timing><p:tnLst><p:par><p:cTn id="0" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst><p:seq concurrent="1" nextAc="seek"><p:cTn id="0" dur="indefinite" nodeType="mainSeq"><p:childTnLst>` +
		strings.Join(groups, "") +
		`</p:childTnLst></p:cTn><p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst></p:seq></p:childTnLst></p:cTn></p:par></p:tnLst></p:timing>`)
}

// emphasis is an effect no preset of the writer makes.
func emphasis(nodeType, target string) string {
	return strings.Replace(effect(nodeType, target), `presetID="1" presetClass="entr"`, `presetID="6" presetClass="emph"`, 1)
}

// typedAnimations is the timing of an entrance at a click, an emphasis with
// it, an exit after it, and an entrance by paragraph at another click.
func typedAnimations() (timing, emph string) {
	emph = emphasis("withEffect", spTgt("4"))
	defer func() { emph = regexp.MustCompile(`<p:par><p:cTn id="\d+" presetID="6".*?</p:par>`).FindString(timing) }()
	exit, _ := body(preset{"exit", 1, 0}, spTgt("5"), 0)
	exitNode := fmt.Sprintf(`<p:par><p:cTn id="0" presetID="1" presetClass="exit" presetSubtype="0" fill="hold" grpId="0" nodeType="afterEffect"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst>%s</p:childTnLst></p:cTn></p:par>`, exit)
	return mainTiming(
		par("indefinite",
			par("0", effect("clickEffect", spTgt("3")), emph),
			par("1", exitNode),
		) + par("indefinite",
			par("0", effect("clickEffect", paragraph("2"))),
		),
	), emph
}

func animationsOf(n *ot.Node) []Effect {
	return slideAnimations(n)
}

func TestReadAnimations(t *testing.T) {
	timing, emph := typedAnimations()
	_, tree := slideWithTiming(t, timing)
	slide := slidesOf(tree)[0]
	id := func(spid string) string { return slide.ID + "-" + spid }
	got := animationsOf(slide)
	want := []Effect{
		{Target: id("3"), Class: "entr", Preset: 1, Trigger: "click"},
		{Target: id("4"), Class: "emph", Preset: 6, Trigger: "with", XML: emph},
		{Target: id("5"), Class: "exit", Preset: 1, Trigger: "after"},
		{Target: id("2"), Para: &Span{0, 0}, Class: "entr", Preset: 1, Trigger: "click"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v\nwant %+v", got, want)
	}
	if !strings.Contains(string(slide.Attrs["animations"]), `"target":"`+id("3")+`","class":"entr","preset":1,"trigger":"click"`) {
		t.Errorf("json %s", slide.Attrs["animations"])
	}
}

// savedAnimations saves a presentation whose first slide has animations
// changed to list, and gives back its XML and the animations read again.
func savedAnimations(t *testing.T, timing string, change func(slide string, list []Effect) []Effect) (string, []Effect) {
	t.Helper()
	d, tree := slideWithTiming(t, timing)
	slide := slidesOf(tree)[0]
	list := change(slide.ID, animationsOf(slide))
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": mustJSON(list)}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := Open(saved, d.limits)
	if err != nil {
		t.Fatal(err)
	}
	return string(rawParts(t, saved)["ppt/slides/slide1.xml"]), animationsOf(slidesOf(again)[0])
}

func summary(list []Effect) string {
	var out []string
	for _, e := range list {
		out = append(out, fmt.Sprintf("%s:%s:%d", e.Trigger, e.Target[strings.LastIndex(e.Target, "-")+1:], e.Preset))
	}
	return strings.Join(out, " ")
}

func TestEditAnimations(t *testing.T) {
	timing, _ := typedAnimations()
	for name, tc := range map[string]struct {
		change func(slide string, list []Effect) []Effect
		want   string
	}{
		"add": {func(slide string, list []Effect) []Effect {
			return append(list, Effect{Target: slide + "-5", Class: "entr", Preset: 10, Trigger: "with", Dur: 750})
		}, "click:3:1 with:4:6 after:5:1 click:2:1 with:5:10"},
		"remove": {func(_ string, list []Effect) []Effect {
			return list[1:]
		}, "with:4:6 after:5:1 click:2:1"},
		"reorder": {func(_ string, list []Effect) []Effect {
			return []Effect{list[3], list[0], list[2], list[1]}
		}, "click:2:1 click:3:1 after:5:1 with:4:6"},
		"trigger": {func(_ string, list []Effect) []Effect {
			list[2].Trigger = "with"
			list[3].Trigger = "after"
			return list
		}, "click:3:1 with:4:6 with:5:1 after:2:1"},
		"delay and duration": {func(_ string, list []Effect) []Effect {
			list[0].Preset, list[0].Dur, list[0].Delay = 10, 1500, 250
			return list
		}, "click:3:10 with:4:6 after:5:1 click:2:1"},
	} {
		t.Run(name, func(t *testing.T) {
			xml, again := savedAnimations(t, timing, tc.change)
			if got := summary(again); got != tc.want {
				t.Fatalf("%s, want %s\n%s", got, tc.want, timingPart(xml))
			}
			if name == "add" {
				if last := again[len(again)-1]; last.Dur != 750 || last.XML != "" {
					t.Errorf("%+v", last)
				}
				if !strings.Contains(xml, `<p:bldP spid="5" grpId="0" animBg="1"/>`) {
					t.Errorf("no build for the shape: %s", timingPart(xml))
				}
			}
			if name == "delay and duration" && (again[0].Dur != 1500 || again[0].Delay != 250) {
				t.Errorf("%+v", again[0])
			}
		})
	}
}

func TestEditAnimationsKeepsOtherEffects(t *testing.T) {
	timing, emph := typedAnimations()
	xml, again := savedAnimations(t, timing, func(_ string, list []Effect) []Effect {
		return []Effect{list[3], list[1], list[0]}
	})
	if !strings.Contains(xml, emph) {
		t.Errorf("effect lost: %s", timingPart(xml))
	}
	if again[1].XML == "" || again[1].Preset != 6 {
		t.Errorf("%+v", again)
	}
}

func TestAnimationsUntouchedKeepTheTiming(t *testing.T) {
	timing, emph := typedAnimations()
	d, tree := slideWithTiming(t, timing)
	for _, c := range tree.Children(slidesOf(tree)[0].ID) {
		if string(c.Attrs["name"]) == `"Forme 5"` {
			apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: c.ID, Attrs: ot.Values{"name": json.RawMessage(`"Renommée"`)}}})
		}
	}
	slide := slidesOf(tree)[0]
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"hidden": json.RawMessage("true")}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	xml := string(rawParts(t, saved)["ppt/slides/slide1.xml"])
	if got := timingPart(xml); got != timing || !strings.Contains(got, emph) {
		t.Fatalf("timing changed:\n%s", got)
	}
}

func TestAnimationsFromNothing(t *testing.T) {
	d, tree := slideWithTiming(t, "")
	slide := slidesOf(tree)[0]
	if slide.Attrs["animations"] != nil {
		t.Fatalf("%s", slide.Attrs["animations"])
	}
	list := []Effect{{Target: slide.ID + "-3", Class: "entr", Preset: 53, Subtype: 16, Trigger: "click", Dur: 500}}
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": mustJSON(list)}}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := Open(saved, d.limits)
	if err != nil {
		t.Fatal(err)
	}
	if got := animationsOf(slidesOf(again)[0]); !reflect.DeepEqual(got, []Effect{{Target: slidesOf(again)[0].ID + "-3", Class: "entr", Preset: 53, Subtype: 16, Trigger: "click", Dur: 500}}) {
		t.Fatalf("%+v", got)
	}
	apply(t, d, tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": nil}}})
	saved, err = d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	if xml := string(rawParts(t, saved)["ppt/slides/slide1.xml"]); strings.Contains(xml, "<p:timing") {
		t.Errorf("timing left: %s", timingPart(xml))
	}
}

func TestCheckAnimations(t *testing.T) {
	timing, emph := typedAnimations()
	d, tree := slideWithTiming(t, timing)
	slide := slidesOf(tree)[0]
	good := Effect{Target: slide.ID + "-3", Class: "entr", Preset: 10, Trigger: "click", Dur: 500}
	for name, tc := range map[string]struct {
		effect Effect
		ok     bool
	}{
		"good":             {good, true},
		"opaque":           {Effect{Target: slide.ID + "-4", Class: "emph", Preset: 6, Trigger: "with", XML: emph}, true},
		"absent shape":     {Effect{Target: slide.ID + "-99", Class: "entr", Preset: 10, Trigger: "click", Dur: 500}, false},
		"no shape":         {Effect{Class: "entr", Preset: 10, Trigger: "click", Dur: 500}, false},
		"the slide":        {Effect{Target: slide.ID, Class: "entr", Preset: 10, Trigger: "click", Dur: 500}, false},
		"negative delay":   {Effect{Target: good.Target, Class: "entr", Preset: 10, Trigger: "click", Delay: -1}, false},
		"endless duration": {Effect{Target: good.Target, Class: "entr", Preset: 10, Trigger: "click", Dur: 1 << 40}, false},
		"trigger":          {Effect{Target: good.Target, Class: "entr", Preset: 10, Trigger: "hover"}, false},
		"unknown preset":   {Effect{Target: good.Target, Class: "entr", Preset: 7777, Trigger: "click"}, false},
		"class":            {Effect{Target: good.Target, Class: "exit", Preset: 53, Subtype: 16, Trigger: "click"}, false},
		"paragraphs":       {Effect{Target: good.Target, Class: "entr", Preset: 10, Trigger: "click", Para: &Span{3, 1}}, false},
		"forged xml":       {Effect{Target: good.Target, Class: "emph", Preset: 6, Trigger: "with", XML: "<p:par/>"}, false},
	} {
		e := ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": mustJSON([]Effect{tc.effect})}}}
		err := d.Check(tree, e, "Test")
		if tc.ok && err != nil || !tc.ok && !errors.Is(err, ErrAnimation) {
			t.Errorf("%s: %v", name, err)
		}
	}

	delShape := ot.Change{Op: ot.OpDel, ID: slide.ID + "-3"}
	set := ot.Change{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": mustJSON([]Effect{good})}}
	if err := d.Check(tree, ot.Edit{delShape, set}, "Test"); !errors.Is(err, ErrAnimation) {
		t.Errorf("effect on a shape deleted: %v", err)
	}
	if err := d.Check(tree, ot.Edit{{Op: ot.OpSet, ID: slide.ID, Attrs: ot.Values{"animations": json.RawMessage(`{`)}}}, "Test"); err == nil {
		t.Error("broken JSON")
	}
}

func TestDeletedShapeLeavesTheAnimations(t *testing.T) {
	d, tree := animatedSlide(t)
	slide := slidesOf(tree)[0]
	e := ot.Edit{{Op: ot.OpDel, ID: slide.ID + "-2"}}
	apply(t, d, tree, e)
	more := d.Follow(tree, e)
	if len(more) != 1 {
		t.Fatalf("%v", more)
	}
	// the effect after the click of the deleted one starts at a click
	if got := summary(animationsOf(tree.Node(slide.ID))); got != "click:3:1 click:4:1" {
		t.Fatalf("%s", got)
	}
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	timing := timingPart(string(rawParts(t, saved)["ppt/slides/slide1.xml"]))
	if got := fmt.Sprint(spidsOf(timing)); got != "[3 4]" {
		t.Errorf("targets %s", got)
	}
}

// afterAutoTiming starts by itself: two steps of effects, then a click and
// two steps again, each shape on one effect or two.
func afterAutoTiming() string {
	return mainTiming(
		par("indefinite",
			par("0", effect("afterEffect", spTgt("2")), effect("withEffect", spTgt("3"))),
			par("1", effect("afterEffect", spTgt("4"))),
		) + par("indefinite",
			par("0", effect("clickEffect", spTgt("5")), effect("withEffect", spTgt("2"))),
			par("1", effect("afterEffect", spTgt("3")), effect("withEffect", spTgt("4"))),
		),
	)
}

// Deleting shapes gives the animations the list gives without their effects.
func TestDeleteShapesAsDeleteEffects(t *testing.T) {
	for mask := 1; mask < 16; mask++ {
		d, tree := slideWithTiming(t, afterAutoTiming())
		slide := slidesOf(tree)[0]
		list := animationsOf(slide)
		var e ot.Edit
		gone := map[string]bool{}
		for i, spid := range []string{"2", "3", "4", "5"} {
			if mask>>i&1 == 1 {
				e = append(e, ot.Change{Op: ot.OpDel, ID: slide.ID + "-" + spid})
				gone[slide.ID+"-"+spid] = true
			}
		}
		want := summary(without(list, func(e Effect) bool { return gone[e.Target] }))
		apply(t, d, tree, e)
		saved, err := d.Save(tree)
		if err != nil {
			t.Fatal(err)
		}
		_, again, err := Open(saved, d.limits)
		if err != nil {
			t.Fatal(err)
		}
		if got := summary(animationsOf(slidesOf(again)[0])); got != want {
			t.Errorf("deleting %v: %q, want %q", e, got, want)
		}
	}
}

func TestDeleteStarterOfTheSequenceKeepsTheNextClick(t *testing.T) {
	d, tree := slideWithTiming(t, afterAutoTiming())
	slide := slidesOf(tree)[0]
	apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: slide.ID + "-2"}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	timing := timingPart(string(rawParts(t, saved)["ppt/slides/slide1.xml"]))
	if got := regexp.MustCompile(`nodeType="(\w+Effect)"`).FindAllStringSubmatch(timing, -1); fmt.Sprint(got) != "[[nodeType=\"afterEffect\" afterEffect] [nodeType=\"afterEffect\" afterEffect] [nodeType=\"clickEffect\" clickEffect] [nodeType=\"afterEffect\" afterEffect] [nodeType=\"withEffect\" withEffect]]" {
		t.Errorf("%s", timing)
	}
}

func TestDeleteStepRetimesTheNext(t *testing.T) {
	node := func(nodeType, spid string) string {
		b, _ := body(preset{"entr", 10, 0}, spTgt(spid), 500)
		return fmt.Sprintf(`<p:par><p:cTn id="0" presetID="10" presetClass="entr" presetSubtype="0" fill="hold" nodeType="%s"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst>%s</p:childTnLst></p:cTn></p:par>`, nodeType, b)
	}
	timing := mainTiming(par("indefinite",
		par("0", node("clickEffect", "2")),
		par("500", node("afterEffect", "3")),
		par("1000", node("afterEffect", "4")),
		par("1500", node("afterEffect", "5")),
	))
	d, tree := slideWithTiming(t, timing)
	slide := slidesOf(tree)[0]
	apply(t, d, tree, ot.Edit{{Op: ot.OpDel, ID: slide.ID + "-3"}})
	saved, err := d.Save(tree)
	if err != nil {
		t.Fatal(err)
	}
	got := timingPart(string(rawParts(t, saved)["ppt/slides/slide1.xml"]))
	var delays []string
	for _, m := range regexp.MustCompile(`<p:cTn id="\d+" fill="hold"><p:stCondLst><p:cond delay="(\w+)"/>`).FindAllStringSubmatch(got, -1) {
		delays = append(delays, m[1])
	}
	if fmt.Sprint(delays) != "[indefinite 0 500 1000]" {
		t.Errorf("delays %v in %s", delays, got)
	}
}
