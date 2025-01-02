package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type DurationRace struct {
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	MinTime      string          `json:"minTime"`
	Chips        map[string]bool `json:"chips"`
	ResultsFile  string          `json:"resultsFile"`
	InvalidTimes map[string]bool `json:"invalidTimes"`
	LiveUpdate   bool            `json:"liveUpdate"`
}

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

type DurationChipResult struct {
	Chip     string    `json:"chip"`
	Time     time.Time `json:"time"`
	Duration string    `json:"duration"`
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

type Race struct {
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	MinTime      time.Duration   `json:"minTime"`
	Chips        map[string]bool `json:"chips"`
	ResultsFile  string          `json:"resultsFile"`
	InvalidTimes map[string]bool `json:"invalidTimes"`
	LiveUpdate   bool            `json:"liveUpdate"`
}

type ChipResult struct {
	Chip     string        `json:"chip"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"duration"`
	Invalid  bool          `json:"invalid"`
}

// Hjälpfunktion för att skapa nyckel för InvalidTimes
func makeInvalidTimeKey(chip string, timestamp time.Time) string {
	return fmt.Sprintf("%s:%d", chip, timestamp.UnixNano())
}

// Hjälpfunktion för att avrunda tid uppåt till närmsta sekund
func roundUpToSecond(t time.Time) time.Time {
	return t.Add(time.Second - time.Duration(t.Nanosecond()))
}

// Lägg till en global map för att hålla stopWatcher-funktioner
var stopWatchers = make(map[string]func())

// Lägg till en funktion för att övervaka filändringar
func watchFile(filename string, raceName string, callback func()) (func(), error) {
	if filename == "" {
		return nil, fmt.Errorf("ingen fil att övervaka")
	}

	lastModified, err := getFileModTime(filename)
	if err != nil {
		return nil, err
	}

	quit := make(chan bool)
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-quit:
				return
			case <-ticker.C:
				currentModTime, err := getFileModTime(filename)
				if err != nil {
					continue
				}
				if currentModTime != lastModified {
					lastModified = currentModTime
					logToFile(fmt.Sprintf("Fil uppdaterad för lopp: %s", raceName))
					callback()
				}
			}
		}
	}()

	return func() { quit <- true }, nil
}

