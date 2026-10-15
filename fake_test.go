// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

type fakeCall struct{ Cmd, Stdin string }

// fakeRunner records calls and answers them with reply.
type fakeRunner struct {
	calls []fakeCall
	reply func(cmd, stdin string) (string, string, error)
}

func (f *fakeRunner) Exec(cmd, stdin string) (string, string, error) {
	f.calls = append(f.calls, fakeCall{cmd, stdin})
	if f.reply == nil {
		return "", "", nil
	}
	return f.reply(cmd, stdin)
}
func (f *fakeRunner) Interactive(cmd string) error {
	f.calls = append(f.calls, fakeCall{"TTY " + cmd, ""})
	return nil
}
func (f *fakeRunner) Label() string { return "fake" }
func (f *fakeRunner) Close()        {}
