// founder-sim-bridge runs on a player's machine and lets the game server
// reach services only that machine can: a local model, a plugin under
// development, anything HTTP. It dials out and long-polls; nothing listens.
//
//	founder-sim-bridge -server https://sim.example.com -token <from Settings> \
//	    -service llm=http://localhost:11434/v1 \
//	    -service myplugin=http://localhost:9001
//
// Environment alternatives: FOUNDER_SIM_SERVER, FOUNDER_SIM_BRIDGE_TOKEN.
// This binary is deliberately separate from the game server and has no
// dependency on it beyond the wire protocol in package bridge.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/ramank775/founder-sim/bridge"
)

type serviceFlags map[string]string

func (s serviceFlags) String() string { return fmt.Sprint(map[string]string(s)) }
func (s serviceFlags) Set(v string) error {
	name, url, ok := strings.Cut(v, "=")
	name = strings.ToLower(strings.TrimSpace(name))
	url = strings.TrimSpace(url)
	if !ok || name == "" || url == "" {
		return fmt.Errorf("want name=url, got %q", v)
	}
	s[name] = url
	return nil
}

func main() {
	services := serviceFlags{}
	server := flag.String("server", os.Getenv("FOUNDER_SIM_SERVER"), "game server base URL")
	token := flag.String("token", os.Getenv("FOUNDER_SIM_BRIDGE_TOKEN"), "bridge token from Settings")
	concurrent := flag.Int("concurrency", 2, "max simultaneous local calls")
	flag.Var(services, "service", "name=url of a local service to expose (repeatable), e.g. llm=http://localhost:11434/v1")
	flag.Parse()

	if *server == "" || *token == "" || len(services) == 0 {
		flag.Usage()
		os.Exit(2)
	}
	for name, url := range services {
		log.Printf("exposing %s -> %s", name, url)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	c := &bridge.Client{Server: *server, Token: *token, Services: services, MaxConcurrent: *concurrent}
	log.Printf("connecting to %s", *server)
	if err := c.Run(ctx); err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
