/*

The Rep type that models a replay (and everything in it).

*/

package repm

import (
	"encoding/json"
	"io"

	"github.com/icza/mpq"
	"github.com/icza/s2prot"
	s2protrep "github.com/icza/s2prot/rep"
)

// Rep describes a replay.
type Rep struct {
	m *mpq.MPQ // MPQ parser for reading the file

	protocol *s2prot.Protocol // Protocol to decode the replay

	Header   s2protrep.Header   // Replay header (replay game version and length)
	Details  s2protrep.Details  // Game details (overall replay details)
	InitData s2protrep.InitData // Replay init data (the initial lobby)
	AttrEvts s2protrep.AttrEvts // Attributes events

	Metadata s2protrep.Metadata // Game metadata (calculated, confirmed results)

	GameEvts    []s2prot.Event // Game events
	MessageEvts []s2prot.Event // Message events
	TrackerEvts *TrackerEvts   // Tracker events

	GameEvtsErr    bool // Tells if decoding game events had errors
	MessageEvtsErr bool // Tells if decoding message events had errors
	TrackerEvtsErr bool // Tells if decoding tracker events had errors
}

// NewFromFile returns a new Rep constructed from a file.
// All types of events are decoded from the replay.
// The returned Rep must be closed with the Close method!
//
// ErrInvalidRepFile is returned if the specified name does not denote a valid SC2Replay file.
//
// ErrUnsupportedRepVersion is returned if the file exists and is a valid SC2Replay file but its version is not supported.
//
// ErrDecoding is returned if decoding the replay fails. This is most likely because the replay file is invalid, but also might be due to an implementation bug.
func NewFromFile(name string) (*Rep, error) {
	return NewFromFileEvts(name, true, true, true)
}

// NewFromFileEvts returns a new Rep constructed from a file, only the specified types of events decoded.
// The game, message and tracker tells if game events, message events and tracker events are to be decoded.
// Replay header, init data, details, attributes events and game metadata are always decoded.
// The returned Rep must be closed with the Close method!
//
// ErrInvalidRepFile is returned if the specified name does not denote a valid SC2Replay file.
//
// ErrUnsupportedRepVersion is returned if the file exists and is a valid SC2Replay file but its version is not supported.
//
// ErrDecoding is returned if decoding the replay fails. This is most likely because the replay file is invalid, but also might be due to an implementation bug.
func NewFromFileEvts(name string, game, message, tracker bool) (*Rep, error) {
	m, err := mpq.NewFromFile(name)
	if err != nil {
		return nil, s2protrep.ErrInvalidRepFile
	}
	return newRep(m, game, message, tracker)
}

// New returns a new Rep using the specified io.ReadSeeker as the SC2Replay file source.
// All types of events are decoded from the replay.
// The returned Rep must be closed with the Close method!
//
// ErrInvalidRepFile is returned if the input is not a valid SC2Replay file content.
//
// ErrUnsupportedRepVersion is returned if the input is a valid SC2Replay file but its version is not supported.
//
// ErrDecoding is returned if decoding the replay fails. This is most likely because the input is invalid, but also might be due to an implementation bug.
func New(input io.ReadSeeker) (*Rep, error) {
	return NewEvts(input, true, true, true)
}

// NewEvts returns a new Rep using the specified io.ReadSeeker as the SC2Replay file source, only the specified types of events decoded.
// The game, message and tracker tells if game events, message events and tracker events are to be decoded.
// Replay header, init data, details, attributes events and game metadata are always decoded.
// The returned Rep must be closed with the Close method!
//
// ErrInvalidRepFile is returned if the input is not a valid SC2Replay file content.
//
// ErrUnsupportedRepVersion is returned if the input is a valid SC2Replay file but its version is not supported.
//
// ErrDecoding is returned if decoding the replay fails. This is most likely because the input is invalid, but also might be due to an implementation bug.
func NewEvts(input io.ReadSeeker, game, message, tracker bool) (*Rep, error) {
	m, err := mpq.New(input)
	if err != nil {
		return nil, s2protrep.ErrInvalidRepFile
	}
	return newRep(m, game, message, tracker)
}

