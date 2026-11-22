package ui

import (
	"fmt"
	"strings"
	"time"

	"learn-linux/internal/contract"
)

// PlayDestructivePanic plays a dramatic simulated `rm -rf /` kernel panic and auto-recovery sequence.
func (r *Renderer) PlayDestructivePanic(term contract.Terminal, i18n contract.I18N, cmd string, seconds int) error {
	if term == nil {
		return fmt.Errorf("nil terminal")
	}

	_ = term.HideCursor()
	defer func() {
		_ = term.ShowCursor()
	}()

	_ = term.ClearScreen()
	_ = term.MoveCursor(1, 1)

	header := trText(i18n, contract.KeyPanicHeader)
	if err := term.WriteString(styled(header, boldRed) + "\r\n"); err != nil {
		return err
	}

	fakePaths := []string{
		"/bin/ls",
		"/bin/bash",
		"/etc/passwd",
		"/etc/shadow",
		"/usr/lib/libc.so.6",
		"/var/log/syslog",
		"/boot/vmlinuz-6.8.0-virtual",
		"/sbin/init",
	}

	cascadeDelay := r.PanicTickDuration / 12
	rawDeleting := trText(i18n, contract.KeyPanicDeleting)
	hasDeletingVerb := countFormatVerbs(rawDeleting) > 0
	if !hasDeletingVerb && rawDeleting != "" {
		if err := term.WriteString(styled(rawDeleting, fgRed) + "\r\n"); err != nil {
			return err
		}
	}
	for _, p := range fakePaths {
		var line string
		if hasDeletingVerb {
			line, _ = tr(i18n, contract.KeyPanicDeleting, p)
		} else {
			line = fmt.Sprintf("removed '%s'", p)
		}
		if err := term.WriteString(styled(line, fgRed) + "\r\n"); err != nil {
			return err
		}
		if cascadeDelay > 0 && seconds > 0 {
			time.Sleep(cascadeDelay)
		}
	}

	// Simulated Linux kernel panic block
	panicLines := []string{
		"",
		"[   42.019381] Kernel panic - not syncing: Attempted to kill init! exitcode=0x00000009",
		"[   42.020114] CPU: 0 PID: 1 Comm: rm Not tainted 6.8.0-virtual #1",
		"[   42.021005] Call Trace: <TASK> panic+0x10a/0x2e0 do_exit+0x812/0x9e0",
	}
	for _, l := range panicLines {
		if err := term.WriteString(styled(l, boldRed) + "\r\n"); err != nil {
			return err
		}
	}

	kpMsg, usedCmd := tr(i18n, contract.KeyPanicKernelPanic, cmd)
	if !usedCmd && cmd != "" {
		kpMsg = fmt.Sprintf("%s (%s)", kpMsg, cmd)
	}
	if err := term.WriteString(styled(kpMsg, boldRed) + "\r\n\r\n"); err != nil {
		return err
	}

	joke := trText(i18n, contract.KeyPanicJoke)
	if err := term.WriteString(styled("▸ "+joke, boldYellow) + "\r\n\r\n"); err != nil {
		return err
	}

	// Live updating countdown
	if seconds > 0 {
		for rem := seconds; rem >= 1; rem-- {
			done := seconds - rem + 1
			bar := progressBar(done, seconds, 20)
			cdText, usedRem := tr(i18n, contract.KeyPanicCountdown, rem)
			if !usedRem {
				cdText = fmt.Sprintf("%s %ds", cdText, rem)
			}
			line := fmt.Sprintf("\r\x1b[K%s  %s", bar, styled(cdText, fgCyan))
			if err := term.WriteString(line); err != nil {
				return err
			}
			if r.PanicTickDuration > 0 {
				time.Sleep(r.PanicTickDuration)
			}
		}
		if err := term.WriteString("\r\x1b[K"); err != nil {
			return err
		}
	}

	restored := trText(i18n, contract.KeyPanicRestored)
	if err := term.WriteString(styled("✓ "+strings.TrimSpace(restored), boldGreen) + "\r\n"); err != nil {
		return err
	}

	return nil
}
