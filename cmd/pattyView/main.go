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

// PattyViewVersion is checked against pattyGraph and package.json by compile.sh.
const PattyViewVersion = "0.1.8"

//go:embed dist
var embeddedDist embed.FS

type applicationOptions struct {
	listenAddress string
	showVersion   bool
	bundle        *bundleOptions
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
	options, err := parseApplicationOptions(args, stderr)
	if err != nil {
		return err
	}
	if options.showVersion {
		fmt.Fprintf(stdout, "pattyView %s\n", PattyViewVersion)
		return nil
	}
	if options.bundle != nil {
		return createIncidentBundle(*options.bundle, stdout)
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

	fmt.Fprintf(stdout, "pattyView %s available at http://%s/\n", PattyViewVersion, listener.Addr())
	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve PattyView: %w", err)
	}
	return nil
}

func parseApplicationOptions(args []string, stderr io.Writer) (applicationOptions, error) {
	flags := flag.NewFlagSet("pattyView", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Usage: pattyView [options]")
		fmt.Fprintln(stderr, "  -l, --listen address")
		fmt.Fprintf(stderr, "      HTTP listen address (default %q)\n", defaultListenAddress)
		fmt.Fprintln(stderr, "  -v, --version")
		fmt.Fprintln(stderr, "      print version and exit")
		fmt.Fprintln(stderr, "  --bundle PattyLog")
		fmt.Fprintln(stderr, "      create an incident bundle from a PattyLog JSONL file")
		fmt.Fprintln(stderr, "  --from log-time")
		fmt.Fprintln(stderr, "      first interval log-time to include (RFC3339)")
		fmt.Fprintln(stderr, "  --through log-time")
		fmt.Fprintln(stderr, "      final interval log-time to include (RFC3339)")
		fmt.Fprintln(stderr, "  --session id")
		fmt.Fprintln(stderr, "      session to bundle; inferred when the PattyLog has only one")
		fmt.Fprintln(stderr, "  --output file")
		fmt.Fprintln(stderr, "      output ZIP path (default: range-named file beside PattyLog)")
		fmt.Fprintln(stderr, "  -h, --help")
		fmt.Fprintln(stderr, "      show this help")
	}
	listenAddress := defaultListenAddress
	showVersion := false
	bundleInput := ""
	fromLogTime := ""
	throughLogTime := ""
	sessionID := ""
	outputPath := ""
	flags.StringVar(&listenAddress, "listen", defaultListenAddress, "HTTP listen address")
	flags.StringVar(&listenAddress, "l", defaultListenAddress, "HTTP listen address (shorthand)")
	flags.BoolVar(&showVersion, "version", false, "print version and exit")
	flags.BoolVar(&showVersion, "v", false, "print version and exit (shorthand)")
	flags.StringVar(&bundleInput, "bundle", "", "PattyLog JSONL file to bundle")
	flags.StringVar(&fromLogTime, "from", "", "first interval log-time to include")
	flags.StringVar(&throughLogTime, "through", "", "final interval log-time to include")
	flags.StringVar(&sessionID, "session", "", "PattyLog session ID")
	flags.StringVar(&outputPath, "output", "", "incident bundle output path")
	if err := flags.Parse(args); err != nil {
		return applicationOptions{}, err
	}
	if flags.NArg() != 0 {
		return applicationOptions{}, fmt.Errorf("unexpected positional argument %q", flags.Arg(0))
	}

	visited := make(map[string]bool)
	flags.Visit(func(option *flag.Flag) { visited[option.Name] = true })
	bundleFlagUsed := visited["bundle"]
	bundleOptionUsed := visited["from"] || visited["through"] || visited["session"] || visited["output"]
	if !bundleFlagUsed && bundleOptionUsed {
		return applicationOptions{}, errors.New("--from, --through, --session, and --output require --bundle")
	}
	if bundleFlagUsed {
		if showVersion {
			return applicationOptions{}, errors.New("--bundle cannot be combined with --version")
		}
		if visited["listen"] || visited["l"] {
			return applicationOptions{}, errors.New("--bundle cannot be combined with --listen")
		}
		bundle, err := parseBundleOptions(bundleInput, fromLogTime, throughLogTime, sessionID, outputPath)
		if err != nil {
			return applicationOptions{}, err
		}
		return applicationOptions{listenAddress: listenAddress, bundle: &bundle}, nil
	}
	return applicationOptions{listenAddress: listenAddress, showVersion: showVersion}, nil
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
		response.Header().Set("Cache-Control", "no-store")
		files.ServeHTTP(response, request)
	}), nil
}
