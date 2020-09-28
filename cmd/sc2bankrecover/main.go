package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	bankrecover "github.com/nanitefactory/sc2bankrecover"
	"github.com/nanitefactory/sc2bankrecover/repm"
)

// Flag variables
var (
	flagFileName = flag.String("filename", "", "filename of a replay")
)

func init() {
	flag.Parse()
}

func main() {
	// args
	if *flagFileName == "" && len(os.Args) > 1 {
		*flagFileName = os.Args[1]
	}

	// get .
	wd := func() string {
		ret, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		return ret
	}()

	// get rep
	r, err := repm.NewFromFile(filepath.Join(wd, *flagFileName))
	if err != nil {
		fmt.Printf("Failed to open file: %v\n", err) // likely to return unsupported version error
		return
	}
	defer r.Close()

	// 1
	fmt.Printf("Version:        %v\n", r.Header.VersionString())
	fmt.Printf("Loops:          %d\n", r.Header.Loops())
	fmt.Printf("Length:         %v\n", r.Header.Duration())
	fmt.Printf("Map:            %s\n", r.Details.Title())
	fmt.Printf("Speed:          %s\n", r.Details.GameSpeed())
	fmt.Printf("Game events:    %d\n", len(r.GameEvts))
	fmt.Printf("Message events: %d\n", len(r.MessageEvts))
	fmt.Printf("Tracker events: %d\n", len(r.TrackerEvts.Evts))

	// 2
	fmt.Println("Players:")
	for _, p := range r.Details.Players() {
		fmt.Printf("\tName: %-20s, Race: %c, Team: %d, Result: %v, Toon: %v\n",
			p.Name, p.Race().Letter, p.TeamID()+1, p.Result(), p.Toon)
	}

	// 3
	fmt.Println("Begin")
	for iPlayer, playerBanks := range bankrecover.NewBanksFromReplay(r) {
		for bankName, bank := range playerBanks {
			d := fmt.Sprintf("%d__%s", iPlayer, bank.Player.Toon)
			f := fmt.Sprintf("%s.SC2Bank", bankName)
			log.Println("Save file: ", filepath.Join(d, f))
			if err := bank.SaveAsFile(filepath.Join(wd, d, f)); err != nil {
				panic(err)
			}
		}
	}
	fmt.Println("End")

}