func getFileModTime(filename string) (time.Time, error) {
	info, err := os.Stat(filename)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// Lägg till en ny struct för att spara manuella tider
type ManualTime struct {
	Chip     string    `json:"chip"`
	Time     time.Time `json:"time"`
	RaceName string    `json:"raceName"`
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

// Lägg till denna funktion för loggning
func logToFile(message string) {
	file, err := os.OpenFile("tidtagning.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer file.Close()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	logMessage := fmt.Sprintf("[%s] %s\n", timestamp, message)
	file.WriteString(logMessage)
}

// Lägg till en global variabel för att hålla aktiva söktermer per fönster
var activeSearches = make(map[string]string)

// Uppdatera showResults-funktionen för att hantera sökning
func showResults(resultTable *widget.Table, race Race, races []Race, index int, app fyne.App) {
	resultWindow := app.NewWindow(fmt.Sprintf("Resultat - %s", race.Name))
	windowID := fmt.Sprintf("results_%s", race.Name)

	// Spara originalresultaten
	originalResults := getResults(race.ResultsFile, race)
	currentResults := originalResults

	// Sökfält
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Sök startnummer...")

	// Skapa tabellen först
	table := widget.NewTable(
		func() (int, int) {
			return len(currentResults) + 1, 3
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)

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
			} else if id.Row <= len(currentResults) {
				result := currentResults[id.Row-1]
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
					} else {
						label.SetText("OK")
					}
				}

				if result.Invalid {
					label.TextStyle = fyne.TextStyle{Italic: true}
				} else {
					label.TextStyle = fyne.TextStyle{}
				}
			}
		})

	table.SetColumnWidth(0, 150)
	table.SetColumnWidth(1, 150)
	table.SetColumnWidth(2, 150)

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
		addTimeButton,
		widget.NewLabel("Klicka på en rad för att markera/avmarkera den som felaktig"),
		tableContainer,
	)

	paddedContent := container.NewPadded(content)
	resultWindow.SetContent(paddedContent)
	resultWindow.Resize(fyne.NewSize(800, 900))
	resultWindow.CenterOnScreen()
	resultWindow.Show()

	// Spara stopWatcher för att kunna stänga av övervakningen när fönstret stängs
	var stopWatcher func()
	var watchErr error
	if race.LiveUpdate {
		stopWatcher, watchErr = watchFile(race.ResultsFile, race.Name, func() {
			logToFile(fmt.Sprintf("Processar resultat för lopp: %s", race.Name))

			// Ta bort cache så vi får färska resultat
			os.Remove(fmt.Sprintf("results_%s.json", race.Name))

			// Hämta nya resultat
			newResults := getResults(race.ResultsFile, race)

			// Uppdatera både original- och currentResults
			originalResults = newResults

			// Applicera eventuell aktiv sökning på currentResults
			if searchText, exists := activeSearches[windowID]; exists && searchText != "" {
				currentResults = updateResults(originalResults, searchText)
			} else {
				currentResults = originalResults
			}

			// Uppdatera tabellen
			table.Length = func() (int, int) {
				return len(currentResults) + 1, 3
			}

			// Uppdatera alla celler
			for row := 0; row <= len(currentResults); row++ {
				for col := 0; col < 3; col++ {
					if row == 0 {
						// Rubrikrad
						switch col {
						case 0:
							table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Startnr"))
						case 1:
							table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Tid"))
						case 2:
							table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Status"))
						}
					} else {
						result := currentResults[row-1]
						switch col {
						case 0:
							table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(result.Chip))
						case 1:
							minutes := int(result.Duration.Minutes())
							seconds := int(result.Duration.Seconds()) % 60
							timeStr := fmt.Sprintf("%02d:%02d", minutes, seconds)
							if minutes >= 60 {
								hours := minutes / 60
								minutes = minutes % 60
								timeStr = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
							}
							table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(timeStr))
						case 2:
							statusStr := "OK"
							if result.Invalid {
								statusStr = "Felaktig"
							}
							table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(statusStr))
						}
					}
				}
			}

			// Tvinga omritning av tabellen
			table.Refresh()
		})
		if watchErr != nil {
			dialog.ShowError(watchErr, resultWindow)
		}
	}

	// Rensa sökningen och stoppa övervakningen när fönstret stängs
	resultWindow.SetOnClosed(func() {
		delete(activeSearches, windowID)
		if stopWatcher != nil {
			stopWatcher()
		}
	})

	// Sedan lägg till sökfunktionen
	searchEntry.OnChanged = func(searchText string) {
		activeSearches[windowID] = searchText
		currentResults = updateResults(originalResults, searchText)
		table.Length = func() (int, int) {
			return len(currentResults) + 1, 3
		}
		table.Refresh()
	}

	// Återställ tidigare sökning om den finns
	if previousSearch, exists := activeSearches[windowID]; exists {
		searchEntry.SetText(previousSearch)
		currentResults = updateResults(originalResults, previousSearch)
	}
}

