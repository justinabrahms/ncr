package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	out := flag.String("o", "out/review-go.html", "output HTML path")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ncr-render [-o out.html] reading-plan.json block-index.json")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}

	var plan ReadingPlan
	if err := readJSON(flag.Arg(0), &plan); err != nil {
		die(err)
	}
	var index Index
	if err := readJSON(flag.Arg(1), &index); err != nil {
		die(err)
	}

	html, err := BuildHTML(plan, index)
	if err != nil {
		die(err)
	}
	if err := os.WriteFile(*out, html, 0o644); err != nil {
		die(err)
	}
	fmt.Fprintf(os.Stderr, "› wrote %s (%d bytes)\n", *out, len(html))
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func die(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
