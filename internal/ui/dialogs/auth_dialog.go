package dialogs

import (
	"net/url"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func ShowAuthDialog(window fyne.Window, authURL string, onCode func(string) error) {
	// Skapa en hyperlänk till autentiserings-URL:en
	parsedURL, _ := url.Parse(authURL)
	link := widget.NewHyperlink("Klicka här för att öppna Google-inloggningen", parsedURL)

	// Skapa textfält för auktoriseringskoden
	codeEntry := widget.NewEntry()
	codeEntry.SetPlaceHolder("Klistra in auktoriseringskoden här")

	// Skapa instruktioner
	instructions := widget.NewTextGridFromString(
		"1. Klicka på länken ovan för att öppna Google-inloggningen\n" +
			"2. Logga in med ditt Google-konto\n" +
			"3. Kopiera auktoriseringskoden\n" +
			"4. Klistra in koden i fältet nedan")

	content := container.NewVBox(
		instructions,
		link,
		widget.NewLabel(""), // Mellanrum
		codeEntry,
	)

	// Skapa dialogen
	d := dialog.NewCustomConfirm(
		"Google-autentisering",
		"Fortsätt",
		"Avbryt",
		content,
		func(submit bool) {
			if submit {
				onCode(codeEntry.Text)
			} else {
				onCode("")
			}
		},
		window,
	)

	d.Resize(fyne.NewSize(400, 300))
	d.Show()
}
