package web

import (
	"testing"

	"AssayManager/internal/assayparser"
)

// The structured form only exposes some target fields; build() must overwrite
// the exposed ones from the form while preserving the rest from Base.
func TestStructuredBuildPreservesUnexposedFields(t *testing.T) {
	base := assayparser.ValidAssay{
		Header: assayparser.AssayHeader{Name: "Old", Author: "old", Version: "v0.3"},
		Targets: assayparser.AssayTargets{
			TgtTaxids:       []int{1},
			OffTaxids:       []int{999},
			RefAmpliconSeq:  "OLD",
			RefAmpliconSize: 42,
			RefAmpliconSrc:  assayparser.GenbankAmplicon{Accession: "ACC1", Start: 10, End: 20},
			SearchString:    []string{"query1"},
		},
	}
	bjson, err := assayparser.ConvertJson(base)
	if err != nil {
		t.Fatal(err)
	}

	f := structuredForm{
		Name: "New", Author: "me", Description: "d",
		Oligos:         []oligoRow{{Name: "F", Function: "forward-primer", Seq: "ATGC"}},
		TgtTaxids:      "562, 11320",
		RefAmpliconSeq: "NEWAMP",
		Base:           string(bjson),
	}
	a, err := f.build()
	if err != nil {
		t.Fatal(err)
	}

	// Exposed fields overwritten from the form.
	if a.Header.Name != "New" || a.Header.Author != "me" || a.Header.Description != "d" {
		t.Errorf("header not overwritten: %+v", a.Header)
	}
	if a.Header.Version != "" {
		t.Errorf("version should be cleared, got %q", a.Header.Version)
	}
	if len(a.Oligos.OligoList) != 1 || a.Oligos.OligoList[0].Name != "F" {
		t.Errorf("oligos not built from form: %+v", a.Oligos.OligoList)
	}
	if len(a.Targets.TgtTaxids) != 2 || a.Targets.TgtTaxids[0] != 562 || a.Targets.TgtTaxids[1] != 11320 {
		t.Errorf("taxids not overwritten: %v", a.Targets.TgtTaxids)
	}
	if a.Targets.RefAmpliconSeq != "NEWAMP" {
		t.Errorf("amplicon not overwritten: %q", a.Targets.RefAmpliconSeq)
	}

	// Unexposed fields preserved from Base.
	if len(a.Targets.OffTaxids) != 1 || a.Targets.OffTaxids[0] != 999 {
		t.Errorf("offTaxids not preserved: %v", a.Targets.OffTaxids)
	}
	if a.Targets.RefAmpliconSize != 42 {
		t.Errorf("refAmpliconSize not preserved: %d", a.Targets.RefAmpliconSize)
	}
	if a.Targets.RefAmpliconSrc.Accession != "ACC1" {
		t.Errorf("refAmpliconSrc not preserved: %+v", a.Targets.RefAmpliconSrc)
	}
	if len(a.Targets.SearchString) != 1 || a.Targets.SearchString[0] != "query1" {
		t.Errorf("searchString not preserved: %v", a.Targets.SearchString)
	}
}

func TestParseTaxids(t *testing.T) {
	ok := []struct {
		in   string
		want []int
	}{
		{"", []int{}},
		{"562", []int{562}},
		{"562, 11320", []int{562, 11320}},
		{" 1 , 2 ,3 ", []int{1, 2, 3}},
	}
	for _, c := range ok {
		got, err := parseTaxids(c.in)
		if err != nil {
			t.Errorf("parseTaxids(%q) unexpected error: %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("parseTaxids(%q) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("parseTaxids(%q) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
	for _, bad := range []string{"562, x", "0", "-3", "abc"} {
		if _, err := parseTaxids(bad); err == nil {
			t.Errorf("parseTaxids(%q) expected an error", bad)
		}
	}
}
