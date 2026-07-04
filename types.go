// Package main is `ncr` — narrative, outside-in code review. It reorders a PR
// diff into a call-path reading order and explains it (see README + docs/). A
// deterministic indexer + reconciler guarantee every change block is placed; the
// LLM only reorders and narrates. Port of the original Python implementation.
package main

import "encoding/json"

// ---- canonical reading plan (docs/schema.md) ----

type ReadingPlan struct {
	Title    string    `json:"title"`
	PRNumber int       `json:"prNumber,omitempty"`
	Overview string    `json:"overview"`
	Chapters []Chapter `json:"chapters"`
	Orphans  []Orphan  `json:"orphans"`
	Units    []Unit    `json:"units"`
	Edges    []Edge    `json:"edges"`
	Coverage *Coverage `json:"coverage,omitempty"`
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
	Language    string   `json:"language,omitempty"`
	Symbol      string   `json:"symbol"`
	Layer       *int     `json:"layer"`
	LayerReason string   `json:"layerReason,omitempty"`
	Uncertain   bool     `json:"uncertain,omitempty"`
	Summary     string   `json:"summary"`
	Detail      string   `json:"detail,omitempty"`
	References  []string `json:"references,omitempty"`
	Blocks      []string `json:"blocks"`
}

type Edge struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Resolved bool   `json:"resolved"`
}

type Coverage struct {
	OK     bool `json:"ok"`
	Counts struct {
		Indexed int `json:"indexed"`
		Placed  int `json:"placed"`
	} `json:"counts"`
	Missing       []string `json:"missing"`
	Duplicated    []string `json:"duplicated"`
	UnplacedUnits []string `json:"unplacedUnits"`
	DanglingRefs  []string `json:"danglingRefs"`
	CommentGaps   []string `json:"commentGaps"`
	Repaired      bool     `json:"repaired"`
}

// ---- block index (block-index.json) ----

type Index struct {
	Blocks   []Block  `json:"blocks"`
	BlockIDs []string `json:"blockIds"`
}

type Block struct {
	BlockID       string   `json:"blockId"`
	Path          string   `json:"path"`
	ChangeType    string   `json:"changeType"`
	OldStart      *int     `json:"oldStart"`
	OldLines      int      `json:"oldLines"`
	NewStart      *int     `json:"newStart"`
	NewLines      int      `json:"newLines"`
	Header        string   `json:"header"`
	Text          string   `json:"text"`
	Sha           string   `json:"sha"`
	ContextBefore []string `json:"contextBefore"`
	ContextAfter  []string `json:"contextAfter"`
}

// ---- raw model output (flexible; normalized into ReadingPlan) ----

type rawPlan struct {
	Title    string       `json:"title"`
	Overview string       `json:"overview"`
	Summary  string       `json:"summary"`
	Chapters []rawChapter `json:"chapters"`
	Orphans  []rawOrphan  `json:"orphans"`
	Units    []rawUnit    `json:"units"`
	Edges    []Edge       `json:"edges"`
	PRNumber int          `json:"prNumber"`
}

type rawChapter struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	Nodes       []Node    `json:"nodes"`
	ChangeUnits []rawUnit `json:"changeUnits"`
	Units       []rawUnit `json:"units"`
}

type rawOrphan struct {
	Layer       int               `json:"layer"`
	Units       []json.RawMessage `json:"units"` // string ids or inline unit objects
	ChangeUnits []rawUnit         `json:"changeUnits"`
}

type rawUnit struct {
	ID          string          `json:"id"`
	Blocks      []string        `json:"blocks"`
	Symbol      string          `json:"symbol"`
	Label       string          `json:"label"`
	Name        string          `json:"name"`
	Summary     string          `json:"summary"`
	Detail      string          `json:"detail"`
	Layer       *int            `json:"layer"`
	LayerReason string          `json:"layerReason"`
	File        string          `json:"file"`
	Path        string          `json:"path"`
	Language    string          `json:"language"`
	References  json.RawMessage `json:"references"` // shape varies (strings or objects); unused downstream
	Depth       int             `json:"depth"`
}

// ---- ingest (gh) ----

type PRContext struct {
	Diff     string            `json:"diff"`
	Meta     Meta              `json:"meta"`
	Files    map[string]string `json:"files"`
	Comments []Comment         `json:"comments"`
}

type Meta struct {
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Number     int        `json:"number"`
	HeadRefOid string     `json:"headRefOid"`
	Files      []FileMeta `json:"files"`
}

type FileMeta struct {
	Path string `json:"path"`
}

type Comment struct {
	Path         string `json:"path"`
	Line         *int   `json:"line"`
	OriginalLine *int   `json:"original_line"`
}
