package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/FenkoHQ/vulpes-core/internal/config"
)

func main() {
	configPath := flag.String("config", "gateway.yaml", "path to gateway YAML config")
	flag.Parse()
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("plugins: %d\n", len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		fmt.Printf("- %s (%s): %v\n", p.Name, p.Source.Type, p.Capabilities)
	}
}
