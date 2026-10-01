// Local administrator-only export; no public endpoint exposes feedback contacts.
package main

import (
	"chanzhushui/internal/history"
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"time"
)

func main() {
	db := flag.String("db", ".local/history.db", "service database")
	out := flag.String("out", "", "new private JSON destination (required)")
	flag.Parse()
	if *out == "" {
		log.Fatal("-out is required")
	}
	s, e := history.Open(*db)
	if e != nil {
		log.Fatal(e)
	}
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, e := s.ListFeedback(ctx)
	if e != nil {
		log.Fatal(e)
	}
	raw, e := json.MarshalIndent(rows, "", "  ")
	if e != nil {
		log.Fatal(e)
	}
	f, e := os.OpenFile(*out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		log.Fatal(e)
	}
	if _, e = f.Write(raw); e != nil {
		f.Close()
		log.Fatal(e)
	}
	if e = f.Close(); e != nil {
		log.Fatal(e)
	}
	log.Printf("exported %d feedback records to private file", len(rows))
}
