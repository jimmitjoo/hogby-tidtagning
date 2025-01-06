package dialogs

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

func ShowExportDialog(window fyne.Window, onExport func(spreadsheetId, sheetName string)) {
	spreadsheetIdEntry := widget.NewEntry()
	spreadsheetIdEntry.SetPlaceHolder("Google Sheets ID")

	sheetNameEntry := widget.NewEntry()
	sheetNameEntry.SetPlaceHolder("Fliknamn")
	sheetNameEntry.Text = "Resultat" // Standardvärde

	items := []*widget.FormItem{
		{Text: "Kalkylark ID", Widget: spreadsheetIdEntry},
		{Text: "Fliknamn", Widget: sheetNameEntry},
	}

	dialog.ShowForm("Exportera till Google Sheets", "Exportera", "Avbryt", items,
		func(submitted bool) {
			if !submitted {
				return
			}
			onExport(spreadsheetIdEntry.Text, sheetNameEntry.Text)
		}, window)
}
