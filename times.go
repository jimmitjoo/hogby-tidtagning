package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Hjälpfunktion för att skapa nyckel för InvalidTimes
func makeInvalidTimeKey(chip string, timestamp time.Time) string {
	return fmt.Sprintf("%s:%d", chip, timestamp.UnixNano())
}

// Hjälpfunktion för att avrunda tid uppåt till närmsta sekund
func roundUpToSecond(t time.Time) time.Time {
	return t.Add(time.Second - time.Duration(t.Nanosecond()))
}

// Funktion för att spara manuella tider
func saveManualTimes(raceName string, times []ManualTime) error {
	filename := fmt.Sprintf("manual_times_%s.json", raceName)
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(times)
}

// Funktion för att läsa manuella tider
func loadManualTimes(raceName string) ([]ManualTime, error) {
	filename := fmt.Sprintf("manual_times_%s.json", raceName)
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return []ManualTime{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var times []ManualTime
	err = json.NewDecoder(file).Decode(&times)
	return times, err
}

// Hitta nästa giltiga tid för ett chip
func findNextValidTime(filename string, race Race, invalidTime time.Time, chip string) (time.Time, bool) {
	file, err := os.Open(filename)
	if err != nil {
		return time.Time{}, false
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1

	var nextValidTime time.Time
	foundValid := false

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) < 2 {
			continue
		}

		if record[0] != chip {
			continue
		}

		recordTime, err := time.Parse("2006-01-02 15:04:05.000", record[1])
		if err != nil {
			continue
		}

		// Avrunda tiden uppåt till närmsta sekund
		recordTime = roundUpToSecond(recordTime)

		// Kontrollera om tiden är efter den ogiltiga tiden och efter starttiden
		if recordTime.After(invalidTime) && recordTime.After(race.StartTime) {
			duration := recordTime.Sub(race.StartTime)
			if duration >= race.MinTime {
				// Kontrollera om denna specifika tid är markerad som ogiltig
				timeKey := makeInvalidTimeKey(chip, recordTime)
				if !race.InvalidTimes[timeKey] {
					nextValidTime = recordTime
					foundValid = true
					break
				}
			}
		}
	}

	return nextValidTime, foundValid
}

// Manuell tidsinmatning
func showAddTimeDialog(race Race, races []Race, index int, currentResults *[]ChipResult, originalResults *[]ChipResult, table *widget.Table, window fyne.Window) {
	chipEntry := widget.NewEntry()
	chipEntry.SetPlaceHolder("Startnummer")

	timeEntry := widget.NewEntry()
	timeEntry.SetPlaceHolder("Tid (HH:MM:SS)")
	timeEntry.Text = "00:00:00"

	dialog.ShowForm("Lägg till tid", "Lägg till", "Avbryt", []*widget.FormItem{
		{Text: "Startnummer", Widget: chipEntry},
		{Text: "Tid", Widget: timeEntry},
	}, func(submitted bool) {
		if !submitted {
			return
		}

		// Validera chip
		chip := strings.TrimSpace(chipEntry.Text)
		if chip == "" {
			dialog.ShowError(fmt.Errorf("Startnummer måste anges"), window)
			return
		}
		if !race.Chips[chip] {
			dialog.ShowError(fmt.Errorf("Startnummer %s finns inte registrerat i loppet", chip), window)
			return
		}

		// Parsa tiden
		timeParts := strings.Split(timeEntry.Text, ":")
		if len(timeParts) != 3 {
			dialog.ShowError(fmt.Errorf("Ogiltig tid, använd format HH:MM:SS"), window)
			return
		}

		hours, err1 := strconv.Atoi(timeParts[0])
		minutes, err2 := strconv.Atoi(timeParts[1])
		seconds, err3 := strconv.Atoi(timeParts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			dialog.ShowError(fmt.Errorf("Ogiltig tid"), window)
			return
		}

		// Skapa tidpunkt baserat på loppets startdatum
		recordTime := race.StartTime.Add(time.Duration(hours)*time.Hour +
			time.Duration(minutes)*time.Minute +
			time.Duration(seconds)*time.Second)

		// Spara den manuella tiden
		manualTimes, _ := loadManualTimes(race.Name)
		manualTimes = append(manualTimes, ManualTime{
			Chip:     chip,
			Time:     recordTime,
			RaceName: race.Name,
		})
		saveManualTimes(race.Name, manualTimes)

		// Kontrollera att tiden är efter starttiden och uppfyller minimitiden
		duration := recordTime.Sub(race.StartTime)
		if duration < race.MinTime {
			dialog.ShowError(fmt.Errorf("Tiden är kortare än minimitiden"), window)
			return
		}

		// Markera alla existerande tider för detta chip som ogiltiga
		for i := range *currentResults {
			if (*currentResults)[i].Chip == chip {
				(*currentResults)[i].Invalid = true
				timeKey := makeInvalidTimeKey((*currentResults)[i].Chip, (*currentResults)[i].Time)
				if race.InvalidTimes == nil {
					race.InvalidTimes = make(map[string]bool)
				}
				race.InvalidTimes[timeKey] = true
			}
		}
		for i := range *originalResults {
			if (*originalResults)[i].Chip == chip {
				(*originalResults)[i].Invalid = true
				timeKey := makeInvalidTimeKey((*originalResults)[i].Chip, (*originalResults)[i].Time)
				race.InvalidTimes[timeKey] = true
			}
		}

		// Lägg till det nya resultatet
		newResult := ChipResult{
			Chip:     chip,
			Time:     recordTime,
			Duration: duration,
			Invalid:  false,
		}

		*currentResults = append(*currentResults, newResult)
		*originalResults = append(*originalResults, newResult)

		// Sortera resultaten
		sort.Slice(*currentResults, func(i, j int) bool {
			return (*currentResults)[i].Time.Before((*currentResults)[j].Time)
		})
		sort.Slice(*originalResults, func(i, j int) bool {
			return (*originalResults)[i].Time.Before((*originalResults)[j].Time)
		})

		// Spara ändringarna
		races[index] = race
		saveRaces(races)
		cacheResults(race.Name, *originalResults)

		// Uppdatera tabellen
		table.Refresh()
	}, window)
}
