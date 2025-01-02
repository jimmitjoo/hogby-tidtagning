package main

import (
	"fmt"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"
	"os"
	"sort"
	"strings"
)

func updateAllUI(race *Race, updateMainWindow func(), appState *AppState) {
	// 1. Uppdatera alla öppna resultatfönster för detta lopp
	windowID := fmt.Sprintf("results_%s", race.Name)
	if rw, exists := appState.GetResultWindow(windowID); exists {
		// Uppdatera data
		newResults := getAllResults(*race)
		rw.originalResults = newResults
		if rw.searchEntry != nil && rw.searchEntry.Text != "" {
			rw.currentResults = updateResults(newResults, rw.searchEntry.Text)
		} else {
			rw.currentResults = newResults
		}

		// Uppdatera tabellen
		rw.table.Refresh()

		// Uppdatera watchButton i resultatfönstret
		content := rw.window.Content().(*fyne.Container)
		for _, obj := range content.Objects {
			if container, ok := obj.(*fyne.Container); ok {
				for _, containerObj := range container.Objects {
					if btn, ok := containerObj.(*widget.Button); ok {
						if strings.Contains(btn.Text, "automatisk uppdatering") {
							if race.LiveUpdate {
								btn.SetIcon(theme.MediaPauseIcon())
								btn.SetText("Stoppa automatisk uppdatering")
								btn.Importance = widget.HighImportance // Blå för att stoppa
							} else {
								btn.SetIcon(theme.MediaPlayIcon())
								btn.SetText("Starta automatisk uppdatering")
								btn.Importance = widget.SuccessImportance // Grön för att starta
							}
							btn.Refresh()
							break
						}
					}
				}
			}
		}
	}

	// 2. Uppdatera huvudfönstret
	updateMainWindow()
}

// Uppdatera toggleLiveUpdate för att använda den nya funktionen
func toggleLiveUpdate(race *Race, races []Race, index int, updateUI func(), appState *AppState) {
	// Ändra status först
	race.LiveUpdate = !race.LiveUpdate
	races[index] = *race
	saveRaces(races)

	// Uppdatera alla UI-komponenter först
	updateAllUI(race, updateUI, appState)

	// Hantera filewatcher efter UI-uppdateringen
	if race.LiveUpdate {
		if _, exists := appState.stopWatchers[race.Name]; exists {
			return
		}
		stopWatcher, err := CreateFileWatcher(*race, races, index, nil, func() {
			updateAllUI(race, updateUI, appState)
		}, appState)
		if err != nil {
			getLogger().Log("Fel vid start av övervakning: %v", err)
			race.LiveUpdate = false
			races[index] = *race
			saveRaces(races)
			// Uppdatera UI igen efter felhantering
			updateAllUI(race, updateUI, appState)
		} else {
			appState.AddStopWatcher(race.Name, stopWatcher)
		}
	} else {
		appState.RemoveStopWatcher(race.Name)
	}
}

