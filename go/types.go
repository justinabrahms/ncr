// Package main is a render-parity skeleton for the planned Go port (ADR 001).
// It consumes reading-plan.json + block-index.json (produced by the Python
// pipeline) and emits the review HTML, to prove chroma + html/template match the
// Python renderer before committing to the full port. Ingest/plan/LLM/cache are
// intentionally out of scope here.
package main

// ReadingPlan mirrors docs/schema.md (the fields the renderer needs).
type ReadingPlan struct {
	Title    string    `json:"title"`
	PRNumber int       `json:"prNumber"`
	Overview string    `json:"overview"`
	Chapters []Chapter `json:"chapters"`
	Orphans  []Orphan  `json:"orphans"`
	Units    []Unit    `json:"units"`
	Coverage Coverage  `json:"coverage"`
}

type Chapter struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Nodes   []Node `json:"nodes"`
}

type Node struct {
	Unit  string `json:"unit"`
	Depth int    `json:"depth"`
}

type Orphan struct {
	Layer int      `json:"layer"`
	Units []string `json:"units"`
}

type Unit struct {
	ID          string   `json:"id"`
	File        string   `json:"file"`
	Language    string   `json:"language"`
	Symbol      string   `json:"symbol"`
	Layer       *int     `json:"layer"`
	LayerReason string   `json:"layerReason"`
	Summary     string   `json:"summary"`
	Detail      string   `json:"detail"`
	Blocks      []string `json:"blocks"`
}

type Coverage struct {
	OK     bool `json:"ok"`
	Counts struct {
		Indexed int `json:"indexed"`
		Placed  int `json:"placed"`
	} `json:"counts"`
}

// Index mirrors block-index.json.
type Index struct {
	Blocks []Block `json:"blocks"`
}

type Block struct {
	BlockID       string   `json:"blockId"`
	Path          string   `json:"path"`
	Text          string   `json:"text"`
	ContextBefore []string `json:"contextBefore"`
	ContextAfter  []string `json:"contextAfter"`
}