func main() {
	myApp := app.NewWithID("se.tidtagning.app")
	window := myApp.NewWindow("Tidtagning")

	// Ladda sparade lopp vid start
	races, err := loadRaces()
	if err != nil {
		dialog.ShowError(err, window)
	}

	raceContainer := container.NewVBox()

	// Deklarera updateRaceList först
	var updateRaceList func()

	// Sedan definiera den
	updateRaceList = func() {
		// Rensa alla objekt från containern
		raceContainer.RemoveAll()

		// Skapa nya objekt för varje lopp
		for i := range races {
			i := i           // Skapa en ny variabel för varje iteration
			race := races[i] // Skapa en kopia av race för denna iteration

			// Skapa en live-uppdateringsknapp
			liveButton := widget.NewButton("", nil)
			if race.LiveUpdate {
				liveButton.SetIcon(theme.MediaPauseIcon())
				liveButton.Importance = widget.HighImportance // Blå
			} else {
				liveButton.SetIcon(theme.MediaPlayIcon())
				liveButton.Importance = widget.SuccessImportance // Grön
			}

			liveButton.OnTapped = func() {
				race.LiveUpdate = !race.LiveUpdate
				races[i] = race
				saveRaces(races)

				if race.LiveUpdate {
					stopWatcher, err := watchFile(race.ResultsFile, race.Name, func() {
						logToFile(fmt.Sprintf("Processar resultat för lopp: %s", race.Name))

						// Ta bort cache så vi får färska resultat
						os.Remove(fmt.Sprintf("results_%s.json", race.Name))

						// Hitta det öppna resultatfönstret
						for _, w := range myApp.Driver().AllWindows() {
							if w.Title() == fmt.Sprintf("Resultat - %s", race.Name) {
								// Hämta nya resultat direkt från filen och cacha dem
								results := getResults(race.ResultsFile, race)

								// Uppdatera UI
								content := w.Content().(*fyne.Container)
								for _, obj := range content.Objects {
									if scroll, ok := obj.(*container.Scroll); ok {
										if table, ok := scroll.Content.(*widget.Table); ok {
											// Uppdatera tabellens längd
											table.Length = func() (int, int) {
												return len(results) + 1, 3
											}

											// Uppdatera alla celler
											for row := 0; row <= len(results); row++ {
												for col := 0; col < 3; col++ {
													if row == 0 {
														// Rubrikrad
														switch col {
														case 0:
															table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Startnr"))
														case 1:
															table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Tid"))
														case 2:
															table.UpdateCell(widget.TableCellID{Row: 0, Col: col}, widget.NewLabel("Status"))
														}
													} else {
														result := results[row-1]
														switch col {
														case 0:
															table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(result.Chip))
														case 1:
															minutes := int(result.Duration.Minutes())
															seconds := int(result.Duration.Seconds()) % 60
															timeStr := fmt.Sprintf("%02d:%02d", minutes, seconds)
															if minutes >= 60 {
																hours := minutes / 60
																minutes = minutes % 60
																timeStr = fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
															}
															table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(timeStr))
														case 2:
															statusStr := "OK"
															if result.Invalid {
																statusStr = "Felaktig"
															}
															table.UpdateCell(widget.TableCellID{Row: row, Col: col}, widget.NewLabel(statusStr))
														}
													}
												}
											}

											// Tvinga omritning av tabellen
											table.Refresh()
											break
										}
									}
								}
							}
						}
					})
					if err != nil {
						dialog.ShowError(err, window)
						race.LiveUpdate = false
						races[i] = race
						saveRaces(races)
					} else {
						stopWatchers[race.Name] = stopWatcher
					}
				} else {
					// Stoppa övervakning av filen
					if stopWatcher, exists := stopWatchers[race.Name]; exists {
						stopWatcher()
						delete(stopWatchers, race.Name)
					}
				}
				updateRaceList()
			}

			editButton := widget.NewButton("Redigera", func() {
				showEditRaceForm(race, i, races, myApp, window, updateRaceList)
			})

			processButton := widget.NewButton("Processa resultat", func() {
				if race.ResultsFile != "" {
					results := processRaceResults(race.ResultsFile, race, races, i)
					showResults(results, race, races, i, myApp)
				} else {
					showLargeFileDialog(func(reader fyne.URIReadCloser, err error) {
						if err != nil {
							dialog.ShowError(err, window)
							return
						}
						if reader == nil {
							return
						}

						filename := reader.URI().Path()
						reader.Close()

						race.ResultsFile = filename
						races[i] = race
						saveRaces(races)

						results := processRaceResults(filename, race, races, i)
						showResults(results, race, races, i, myApp)
					}, window)
				}
			})

			changeFileButton := widget.NewButton("Välj ny fil", func() {
				showLargeFileDialog(func(reader fyne.URIReadCloser, err error) {
					if err != nil {
						dialog.ShowError(err, window)
						return
					}
					if reader == nil {
						return
					}

					filename := reader.URI().Path()
					reader.Close()

					race.ResultsFile = filename
					races[i] = race
					saveRaces(races)

					results := processRaceResults(filename, race, races, i)
					showResults(results, race, races, i, myApp)
				}, window)
			})

			raceBox := container.NewHBox(
				liveButton,
				widget.NewLabel(fmt.Sprintf("%s - %s (%d deltagare)",
					race.Name,
					race.StartTime.Format("2006-01-02 15:04"),
					len(race.Chips))),
				editButton,
				processButton,
				changeFileButton,
			)
			raceContainer.Add(raceBox)
		}

		// Tvinga omritning av containern
		raceContainer.Refresh()
	}

	// Anropa updateRaceList direkt efter att vi har laddat loppen
	updateRaceList()

	addRace := func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("Loppets namn (t.ex. '10km')")
		nameEntry.Resize(fyne.NewSize(300, 40))

		dateEntry := widget.NewEntry()
		dateEntry.SetPlaceHolder("Datum (YYYY-MM-DD)")
		dateEntry.Text = time.Now().Format("2006-01-02")
		dateEntry.Resize(fyne.NewSize(300, 40))

		timeEntry := widget.NewEntry()
		timeEntry.SetPlaceHolder("Starttid (HH:MM)")
		timeEntry.Resize(fyne.NewSize(300, 40))

		minTimeEntry := widget.NewEntry()
		minTimeEntry.SetPlaceHolder("Minsta tid (MM:SS)")
		minTimeEntry.Resize(fyne.NewSize(300, 40))

		chipsEntry := widget.NewMultiLineEntry()
		chipsEntry.SetPlaceHolder("Klistra in startnummer (ett per rad)")
		chipsEntry.Resize(fyne.NewSize(300, 200))

		formItems := []*widget.FormItem{
			{Text: "Namn", Widget: nameEntry},
			{Text: "Datum", Widget: dateEntry},
			{Text: "Starttid", Widget: timeEntry},
			{Text: "Minsta tid", Widget: minTimeEntry},
			{Text: "Startnummer", Widget: chipsEntry},
		}

		formDialog := dialog.NewForm("Lägg till lopp", "Lägg till", "Avbryt", formItems, func(submitted bool) {
			if !submitted {
				return
			}

			// Parsa minimitid
			minTimeParts := strings.Split(minTimeEntry.Text, ":")
			var minTime time.Duration
			if len(minTimeParts) == 2 {
				minutes, _ := strconv.Atoi(minTimeParts[0])
				seconds, _ := strconv.Atoi(minTimeParts[1])
				minTime = time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
			}

			dateTime := dateEntry.Text + " " + timeEntry.Text
			startTime, err := time.Parse("2006-01-02 15:04", dateTime)
			if err != nil {
				dialog.ShowError(fmt.Errorf("Ogiltigt datum eller tid: %v", err), window)
				return
			}

			chips := make(map[string]bool)
			for _, chip := range strings.Split(chipsEntry.Text, "\n") {
				chip = strings.TrimSpace(chip)
				if chip != "" {
					chips[chip] = true
				}
			}

			race := Race{
				Name:         nameEntry.Text,
				StartTime:    startTime,
				MinTime:      minTime,
				Chips:        chips,
				InvalidTimes: make(map[string]bool),
				LiveUpdate:   false,
			}
			races = append(races, race)
			updateRaceList()
		}, window)

		formDialog.Resize(fyne.NewSize(600, 800))
		formDialog.Show()
	}

	addButton := widget.NewButton("Lägg till lopp", addRace)

	content := container.New(layout.NewVBoxLayout(),
		widget.NewLabel("Aktiva lopp:"),
		raceContainer,
		addButton,
	)

	window.SetContent(content)

	window.Resize(fyne.NewSize(1536, 864))
	window.CenterOnScreen()

	window.ShowAndRun()
}

