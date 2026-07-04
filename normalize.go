package main

import (
	"encoding/json"
	"fmt"
)

// Coerce flexible model output into the canonical ReadingPlan — port of
// ncr/normalize.py. Models nest change units in chapters, use label/name for
// symbol, summary for overview, and omit file; this flattens all that and fills
// file from the block index. Runs before reconcile.

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func fillUnit(u *Unit, blocksByID map[string]Block) {
	if u.File == "" {
		for _, b := range u.Blocks {
			if blk, ok := blocksByID[b]; ok {
				u.File = blk.Path
				break
			}
		}
	}
}

func toUnit(ru rawUnit, id string, blocksByID map[string]Block) Unit {
	u := Unit{
		ID:          id,
		Blocks:      ru.Blocks,
		Symbol:      firstNonEmpty(ru.Symbol, ru.Label, ru.Name),
		Summary:     ru.Summary,
		Detail:      ru.Detail,
		Layer:       ru.Layer,
		LayerReason: ru.LayerReason,
		File:        firstNonEmpty(ru.File, ru.Path),
		Language:    ru.Language,
	}
	fillUnit(&u, blocksByID)
	return u
}

func normalizePlan(raw rawPlan, index Index) ReadingPlan {
	blocksByID := map[string]Block{}
	for _, b := range index.Blocks {
		blocksByID[b.BlockID] = b
	}

	var units []Unit
	unitIdx := map[string]int{}
	ensure := func(ru rawUnit, fallbackID string) string {
		id := ru.ID
		if id == "" {
			id = fallbackID
		}
		if idx, ok := unitIdx[id]; ok {
			fillUnit(&units[idx], blocksByID)
			return id
		}
		units = append(units, toUnit(ru, id, blocksByID))
		unitIdx[id] = len(units) - 1
		return id
	}

	for i, ru := range raw.Units { // already-flat units
		ensure(ru, fmt.Sprintf("u-%d", i))
	}

	var chapters []Chapter
	for ci, rc := range raw.Chapters {
		var nodes []Node
		if len(rc.Nodes) > 0 { // canonical node references
			for _, n := range rc.Nodes {
				if idx, ok := unitIdx[n.Unit]; ok {
					fillUnit(&units[idx], blocksByID)
				}
				nodes = append(nodes, n)
			}
		} else {
			inline := rc.ChangeUnits
			if len(inline) == 0 {
				inline = rc.Units
			}
			for j, cu := range inline {
				id := ensure(cu, fmt.Sprintf("u-c%d-%d", ci, j))
				nodes = append(nodes, Node{Unit: id, Depth: cu.Depth})
			}
		}
		chapters = append(chapters, Chapter{ID: rc.ID, Title: rc.Title, Summary: rc.Summary, Nodes: nodes})
	}

	var orphans []Orphan
	for _, ro := range raw.Orphans {
		var ids []string
		for _, item := range ro.Units {
			var s string
			if json.Unmarshal(item, &s) == nil {
				ids = append(ids, s)
				continue
			}
			var ru rawUnit
			if json.Unmarshal(item, &ru) == nil {
				ids = append(ids, ensure(ru, fmt.Sprintf("u-orphan-%d", len(units))))
			}
		}
		orphans = append(orphans, Orphan{Layer: ro.Layer, Units: ids})
	}

	for i := range units {
		fillUnit(&units[i], blocksByID)
	}

	edges := raw.Edges
	if edges == nil {
		edges = []Edge{}
	}
	return ReadingPlan{
		Title:    raw.Title,
		PRNumber: raw.PRNumber,
		Overview: firstNonEmpty(raw.Overview, raw.Summary),
		Chapters: chapters,
		Orphans:  orphans,
		Units:    units,
		Edges:    edges,
	}
}
