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
	"fyne.io/fyne/v2/widget"
)

type DurationRace struct {
	Name         string          `json:"name"`
	StartTime    time.Time       `json:"startTime"`
	MinTime      string          `json:"minTime"`
	Chips        map[string]bool `json:"chips"`
	ResultsFile  string          `json:"resultsFile"`
	InvalidTimes map[string]bool `json:"invalidTimes"`
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
			}
			races = append(races, race)
			updateRaceList()
		}, window)

		formDialog.Resize(fyne.NewSize(600, 800))
		formDialog.Show()
	}

	addButton := widget.NewButton("Lägg till lopp", addRace)

	content := container.NewVBox(
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
	return widget.NewTable(
		func() (int, int) { return 0, 0 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(id widget.TableCellID, cell fyne.CanvasObject) {},
	)
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
	file, err := os.Create(fmt.Sprintf("results_%s.json", raceName))
	if err != nil {
		return err
	}
	defer file.Close()

	// Spara resultaten med Invalid-status
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

	// Öppna filen
	file, err := os.Open(filename)
	if err != nil {
		return []ChipResult{}
	}
	defer file.Close()

	// Skapa en CSV-läsare med tab som separator
	reader := csv.NewReader(file)
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1 // Tillåt varierande antal fält

	// Skapa en map för att hålla första tiden per chip
	chipTimes := make(map[string]time.Time)

	// Läs varje rad
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		if len(record) < 2 {
			continue
		}

		chip := record[0]
		timeStr := record[1]

		// Kontrollera om detta chip tillhör detta lopp
		if !race.Chips[chip] {
			continue
		}

		// Parsa tiden
		recordTime, err := time.Parse("2006-01-02 15:04:05.000", timeStr)
		if err != nil {
			continue
		}

		// Kontrollera om tiden är efter starttiden och efter minimitiden
		if recordTime.After(race.StartTime) {
			duration := recordTime.Sub(race.StartTime)
			if duration >= race.MinTime {
				if _, exists := chipTimes[chip]; !exists {
					chipTimes[chip] = recordTime
				}
			}
		}
	}

	// Konvertera map till slice för sortering
	var results []ChipResult

	for chip, time := range chipTimes {
		duration := time.Sub(race.StartTime)
		timeKey := makeInvalidTimeKey(chip, time)
		isInvalid := race.InvalidTimes[timeKey]
		results = append(results, ChipResult{
			Chip:     chip,
			Time:     time,
			Duration: duration,
			Invalid:  isInvalid,
		})
	}

	// Sortera slice efter tid
	sort.Slice(results, func(i, j int) bool {
		return results[i].Time.Before(results[j].Time)
	})

	// Cacha resultaten
	cacheResults(race.Name, results)

	return results
}

// Visa resultat
func showResults(resultTable *widget.Table, race Race, races []Race, index int, app fyne.App) {
	resultWindow := app.NewWindow(fmt.Sprintf("Resultat - %s", race.Name))

	// Spara originalresultaten
	originalResults := getResults(race.ResultsFile, race)
	currentResults := originalResults

	// Skapa tabellen här istället för i processRaceResults
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

	// Sökfält
	searchEntry := widget.NewEntry()
	searchEntry.SetPlaceHolder("Sök startnummer...")

	// Sökfunktion
	searchEntry.OnChanged = func(searchText string) {
		if searchText == "" {
			currentResults = originalResults
		} else {
			currentResults = []ChipResult{}
			for _, result := range originalResults {
				if strings.Contains(result.Chip, searchText) {
					currentResults = append(currentResults, result)
				}
			}
		}
		table.Refresh()
	}

	// OnSelected-hantering
	table.OnSelected = func(id widget.TableCellID) {
		if id.Row <= 0 || id.Row > len(currentResults) {
			return
		}

		currentResult := &currentResults[id.Row-1]
		var originalResult *ChipResult
		for i := range originalResults {
			if originalResults[i].Chip == currentResult.Chip &&
				originalResults[i].Time == currentResult.Time {
				originalResult = &originalResults[i]
				break
			}
		}
		if originalResult == nil {
			return
		}

		currentResult.Invalid = !currentResult.Invalid
		originalResult.Invalid = currentResult.Invalid

		timeKey := makeInvalidTimeKey(currentResult.Chip, currentResult.Time)

		if currentResult.Invalid {
			if race.InvalidTimes == nil {
				race.InvalidTimes = make(map[string]bool)
			}
			race.InvalidTimes[timeKey] = true

			// Leta efter nästa giltiga tid för detta chip
			if nextTime, found := findNextValidTime(race.ResultsFile, race, currentResult.Time, currentResult.Chip); found {
				duration := nextTime.Sub(race.StartTime)
				newResult := ChipResult{
					Chip:     currentResult.Chip,
					Time:     nextTime,
					Duration: duration,
					Invalid:  false,
				}
				currentResults = append(currentResults, newResult)
				originalResults = append(originalResults, newResult)

				sort.Slice(currentResults, func(i, j int) bool {
					return currentResults[i].Time.Before(currentResults[j].Time)
				})
				sort.Slice(originalResults, func(i, j int) bool {
					return originalResults[i].Time.Before(originalResults[j].Time)
				})
			}
		} else {
			delete(race.InvalidTimes, timeKey)

			// Ta bort alla senare tider för detta chip
			newCurrentResults := []ChipResult{}
			newOriginalResults := []ChipResult{}

			for _, result := range currentResults {
				if result.Chip == currentResult.Chip && result.Time.After(currentResult.Time) {
					// Skippa denna tid
					continue
				}
				newCurrentResults = append(newCurrentResults, result)
			}

			for _, result := range originalResults {
				if result.Chip == currentResult.Chip && result.Time.After(currentResult.Time) {
					// Skippa denna tid
					continue
				}
				newOriginalResults = append(newOriginalResults, result)
			}

			currentResults = newCurrentResults
			originalResults = newOriginalResults
		}

		races[index] = race
		saveRaces(races)
		cacheResults(race.Name, originalResults)

		table.Refresh()
		table.UnselectAll()
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
		addTimeButton,
		widget.NewLabel("Klicka på en rad för att markera/avmarkera den som felaktig"),
		tableContainer,
	)

	paddedContent := container.NewPadded(content)
	resultWindow.SetContent(paddedContent)
	resultWindow.Resize(fyne.NewSize(800, 900))
	resultWindow.CenterOnScreen()
	resultWindow.Show()
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
