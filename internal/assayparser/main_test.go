package assayparser

import (
	"reflect"
	"testing"
)

func TestMkOligoBasesOnly(t *testing.T) {
	o, err := MkOligo("t", FuncForwardPrimer, "ATGC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.SeqClean != "ATGC" {
		t.Errorf("SeqClean = %q, want %q", o.SeqClean, "ATGC")
	}
	if o.SeqActual != "ATGC" {
		t.Errorf("SeqActual = %q, want %q", o.SeqActual, "ATGC")
	}
	if len(o.Mods) != 0 {
		t.Errorf("Mods = %v, want none", o.Mods)
	}
}

func TestMkOligoWhitespaceIgnored(t *testing.T) {
	o, err := MkOligo("t", "", "AT GC\n\tAT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.SeqClean != "ATGCAT" {
		t.Errorf("SeqClean = %q, want %q", o.SeqClean, "ATGCAT")
	}
	if o.SeqActual != "ATGCAT" {
		t.Errorf("SeqActual = %q, want %q", o.SeqActual, "ATGCAT")
	}
}

// Mirrors internal/assayparser/example/test.yaml: the mG base-mod sits at
// 1-based clean position 11.
func TestMkOligoBaseMod(t *testing.T) {
	o, err := MkOligo("Testoligo", FuncForwardPrimer, "AATACTAATC/mG/T")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.SeqClean != "AATACTAATCGT" {
		t.Errorf("SeqClean = %q, want %q", o.SeqClean, "AATACTAATCGT")
	}
	if o.SeqActual != "AATACTAATC/mG/T" {
		t.Errorf("SeqActual = %q, want %q", o.SeqActual, "AATACTAATC/mG/T")
	}
	want := []Modification{{Id: 1, Pos: 11, Content: "mG", ActsAsBase: "G"}}
	if !reflect.DeepEqual(o.Mods, want) {
		t.Errorf("Mods = %+v, want %+v", o.Mods, want)
	}
}

func TestMkOligoNonBaseMods(t *testing.T) {
	// 5' fluorophore, internal base-mod, 3' quencher.
	o, err := MkOligo("Probe", FuncProbe, "/56-FAM/ATG/mC/A/3BHQ_1/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if o.SeqClean != "ATGCA" {
		t.Errorf("SeqClean = %q, want %q", o.SeqClean, "ATGCA")
	}
	want := []Modification{
		{Id: 1, Pos: 0, Content: "56-FAM", ActsAsBase: NonBase},
		{Id: 2, Pos: 4, Content: "mC", ActsAsBase: "C"},
		{Id: 3, Pos: 5, Content: "3BHQ_1", ActsAsBase: NonBase},
	}
	if !reflect.DeepEqual(o.Mods, want) {
		t.Errorf("Mods = %+v, want %+v", o.Mods, want)
	}
}

func TestMkOligoErrors(t *testing.T) {
	cases := map[string]string{
		"unknown mod":         "AT/bogus/GC",
		"unterminated marker": "ATGC/mG",
		"invalid character":   "ATG5",
		"empty marker":        "AT//GC",
	}
	for name, seq := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := MkOligo("t", "", seq); err == nil {
				t.Errorf("MkOligo(%q) = nil error, want error", seq)
			}
		})
	}
}

func TestIsValidFunction(t *testing.T) {
	for _, f := range []string{FuncForwardPrimer, FuncReversePrimer, FuncProbe} {
		if !IsValidFunction(f) {
			t.Errorf("IsValidFunction(%q) = false, want true", f)
		}
	}
	for _, f := range []string{"", "fwd", "primer", "PROBE"} {
		if IsValidFunction(f) {
			t.Errorf("IsValidFunction(%q) = true, want false", f)
		}
	}
}

func sampleAssay(t *testing.T) ValidAssay {
	t.Helper()
	fwd, err := MkOligo("Fwd", FuncForwardPrimer, "AATACTAATC/mG/T")
	if err != nil {
		t.Fatalf("build fwd: %v", err)
	}
	probe, err := MkOligo("Probe", FuncProbe, "/56-FAM/ATGCATGC/3BHQ_1/")
	if err != nil {
		t.Fatalf("build probe: %v", err)
	}
	targets := AssayTargets{
		TgtTaxids:       []int{1128},
		OffTaxids:       []int{},
		RefAmpliconSrc:  GenbankAmplicon{Accession: "NC_012345.1", Start: 100, End: 128},
		RefAmpliconSeq:  "AATACTAATCGTTTTTTCCTACCCTAGAA",
		RefAmpliconSize: 29,
	}
	return WrapAssay(
		MkHeader("Test assay", "0.0.1", "Test group", "round-trip sample"),
		AssayOligos{OligoList: []Oligo{fwd, probe}},
		targets,
	)
}

// Round-trip stability: marshalling, unmarshalling, and re-marshalling must
// yield identical bytes. (Comparing bytes avoids nil-vs-empty-slice noise.)
func TestRoundTripJSON(t *testing.T) {
	a := sampleAssay(t)
	js1, err := ConvertJson(a)
	if err != nil {
		t.Fatalf("ConvertJson: %v", err)
	}
	back, err := UnwindJson(js1)
	if err != nil {
		t.Fatalf("UnwindJson: %v", err)
	}
	js2, err := ConvertJson(back)
	if err != nil {
		t.Fatalf("ConvertJson (2): %v", err)
	}
	if string(js1) != string(js2) {
		t.Errorf("JSON round trip not stable:\n first: %s\nsecond: %s", js1, js2)
	}
}

func TestRoundTripYAML(t *testing.T) {
	a := sampleAssay(t)
	y1, err := ConvertYaml(a)
	if err != nil {
		t.Fatalf("ConvertYaml: %v", err)
	}
	back, err := UnwindYaml(y1)
	if err != nil {
		t.Fatalf("UnwindYaml: %v", err)
	}
	y2, err := ConvertYaml(back)
	if err != nil {
		t.Fatalf("ConvertYaml (2): %v", err)
	}
	if string(y1) != string(y2) {
		t.Errorf("YAML round trip not stable:\n first: %s\nsecond: %s", y1, y2)
	}
}
