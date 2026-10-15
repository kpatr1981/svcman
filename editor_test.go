// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strings"
	"testing"
)

func TestParseEntryInput(t *testing.T) {
	cases := []struct {
		in   string
		want Entry
		err  string
	}{
		{"Restart=always", Entry{"Service", "Restart", "always"}, ""},
		{"Unit/Description=hi there", Entry{"Unit", "Description", "hi there"}, ""},
		{"  Environment = A=1 ", Entry{"Service", "Environment", "A=1"}, ""},
		{"Restart", Entry{}, "write Key=value"},
		{"=x", Entry{}, "write Key=value"},
		{"Bad Key=x", Entry{}, "a key is letters and digits"},
	}
	for _, c := range cases {
		e, err := parseEntryInput(c.in, "Service")
		if e != c.want || !strings.Contains(err, c.err) || (c.err == "" && err != "") {
			t.Errorf("%q: got %+v %q", c.in, e, err)
		}
	}
}

func TestDiffDropIn(t *testing.T) {
	useColor = false
	d := newDropIn("a.service", "[Service]\nRestart=no\n", true)
	d.Entries = []Entry{{"Service", "Restart", "always"}}
	got := strings.Join(diffDropIn(d), "\n")
	if !strings.Contains(got, "- Restart=no") || !strings.Contains(got, "+ Restart=always") || !strings.Contains(got, d.Path) {
		t.Errorf("%s", got)
	}
	d.Entries = nil
	if got := strings.Join(diffDropIn(d), "\n"); !strings.Contains(got, "(file will be deleted)") {
		t.Errorf("%s", got)
	}
}

// systemctl show leaves out empty properties unless --all is given, which
// made unset keys such as Environment look unknown.
func TestPropNamesAsksForAllProperties(t *testing.T) {
	f := &fakeRunner{reply: func(cmd, in string) (string, string, error) { return "Environment\nRestart\n", "", nil }}
	a := &app{h: &Host{r: f}}
	m := a.propNames(&Unit{Name: "a.service"})
	if !strings.Contains(f.calls[0].Stdin, "show --all") || !m["Environment"] {
		t.Errorf("%q %v", f.calls[0].Stdin, m)
	}
}
