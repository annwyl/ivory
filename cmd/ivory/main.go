package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/annwyl/ivory"
	_ "github.com/annwyl/ivory/crawlers"
	_ "github.com/annwyl/ivory/stores"
	"github.com/annwyl/ivory/tui"
)

func main() {
	configPath := flag.String("config", "config.json", "path to the config file")
	plain := flag.Bool("plain", false, "use the plain line shell instead of the tui")
	query := flag.String("query", "", "search stored records containing this text, then exit")
	export := flag.Bool("export", false, "dump all stored records as jsonl, then exit")
	limit := flag.Int("limit", 50, "max records to return for -query")
	flag.Parse()

	config, err := ivory.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("failed to load config: %v\n", err)
		os.Exit(1)
	}

	engine, err := ivory.NewEngine(config)
	if err != nil {
		fmt.Printf("failed to create engine: %v\n", err)
		os.Exit(1)
	}
	defer engine.Close()

	switch {
	case *export || *query != "":
		runQuery(engine, *query, *export, *limit)
	case *plain:
		runPlain(engine)
	default:
		engine.SetConsole(false)
		if err := tui.Run(engine, *configPath); err != nil {
			fmt.Printf("tui error: %v\n", err)
		}
	}
}

func runQuery(engine *ivory.Engine, term string, export bool, limit int) {
	if export {
		term, limit = "", 0
	}
	records, err := engine.Query(term, limit)
	if err != nil {
		fmt.Printf("query failed: %v\n", err)
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	for _, record := range records {
		enc.Encode(record)
	}
}

func runPlain(engine *ivory.Engine) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shellDone := make(chan struct{})
	go func() {
		ivory.RunShell(engine)
		close(shellDone)
	}()

	select {
	case <-ctx.Done():
		fmt.Println("\ngot signal, shutting down")
	case <-shellDone:
	}
}
