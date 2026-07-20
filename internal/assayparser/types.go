package assayparser

// ----- wrapper struct

type ValidAssay struct {
	Header  AssayHeader  `json:"header" yaml:"header"`
	Oligos  AssayOligos  `json:"oligos" yaml:"oligos"`
	Targets AssayTargets `json:"targets" yaml:"targets"`
}

// ----- high level structs

type AssayHeader struct {
	Name        string `json:"name" yaml:"name"`
	Version     string `json:"version" yaml:"version"`
	Author      string `json:"author" yaml:"author"`
	Description string `json:"description" yaml:"description"`
}

type AssayOligos struct {
	OligoList []Oligo `json:"oligoList" yaml:"oligoList"`
}

type AssayTargets struct {
	TgtTaxids       []int           `json:"tgtTaxids" yaml:"tgtTaxids"`
	OffTaxids       []int           `json:"offTaxids" yaml:"offTaxids"`
	RefAmpliconSrc  GenbankAmplicon `json:"refAmpliconSrc" yaml:"refAmpliconSrc"`
	RefAmpliconSeq  string          `json:"refAmpliconSeq" yaml:"refAmpliconSeq"`
	RefAmpliconSize int             `json:"refAmpliconSize" yaml:"refAmpliconSize"`
	SearchString    []string        `json:"searchString" yaml:"searchString"`
}

// ----- element structs

type Oligo struct {
	Name      string         `json:"name" yaml:"name"`
	Function  string         `json:"function" yaml:"function"`
	SeqActual string         `json:"seqActual" yaml:"seqActual"`
	SeqClean  string         `json:"seqClean" yaml:"seqClean"`
	Mods      []Modification `json:"mods" yaml:"mods"`
}

type Modification struct {
	Id         int    `json:"id" yaml:"id"`
	Pos        int    `json:"pos" yaml:"pos"`
	Content    string `json:"content" yaml:"content"`
	ActsAsBase string `json:"actsAsBase" yaml:"actsAsBase"`
}

type ModTemplate struct {
	Content    string `json:"content" yaml:"content"`
	Details    string `json:"details" yaml:"details"`
	ActsAsBase string `json:"actsAsBase" yaml:"actsAsBase"`
}

type GenbankAmplicon struct {
	Accession string `json:"accession" yaml:"accession"`
	Start     int    `json:"start" yaml:"start"`
	End       int    `json:"end" yaml:"end"`
}
