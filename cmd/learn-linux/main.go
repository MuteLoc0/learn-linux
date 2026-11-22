package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"learn-linux/internal/app"
	"learn-linux/internal/contract"
)

const appVersion = "1.0.0"

func main() {
	var (
		langFlag    string
		chapterFlag int
		sandboxFlag bool
		noAltFlag   bool
		versionFlag bool
	)

	flag.StringVar(&langFlag, "lang", "", "Initial interface language (zh or en)")
	flag.IntVar(&chapterFlag, "chapter", 0, "Start directly at chapter N (1-8)")
	flag.BoolVar(&sandboxFlag, "sandbox", false, "Start directly in free sandbox mode")
	flag.BoolVar(&noAltFlag, "no-alt-screen", false, "Disable alternate screen buffer")
	flag.BoolVar(&versionFlag, "version", false, "Print version and exit")
	flag.Parse()

	if versionFlag {
		fmt.Printf("learn-linux %s\n", appVersion)
		return
	}

	var initLang contract.Lang
	if langFlag != "" {
		switch strings.ToLower(strings.TrimSpace(langFlag)) {
		case "zh":
			initLang = contract.LangZH
		case "en":
			initLang = contract.LangEN
		default:
			fmt.Fprintf(os.Stderr, "learn-linux: unsupported language %q (use zh or en)\n", langFlag)
			os.Exit(2)
		}
	}

	application, err := app.New(app.Config{
		InitialChapter: chapterFlag,
		StartSandbox:   sandboxFlag,
		InitialLang:    initLang,
		NoAltScreen:    noAltFlag,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "learn-linux: %v\n", err)
		os.Exit(1)
	}

	if err := application.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "learn-linux: %v\n", err)
		os.Exit(1)
	}
}
