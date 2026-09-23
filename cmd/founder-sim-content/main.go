// founder-sim-content is the publishing step for game content. Content is
// data, kept apart from the engine so story and research can ship on their
// own cadence.
//
//	founder-sim-content validate ./content
//	founder-sim-content build    ./content -o content.bundle.json
//	founder-sim-content stats    ./content
//	founder-sim-content ingest   ./content -story my-story.md [-slug klixa]
//
// ingest is the story pipeline: a founder's story in any form (voice
// transcript, notes, chat dump) becomes scenes, risks, friction templates,
// archetypes and facts in the content tree, all marked as anecdotal
// (estimated/invented) with research to-dos so agents can confirm them.
// It needs a model: FOUNDER_SIM_LLM_BASE_URL, FOUNDER_SIM_LLM_MODEL,
// FOUNDER_SIM_LLM_API_KEY.
//
// build validates, sorts, hashes and writes one bundle the server loads with
// `founder-sim serve -content content.bundle.json`. This is also where a
// retrieval index (embeddings) will be computed and stored in the bundle's
// "index" field, so the engine never has to do it at runtime.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ramank775/founder-sim/internal/ingest"
	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/kb"
)

func main() {
	if len(os.Args) < 3 {
		usage()
		os.Exit(2)
	}
	cmd, dir := os.Args[1], os.Args[2]
	switch cmd {
	case "validate":
		b, err := kb.LoadDir(dir)
		if err != nil {
			fail(err)
		}
		fmt.Printf("ok: %s v%s (as of %s) hash %s\n", b.Manifest.Name, b.Manifest.Version, b.Manifest.AsOf, kb.Compile(b).Hash)
	case "build":
		fs := flag.NewFlagSet("build", flag.ExitOnError)
		out := fs.String("o", "content.bundle.json", "output bundle path")
		_ = fs.Parse(os.Args[3:])
		b, err := kb.LoadDir(dir)
		if err != nil {
			fail(err)
		}
		kb.Compile(b)
		raw, _ := json.MarshalIndent(b, "", " ")
		if err := os.WriteFile(*out, raw, 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("wrote %s: %s v%s hash %s (%d facts, %d risks, %d friction, %d archetypes, %d scenes)\n",
			*out, b.Manifest.Name, b.Manifest.Version, b.Hash, len(b.Facts), len(b.Risks), len(b.Friction), len(b.Archetypes), len(b.Scenes))
	case "stats":
		b, err := kb.LoadDir(dir)
		if err != nil {
			fail(err)
		}
		prov := map[string]int{}
		for _, f := range b.Facts {
			prov["fact/"+f.Provenance]++
		}
		for _, r := range b.Risks {
			prov["risk/"+r.Provenance]++
		}
		for _, f := range b.Friction {
			prov["friction/"+f.Provenance]++
		}
		fmt.Printf("%s v%s as of %s\n", b.Manifest.Name, b.Manifest.Version, b.Manifest.AsOf)
		fmt.Printf("facts %d, risks %d, friction %d, archetypes %d, scenes %d\n", len(b.Facts), len(b.Risks), len(b.Friction), len(b.Archetypes), len(b.Scenes))
		for k, v := range prov {
			fmt.Printf("  %-20s %d\n", k, v)
		}
		fmt.Println("replace invented/estimated records with sourced ones; see docs/STORY_AND_RESEARCH.md §B")
	case "ingest":
		fs := flag.NewFlagSet("ingest", flag.ExitOnError)
		story := fs.String("story", "", "path to the raw story (any text); '-' reads stdin")
		slug := fs.String("slug", "", "short name for this story (default: from the file name)")
		_ = fs.Parse(os.Args[3:])
		if *story == "" {
			fail(fmt.Errorf("-story is required"))
		}
		var raw []byte
		var err error
		if *story == "-" {
			raw, err = readAll(os.Stdin)
		} else {
			raw, err = os.ReadFile(*story)
		}
		if err != nil {
			fail(err)
		}
		if *slug == "" {
			*slug = strings.TrimSuffix(filepath.Base(*story), filepath.Ext(*story))
		}
		cfg, err := llm.ConfigFromEnv()
		if err != nil {
			fail(err)
		}
		p := &ingest.Pipeline{C: llm.NewOpenAICompat(cfg), ContentDir: dir}
		fmt.Printf("ingesting %q with %s @ %s ...\n", *slug, cfg.Model, cfg.BaseURL)
		res, err := p.Run(context.Background(), *slug, string(raw))
		if err != nil {
			if res != nil && res.Raw[1] != "" {
				fmt.Fprintln(os.Stderr, "--- extraction output ---")
				fmt.Fprintln(os.Stderr, res.Raw[1])
			}
			fail(err)
		}
		fmt.Print(res.Report)
	default:
		usage()
		os.Exit(2)
	}
}

func readAll(f *os.File) ([]byte, error) {
	var b strings.Builder
	buf := make([]byte, 64<<10)
	for {
		n, err := f.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return []byte(b.String()), nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: founder-sim-content validate|build|stats|ingest <content-dir> [-o bundle.json] [-story file -slug name]")
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