func processRaceResults(filename string, race Race, races []Race, index int) *widget.Table {
	// Hämta resultaten
	results := getResults(filename, race)

	// Skapa tabellen
	table := widget.NewTable(
		func() (int, int) {
			return len(results) + 1, 3
		},
		func() fyne.CanvasObject {
			return widget.NewLabel("")
		},
		func(id widget.TableCellID, cell fyne.CanvasObject) {
			label := cell.(*widget.Label)

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
			} else if id.Row <= len(results) {
				result := results[id.Row-1]
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
					} else {
						label.SetText("OK")
					}
				}

				if result.Invalid {
					label.TextStyle = fyne.TextStyle{Italic: true}
				} else {
					label.TextStyle = fyne.TextStyle{}
				}
			}
		})

	table.SetColumnWidth(0, 150)
	table.SetColumnWidth(1, 150)
	table.SetColumnWidth(2, 150)

	return table
}

// Hjälpfunktion för att formatera resultat
func formatResults(race Race, results []ChipResult) string {
	var result strings.Builder
	result.WriteString(fmt.Sprintf("Resultat för %s\n", race.Name))
	result.WriteString(fmt.Sprintf("Startade: %s\n", race.StartTime.Format("2006-01-02 15:04")))
	result.WriteString(fmt.Sprintf("Minsta tid: %d:%02d\n\n",
		int(race.MinTime.Minutes()),
		int(race.MinTime.Seconds())%60))
	result.WriteString("Startnr\tTid\n")
	result.WriteString("--------------------\n")

	for _, st := range results {
		minutes := int(st.Duration.Minutes())
		seconds := int(st.Duration.Seconds()) % 60
		if minutes >= 60 {
			hours := minutes / 60
			minutes = minutes % 60
			result.WriteString(fmt.Sprintf("%s\t%02d:%02d:%02d\n",
				st.Chip, hours, minutes, seconds))
		} else {
			result.WriteString(fmt.Sprintf("%s\t%02d:%02d\n",
				st.Chip, minutes, seconds))
		}
	}

	return result.String()
}

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

