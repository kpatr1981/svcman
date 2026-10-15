// svcman — manage and audit systemd services on remote hosts over SSH.
//
// Author: Konstantinos Patronas <kpatronas@gmail.com>
// https://github.com/kpatr1981/svcman
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
)

var version = "0.1.0"

const (
	author  = "Konstantinos Patronas <kpatronas@gmail.com>"
	repoURL = "https://github.com/kpatr1981/svcman"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func usage() {
	fmt.Fprintf(os.Stderr, `svcman %s — manage systemd services over SSH

Usage:
  svcman [options] [user@]host           interactive manager for the host's services and timers
  svcman --audit [options] [user@]host   check every enabled, failed and timer unit
  svcman --local [--audit]               this machine instead of SSH (Linux)

Interactive keys (also shown on screen):
  ↑/↓ move   / search   f filter   s start   S stop   R restart   space enable/disable
  l journal   enter details & fixes   e edit drop-in   w save   c cat   r reload   ? help   q quit

Options:
`, version)
	flag.PrintDefaults()
	fmt.Fprintf(os.Stderr, `
Exit status (--audit): 0 no failing units, 1 failing units found, 2 error.

Author: %s
%s
`, author, repoURL)
}

func main() {
	var (
		audit    = flag.Bool("audit", false, "check enabled, failed and timer units for problems")
		local    = flag.Bool("local", false, "use this machine instead of SSH (Linux)")
		port     = flag.String("p", "", "SSH port")
		identity = flag.String("i", "", "SSH identity file")
		jump     = flag.String("J", "", "SSH jump host")
		verbose  = flag.Bool("v", false, "audit: list every unit, not only problems")
		fix      = flag.Bool("fix", false, "audit: offer to apply each proposed fix")
		noColor  = flag.Bool("no-color", false, "disable colours")
		showVer  = flag.Bool("version", false, "print version")
		sshOpts  multiFlag
	)
	flag.Var(&sshOpts, "o", "extra SSH option, e.g. -o StrictHostKeyChecking=no (repeatable)")
	flag.Usage = usage

	// Allow options after the host too: svcman user@host --audit
	args := os.Args[1:]
	var positional []string
	for {
		if err := flag.CommandLine.Parse(args); err != nil {
			os.Exit(2)
		}
		rest := flag.Args()
		if len(rest) == 0 {
			break
		}
		positional = append(positional, rest[0])
		args = rest[1:]
	}
	if *showVer {
		fmt.Println("svcman", version)
		fmt.Println("Author:", author)
		fmt.Println(repoURL)
		return
	}
	initColor()
	if *noColor {
		useColor = false
	}

	var r Runner
	switch {
	case *local && len(positional) == 0:
		if runtime.GOOS != "linux" {
			fatal("--local needs Linux with systemd; connect to a host instead")
		}
		r = localRunner{}
	case !*local && len(positional) == 1:
		r = newSSHRunner(positional[0], *port, *identity, *jump, sshOpts)
	case len(positional) > 1:
		fatal("svcman takes one host (multi-host mode is planned)")
	default:
		usage()
		os.Exit(2)
	}
	defer r.Close()

	if *audit {
		code, err := runAudit(r, auditOpts{verbose: *verbose, fix: *fix})
		r.Close()
		if err != nil {
			fatal(err.Error())
		}
		os.Exit(code)
	}
	if err := runInteractive(r); err != nil {
		r.Close()
		fatal(err.Error())
	}
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, red("svcman: ")+msg)
	os.Exit(2)
}

// runInteractive is replaced in Task 10.
func runInteractive(r Runner) error { return fmt.Errorf("interactive mode not built yet") }
