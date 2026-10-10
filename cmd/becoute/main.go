// Command becoute is a dedicated audiobook player built with Fyne.
package main

import (
	"log"
	"os"

	"fyne.io/fyne/v2/app"

	"github.com/foundev/becoute/internal/audio"
	"github.com/foundev/becoute/internal/config"
	"github.com/foundev/becoute/internal/library"
	"github.com/foundev/becoute/internal/progress"
	"github.com/foundev/becoute/internal/ui"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("becoute: %v", err)
	}
}

func run() error {
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	libPath, err := config.LibraryPath()
	if err != nil {
		return err
	}
	progPath, err := config.ProgressPath()
	if err != nil {
		return err
	}

	// Cover art is cached under the data directory rather than beside the
	// library file, so the layout matches the documented data location.
	lib, err := library.OpenWithDataDir(libPath, dataDir)
	if err != nil {
		return err
	}
	prog, err := progress.Open(progPath)
	if err != nil {
		return err
	}

	if err := audio.InitSpeaker(); err != nil {
		return err
	}
	player := audio.NewPlayer()
	defer player.Close()

	fyneApp := app.NewWithID("io.github.foundev.becoute")
	ctrl := ui.New(fyneApp, lib, prog, player)
	window := ui.NewApp(fyneApp, ctrl)

	// Allow a file or folder passed on the command line to be imported at
	// startup, which makes `becoute ~/Books/SomeBook.mp3` work.
	if len(os.Args) > 1 {
		go importArgs(ctrl, os.Args[1:])
	}

	window.Show()
	fyneApp.Run()
	return nil
}

func importArgs(ctrl *ui.Controller, args []string) {
	for _, arg := range args {
		info, err := os.Stat(arg)
		if err != nil {
			log.Printf("becoute: %s: %v", arg, err)
			continue
		}
		if info.IsDir() {
			if _, err := ctrl.ImportDir(arg); err != nil {
				log.Printf("becoute: %s: %v", arg, err)
			}
			continue
		}
		if _, err := ctrl.ImportFile(arg); err != nil {
			log.Printf("becoute: %s: %v", arg, err)
		}
	}
}
