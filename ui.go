// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

func key(k string) string { return bold(cyan(k)) }

func keybar(w int, items [][2]string) []string {
	var lines []string
	cur, curLen := " ", 1
	for _, it := range items {
		seg := key(it[0]) + " " + it[1]
		sl := visibleLen(seg)
		if curLen+sl+2 > w && curLen > 1 {
			lines = append(lines, cur)
			cur, curLen = " ", 1
		}
		if curLen > 1 {
			cur += "  "
			curLen += 2
		}
		cur += seg
		curLen += sl
	}
	return append(lines, cur)
}

// ask shows a question on the bottom lines and returns the key pressed (0 on esc).
func (a *app) ask(base func([]string) []string, question, choices string) rune {
	for {
		a.scr.draw(base([]string{" " + bold(question), " " + dim(choices)}), -1, -1)
		k, ok := a.scr.keys.Read()
		if !ok || k.Name == "esc" || k.Name == "ctrl-c" {
			return 0
		}
		if k.Name == "enter" {
			return '\n'
		}
		if k.Rune != 0 {
			return k.Rune
		}
	}
}

// prompt edits a value on the bottom lines; validate returns an error text or "".
func (a *app) prompt(base func([]string) []string, label, initial string, validate func(string) string) (string, bool) {
	ed := &lineEditor{buf: []rune(initial), pos: len([]rune(initial))}
	msg := ""
	for {
		w, _ := a.scr.size()
		head := " " + bold(label) + dim("  enter=accept  esc=cancel  ctrl-u clear")
		if msg != "" {
			head = " " + red(msg)
		}
		text, col := ed.view(w - 4)
		lines := base([]string{head, " " + cyan("›") + " " + text})
		a.scr.draw(lines, len(lines)-1, 3+col)
		k, ok := a.scr.keys.Read()
		if !ok {
			return "", false
		}
		done, cancel := ed.handle(k)
		if cancel {
			return "", false
		}
		if done {
			v := string(ed.buf)
			if validate != nil {
				if m := validate(v); m != "" {
					msg = m
					continue
				}
			}
			return v, true
		}
		msg = ""
	}
}

// pager shows lines with a title (startAtEnd: begin at the last page);
// extra handles keys other than scrolling and returns true to close it.
func (a *app) pager(title string, lines []string, keys [][2]string, startAtEnd bool, extra func(Key) bool) {
	off := 0
	if startAtEnd {
		off = len(lines) // clamped below
	}
	for {
		w, h := a.scr.size()
		bar := keybar(w, append(append([][2]string{}, keys...), [2]string{"↑↓ pgup/pgdn", "scroll"}, [2]string{"esc/q", "back"}))
		body := h - 2 - len(bar)
		if off > max(0, len(lines)-body) {
			off = max(0, len(lines)-body)
		}
		out := []string{inv(bold(" "+title+" ")) + " " + dim(a.h.r.Label())}
		for i := off; i < len(lines) && i < off+body; i++ {
			out = append(out, fit(lines[i], w))
		}
		for len(out) < h-len(bar)-1 {
			out = append(out, "")
		}
		out = append(out, dim(a.status))
		out = append(out, bar...)
		a.scr.draw(out, -1, -1)
		k, ok := a.scr.keys.Read()
		if !ok {
			return
		}
		a.status = ""
		switch {
		case k.Name == "up" || k.Rune == 'k':
			off = max(0, off-1)
		case k.Name == "down" || k.Rune == 'j':
			off++
		case k.Name == "pgup":
			off = max(0, off-body)
		case k.Name == "pgdn" || k.Rune == ' ':
			off += body
		case k.Name == "home" || k.Rune == 'g':
			off = 0
		case k.Name == "end" || k.Rune == 'G':
			off = len(lines)
		case k.Rune == 'q' || k.Name == "esc" || k.Name == "ctrl-c":
			return
		default:
			if extra != nil && extra(k) {
				return
			}
		}
	}
}

func wrapPrefixed(first, rest, text string, w int) []string {
	avail := max(20, w-visibleLen(first))
	var out []string
	r := []rune(text)
	prefix := first
	for len(r) > 0 {
		n := min(avail, len(r))
		if n < len(r) {
			if i := lastSpace(r[:n]); i > avail/2 {
				n = i + 1
			}
		}
		out = append(out, prefix+string(r[:n]))
		r = r[n:]
		prefix = rest
		avail = max(20, w-visibleLen(rest))
	}
	if len(out) == 0 {
		out = append(out, first)
	}
	return out
}

func lastSpace(r []rune) int {
	for i := len(r) - 1; i >= 0; i-- {
		if r[i] == ' ' {
			return i
		}
	}
	return -1
}
