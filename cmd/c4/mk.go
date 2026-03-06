package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Avalanche-io/c4/cmd/c4/internal/establish"
	"github.com/Avalanche-io/c4/cmd/c4/internal/managed"
)

// runMk implements "c4 mk" — establish a capsule or location for writing.
//
//	c4 mk project.c4m:                    # capsule
//	c4 mk studio: cloud.example.com:7433  # location
func runMk(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "  c4 mk <name>.c4m:              # establish capsule for writing\n")
		fmt.Fprintf(os.Stderr, "  c4 mk <name>: <host:port>      # establish location for writing\n")
		os.Exit(1)
	}

	target := args[0]

	// Bare colon = managed directory
	if target == ":" {
		// Collect --exclude flags
		var excludes []string
		for i := 1; i < len(args); i++ {
			if args[i] == "--exclude" && i+1 < len(args) {
				excludes = append(excludes, args[i+1])
				i++
			}
		}

		if managed.IsManaged(".") {
			if len(excludes) > 0 {
				d, err := managed.Open(".")
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				if err := d.AddIgnorePatterns(excludes); err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				fmt.Println("added exclude patterns")
			} else {
				fmt.Fprintf(os.Stderr, ": already established\n")
			}
			os.Exit(0)
		}

		d, err := managed.Init(".", excludes)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		n, _ := d.HistoryLen()
		m, _ := d.Current()
		fmt.Printf("established : (%d entries, snapshot 0)\n", len(m.Entries))
		_ = n
		return
	}

	// Must end with colon
	if !strings.HasSuffix(target, ":") {
		fmt.Fprintf(os.Stderr, "Error: target must end with colon (e.g. project.c4m: or studio: or :)\n")
		os.Exit(1)
	}

	name := strings.TrimSuffix(target, ":")

	if strings.HasSuffix(name, ".c4m") {
		// Capsule establishment
		if establish.IsCapsuleEstablished(name) {
			fmt.Fprintf(os.Stderr, "%s already established\n", target)
			os.Exit(0)
		}
		if err := establish.EstablishCapsule(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("established %s\n", target)
	} else {
		// Location establishment — requires address argument
		if len(args) < 2 {
			fmt.Fprintf(os.Stderr, "Error: location requires address argument\n")
			fmt.Fprintf(os.Stderr, "Usage: c4 mk %s <host:port>\n", target)
			os.Exit(1)
		}
		address := args[1]
		if establish.IsLocationEstablished(name) {
			existing := establish.GetLocation(name)
			if existing != nil && existing.Address == address {
				fmt.Fprintf(os.Stderr, "%s already established at %s\n", target, address)
				os.Exit(0)
			}
			// Update address
		}
		if err := establish.EstablishLocation(name, address); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("established %s → %s\n", target, address)
	}
}

// runRm implements "c4 rm" — remove a location registration.
// Capsules are removed with OS rm (which implicitly removes establishment).
//
//	c4 rm studio:
func runRm(args []string) {
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: c4 rm <name>:    # remove location\n")
		os.Exit(1)
	}

	target := args[0]

	// Bare colon = tear down managed directory
	if target == ":" {
		if !managed.IsManaged(".") {
			fmt.Fprintf(os.Stderr, "Error: not a managed directory\n")
			os.Exit(1)
		}
		d, err := managed.Open(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if err := d.Teardown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("removed tracking for :")
		return
	}

	if !strings.HasSuffix(target, ":") {
		fmt.Fprintf(os.Stderr, "Error: target must end with colon (e.g. studio: or :)\n")
		os.Exit(1)
	}

	name := strings.TrimSuffix(target, ":")

	if strings.HasSuffix(name, ".c4m") {
		// Capsule — remove establishment marker only
		if !establish.IsCapsuleEstablished(name) {
			fmt.Fprintf(os.Stderr, "%s is not established\n", target)
			os.Exit(1)
		}
		if err := establish.RemoveCapsuleEstablishment(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed establishment for %s\n", target)
	} else {
		// Location
		if !establish.IsLocationEstablished(name) {
			fmt.Fprintf(os.Stderr, "%s is not a known location\n", target)
			os.Exit(1)
		}
		if err := establish.RemoveLocation(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("removed %s\n", target)
	}
}
