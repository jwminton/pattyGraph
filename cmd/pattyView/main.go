package main

import (
	"embed"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"time"
)

const defaultListenAddress = "127.0.0.1:4177"

//go:embed dist
var embeddedDist embed.FS

type serverOptions struct {
	listenAddress string
}

func main() {
	err := run(os.Args[1:], os.Stdout, os.Stderr)
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return
	}
	fmt.Fprintf(os.Stderr, "pattyView: %v\n", err)
	os.Exit(1)
}

func run(args []string, stdout, stderr io.Writer) error {
	options, err := parseServerOptions(args, stderr)
	if err != nil {
		return err
	}

	handler, err := embeddedHandler()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", options.listenAddress)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", options.listenAddress, err)
	}
	defer listener.Close()

	fmt.Fprintf(stdout, "PattyView available at http://%s/\n", listener.Addr())
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve PattyView: %w", err)
	}
	return nil
}

func parseServerOptions(args []string, stderr io.Writer) (serverOptions, error) {
	flags := flag.NewFlagSet("pattyView", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listenAddress := flags.String("listen", defaultListenAddress, "HTTP listen address")
	if err := flags.Parse(args); err != nil {
		return serverOptions{}, err
	}
	if flags.NArg() != 0 {
		return serverOptions{}, fmt.Errorf("unexpected positional argument %q", flags.Arg(0))
	}
	return serverOptions{listenAddress: *listenAddress}, nil
}

func embeddedHandler() (http.Handler, error) {
	dist, err := fs.Sub(embeddedDist, "dist")
	if err != nil {
		return nil, fmt.Errorf("open embedded PattyView build: %w", err)
	}

	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			response.Header().Set("Allow", "GET, HEAD")
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		files.ServeHTTP(response, request)
	}), nil
}