// Uppdatera showResults-funktionen för att hantera sökning
func showResults(resultTable *widget.Table, race Race, races []Race, index int, app fyne.App, updateRaceList func(), appState *AppState) {
	resultWindow := app.NewWindow(fmt.Sprintf("Resultat - %s", race.Name))
	windowID := fmt.Sprintf("results_%s", race.Name)

	// Spara originalresultaten
	originalResults := getAllResults(race)
	currentResults := originalResults

	// Skapa en variabel för att hålla stopWatcher-funktionen
	var stopWatcher func()

	// Skapa tabellen först (flytta upp table-deklarationen)
	table := widget.NewTable(
		func() (int, int) {
			return len(currentResults) + 1, 3
		},
		func() fyne.CanvasObject {
			rect := canvas.NewRectangle(theme.BackgroundColor())
			label := widget.NewLabel("")
			return container.NewMax(rect, label)
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			container := cell.(*fyne.Container)
			rect := container.Objects[0].(*canvas.Rectangle)
			label := container.Objects[1].(*widget.Label)

			if id.Row == 0 {
				// Rubrikrad
				switch id.Col {
				case 0:
					label.SetText("Startnr")
				case 1:
					label.SetText("Tid")
				case 2:
					label.SetText("Status")
				}
				label.TextStyle = fyne.TextStyle{Bold: true}
				rect.FillColor = theme.BackgroundColor()
			} else if id.Row <= len(currentResults) {
				result := currentResults[id.Row-1]

				// Sätt mycket tydligare röd bakgrund för felaktiga tider
				if result.Invalid {
					rect.FillColor = color.NRGBA{R: 255, G: 150, B: 150, A: 255}
				} else {
					rect.FillColor = theme.BackgroundColor()
				}

				switch id.Col {
				case 0:
					label.SetText(result.Chip)
				case 1:
					minutes := int(result.Duration.Minutes())
					seconds := int(result.Duration.Seconds()) % 60
					if minutes >= 60 {
						hours := minutes / 60
						minutes = minutes % 60
						label.SetText(fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds))
					} else {
						label.SetText(fmt.Sprintf("%02d:%02d", minutes, seconds))
					}
				case 2:
					if result.Invalid {
						label.SetText("Felaktig")
						label.TextStyle = fyne.TextStyle{Italic: true}
					} else {
						label.SetText("OK")
						label.TextStyle = fyne.TextStyle{}
					}
				}
			}
			rect.Refresh()
		})

	// Sätt kolumnbredder
	table.SetColumnWidth(0, 150)
	table.SetColumnWidth(1, 150)
	table.SetColumnWidth(2, 150)

	// Lägg till klickhantering för tabellen
	table.OnSelected = func(id widget.TableCellID) {
		// Ignorera klick på rubrikraden
		if id.Row == 0 {
			table.UnselectAll()
			return
		}

		// Hämta resultat för den klickade raden
		result := currentResults[id.Row-1]

		// Skapa tidsnyckel
		timeKey := makeInvalidTimeKey(result.Chip, result.Time)

		// Växla ogiltig-status
		result.Invalid = !result.Invalid
		currentResults[id.Row-1].Invalid = result.Invalid

		if result.Invalid && race.ResultsFile != "" {
			// Om tiden markeras som ogiltig, försök hitta nästa giltiga tid
			if nextTime, found := findNextValidTime(race.ResultsFile, race, result.Time, result.Chip); found {
				// Kontrollera om tiden redan finns i resultaten
				timeExists := false
				for _, existing := range currentResults {
					if existing.Chip == result.Chip && existing.Time.Equal(nextTime) {
						timeExists = true
						break
					}
				}

				// Lägg bara till tiden om den inte redan finns
				if !timeExists {
					// Lägg till den nya tiden i resultaten
					duration := nextTime.Sub(race.StartTime)
					newResult := ChipResult{
						Chip:     result.Chip,
						Time:     nextTime,
						Duration: duration,
						Invalid:  false,
					}

					// Lägg till i både current och original results
					currentResults = append(currentResults, newResult)
					originalResults = append(originalResults, newResult)

					// Sortera resultaten efter tid
					sort.Slice(currentResults, func(i, j int) bool {
						return currentResults[i].Time.Before(currentResults[j].Time)
					})
					sort.Slice(originalResults, func(i, j int) bool {
						return originalResults[i].Time.Before(originalResults[j].Time)
					})
				}
			}
		} else {
			// Om vi avmarkerar en tid som felaktig, ta bort eventuella senare tider för samma chip
			// Skapa nya slices utan de senare tiderna
			newCurrentResults := []ChipResult{}
			newOriginalResults := []ChipResult{}

			foundValidTime := false
			for _, r := range currentResults {
				if r.Chip == result.Chip {
					if !foundValidTime {
						newCurrentResults = append(newCurrentResults, r)
						foundValidTime = true
					}
				} else {
					newCurrentResults = append(newCurrentResults, r)
				}
			}

			foundValidTime = false
			for _, r := range originalResults {
				if r.Chip == result.Chip {
					if !foundValidTime {
						newOriginalResults = append(newOriginalResults, r)
						foundValidTime = true
					}
				} else {
					newOriginalResults = append(newOriginalResults, r)
				}
			}

			currentResults = newCurrentResults
			originalResults = newOriginalResults
		}

		// Uppdatera originalResults också
		for i := range originalResults {
			if originalResults[i].Chip == result.Chip && originalResults[i].Time.Equal(result.Time) {
				originalResults[i].Invalid = result.Invalid
				break
			}
		}

		// Uppdatera race.InvalidTimes
		if result.Invalid {
			race.InvalidTimes[timeKey] = true
		} else {
			delete(race.InvalidTimes, timeKey)
		}

		// Spara ändringarna
		races[index] = race
		saveRaces(races)

		// Uppdatera cachade resultat
		cacheResults(race.Name, originalResults)

		// Avmarkera raden och uppdatera tabellen
		table.UnselectAll()
		table.Refresh()
	}

	// Sedan kan vi skapa sökfältet och watch-knappen
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Sök startnummer...")

	// Skapa watch-knappen med alla egenskaper direkt
	watchButtonText := "Starta automatisk uppdatering"
	watchButtonIcon := theme.MediaPlayIcon()
	if race.LiveUpdate {
		watchButtonText = "Stoppa automatisk uppdatering"
		watchButtonIcon = theme.MediaPauseIcon()
	}
	watchButton := &widget.Button{
		Text:       watchButtonText,
		Icon:       watchButtonIcon,
		Importance: widget.SuccessImportance,
		OnTapped:   nil, // Vi sätter denna senare
	}
	if race.LiveUpdate {
		watchButton.Importance = widget.HighImportance
	}

	// Funktion för att uppdatera knappens utseende
	updateWatchButtonState := func() {
		if race.LiveUpdate {
			watchButton.SetIcon(theme.MediaPauseIcon())
			watchButton.SetText("Stoppa automatisk uppdatering")
			watchButton.Importance = widget.HighImportance // Blå för att stoppa
		} else {
			watchButton.SetIcon(theme.MediaPlayIcon())
			watchButton.SetText("Starta automatisk uppdatering")
			watchButton.Importance = widget.SuccessImportance // Grön för att starta
		}
	}

	// Skapa en updateUI-funktion som uppdaterar både huvudfönstret och resultatfönstret
	updateUI := func() {
		updateRaceList()
		updateWatchButtonState()
		table.Refresh()
	}

	// Sätt OnTapped
	watchButton.OnTapped = func() {
		toggleLiveUpdate(&race, races, index, updateUI, appState)
	}

	// Skapa en scroll container för tabellen
	tableContainer := container.NewScroll(table)
	tableContainer.SetMinSize(fyne.NewSize(600, 600))

	// Lägg till knapp för manuell tidsinmatning
	addTimeButton := widget.NewButton("Lägg till tid", func() {
		showAddTimeDialog(race, races, index, &currentResults, &originalResults, table, resultWindow)
	})

	content := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Resultat för %s", race.Name)),
		widget.NewLabel(fmt.Sprintf("Startade: %s", race.StartTime.Format("2006-01-02 15:04"))),
		widget.NewLabel(fmt.Sprintf("Minsta tid: %d:%02d",
			int(race.MinTime.Minutes()),
			int(race.MinTime.Seconds())%60)),
		searchEntry,
		watchButton,
		addTimeButton,
		widget.NewLabel("Klicka på en rad för att markera/avmarkera den som felaktig"),
		tableContainer,
	)

	paddedContent := container.NewPadded(content)
	resultWindow.SetContent(paddedContent)
	resultWindow.Resize(fyne.NewSize(800, 900))
	resultWindow.CenterOnScreen()
	resultWindow.Show()

	// Rensa sökningen och stoppa övervakningen när fönstret stängs
	resultWindow.SetOnClosed(func() {
		appState.RemoveResultWindow(windowID)
		if stopWatcher != nil {
			stopWatcher()
		}
		// Ta även bort watchern från stopWatchers om den finns där
		if race.LiveUpdate {
			if sw, exists := appState.stopWatchers[race.Name]; exists {
				sw()
				delete(appState.stopWatchers, race.Name)
				race.LiveUpdate = false
				races[index] = race
				saveRaces(races)
				updateRaceList()
			}
		}
	})

	// Sedan lägg till sökfunktionen
	searchEntry.OnChanged = func(searchText string) {
		appState.SetActiveSearch(windowID, searchText)
		currentResults = updateResults(originalResults, searchText)
		table.Length = func() (int, int) {
			return len(currentResults) + 1, 3
		}
		table.Refresh()
	}

	// Återställ tidigare sökning om den finns
	if previousSearch := appState.GetActiveSearch(windowID); previousSearch != "" {
		searchEntry.SetText(previousSearch)
		currentResults = updateResults(originalResults, previousSearch)
	}

	// I showResults, efter att vi skapat fönstret:
	rw := &ResultWindow{
		currentResults:  originalResults,
		originalResults: originalResults,
		window:          resultWindow,
		table:           table,
		searchEntry:     searchEntry,
	}
	appState.AddResultWindow(windowID, rw)
}

