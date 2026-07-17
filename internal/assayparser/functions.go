package assayparser

import (
	"fmt"
	"strings"
)

var iupac = []byte{'A', 'T', 'G', 'C', 'R', 'Y', 'S', 'W', 'K', 'M', 'B', 'D', 'H', 'V', 'N', 'I'}

// ----- oligo function roles

// Oligo function roles recognised by the downstream analysis pipeline. An oligo
// must carry one of these values to be usable in a run. MkOligo itself does not
// enforce this (the full assay-validation rules are still being finalised); use
// IsValidFunction where you need to check it.
const (
	FuncForwardPrimer = "forward-primer"
	FuncReversePrimer = "reverse-primer"
	FuncProbe         = "probe"
)

// IsValidFunction reports whether f is one of the recognised oligo roles.
func IsValidFunction(f string) bool {
	switch f {
	case FuncForwardPrimer, FuncReversePrimer, FuncProbe:
		return true
	default:
		return false
	}
}

// ----- type infrastructure

func MkHeader(n string, v string, a string) AssayHeader {
	return AssayHeader{
		Name:    n,
		Version: v,
		Author:  a,
	}
}

func MkOligos() AssayOligos {
	return AssayOligos{}
}

func MkTargets() AssayTargets {
	return AssayTargets{}
}

func WrapAssay(h AssayHeader, o AssayOligos, t AssayTargets) ValidAssay {
	return ValidAssay{
		Header:  h,
		Oligos:  o,
		Targets: t,
	}
}

// ----- working with assays

func isValidBase(x byte) bool {
	for _, letter := range iupac {
		if x == letter {
			return true
		}
	}
	return false
}

// resolveMod validates one modification token against ModCatalogue and appends
// the corresponding Modification to mods. basesBefore is the number of clean
// (base) characters already emitted for the oligo.
//
// Position semantics (1-based, in clean-sequence coordinates):
//   - a base-acting mod occupies the next clean position, so Pos = basesBefore+1
//   - a non-base mod (label/quencher/spacer) is anchored after the bases seen so
//     far, so Pos = basesBefore (0 means the 5' end, len(clean) means the 3' end)
//
// It returns the base to append to the clean sequence and true for base-acting
// mods; for non-base mods it returns (0, false). Unknown tokens are rejected.
func resolveMod(id int, basesBefore int, token string, mods *[]Modification) (byte, bool, error) {
	tmpl, ok := ModCatalogue[token]
	if !ok {
		return 0, false, fmt.Errorf("unknown modification %q", token)
	}

	if tmpl.ActsAsBase == NonBase {
		*mods = append(*mods, Modification{
			Id:         id,
			Pos:        basesBefore,
			Content:    token,
			ActsAsBase: NonBase,
		})
		return 0, false, nil
	}

	if len(tmpl.ActsAsBase) != 1 || !isValidBase(tmpl.ActsAsBase[0]) {
		return 0, false, fmt.Errorf("modification %q maps to invalid base %q in catalogue", token, tmpl.ActsAsBase)
	}

	*mods = append(*mods, Modification{
		Id:         id,
		Pos:        basesBefore + 1,
		Content:    token,
		ActsAsBase: tmpl.ActsAsBase,
	})
	return tmpl.ActsAsBase[0], true, nil
}

// MkOligo parses a sequence written with inline modification markers of the
// form "/token/" and builds an Oligo. It produces:
//
//   - SeqActual: the input with whitespace removed, markers retained
//   - SeqClean:  the pure IUPAC base sequence fed to analysis (mods that act as
//     a base contribute that base; non-base mods contribute nothing)
//   - Mods:      one entry per marker (see resolveMod for Pos semantics)
//
// Bases must be uppercase IUPAC symbols. Unknown modifications, unterminated
// markers, and any other character are rejected with an error.
func MkOligo(name string, function string, seq string) (Oligo, error) {
	mods := []Modification{}
	clean := make([]byte, 0, len(seq))
	var actual strings.Builder
	modCount := 0

	for i := 0; i < len(seq); i++ {
		c := seq[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// whitespace is ignored so users can format input for readability
			continue
		case c == '/':
			rel := strings.IndexByte(seq[i+1:], '/')
			if rel < 0 {
				return Oligo{}, fmt.Errorf("unterminated modification marker at position %d", i)
			}
			end := i + 1 + rel // absolute index of the closing '/'
			token := seq[i+1 : end]
			modCount++
			base, isBase, err := resolveMod(modCount, len(clean), token, &mods)
			if err != nil {
				return Oligo{}, err
			}
			if isBase {
				clean = append(clean, base)
			}
			actual.WriteString(seq[i : end+1])
			i = end
		case isValidBase(c):
			clean = append(clean, c)
			actual.WriteByte(c)
		default:
			return Oligo{}, fmt.Errorf("invalid character %q at position %d in sequence", string(c), i)
		}
	}

	return Oligo{
		Name:      name,
		Function:  function,
		SeqActual: actual.String(),
		SeqClean:  string(clean),
		Mods:      mods,
	}, nil
}
