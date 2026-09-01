// Command fakecloudflared is a tiny stand-in for the real cloudflared
// binary, built by supervisor_test.go's TestMain and used to
// integration-test Supervisor's restart logic without any real network
// calls. It understands just enough of the real argv shape to be a
// convincing double:
//
//	fakecloudflared tunnel --protocol http2 --metrics <addr> --url <url>
//	fakecloudflared tunnel --config <path> --metrics <addr> run --token <token>
//
// Behavior is controlled by env vars so the test can script different
// scenarios:
//
//	FAKE_CF_MODE=quick|named
//	FAKE_CF_EXIT_AFTER=<duration>  — exit(1) after this long (simulates a crash); "" = never
//	FAKE_CF_READY_AFTER=<duration> — /ready starts returning 200 after this long; "" = never ready
//	FAKE_CF_IGNORE_SIGINT=1        — swallow SIGINT (via signal.Notify with
//	                                  no reader draining it) instead of
//	                                  letting the Go runtime's default
//	                                  terminate-on-SIGINT apply; simulates a
//	                                  process that doesn't honor a graceful
//	                                  Terminate() and must be Kill()ed.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	metrics := flag.String("metrics", "", "")
	_ = flag.String("protocol", "", "")
	_ = flag.String("url", "", "")
	_ = flag.String("config", "", "")
	_ = flag.String("token", "", "")
	flag.CommandLine.Parse(os.Args[2:]) // skip the "tunnel" subcommand token

	if os.Getenv("FAKE_CF_IGNORE_SIGINT") == "1" {
		// Registering a channel that nothing ever reads from suppresses
		// the Go runtime's default terminate-on-SIGINT behavior, without
		// this process ever acting on the signal — a stand-in for a
		// real-world process that doesn't exit on a graceful Terminate().
		signal.Notify(make(chan os.Signal, 1), os.Interrupt)
	}

	mode := os.Getenv("FAKE_CF_MODE")

	ready := false
	if *metrics != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
			status := 503
			if ready {
				status = 200
			}
			w.WriteHeader(status)
			json.NewEncoder(w).Encode(map[string]any{"status": status, "readyConnections": boolToInt(ready)})
		})
		go http.ListenAndServe(*metrics, mux)
	}

	if readyAfter := os.Getenv("FAKE_CF_READY_AFTER"); readyAfter != "" {
		d, err := time.ParseDuration(readyAfter)
		if err == nil {
			go func() {
				time.Sleep(d)
				ready = true
			}()
		}
	}

	if mode == "quick" {
		fmt.Println("INF Requesting new quick Tunnel on trycloudflare.com...")
		time.Sleep(50 * time.Millisecond)
		fmt.Println("INF +--------------------------------------------------------------+")
		fmt.Println("INF |  Your quick Tunnel has been created! Visit it at:             |")
		fmt.Printf("INF |  https://%s.trycloudflare.com                                   |\n", os.Getenv("FAKE_CF_SUBDOMAIN"))
		fmt.Println("INF +--------------------------------------------------------------+")
	} else {
		fmt.Println("INF Registered tunnel connection")
	}

	if exitAfter := os.Getenv("FAKE_CF_EXIT_AFTER"); exitAfter != "" {
		d, err := time.ParseDuration(exitAfter)
		if err == nil {
			time.Sleep(d)
			os.Exit(1)
		}
	}

	// Otherwise run "forever" (until the test kills it).
	select {}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
