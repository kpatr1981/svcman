// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

// Severity of a check result; higher is worse.
type Severity int

const (
	SevUnknown Severity = iota // not checked
	SevOK
	SevInfo
	SevWarn
	SevFail
)

// Fix is a shell command, run as root on the target, that resolves an Issue.
type Fix struct {
	Label string
	Cmd   string
}

// Issue is one problem found with a unit or a host.
type Issue struct {
	Sev   Severity
	Text  string
	Fixes []Fix
}

// worst returns the highest severity in is, or SevOK when is is empty.
func worst(is []Issue) Severity {
	w := SevOK
	for _, i := range is {
		if i.Sev > w {
			w = i.Sev
		}
	}
	return w
}
