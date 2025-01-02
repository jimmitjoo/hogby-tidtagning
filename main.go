package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
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
	Manual   bool          `json:"manual"`
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

// Lägg till denna struct och variabler i början av filen
type Logger struct {
	file   *os.File
	logger *log.Logger
	mu     sync.Mutex
}

var (
	appLogger *Logger
	once      sync.Once
)

// Ersätt den gamla logToFile med dessa nya funktioner
func initLogger() (*Logger, error) {
	file, err := os.OpenFile("tidtagning.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("kunde inte öppna loggfil: %v", err)
	}

	return &Logger{
		file:   file,
		logger: log.New(file, "", log.LstdFlags),
		mu:     sync.Mutex{},
	}, nil
}

func getLogger() *Logger {
	once.Do(func() {
		var err error
		appLogger, err = initLogger()
		if err != nil {
			log.Printf("Kunde inte initiera logger: %v", err)
			return
		}
	})
	return appLogger
}

func (l *Logger) Log(format string, v ...interface{}) {
	if l == nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.logger.Printf(format, v...)
}

// Lägg till en global variabel för att hålla aktiva söktermer per fönster
var activeSearches = make(map[string]string)

// Lägg till denna struct i början av filen
type ResultWindow struct {
	currentResults  []ChipResult
	originalResults []ChipResult
	window          fyne.Window
	table           *widget.Table
	searchEntry     *widget.Entry
}

// Lägg till en global map för att hålla alla öppna resultatfönster
var openResultWindows = make(map[string]*ResultWindow)

// Först, skapa en funktion som uppdaterar alla UI-komponenter
func updateAllUI(race *Race, races []Race, index int, updateMainWindow func(), appState *AppState) {
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
	updateAllUI(race, races, index, updateUI, appState)

	// Hantera filewatcher efter UI-uppdateringen
	if race.LiveUpdate {
		if _, exists := appState.stopWatchers[race.Name]; exists {
			return
		}
		stopWatcher, err := createFileWatcher(*race, races, index, nil, func() {
			updateAllUI(race, races, index, updateUI, appState)
		}, appState)
		if err != nil {
			getLogger().Log("Fel vid start av övervakning: %v", err)
			race.LiveUpdate = false
			races[index] = *race
			saveRaces(races)
			// Uppdatera UI igen efter felhantering
			updateAllUI(race, races, index, updateUI, appState)
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

// Uppdatera createFileWatcher för att hantera både huvudfönster och resultatfönster
func createFileWatcher(race Race, races []Race, index int, window fyne.Window, updateUI func(), appState *AppState) (func(), error) {
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

func main() {
	myApp := app.NewWithID("se.tidtagning.app")
	window := myApp.NewWindow("Tidtagning")

	// Skapa AppState
	appState := NewAppState()

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

			raceBox := makeRaceListItem(race, races, i, myApp, updateRaceList, appState)
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
	results := getAllResults(race)

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

func makeRaceListItem(race Race, races []Race, index int, app fyne.App, updateUI func(), appState *AppState) *fyne.Container {
	// Skapa etiketter för loppinformation
	nameLabel := widget.NewLabel(race.Name)
	nameLabel.TextStyle = fyne.TextStyle{Bold: true}

	startTimeStr := race.StartTime.Format("2006-01-02 15:04")
	timeLabel := widget.NewLabel(fmt.Sprintf("Starttid: %s", startTimeStr))

	participantsLabel := widget.NewLabel(fmt.Sprintf("Antal deltagare: %d", len(race.Chips)))

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
					races = append(races[:index], races[index+1:]...)
					saveRaces(races)
					updateUI()
				}
			}, app.Driver().AllWindows()[0])
	})
	deleteButton.Importance = widget.DangerImportance

	// Skapa en container för knapparna
	buttons := container.NewHBox(resultsButton, fileButton, watchButton, deleteButton)

	// Skapa en container för all information
	return container.NewVBox(
		nameLabel,
		timeLabel,
		participantsLabel,
		buttons,
		widget.NewSeparator(),
	)
}

// Lägg till denna struct nära början av filen
type AppState struct {
	mu             sync.RWMutex
	stopWatchers   map[string]func()
	activeSearches map[string]string
	resultWindows  map[string]*ResultWindow
	logger         *Logger
}

func NewAppState() *AppState {
	return &AppState{
		stopWatchers:   make(map[string]func()),
		activeSearches: make(map[string]string),
		resultWindows:  make(map[string]*ResultWindow),
		logger:         getLogger(),
	}
}

// Metoder för att hantera stopWatchers
func (s *AppState) AddStopWatcher(raceName string, stopFunc func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopWatchers[raceName] = stopFunc
}

func (s *AppState) RemoveStopWatcher(raceName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if stopFunc, exists := s.stopWatchers[raceName]; exists {
		stopFunc()
		delete(s.stopWatchers, raceName)
	}
}

// Metoder för att hantera activeSearches
func (s *AppState) SetActiveSearch(windowID, searchText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSearches[windowID] = searchText
}

func (s *AppState) GetActiveSearch(windowID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.activeSearches[windowID]
}

// Metoder för att hantera resultWindows
func (s *AppState) AddResultWindow(windowID string, rw *ResultWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resultWindows[windowID] = rw
}

func (s *AppState) RemoveResultWindow(windowID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.resultWindows, windowID)
}

func (s *AppState) GetResultWindow(windowID string) (*ResultWindow, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rw, exists := s.resultWindows[windowID]
	return rw, exists
}
