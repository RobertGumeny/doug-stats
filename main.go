package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

//go:embed all:frontend/dist
var frontendDist embed.FS

func findAvailablePort() (int, error) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
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
	port, err := findAvailablePort()
	if err != nil {
		log.Fatalf("failed to find available port: %v", err)
	}

	distFS, err := fs.Sub(frontendDist, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to create sub filesystem: %v", err)
	}

	http.Handle("/", http.FileServer(http.FS(distFS)))

	url := fmt.Sprintf("http://localhost:%d", port)
	fmt.Printf("doug-stats running at %s\n", url)

	go func() {
		time.Sleep(100 * time.Millisecond)
		openBrowser(url)
	}()

	log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", port), nil))
}
