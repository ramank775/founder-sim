// notepad-plugin runs the built-in notepad as a standalone HTTP plugin.
// It exists to prove the remote path: start it, then run the server with
// FOUNDER_SIM_PLUGINS=http://localhost:9001 and register it under another
// name is not needed; the engine treats it exactly like the built-in.
//
// This is also the template for writing your own plugin in Go: implement
// plugin.Plugin, call httpadapter.ListenAndServe.
package main

import (
	"flag"
	"log"

	"github.com/ramank775/founder-sim/plugin/httpadapter"
	"github.com/ramank775/founder-sim/plugins/notepad"
)

func main() {
	addr := flag.String("addr", ":9001", "listen address")
	flag.Parse()
	log.Printf("notepad plugin listening on %s", *addr)
	log.Fatal(httpadapter.ListenAndServe(*addr, notepad.New()))
}