// newRep returns a new Rep constructed using the specified mpq.MPQ handler of the SC2Replay file, only the specified types of events decoded.
// The game, message and tracker tells if game events, message events and tracker events are to be decoded.
// Replay header, init data, details, attributes events and game metadata are always decoded.
// The returned Rep must be closed with the Close method!
//
// ErrInvalidRepFile is returned if the specified name does not denote a valid SC2Replay file.
//
// ErrUnsupportedRepVersion is returned if the input is a valid SC2Replay file but its version is not supported.
//
// ErrDecoding is returned if decoding the replay fails. This is most likely because the input is invalid, but also might be due to an implementation bug.
func newRep(m *mpq.MPQ, game, message, tracker bool) (parsedRep *Rep, errRes error) {
	closeMPQ := true
	defer func() {
		// If returning due to an error, MPQ must be closed!
		if closeMPQ {
			m.Close()
		}

		// The input is completely untrusted and the decoding implementation omits error checks for efficiency:
		// Protect replay decoding:
		if r := recover(); r != nil {
			errRes = s2protrep.ErrDecoding
		}
	}()

	rep := Rep{m: m}

	rep.Header = s2protrep.Header{Struct: s2prot.DecodeHeader(m.UserData())}
	if rep.Header.Struct == nil {
		return nil, s2protrep.ErrInvalidRepFile
	}

	bb := rep.Header.BaseBuild()
	p := s2prot.GetProtocol(int(bb))
	// What's modified from what's written by icza.
	if p == nil {
		p = s2prot.GetProtocol(s2prot.MaxBaseBuild)
	}
	// What's modified from what's written by icza.
	if p == nil {
		return nil, s2protrep.ErrUnsupportedRepVersion
	}
	rep.protocol = p

	data, err := m.FileByHash(620083690, 3548627612, 4013960850) // "replay.details"
	if err != nil || len(data) == 0 {
		// Attempt to open the anonymized version
		data, err = m.FileByHash(1421087648, 3590964654, 3400061273) // "replay.details.backup"
		if err != nil || len(data) == 0 {
			return nil, s2protrep.ErrInvalidRepFile
		}
	}
	rep.Details = s2protrep.Details{Struct: p.DecodeDetails(data)}

	data, err = m.FileByHash(3544165653, 1518242780, 4280631132) // "replay.initData"
	if err != nil || len(data) == 0 {
		// Attempt to open the anonymized version
		data, err = m.FileByHash(868899905, 1282002788, 1614930827) // "replay.initData.backup"
		if err != nil || len(data) == 0 {
			return nil, s2protrep.ErrInvalidRepFile
		}
	}
	rep.InitData = s2protrep.NewInitData(p.DecodeInitData(data))

	data, err = m.FileByHash(1306016990, 497594575, 2731474728) // "replay.attributes.events"
	if err != nil {
		return nil, s2protrep.ErrInvalidRepFile
	}
	rep.AttrEvts = s2protrep.NewAttrEvts(p.DecodeAttributesEvts(data))

	data, err = m.FileByHash(3675439372, 3912155403, 1108615308) // "replay.gamemetadata.json"
	if err != nil {
		return nil, s2protrep.ErrInvalidRepFile
	}
	if data != nil { // Might not be present, was added around 3.7
		if err = json.Unmarshal(data, &rep.Metadata.Struct); err != nil {
			return nil, s2protrep.ErrInvalidRepFile
		}
	}

	if game {
		data, err = m.FileByHash(496563520, 2864883019, 4101385109) // "replay.game.events"
		if err != nil {
			return nil, s2protrep.ErrInvalidRepFile
		}
		rep.GameEvts, err = p.DecodeGameEvts(data)
		rep.GameEvtsErr = err != nil
	}

	if message {
		data, err = m.FileByHash(1089231967, 831857289, 1784674979) // "replay.message.events"
		if err != nil {
			return nil, s2protrep.ErrInvalidRepFile
		}
		rep.MessageEvts, err = p.DecodeMessageEvts(data)
		rep.MessageEvtsErr = err != nil
	}

	if tracker {
		data, err = m.FileByHash(1501940595, 4263103390, 1648390237) // "replay.tracker.events"
		if err != nil {
			return nil, s2protrep.ErrInvalidRepFile
		}
		evts, err := p.DecodeTrackerEvts(data)
		rep.TrackerEvts = &TrackerEvts{Evts: evts}
		rep.TrackerEvts.init(&rep)
		rep.TrackerEvtsErr = err != nil
	}

	// Everything went well, Rep is about to be returned, do not close MPQ
	// (it will be the caller's responsibility, done via Rep.Close()).
	closeMPQ = false

	return &rep, nil
}

// Close closes the Rep and its resources.
func (r *Rep) Close() error {
	if r.m == nil {
		return nil
	}
	return r.m.Close()
}

// MPQ gives access to the underlying MPQ parser of the rep.
// Intentionally not a method of Rep to not urge its use.
func MPQ(r *Rep) *mpq.MPQ {
	return r.m
}
