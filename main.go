// svcman — manage and audit systemd services on remote hosts over SSH.
//
// Author: Konstantinos Patronas <kpatronas@gmail.com>
// https://github.com/kpatr1981/svcman
package main

import (
	"flag"
	"fmt"
	"os"
)

var version = "0.1.0"

const (
	author  = "Konstantinos Patronas <kpatronas@gmail.com>"
	repoURL = "https://github.com/kpatr1981/svcman"
)

func main() {
	showVer := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVer {
		fmt.Println("svcman", version)
		fmt.Println("Author:", author)
		fmt.Println(repoURL)
		return
	}
	fatal("not implemented yet")
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, red("svcman: ")+msg)
	os.Exit(2)
}