func loadCachedResults(raceName string) ([]ChipResult, error) {
	file, err := os.Open(fmt.Sprintf("results_%s.json", raceName))
	if err != nil {
		if os.IsNotExist(err) {
			return []ChipResult{}, nil
		}
		return nil, err
	}
	defer file.Close()

	var results []ChipResult
	err = json.NewDecoder(file).Decode(&results)
	return results, err
}

func showEditRaceForm(race Race, index int, races []Race, app fyne.App, parentWindow fyne.Window, updateUI func()) {
	editWindow := app.NewWindow("Redigera lopp")

	nameEntry := widget.NewEntry()
	nameEntry.SetText(race.Name)
	nameEntry.Resize(fyne.NewSize(300, 40))

	dateEntry := widget.NewEntry()
	dateEntry.SetText(race.StartTime.Format("2006-01-02"))
	dateEntry.Resize(fyne.NewSize(300, 40))

	timeEntry := widget.NewEntry()
	timeEntry.SetText(race.StartTime.Format("15:04"))
	timeEntry.Resize(fyne.NewSize(300, 40))

	minTimeEntry := widget.NewEntry()
	minutes := int(race.MinTime.Minutes())
	seconds := int(race.MinTime.Seconds()) % 60
	minTimeEntry.SetText(fmt.Sprintf("%02d:%02d", minutes, seconds))
	minTimeEntry.Resize(fyne.NewSize(300, 40))

	var chipsList strings.Builder
	for chip := range race.Chips {
		chipsList.WriteString(chip + "\n")
	}
	chipsEntry := widget.NewMultiLineEntry()
	chipsEntry.SetText(chipsList.String())
	chipsEntry.Resize(fyne.NewSize(300, 200))

	// Skapa knappar
	saveButton := widget.NewButton("Spara", func() {
		// Parsa minimitid
		minTimeParts := strings.Split(minTimeEntry.Text, ":")
		var minTime time.Duration
		if len(minTimeParts) == 2 {
			minutes, _ := strconv.Atoi(minTimeParts[0])
			seconds, _ := strconv.Atoi(minTimeParts[1])
			minTime = time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second
		}

		dateTime := dateEntry.Text + " " + timeEntry.Text
		startTime, err := time.Parse("2006-01-02 15:04", dateTime)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Ogiltigt datum eller tid: %v", err), editWindow)
			return
		}

		chips := make(map[string]bool)
		for _, chip := range strings.Split(chipsEntry.Text, "\n") {
			chip = strings.TrimSpace(chip)
			if chip != "" {
				chips[chip] = true
			}
		}

		// Uppdatera loppet
		races[index] = Race{
			Name:         nameEntry.Text,
			StartTime:    startTime,
			MinTime:      minTime,
			Chips:        chips,
			InvalidTimes: race.InvalidTimes,
			LiveUpdate:   race.LiveUpdate,
		}

		// Ta bort cachade resultat eftersom loppet har ändrats
		os.Remove(fmt.Sprintf("results_%s.json", race.Name))

		updateUI()
		editWindow.Close()
	})

	deleteButton := widget.NewButton("Ta bort lopp", func() {
		dialog.ShowConfirm("Ta bort lopp",
			"Är du säker på att du vill ta bort detta lopp?",
			func(remove bool) {
				if remove {
					// Ta bort loppet från races slice
					races = append(races[:index], races[index+1:]...)
					// Ta bort cachade resultat
					os.Remove(fmt.Sprintf("results_%s.json", race.Name))
					// Spara races.json INNAN vi uppdaterar UI
					saveRaces(races)
					// Uppdatera UI
					updateUI()
					editWindow.Close()
				}
			}, editWindow)
	})

	cancelButton := widget.NewButton("Avbryt", func() {
		editWindow.Close()
	})

	// Skapa formuläret
	form := &widget.Form{
		Items: []*widget.FormItem{
			{Text: "Namn", Widget: nameEntry},
			{Text: "Datum", Widget: dateEntry},
			{Text: "Starttid", Widget: timeEntry},
			{Text: "Minsta tid", Widget: minTimeEntry},
			{Text: "Startnummer", Widget: chipsEntry},
		},
	}

	// Skapa layout
	buttons := container.NewHBox(saveButton, deleteButton, cancelButton)
	content := container.NewVBox(form, buttons)

	editWindow.SetContent(content)
	editWindow.Resize(fyne.NewSize(600, 900))
	editWindow.CenterOnScreen()
	editWindow.Show()
}

