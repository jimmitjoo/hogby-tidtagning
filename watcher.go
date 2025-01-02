package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"os"
	"time"
)

func CreateFileWatcher(race Race, races []Race, index int, window fyne.Window, updateUI func(), appState *AppState) (func(), error) {
	return watchFile(race.ResultsFile, race.Name, func() {
		getLogger().Log("Processar resultat för lopp: %s", race.Name)

		// Hämta nya resultat
		newResults := getAllResults(race)
		getLogger().Log("Hämtade %d nya resultat", len(newResults))

		// Uppdatera resultatfönstret om det är öppet
		windowID := fmt.Sprintf("results_%s", race.Name)
		if rw, exists := appState.GetResultWindow(windowID); exists {
			getLogger().Log("Uppdaterar öppet resultatfönster för %s", race.Name)

			// Uppdatera data
			rw.originalResults = newResults

			// Uppdatera currentResults med hänsyn till sökning
			if rw.searchEntry != nil && rw.searchEntry.Text != "" {
				rw.currentResults = updateResults(newResults, rw.searchEntry.Text)
			} else {
				rw.currentResults = newResults
			}

			// Uppdatera tabellen
			if rw.table != nil {
				// Uppdatera längdfunktionen
				rw.table.Length = func() (int, int) {
					return len(rw.currentResults) + 1, 3
				}
				rw.table.Refresh()
				getLogger().Log("Uppdaterade tabell med %d resultat", len(rw.currentResults))
			}
		}

		// Spara de nya resultaten i cache
		if err := cacheResults(race.Name, newResults); err != nil {
			getLogger().Log("Fel vid cachning av resultat: %v", err)
		}

		// Uppdatera huvudfönstret
		updateUI()
	})
}

func watchFile(filename string, raceName string, callback func()) (func(), error) {
	if filename == "" {
		return nil, fmt.Errorf("ingen fil att övervaka")
	}

	getLogger().Log("Startar övervakning av fil: %s för lopp: %s", filename, raceName)

	lastModified, err := getFileModTime(filename)
	if err != nil {
		return nil, fmt.Errorf("kunde inte läsa filtid: %v", err)
	}

	quit := make(chan bool)
	done := make(chan bool)

	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				getLogger().Log("Stoppar övervakning av fil: %s för lopp: %s", filename, raceName)
				done <- true
				return
			case <-ticker.C:
				currentModTime, err := getFileModTime(filename)
				if err != nil {
					getLogger().Log("Fel vid kontroll av fil %s: %v", filename, err)
					continue
				}
				if currentModTime.After(lastModified) {
					lastModified = currentModTime
					getLogger().Log("Fil ändrad: %s, anropar callback", filename)

					// Ta bort cache innan vi läser nya resultat
					cacheFile := fmt.Sprintf("results_%s.json", raceName)
					if err := os.Remove(cacheFile); err != nil && !os.IsNotExist(err) {
						getLogger().Log("Kunde inte ta bort cache: %v", err)
					}

					callback()
				}
			}
		}
	}()

	return func() {
		quit <- true
		<-done
	}, nil
}

func getFileModTime(filename string) (time.Time, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
