package bankrecover

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/icza/s2prot"
	"github.com/icza/s2prot/rep"
	"github.com/nanitefactory/sc2bankrecover/repm"
)

// NewBanksFromReplay returns all banks of all players in a replay.
// ret[iPlayer][strBankName] gives a pointer to a bank,
// where player index starts from 0 excluding the neutral force.
func NewBanksFromReplay(r *repm.Rep) (ret []map[string]*Bank) {
	isBankEvent := func(gameEvent s2prot.Event) bool {
		for _, bankEvt := range []string{
			EvtTypeBankFile,
			EvtTypeBankSection,
			EvtTypeBankKey,
			EvtTypeBankValue,
			EvtTypeBankSignature,
		} {
			if gameEvent.EvtType.Name == bankEvt {
				return true
			}
		}
		return false
	}
	r.InitData.GameDescription.MaxObservers()

	// Player index starts from 0 excluding the neutral force. It ranges [0 ~ 9] when there are 10 players in a game.
	// The number of players could be smaller than the actual number of lobby participants since there could be spectators.
	// Slots include both players and spectators.
	usersBank := make([]map[string]*Bank, len(r.InitData.LobbyState.Slots)) // banks
	for iUser := range usersBank {
		usersBank[iUser] = map[string]*Bank{}
	}
	findPlayerByToonHandle := func() map[string]rep.Player {
		ret := map[string]rep.Player{}
		for _, player := range r.Details.Players() {
			if player.Toon.String() != "" { // not to be overwritten
				ret[player.Toon.String()] = player
			}
		}
		return ret
	}()
	// Slots
	type PlayerSlot struct {
		rep.Slot
		index int
	}
	findSlotByUserID := func() map[int64]PlayerSlot {
		ret := map[int64]PlayerSlot{}
		for iSlot, slot := range r.InitData.LobbyState.Slots {
			if slot.ToonHandle() != "" { // not to be overwritten
				ret[slot.UserID()] = PlayerSlot{
					Slot:  slot,
					index: iSlot,
				}
			}
		}
		return ret
	}()
	// Collect banks events
	var bankNameCurr string
	for _, evt := range r.GameEvts {
		if evt.Loop() > 0 {
			break
		}
		if !isBankEvent(evt) {
			continue
		}
		{ // slot
			slot := findSlotByUserID[evt.UserID()] // get player slot
			if evt.EvtType.Name == EvtTypeBankFile {
				bankNameCurr = evt.Stringv("name")
				usersBank[slot.index][bankNameCurr] = NewBank(r, evt, slot.Slot, findPlayerByToonHandle[slot.ToonHandle()])
				// log.Println(slot.index, bankNameCurr) //
				continue
			}
			if usersBank[slot.index][bankNameCurr] != nil {
				// log.Println("Warning: Bank event of unknown bank file: ", evt) // probably map maker's fault //
				usersBank[slot.index][bankNameCurr].AddGameEvent(evt)
			}
		}
		continue
	}

	return usersBank
}

// NNet event protocol types regarding bank
const (
	EvtTypeBankFile      = "BankFile"
	EvtTypeBankSection   = "BankSection"
	EvtTypeBankKey       = "BankKey"
	EvtTypeBankValue     = "BankValue"
	EvtTypeBankSignature = "BankSignature"
)

// Bank represents a bank of a player.
type Bank struct {
	r          *repm.Rep
	Name       string     // filename
	UserSlot   rep.Slot   // owner slot
	Player     rep.Player // owner player
	GameEvents []s2prot.Event
}

// NewBank is a constructor. Returns nil upon error.
func NewBank(r *repm.Rep, evtBankFile s2prot.Event, user rep.Slot, player rep.Player) *Bank {
	if evtBankFile.EvtType.Name != EvtTypeBankFile {
		return nil
	}
	return &Bank{
		r:          r,
		Name:       evtBankFile.Stringv("name"),
		UserSlot:   user,
		Player:     player,
		GameEvents: []s2prot.Event{evtBankFile},
	}
}

func (bank *Bank) String() string {
	return fmt.Sprint(bank.GameEvents)
}

// AddGameEvent accepts all bank events except for the "BankFile" event.
func (bank *Bank) AddGameEvent(evtBankContent s2prot.Event) error {
	switch evtBankContent.EvtType.Name {
	case EvtTypeBankSection:
		fallthrough
	case EvtTypeBankKey:
		fallthrough
	case EvtTypeBankValue:
		fallthrough
	case EvtTypeBankSignature:
		bank.GameEvents = append(bank.GameEvents, evtBankContent)
		return nil
	}
	return errors.New("invalid bank event")
}

// WriteTo writes out this bank to the writer 'w'.
// The function returns the number of bytes written and any error encountered.
func (bank *Bank) WriteTo(w io.Writer) (n int64, err error) {
	doc := etree.NewDocument()
	doc.CreateProcInst("xml", `version="1.0" encoding="UTF-8"`)
	root := doc.CreateElement("Bank")
	root.CreateAttr("version", "1")
	root.CreateComment(fmt.Sprint("Bank recovered from a replay"))
	root.CreateComment(fmt.Sprint(time.Now()))
	root.CreateComment(fmt.Sprint("Title: ", bank.r.Details.Title()))
	root.CreateComment(fmt.Sprint("Version: ", bank.r.Header.VersionString()))
	root.CreateComment(fmt.Sprint("Loops: ", bank.r.Header.Loops()))
	root.CreateComment(fmt.Sprint("Length: ", bank.r.Header.Duration()))
	root.CreateComment(fmt.Sprint("Player: ", bank.UserSlot.ToonHandle()))

	var eCurrSection *etree.Element
	var eCurrKey *etree.Element
	for _, evt := range bank.GameEvents {
		switch evt.EvtType.Name {
		case EvtTypeBankSection:
			eCurrSection = root.CreateElement("Section")
			eCurrSection.CreateAttr("name", evt.Stringv("name"))
			continue
		case EvtTypeBankKey:
			eCurrKey = eCurrSection.CreateElement("Key")
			eCurrKey.CreateAttr("name", evt.Stringv("name"))
			if evt.Value("type") == nil {
				continue
			}
			fallthrough // goto EvtTypeBankValue
		case EvtTypeBankValue:
			nType := evt.Int("type")
			if nType == 7 { // value will be in the next message
				continue
			}
			eVal := eCurrKey.CreateElement(evt.Stringv("name"))
			eVal.CreateAttr([]string{
				"fixed",
				"flag",
				"int",
				"string",
				"point", // "point",
				"unit",  // "unit",
				"text",
			}[nType], evt.Stringv("data"))
			continue
		case EvtTypeBankSignature:
			eCurrSection = root.CreateElement("Signature")
			if len(evt.Array("signature")) > 0 {
				eCurrSection.CreateAttr("value", func() string {
					sb := &strings.Builder{}
					for _, v := range evt.Array("signature") {
						fmt.Fprintf(sb, "%02X", v)
					}
					return sb.String()
				}())
			}
			continue
		} // switch
	} // for

	doc.Indent(2)
	return doc.WriteTo(w)
} // func

// SaveAsFile writes this bank out to the file at path 'strFilepath'.
// Creates directories given as filepath if not present.
func (bank *Bank) SaveAsFile(strFilepath string) error {
	if err := os.MkdirAll(filepath.Dir(strFilepath), os.ModePerm); err != nil {
		return err
	}
	f, err := os.Create(strFilepath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = bank.WriteTo(f)
	return err
}