// Större fildialog
func showLargeFileDialog(callback func(fyne.URIReadCloser, error), window fyne.Window) {
	d := dialog.NewFileOpen(callback, window)
	// Sätt storlek direkt på dialogen
	d.Resize(fyne.NewSize(800, 600))
	d.Show()
}

func getResults(filename string, race Race) []ChipResult {
	// Försök först att läsa cachade resultat
	if cachedResults, err := loadCachedResults(race.Name); err == nil && len(cachedResults) > 0 {
		// Återställ Invalid-status från race.InvalidTimes
		for i := range cachedResults {
			timeKey := makeInvalidTimeKey(cachedResults[i].Chip, cachedResults[i].Time)
			cachedResults[i].Invalid = race.InvalidTimes[timeKey]
		}
		return cachedResults
	}

	results := []ChipResult{}

	// Läs in manuella tider först
	manualTimes, err := loadManualTimes(race.Name)
	if err == nil {
		for _, mt := range manualTimes {
			if mt.RaceName == race.Name {
				duration := mt.Time.Sub(race.StartTime)
				results = append(results, ChipResult{
					Chip:     mt.Chip,
					Time:     mt.Time,
					Duration: duration,
					Invalid:  false,
				})
			}
		}
	}

	// Läs in tider från CSV-filen
	if filename != "" {
		file, err := os.Open(filename)
		if err == nil {
			defer file.Close()

			reader := csv.NewReader(file)
			reader.Comma = '\t'
			reader.FieldsPerRecord = -1

			chipTimes := make(map[string]time.Time)

			for {
				record, err := reader.Read()
				if err == io.EOF {
					break
				}
				if err != nil || len(record) < 2 {
					continue
				}

				chip := record[0]
				timeStr := record[1]

				if !race.Chips[chip] {
					continue
				}

				recordTime, err := time.Parse("2006-01-02 15:04:05.000", timeStr)
				if err != nil {
					continue
				}

				recordTime = roundUpToSecond(recordTime)

				if recordTime.After(race.StartTime) {
					duration := recordTime.Sub(race.StartTime)
					if duration >= race.MinTime {
						if _, exists := chipTimes[chip]; !exists {
							chipTimes[chip] = recordTime
						}
					}
				}
			}

			// Lägg till CSV-tider i resultaten
			for chip, time := range chipTimes {
				duration := time.Sub(race.StartTime)
				timeKey := makeInvalidTimeKey(chip, time)
				results = append(results, ChipResult{
					Chip:     chip,
					Time:     time,
					Duration: duration,
					Invalid:  race.InvalidTimes[timeKey],
				})
			}
		}
	}

	// Sortera alla resultat efter tid
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time.Before(results[j].Time)
	})

	// Cacha resultaten innan vi returnerar
	if err := cacheResults(race.Name, results); err != nil {
		logToFile(fmt.Sprintf("Fel vid cachning av resultat för %s: %v", race.Name, err))
	}

	return results
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
