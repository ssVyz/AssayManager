package assayparser


var iupac = []byte{'A', 'T', 'G', 'C', 'R', 'Y', 'S', 'W', 'K', 'M', 'B', 'D', 'H', 'V', 'N', 'I'}

// Type infrastructure

func MkHeader(n string, v string, a string) AssayHeader {
	new := AssayHeader{
		Name: n,
		Version: v,
		Author: a,
	}
	return new
}

func MkOligos() AssayOligos {
	var new AssayOligos
	return new
}

func MkTargets() AssayTargets {
	var new AssayTargets
	return new
}

func WrapAssay(h AssayHeader, o AssayOligos, t AssayTargets) ValidAssay {
	new := ValidAssay{
		Header: h,
		Oligos: o,
		Targets: t,
	}
	return new
}


// working with assays

func isValidBase(x byte) bool {
	valid := false
	for _, letter := range iupac {
		if x == letter {valid = true; break;}
	}
	return valid
}


func resolveMod(count int, pos int, s string, m *[]Modification) byte {
	// unfinished
	implMod := Modification{
		Id: count,
		Pos: pos,
		Content: "",
		IsBase: ModCatalogue[s].IsBase,
	}
	*m = append(*m, implMod)
	return ModCatalogue[s].IsBase[0]
}


func stripWhites(s string) string {
	newString := ""
	for _, letter := range s {
		if letter != ' ' {
			newString += string(letter)
		}
	}
	return newString
}


func MkOligo(name string, function string, seq string) Oligo {

	newMods := []Modification{}

	var cleanSeqSlice = []byte{}

	modCount := 0
	for i := 0; i < len(seq); i++ {
		current := seq[i]
		if isValidBase(current) {
			cleanSeqSlice = append(cleanSeqSlice, current)
		} else if current == '/' {
			modCount++
			for j := i+1; j< len(seq); j++ {
				if seq[j] == '/' {
					modContent := seq[i+1:j]
					modPos := i
					cleanSeqSlice = append(cleanSeqSlice, resolveMod(modCount, modPos, modContent, &newMods))
					i = j
					break
				}
			}
		}
	}

	cleanSeq := string(cleanSeqSlice)

	stripSeq := stripWhites(seq)

	new := Oligo{
		Name: name,
		Function: function,
		SeqActual: stripSeq,
		SeqClean: cleanSeq,
		Mods: newMods,
	}

	return new
}
