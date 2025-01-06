package main

import (
	"log"
	"os"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
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
	Name          string          `json:"name"`
	StartTime     time.Time       `json:"startTime"`
	MinTime       time.Duration   `json:"minTime"`
	Chips         map[string]bool `json:"chips"`
	ResultsFile   string          `json:"resultsFile"`
	InvalidTimes  map[string]bool `json:"invalidTimes"`
	LiveUpdate    bool            `json:"liveUpdate"`
	SpreadsheetId string          `json:"spreadsheetId"`
	SheetName     string          `json:"sheetName"`
}

type DurationRace struct {
	Name          string          `json:"name"`
	StartTime     time.Time       `json:"startTime"`
	MinTime       string          `json:"minTime"`
	Chips         map[string]bool `json:"chips"`
	ResultsFile   string          `json:"resultsFile"`
	InvalidTimes  map[string]bool `json:"invalidTimes"`
	LiveUpdate    bool            `json:"liveUpdate"`
	SpreadsheetId string          `json:"spreadsheetId"`
	SheetName     string          `json:"sheetName"`
}

type DurationChipResult struct {
	Chip     string    `json:"chip"`
	Time     time.Time `json:"time"`
	Duration string    `json:"duration"`
}
