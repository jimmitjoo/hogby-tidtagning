package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func main() {
	// Initiera logger
	appLogger = getLogger()

	// Initiera tom datastruktur om filen inte finns
	if err := initializeEmptyDataIfNeeded(); err != nil {
		appLogger.Log("Fel vid initiering av data: %v", err)
		return
	}

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
