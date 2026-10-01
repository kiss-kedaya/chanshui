package main

import (
	"chanzhushui/internal/api"
	"chanzhushui/internal/history"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8770", "local listen address")
	db := flag.String("db", ".local/history.db", "local SQLite path")
	web := flag.String("web", "web/out-build", "Next.js static export directory")
	allowPrivate := flag.Bool("allow-private", false, "allow private gateway targets for local mock testing")
	workers := flag.Int("workers", 16, "maximum concurrently executing jobs")
	queue := flag.Int("queue", 128, "bounded waiting jobs")
	upstream := flag.Int("upstream-concurrency", 64, "global upstream request cap")
	flag.Parse()
	host, _, e := net.SplitHostPort(*addr)
	if e != nil || (host != "" && net.ParseIP(host) == nil) {
		log.Fatal("invalid listen address")
	}
	if _, e := os.Stat(filepath.Join(*web, "index.html")); e != nil {
		log.Fatal("missing web export: run scripts/start-local.ps1")
	}
	store, e := history.Open(*db)
	if e != nil {
		log.Fatal(e)
	}
	defer store.Close()
	apiServer, e := api.New(store, api.Options{AutoSync: true, AllowPrivate: *allowPrivate, Workers: *workers, QueueSize: *queue, MaxRequests: *upstream})
	if e != nil {
		log.Fatal(e)
	}
	defer apiServer.Close()
	app := apiServer.Handler()
	static := http.FileServer(http.Dir(*web))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			app.ServeHTTP(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.URL.Path == "/history" {
			copy := r.Clone(r.Context())
			copy.URL.Path = "/history.html"
			static.ServeHTTP(w, copy)
			return
		}
		static.ServeHTTP(w, r)
	})
	srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	fmt.Printf("掺水？本地检测站 http://%s\n", *addr)
	if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
