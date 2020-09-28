/*

Type describing the tracker events.

*/

package repm

import (
	"math"

	s2protrep "github.com/icza/s2prot/rep"
)

// TrackerEvts contains tracker events and some metrics and data calculated from them.
type TrackerEvts s2protrep.TrackerEvts

// init initializes / preprocesses the tracker events.
func (t *TrackerEvts) init(rep *Rep) {
	pidPlayerDescMap := make(map[int64]*s2protrep.PlayerDesc)
	t.PIDPlayerDescMap = pidPlayerDescMap

	// stats per player
	type stats struct {
		samples   int64 // stats samples count
		unspents  int64 // Unspent resources
		incomes   int64 // Resource income
		supCapped int64 // supply capped
	}

	pidStats := make(map[int64]*stats)

	// first read Player setup events:
	for _, e := range t.Evts {
		if e.Loop() > 0 {
			break
		}
		if e.ID != s2protrep.TrackerEvtIDPlayerSetup {
			continue
		}
		pid := e.Int("playerId")
		pd := pidPlayerDescMap[pid]
		if pd == nil {
			pd = &s2protrep.PlayerDesc{PlayerID: pid, SlotID: e.Int("slotId"), UserID: e.Int("userId")}
			pidPlayerDescMap[pid] = pd
			pidStats[pid] = &stats{}
		}
	}

	// Read start locations and player stats

	cx := rep.InitData.GameDescription.MapSizeX() / 2
	cy := rep.InitData.GameDescription.MapSizeY() / 2

	for _, e := range t.Evts {
		if e.Loop() == 0 && e.ID == s2protrep.TrackerEvtIDUnitBorn {
			if isMainBuilding(e.Stringv("unitTypeName")) {
				pd := pidPlayerDescMap[e.Int("controlPlayerId")]
				if pd != nil {
					pd.StartLocX = e.Int("x")
					pd.StartLocY = e.Int("y")
					pd.StartDir = angleToClock(math.Atan2(float64(pd.StartLocY-cy), float64(pd.StartLocX-cx)))
				}
			}
		}

		if e.ID == s2protrep.TrackerEvtIDPlayerStats {
			pid := e.Int("playerId")
			st := pidStats[pid]
			if st != nil {
				ss := e.Structv("stats")
				st.samples++
				st.unspents += ss.Int("scoreValueMineralsCurrent") + ss.Int("scoreValueVespeneCurrent")
				st.incomes += ss.Int("scoreValueMineralsCollectionRate") + ss.Int("scoreValueVespeneCollectionRate")
				if ss.Int("scoreValueFoodUsed") >= ss.Int("scoreValueFoodMade") {
					st.supCapped++
				}
			}
		}
	}

	// Finish SQ and supply-capped calculations
	for pid, pd := range pidPlayerDescMap {
		st := pidStats[pid]
		if st == nil || st.samples == 0 {
			continue
		}
		pd.SQ = calcSQ(st.unspents/st.samples, st.incomes/st.samples)
		pd.SupplyCappedPercent = int32(st.supCapped * 100 / st.samples)
	}

	// Fill ToonPlayerDescMap
	t.ToonPlayerDescMap = make(map[string]*s2protrep.PlayerDesc)
	for _, pd := range pidPlayerDescMap {
		slot := rep.InitData.LobbyState.Slots[pd.SlotID]
		t.ToonPlayerDescMap[slot.ToonHandle()] = pd
	}
}

// isMainBuilding tells if the unit type name denots a main building, that is
// one of Nexus, Command Center and Hatchery.
func isMainBuilding(unitTypeName string) bool {
	return unitTypeName == "Nexus" || unitTypeName == "CommandCenter" || unitTypeName == "Hatchery"
}

// angleToClock converts an angle given in radian to an hour clock value
// in the range of 1..12.
//
// Examples:
//  - PI/2 => 12 (o'clock)
//  - 0 => 3 (o'clock)
//  - PI => 9 (o'clock)
func angleToClock(angle float64) int32 {
	// The algorithm below computes clock value in the range of 0..11 where
	// 0 corresponds to 12.

	// 1 hour is PI/6 angle range
	const oneHour = math.Pi / 6

	// Shift by 3:30 (0 or 12 o-clock starts at 11:30)
	// and invert direction (clockwise):
	angle = -angle + oneHour*3.5

	// Put in range of 0..2*PI
	for angle < 0 {
		angle += oneHour * 12
	}
	for angle >= oneHour*12 {
		angle -= oneHour * 12
	}

	// And convert to a clock value:
	hour := int32(angle / oneHour)
	if hour == 0 {
		return 12
	}
	return hour
}

// calcSQ calculates the SQ (Spending Quotient).
//
// Algorithm:
// SQ = 35 * ( 0.00137 * I - ln( U ) ) + 240
// Where U is the average unspent resources (Resources Current; including minerals and vespene)
// and I is the average income (Resource Colleciton Rate; including minerals and vespene);
// and samples are taken up to the loop of the last cmd game event of the user.
//
// Source: Do you macro like a pro? http://www.teamliquid.net/forum/viewmessage.php?topic_id=266019
func calcSQ(unspentResources, income int64) int32 {
	return int32(35*(0.00137*float64(income)-math.Log(float64(unspentResources))) + 240 + 0.5)
}
