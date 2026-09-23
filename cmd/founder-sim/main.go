// founder-sim is the game server.
//
//	founder-sim serve [-addr :8080] [-data ./data] [-content ./content] [-dev]
//
// Players who run a model on their own machine use the separate
// founder-sim-bridge binary (cmd/founder-sim-bridge); the game only ever
// sees "bridge://llm" as their model's base URL.
//
// Environment:
//
//	FOUNDER_SIM_MASTER_KEY   required for serve; encrypts player API keys at rest
//	FOUNDER_SIM_BASE_URL     public URL used in magic links (default http://localhost:8080)
//	FOUNDER_SIM_FAKE_LLM=1   use the built-in fake model (no key needed) for development
//	FOUNDER_SIM_PLUGINS      comma-separated URLs of remote HTTP plugins to register
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ramank775/founder-sim/bridge"
	"github.com/ramank775/founder-sim/internal/auth"
	"github.com/ramank775/founder-sim/internal/game"
	"github.com/ramank775/founder-sim/internal/llm"
	"github.com/ramank775/founder-sim/internal/store"
	"github.com/ramank775/founder-sim/internal/web"
	"github.com/ramank775/founder-sim/kb"
	"github.com/ramank775/founder-sim/plugin"
	"github.com/ramank775/founder-sim/plugin/httpadapter"
	"github.com/ramank775/founder-sim/plugins/notepad"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "serve":
		serve(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: founder-sim serve [-addr :8080] [-data ./data] [-content ./content] [-dev]")
}

func serve(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "listen address")
	data := fs.String("data", "./data", "data directory")
	content := fs.String("content", "./content", "content directory, or a bundle compiled by founder-sim-content")
	dev := fs.Bool("dev", false, "dev mode: show magic links and raw model output in the UI")
	_ = fs.Parse(args)

	master := os.Getenv("FOUNDER_SIM_MASTER_KEY")
	if master == "" {
		if !*dev {
			log.Fatal("FOUNDER_SIM_MASTER_KEY is required (any string of 16+ chars)")
		}
		master = "dev-only-master-key-not-for-real"
		log.Println("dev: using a fixed master key; do not run like this outside your machine")
	}
	cipher, err := auth.NewCipher(master)
	if err != nil {
		log.Fatal(err)
	}

	st, err := store.OpenFileStore(*data)
	if err != nil {
		log.Fatal(err)
	}
	defer st.Close()

	base, err := kb.Load(*content)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("content %s v%s (as of %s, hash %s): %d facts, %d risks, %d friction, %d scenes",
		base.B.Manifest.Name, base.B.Manifest.Version, base.B.Manifest.AsOf, base.B.Hash,
		len(base.B.Facts), len(base.B.Risks), len(base.B.Friction), len(base.B.Scenes))

	reg := plugin.NewRegistry()
	ctx := context.Background()
	// Built-ins. Same interface as remote ones; just no network.
	mustRegister(ctx, reg, notepad.New())
	// Remote plugins from the environment.
	for _, u := range strings.Split(os.Getenv("FOUNDER_SIM_PLUGINS"), ",") {
		if u = strings.TrimSpace(u); u != "" {
			mustRegister(ctx, reg, httpadapter.NewClient(u))
		}
	}
	// Plugin content packs merge into the KB.
	for _, m := range reg.Manifests() {
		if !m.Has(plugin.CapContent) {
			continue
		}
		p, _, _ := reg.Get(m.Name)
		pack, err := p.Content(ctx)
		if err != nil {
			log.Printf("plugin %s content: %v", m.Name, err)
			continue
		}
		base.Merge(pack)
	}

	newClient := func(cfg llm.Config) llm.Client { return llm.NewOpenAICompat(cfg) }
	if os.Getenv("FOUNDER_SIM_FAKE_LLM") == "1" {
		log.Println("using the fake model: no real inference")
		newClient = func(llm.Config) llm.Client { return &llm.Fake{} }
	}

	baseURL := os.Getenv("FOUNDER_SIM_BASE_URL")
	if baseURL == "" {
		host, port, err := net.SplitHostPort(*addr)
		if err != nil {
			log.Fatalf("bad -addr %q: %v", *addr, err)
		}
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "localhost"
		}
		baseURL = "http://" + net.JoinHostPort(host, port)
	}
	a := &auth.Auth{Store: st, Sender: auth.LogSender{}, BaseURL: baseURL, DevEcho: *dev}
	g := &game.Service{Store: st, KB: base, Plugins: reg, NewClient: newClient}
	srv, err := web.New(a, g, cipher, bridge.NewHub(), *dev)
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("founder-sim listening on %s (data=%s, plugins=%d, dev=%v)", *addr, *data, len(reg.Manifests()), *dev)
	hs := &http.Server{Addr: *addr, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(hs.ListenAndServe())
}

func mustRegister(ctx context.Context, reg *plugin.Registry, p plugin.Plugin) {
	m, err := reg.Register(ctx, p)
	if err != nil {
		log.Fatalf("plugin: %v", err)
	}
	log.Printf("plugin %s v%s: %v", m.Name, m.Version, m.Capabilities)
}
