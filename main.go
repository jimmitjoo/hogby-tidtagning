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
	Name        string          `json:"name"`
	StartTime   time.Time       `json:"startTime"`
	MinTime     string          `json:"minTime"`
	Chips       map[string]bool `json:"chips"`
	ResultsFile string          `json:"resultsFile"`
}

// MarshalJSON för Race
func (r Race) MarshalJSON() ([]byte, error) {
	return json.Marshal(DurationRace{
		Name:        r.Name,
		StartTime:   r.StartTime,
		MinTime:     r.MinTime.String(),
		Chips:       r.Chips,
		ResultsFile: r.ResultsFile,
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
	Name        string          `json:"name"`
	StartTime   time.Time       `json:"startTime"`
	MinTime     time.Duration   `json:"minTime"`
	Chips       map[string]bool `json:"chips"`
	ResultsFile string          `json:"resultsFile"`
}

type ChipResult struct {
	Chip     string        `json:"chip"`
	Time     time.Time     `json:"time"`
	Duration time.Duration `json:"duration"`
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
		raceContainer.Objects = nil
		for i, race := range races {
			race := race
			index := i

			editButton := widget.NewButton("Redigera", func() {
				showEditRaceForm(race, index, races, myApp, window, updateRaceList)
			})

			processButton := widget.NewButton("Processa resultat", func() {
				processRaceResults := func(filename string) {
					results := processRaceResults(filename, race)
					resultWindow := myApp.NewWindow(fmt.Sprintf("Resultat - %s", race.Name))
					resultText := widget.NewTextGrid()
					resultText.SetText(results)
					resultWindow.SetContent(resultText)
					resultWindow.Resize(fyne.NewSize(800, 900))
					resultWindow.CenterOnScreen()
					resultWindow.Show()
				}

				if race.ResultsFile != "" {
					// Om vi redan har en fil, använd den direkt
					processRaceResults(race.ResultsFile)
				} else {
					// Annars visa fil-dialog
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

						// Spara filsökvägen i race-objektet
						race.ResultsFile = filename
						races[index] = race
						saveRaces(races)

						processRaceResults(filename)
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

					// Uppdatera filsökvägen
					race.ResultsFile = filename
					races[index] = race
					saveRaces(races)

					// Använd samma processRaceResults funktion som tidigare
					processRaceResults(filename, race)
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
		saveRaces(races)
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
				Name:      nameEntry.Text,
				StartTime: startTime,
				MinTime:   minTime,
				Chips:     chips,
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

func processRaceResults(filename string, race Race) string {
	// Försök först att läsa cachade resultat
	if cachedResults, err := loadCachedResults(race.Name); err == nil && len(cachedResults) > 0 {
		return formatResults(race, cachedResults)
	}

	// Öppna filen
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Sprintf("Kunde inte öppna filen: %v", err)
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
			return fmt.Sprintf("Fel vid läsning av rad: %v", err)
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
	var sortedTimes []ChipResult

	for chip, time := range chipTimes {
		duration := time.Sub(race.StartTime)
		sortedTimes = append(sortedTimes, ChipResult{
			Chip:     chip,
			Time:     time,
			Duration: duration,
		})
	}

	// Sortera slice efter tid
	sort.Slice(sortedTimes, func(i, j int) bool {
		return sortedTimes[i].Time.Before(sortedTimes[j].Time)
	})

	// Cacha resultaten innan vi returnerar
	cacheResults(race.Name, sortedTimes)

	return formatResults(race, sortedTimes)
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

// Lägg till denna funktion för att spara/läsa lopp
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
			Name:      nameEntry.Text,
			StartTime: startTime,
			MinTime:   minTime,
			Chips:     chips,
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
					races = append(races[:index], races[index+1:]...)
					os.Remove(fmt.Sprintf("results_%s.json", race.Name))
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

// Lägg till denna hjälpfunktion
func showLargeFileDialog(callback func(fyne.URIReadCloser, error), window fyne.Window) {
	d := dialog.NewFileOpen(callback, window)
	// Sätt storlek direkt på dialogen
	d.Resize(fyne.NewSize(800, 600))
	d.Show()
}
