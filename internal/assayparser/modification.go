package assayparser

// NonBase marks a modification that contributes no base to the clean sequence
// (e.g. a 5'/3' fluorophore, a quencher, or a spacer). Base-acting mods instead
// store the single IUPAC base they stand in for.
const NonBase = "-"

// ModCatalogue is the fixed, in-package set of modifications the parser accepts.
// Every "/token/" marker in an oligo sequence must match a key here, otherwise
// MkOligo rejects it. ActsAsBase holds the base a modification stands in for in
// the clean sequence, or NonBase ("-") when it contributes no base.
//
// NOTE: this is seeded with common qPCR modifications. It is intentionally
// hard-coded for now; review and extend it before relying on it for real
// assays (the biology of each entry should be double-checked).
var ModCatalogue = map[string]ModTemplate{
	// 2'-O-methyl RNA bases — stand in for their DNA-equivalent base.
	"mA": {Content: "mA", Details: "2'-O-methyl-A", ActsAsBase: "A"},
	"mC": {Content: "mC", Details: "2'-O-methyl-C", ActsAsBase: "C"},
	"mG": {Content: "mG", Details: "2'-O-methyl-G", ActsAsBase: "G"},
	"mU": {Content: "mU", Details: "2'-O-methyl-U", ActsAsBase: "T"},

	// 5' fluorophores — labels, not bases.
	"56-FAM": {Content: "56-FAM", Details: "5' 6-FAM (fluorescein)", ActsAsBase: NonBase},
	"5HEX":   {Content: "5HEX", Details: "5' HEX", ActsAsBase: NonBase},

	// Quenchers — not bases.
	"3BHQ_1":  {Content: "3BHQ_1", Details: "3' Black Hole Quencher 1", ActsAsBase: NonBase},
	"3IABkFQ": {Content: "3IABkFQ", Details: "3' Iowa Black FQ", ActsAsBase: NonBase},
	"ZEN":     {Content: "ZEN", Details: "internal ZEN quencher", ActsAsBase: NonBase},

	// Spacers — not bases.
	"iSpC3": {Content: "iSpC3", Details: "internal C3 spacer", ActsAsBase: NonBase},
}
