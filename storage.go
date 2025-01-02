package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

// Spara/läsa lopp
func saveRaces(races []Race) error {
	file, err := os.Create("races.json")
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(races)
}

func loadRaces() ([]Race, error) {
	file, err := os.Open("races.json")
	if err != nil {
		if os.IsNotExist(err) {
			return []Race{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var races []Race
	err = json.NewDecoder(file).Decode(&races)
	return races, err
}

// Funktion för att cacha resultat
func cacheResults(raceName string, results []ChipResult) error {
	filename := fmt.Sprintf("results_%s.json", raceName)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(results)
}

// Lägg till updateResults-funktionen
func updateResults(results []ChipResult, searchText string) []ChipResult {
	if searchText == "" {
		return results
	}
	filtered := []ChipResult{}
	for _, result := range results {
		if strings.Contains(result.Chip, searchText) {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

// Ny funktion som samlar alla resultat
func getAllResults(race Race) []ChipResult {
	getLogger().Log("Hämtar alla resultat för lopp: %s", race.Name)
	results := []ChipResult{}

	// Läs in manuella tider först
	manualTimes, err := loadManualTimes(race.Name)
	if err == nil {
		for _, mt := range manualTimes {
			if mt.RaceName == race.Name {
				duration := mt.Time.Sub(race.StartTime)
				timeKey := makeInvalidTimeKey(mt.Chip, mt.Time)
				results = append(results, ChipResult{
					Chip:     mt.Chip,
					Time:     mt.Time,
					Duration: duration,
					Invalid:  race.InvalidTimes[timeKey],
					Manual:   true,
				})
			}
		}
		getLogger().Log("Läste in %d manuella tider", len(manualTimes))
	}

	// Skapa en map för att hålla koll på vilka chip vi redan har
	processedChips := make(map[string]bool)
	for _, result := range results {
		processedChips[result.Chip] = true
	}

	// Läs in tider från CSV-filen om den finns
	if race.ResultsFile != "" {
		csvResults := readCSVResults(race)
		// Lägg till CSV-resultat som inte redan finns som manuell tid
		for _, csvResult := range csvResults {
			if !processedChips[csvResult.Chip] {
				results = append(results, csvResult)
				processedChips[csvResult.Chip] = true
			}
		}
		getLogger().Log("Läste in %d CSV-resultat", len(csvResults))
	}

	// Sortera alla resultat efter tid
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time.Before(results[j].Time)
	})

	// Skapa JSON-fil med resultaten
	jsonData, err := json.Marshal(results)
	if err != nil {
		getLogger().Log("Fel vid skapande av JSON: %v", err)
		return results
	}

	jsonFilename := fmt.Sprintf("results_%s.json", race.Name)
	err = os.WriteFile(jsonFilename, jsonData, 0644)
	if err != nil {
		getLogger().Log("Fel vid sparande av JSON-fil: %v", err)
		return results
	}

	getLogger().Log("Returnerar totalt %d resultat", len(results))
	return results
}

// Separera CSV-läsningen till egen funktion
func readCSVResults(race Race) []ChipResult {
	results := []ChipResult{}
	file, err := os.Open(race.ResultsFile)
	if err != nil {
		getLogger().Log("Fel vid öppning av fil %s: %v", race.ResultsFile, err)
		return results
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1

	chipTimes := make(map[string]time.Time)
	rowCount := 0

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		rowCount++
		if err != nil || len(record) < 2 {
			continue
		}

		chip := record[0]
		if !race.Chips[chip] {
			continue
		}

		timeStr := record[1]
		recordTime, err := time.Parse("2006-01-02 15:04:05.000", timeStr)
		if err != nil {
			continue
		}

		recordTime = roundUpToSecond(recordTime)

		if recordTime.After(race.StartTime) {
			duration := recordTime.Sub(race.StartTime)
			if duration >= race.MinTime {
				if existingTime, exists := chipTimes[chip]; !exists || recordTime.Before(existingTime) {
					chipTimes[chip] = recordTime
				}
			}
		}
	}

	// Konvertera chipTimes till resultat
	for chip, time := range chipTimes {
		duration := time.Sub(race.StartTime)
		timeKey := makeInvalidTimeKey(chip, time)
		results = append(results, ChipResult{
			Chip:     chip,
			Time:     time,
			Duration: duration,
			Invalid:  race.InvalidTimes[timeKey],
			Manual:   false,
		})
	}

	return results
}