func makeRaceListItem(race Race, races []Race, index int, app fyne.App, updateUI func(), appState *AppState) *fyne.Container {
	// Skapa etiketter för loppinformation
	nameLabel := widget.NewLabel(race.Name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	// Hämta resultat
	results := getAllResults(race)

	startTimeStr := race.StartTime.Format("2006-01-02 15:04")
	timeLabel := widget.NewLabel(fmt.Sprintf("Starttid: %s", startTimeStr))

	participantsLabel := widget.NewLabel(fmt.Sprintf("Anmälda: %d", len(race.Chips)))

	finishersLabel := widget.NewLabel(fmt.Sprintf("Antal i mål: %d", len(results)-len(race.InvalidTimes)))

	buttons := raceListButtons(race, races, index, app, updateUI, appState)

	// Skapa en container för all information
	return container.NewVBox(
		nameLabel,
		container.NewHBox(
			timeLabel,
			participantsLabel,
			finishersLabel,
		),
		buttons,
		widget.NewSeparator(),
	)
}

func raceListButtons(race Race, races []Race, index int, app fyne.App, updateUI func(), appState *AppState) *fyne.Container {
	// Skapa resultatknappen
	resultsButton := widget.NewButton("Visa resultat", func() {
		showResults(nil, race, races, index, app, updateUI, appState)
	})

	// Skapa watch-knappen
	watchButton := widget.NewButton("", nil)

	// Funktion för att uppdatera knappens utseende
	updateWatchButtonState := func() {
		if race.LiveUpdate {
			watchButton.SetIcon(theme.MediaPauseIcon())
			watchButton.SetText("Stoppa automatisk uppdatering")
			watchButton.Importance = widget.HighImportance // Blå för att stoppa
		} else {
			watchButton.SetIcon(theme.MediaPlayIcon())
			watchButton.SetText("Starta automatisk uppdatering")
			watchButton.Importance = widget.SuccessImportance // Grön för att starta
		}
	}

	// Sätt initialt utseende
	updateWatchButtonState()

	// Sätt OnTapped
	watchButton.OnTapped = func() {
		toggleLiveUpdate(&race, races, index, updateUI, appState)
	}

	// Skapa knapp för att välja resultatfil
	fileButton := widget.NewButton("Välj resultatfil", func() {
		// Skapa en större fildialog
		d := dialog.NewFileOpen(func(reader fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, app.Driver().AllWindows()[0])
				return
			}
			if reader == nil {
				return
			}

			filename := reader.URI().Path()
			reader.Close()

			// Uppdatera loppet med den nya filen
			race.ResultsFile = filename
			races[index] = race
			saveRaces(races)

			// Visa resultat direkt efter att filen valts
			showResults(nil, race, races, index, app, updateUI, appState)
		}, app.Driver().AllWindows()[0])

		// Sätt storlek på dialogen
		d.Resize(fyne.NewSize(1200, 800))
		d.Show()
	})

	// Skapa ta bort-knappen
	deleteButton := widget.NewButtonWithIcon("", theme.DeleteIcon(), func() {
		dialog.ShowConfirm("Ta bort lopp",
			fmt.Sprintf("Är du säker på att du vill ta bort loppet %s?", race.Name),
			func(ok bool) {
				if ok {
					// Ta bort eventuell watcher
					appState.RemoveStopWatcher(race.Name)

					// Ta bort loppet från slicen
					// Läs in den aktuella listan med lopp
					currentRaces, err := loadRaces()
					if err != nil {
						dialog.ShowError(err, app.Driver().AllWindows()[0])
						return
					}

					// Hitta och ta bort rätt lopp baserat på namn och starttid
					for i, r := range currentRaces {
						if r.Name == race.Name && r.StartTime.Equal(race.StartTime) {
							currentRaces = append(currentRaces[:i], currentRaces[i+1:]...)
							break
						}
					}

					// Spara den uppdaterade listan
					if err := saveRaces(currentRaces); err != nil {
						dialog.ShowError(err, app.Driver().AllWindows()[0])
						return
					}

					// Ta bort cachade resultat
					os.Remove(fmt.Sprintf("results_%s.json", race.Name))

					// Uppdatera UI med den nya listan av lopp
					updateUI()
				}
			}, app.Driver().AllWindows()[0])
	})
	deleteButton.Importance = widget.DangerImportance

	// Skapa en container för knapparna
	buttons := container.NewHBox(resultsButton, fileButton, watchButton, deleteButton)
	return buttons
}
