package main

import (
	"encoding/json"
	"time"
)

// MarshalJSON för Race
func (r Race) MarshalJSON() ([]byte, error) {
	return json.Marshal(DurationRace{
		Name:         r.Name,
		StartTime:    r.StartTime,
		MinTime:      r.MinTime.String(),
		Chips:        r.Chips,
		ResultsFile:  r.ResultsFile,
		InvalidTimes: r.InvalidTimes,
		LiveUpdate:   r.LiveUpdate,
	})
}

// UnmarshalJSON för Race
func (r *Race) UnmarshalJSON(data []byte) error {
	var dr DurationRace
	if err := json.Unmarshal(data, &dr); err != nil {
		return err
	}

	minTime, err := time.ParseDuration(dr.MinTime)
	if err != nil {
		return err
	}

	r.Name = dr.Name
	r.StartTime = dr.StartTime
	r.MinTime = minTime
	r.Chips = dr.Chips
	r.ResultsFile = dr.ResultsFile
	r.InvalidTimes = dr.InvalidTimes
	r.LiveUpdate = dr.LiveUpdate
	return nil
}

// MarshalJSON för ChipResult
func (cr ChipResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(DurationChipResult{
		Chip:     cr.Chip,
		Time:     cr.Time,
		Duration: cr.Duration.String(),
	})
}

// UnmarshalJSON för ChipResult
func (cr *ChipResult) UnmarshalJSON(data []byte) error {
	var dcr DurationChipResult
	if err := json.Unmarshal(data, &dcr); err != nil {
		return err
	}

	duration, err := time.ParseDuration(dcr.Duration)
	if err != nil {
		return err
	}

	cr.Chip = dcr.Chip
	cr.Time = dcr.Time
	cr.Duration = duration
	return nil
}
