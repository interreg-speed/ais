package ais

import (
	"encoding/csv"
	"fmt"
	"hash/fnv"
	"os"
)

// InteractionHeaders is the set of Headers used to write Records of two vessel interactions.
// The first field InteractionHash is an ais.PairHash that uniquely identifies this interaction
// and distance is the haversine distance between the two vessels.
var InteractionHeaders = Headers{
	fields: []string{"InteractionHash", "Distance(nm)",
		"MMSI_1", "BaseDateTime_1", "LAT_1", "LON_1", "SOG_1", "COG_1", "Heading_1", "VesselName_1", "IMO_1", "CallSign_1", "VesselType_1", "Status_1", "Length_1", "Width_1", "Draft_1", "Cargo_1", "Geohash_1",
		"MMSI_2", "BaseDateTime_2", "LAT_2", "LON_2", "SOG_2", "COG_2", "Heading_2", "VesselName_2", "IMO_2", "CallSign_2", "VesselType_2", "Status_2", "Length_2", "Width_2", "Draft_2", "Cargo_2", "Geohash_2",
	},
	dictionary: nil,
}

// RecordPair holds pointers to two Records.
type RecordPair struct {
	rec1 *Record
	rec2 *Record
}

// Interactions is an abstraction two Record hash and the pointer to the RecordPair
// that made up the hash.
type Interactions struct {
	RecordHeaders Headers // for the Records that will be used to create interactions
	OutputHeaders Headers // for an output RecordSet that may be written from the 2-ship interactions
	hashIndices   [4]int  // Headers index values for MMSI, BaseDateTime, LAT, and LON
	data          map[uint64]*RecordPair
}

// NewInteractions creates a new set of interactions.  It requires a set of Headers from the
// RecordSet that will be searched for Interactions.  These Headers are required to contain "MMSI",
// "BaseDateTime", "LAT", and "LON" in order to uniquely identify an interaction. The returned
// *Interactions has its output file Headers set to ais.InteractionHeaders by default.
func NewInteractions(h Headers) (*Interactions, error) {
	if !h.Valid() {
		return nil, fmt.Errorf("new interactions: headers argument did not pass headers.valid()")
	}
	inter := new(Interactions)
	inter.OutputHeaders = InteractionHeaders
	inter.RecordHeaders = h
	inter.data = make(map[uint64]*RecordPair)

	// Find the index values for the required headers now so that the expensive parsing
	// operation only has to be perormed once at initilization
	mmsiIndex, _ := h.Contains("MMSI")
	timeIndex, _ := h.Contains("BaseDateTime")
	latIndex, _ := h.Contains("LAT")
	lonIndex, _ := h.Contains("LON")
	inter.hashIndices = [4]int{mmsiIndex, timeIndex, latIndex, lonIndex}

	return inter, nil
}

// Len returns the number of Interactions in the set.
func (inter *Interactions) Len() int {
	return len(inter.data)
}

// AddCluster adds all of the interactions in a given cluster to the set of Interactions
func (inter *Interactions) AddCluster(c *Cluster) error {
	for i := range c.Data() {
		err := inter.writeInteractions(c.data[i:])
		if err != nil {
			return err
		}
	}
	return nil
}

// WriteInteraction appends to the set for each pair of interaction in the slice.
// Note that calls to writeInteractions stemming from a sliding window will not hold
// their order due to the randomization of ranging over a map.  This occurs because
// the Window holds its data in a map and after a Slide() the order of these records
// will be iterated differently. Therefore, this means that the PairHash for a given
// pair of records may be recorded as the hash of {rec1, rec2} or {rec2, rec1} and
// both must be checked for existence before a new *RecordPair is inserted into the
// interactions map.
func (inter *Interactions) writeInteractions(data []*Record) error {
	if len(data) <= 1 { // only write two vessel interactions
		return nil
	}
	rec1 := data[0]
	for _, rec2 := range data[1:] {
		hash, err := PairHash64(rec1, rec2, inter.hashIndices)
		hash2, err := PairHash64(rec2, rec1, inter.hashIndices)
		if err != nil {
			return fmt.Errorf("write interactions: %v", err)
		}
		_, ok1 := inter.data[hash]
		_, ok2 := inter.data[hash2]
		if !ok1 && !ok2 { // neither Record order has been inserted
			inter.data[hash] = &RecordPair{rec1, rec2}
		}
	}
	return nil
}

// Save the interactions to a CSV file.
func (inter *Interactions) Save(filename string) error {
	out, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("interactions save: v", err)
	}

	w := csv.NewWriter(out)
	err = w.Write(inter.OutputHeaders.fields)
	if err != nil {
		return fmt.Errorf("interactions save: %v", err)
	}
	w.Flush()

	latIndex, _ := inter.RecordHeaders.Contains("LAT")
	lonIndex, _ := inter.RecordHeaders.Contains("LON")

	written := 1
	for hash, pair := range inter.data {
		d, err := pair.rec1.Distance(*(pair.rec2), latIndex, lonIndex)
		if err != nil {
			return fmt.Errorf("interactions save: %v", err)
		}
		pairData := []string{fmt.Sprintf("%0#16x", hash), fmt.Sprintf("%.1f", d)}
		pairData = append(pairData, (*pair.rec1)...)
		pairData = append(pairData, (*pair.rec2)...)
		w.Write(pairData)
		written++
		if written%flushThreshold == 0 {
			w.Flush()
			if err := w.Error(); err != nil {
				return fmt.Errorf("interactions save: flush error: %v", err)
			}
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return fmt.Errorf("interactions save: flush error: %v", err)
	}

	return nil
}

// PairHash64 returns PairHash from two AIS records based on the string values of
// MMSI, BaseDateTime, LAT, and LON for each vessel.  The argument indices must
// contain the index values in rec1 and rec2 for MMSI, BaseDateTime, LAT and LON.
func PairHash64(rec1, rec2 *Record, indices [4]int) (uint64, error) {
	h64 := fnv.New64a()
	for i := range indices {
		_, err := h64.Write([]byte((*rec1)[i]))
		if err != nil {
			return 0, err
		}
		_, err = h64.Write([]byte((*rec2)[i]))
		if err != nil {
			return 0, err
		}
	}

	return h64.Sum64(), nil
}
