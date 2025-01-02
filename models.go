package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"log"
	"os"
	"sync"
	"time"
)

type Logger struct {
	file   *os.File
	logger *log.Logger
	mu     sync.Mutex
}

type ResultWindow struct {
	currentResults  []ChipResult
	originalResults []ChipResult
	window          fyne.Window
	table           *widget.Table
	searchEntry     *widget.Entry
}

type ManualTime struct {
	Chip     string    `json:"chip"`
	Time     time.Time `json:"time"`
	RaceName string    `json:"raceName"`
}

type ChipResult struct {
	Chip     string        `json:"chip"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"duration"`
	Invalid  bool          `json:"invalid"`
	Manual   bool          `json:"manual"`
}

type Race struct {
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	MinTime      time.Duration   `json:"minTime"`
	Chips        map[string]bool `json:"chips"`
	ResultsFile  string          `json:"resultsFile"`
	InvalidTimes map[string]bool `json:"invalidTimes"`
	LiveUpdate   bool            `json:"liveUpdate"`
}

type DurationRace struct {
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	MinTime      string          `json:"minTime"`
	Chips        map[string]bool `json:"chips"`
	ResultsFile  string          `json:"resultsFile"`
	InvalidTimes map[string]bool `json:"invalidTimes"`
	LiveUpdate   bool            `json:"liveUpdate"`
}

type DurationChipResult struct {
	Chip     string    `json:"chip"`
	Time     time.Time `json:"time"`
	Duration string    `json:"duration"`
}
