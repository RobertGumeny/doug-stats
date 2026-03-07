package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"time"

	"github.com/robertgumeny/doug-stats/aggregator"
	"github.com/robertgumeny/doug-stats/api"
	"github.com/robertgumeny/doug-stats/provider"
	claudeprovider "github.com/robertgumeny/doug-stats/provider/claude"
)

//go:embed all:frontend/dist
var frontendDist embed.FS

var providerSubdirs = []string{".claude", ".gemini", ".codex"}

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

// resolvePort returns the requested port if available, otherwise finds a free one.
func resolvePort(requested int) (int, error) {
	if requested == 0 {
		return findAvailablePort()
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", requested))
	if err != nil {
		log.Printf("port %d is busy, selecting next available port", requested)
		return findAvailablePort()
	}
	listener.Close()
	return requested, nil
}

// detectProviderDirs checks for known provider subdirectories under logsDir.
// Missing subdirectories are skipped with a warning.
func detectProviderDirs(logsDir string) []string {
	var found []string
	for _, sub := range providerSubdirs {
		path := filepath.Join(logsDir, sub)
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			log.Printf("warning: provider directory %s not found, skipping", path)
			continue
		}
		found = append(found, path)
	}
	return found
}

func defaultLogsDir() string {
	u, err := user.Current()
	if err != nil {
		return os.Getenv("HOME")
	}
	return u.HomeDir
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	default: // linux
		cmd = "xdg-open"
	}
	if err := exec.Command(cmd, url).Start(); err != nil {
		log.Printf("failed to open browser: %v", err)
	}
}

func main() {
	logsDir := flag.String("logs-dir", "", "root directory to scan for provider log subdirectories (default: ~/)")
	port := flag.Int("port", 0, "HTTP server port (auto-selected if 0 or busy)")
	noUI := flag.Bool("no-ui", false, "disable browser launch (not yet implemented)")
	flag.Parse()

	if *noUI {
		fmt.Println("not yet implemented")
		os.Exit(0)
	}

	if *logsDir == "" {
		*logsDir = defaultLogsDir()
	}

	providerDirs := detectProviderDirs(*logsDir)
	if len(providerDirs) == 0 {
		fmt.Fprintf(os.Stderr, "no provider log directories found under %s (looked for .claude, .gemini, .codex)\n", *logsDir)
		os.Exit(1)
	}

	// Phase 1: load sessions and aggregate costs for all detected providers.
	// The HTTP server does not start until this block completes.
	var allSessions []*provider.SessionMeta
	providerMap := make(map[string]provider.Provider)
	for _, dir := range providerDirs {
		base := filepath.Base(dir)
		switch base {
		case ".claude":
			p := claudeprovider.New(dir)
			sessions, err := p.LoadSessions()
			if err != nil {
				log.Printf("warning: %s: LoadSessions: %v", base, err)
				continue
			}
			allSessions = append(allSessions, sessions...)
			providerMap[p.Name()] = p
		default:
			log.Printf("warning: no provider implementation for %s, skipping", base)
		}
	}
	summary := aggregator.Aggregate(allSessions)
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		log.Fatalf("failed to marshal aggregation summary: %v", err)
	}

	selectedPort, err := resolvePort(*port)
	if err != nil {
		log.Fatalf("failed to find available port: %v", err)
	}

	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(summaryJSON)
	})
	apiHandler := api.New(allSessions, summary, providerMap)
	apiHandler.Register(mux)
	mux.Handle("/", http.FileServer(http.FS(distFS)))

	url := fmt.Sprintf("http://localhost:%d", selectedPort)
	fmt.Printf("doug-stats running at %s\n", url)

	go func() {
		time.Sleep(100 * time.Millisecond)
		openBrowser(url)
	}()

	log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", selectedPort), mux))
}
