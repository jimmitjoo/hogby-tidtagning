package sheets

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Inbyggda credentials (base64-kodade för att inte vara direkt läsbara)
const defaultCredentials = "eyJpbnN0YWxsZWQiOnsiY2xpZW50X2lkIjoiNjU0MTk0NDM1MzA1LThldjNvZDIxazI3YW83cjFkMzdmMGhxaXN1MHU0a3ZtLmFwcHMuZ29vZ2xldXNlcmNvbnRlbnQuY29tIiwicHJvamVjdF9pZCI6Im5hbWVkLWVxdWF0b3ItNDE1NzA2IiwiYXV0aF91cmkiOiJodHRwczovL2FjY291bnRzLmdvb2dsZS5jb20vby9vYXV0aDIvYXV0aCIsInRva2VuX3VyaSI6Imh0dHBzOi8vb2F1dGgyLmdvb2dsZWFwaXMuY29tL3Rva2VuIiwiYXV0aF9wcm92aWRlcl94NTA5X2NlcnRfdXJsIjoiaHR0cHM6Ly93d3cuZ29vZ2xlYXBpcy5jb20vb2F1dGgyL3YxL2NlcnRzIiwiY2xpZW50X3NlY3JldCI6IkdPQ1NQWC1zMllaN2gxa2dIelRZTXVwdGtyRTZNQjJHUXNHIiwicmVkaXJlY3RfdXJpcyI6WyJodHRwOi8vbG9jYWxob3N0Il19fQ=="

type SheetsService struct {
	service *sheets.Service
}

// getClient hämtar en klient för att autentisera mot Google Sheets API
func getClient(config *oauth2.Config, authCallback func(string, func(string) error)) (*http.Client, error) {
	// Använd en specifik mapp för tokens baserat på klient-ID
	configDir, err := getConfigDir()
	if err != nil {
		return nil, fmt.Errorf("kunde inte skapa config-mapp: %v", err)
	}

	tokFile := filepath.Join(configDir, fmt.Sprintf("token_%s.json", config.ClientID))
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = getTokenFromWeb(config, authCallback)
		if err != nil {
			return nil, err
		}
		saveToken(tokFile, tok)
	}
	return config.Client(context.Background(), tok), nil
}

// getConfigDir returnerar sökvägen till konfigurationsmappen
func getConfigDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(homeDir, ".hogby-tidtagning")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", err
	}

	return configDir, nil
}

// getTokenFromWeb hämtar en token från webben genom att be användaren autentisera
func getTokenFromWeb(config *oauth2.Config, authCallback func(string, func(string) error)) (*oauth2.Token, error) {
	// Använd "urn:ietf:wg:oauth:2.0:oob" för desktop-applikationer
	config.RedirectURL = "urn:ietf:wg:oauth:2.0:oob"

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)

	// Skapa en kanal för att vänta på auktoriseringskoden
	codeChan := make(chan string)
	errChan := make(chan error)

	// Anropa callback med URL:en och en funktion för att hantera koden
	authCallback(authURL, func(code string) error {
		if code == "" {
			errChan <- fmt.Errorf("ingen auktoriseringskod angiven")
			return nil
		}
		codeChan <- code
		return nil
	})

	// Vänta på antingen koden eller ett fel
	select {
	case code := <-codeChan:
		tok, err := config.Exchange(context.Background(), code)
		if err != nil {
			return nil, fmt.Errorf("kunde inte hämta token från web: %v", err)
		}
		return tok, nil
	case err := <-errChan:
		return nil, err
	}
}

// tokenFromFile hämtar token från lokal fil
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// saveToken sparar token till fil
func saveToken(path string, token *oauth2.Token) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("kunde inte skapa token-mapp: %v", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("kunde inte spara token: %v", err)
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(token)
}

// NewSheetsService skapar en ny instans av SheetsService
func NewSheetsService(credentialsFile string, authCallback func(string, func(string) error)) (*SheetsService, error) {
	ctx := context.Background()

	var credBytes []byte
	var err error

	if credentialsFile == "" {
		// Använd inbyggda credentials
		credBytes, err = base64.StdEncoding.DecodeString(defaultCredentials)
		if err != nil {
			return nil, fmt.Errorf("kunde inte avkoda inbyggda credentials: %v", err)
		}
	} else {
		// Använd angivna credentials från fil
		credBytes, err = os.ReadFile(credentialsFile)
		if err != nil {
			return nil, fmt.Errorf("kunde inte läsa credentials-fil: %v", err)
		}
	}

	config, err := google.ConfigFromJSON(credBytes, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("kunde inte parsa credentials: %v", err)
	}

	client, err := getClient(config, authCallback)
	if err != nil {
		return nil, fmt.Errorf("kunde inte skapa OAuth2-klient: %v", err)
	}

	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("kunde inte skapa Sheets-klient: %v", err)
	}

	return &SheetsService{service: srv}, nil
}

// ExportResults exporterar resultat till ett specifikt Google Sheet
func (s *SheetsService) ExportResults(spreadsheetId, sheetName string, results []Result) error {
	// Formatera data för export
	var values [][]interface{}

	// Lägg endast till giltiga resultat
	for _, result := range results {
		if result.Invalid {
			continue
		}

		// Formatera tiden
		duration := result.Duration
		timeStr := ""
		if duration.Hours() >= 1 {
			timeStr = fmt.Sprintf("%02d:%02d:%02d",
				int(duration.Hours()),
				int(duration.Minutes())%60,
				int(duration.Seconds())%60)
		} else {
			timeStr = fmt.Sprintf("%02d:%02d",
				int(duration.Minutes()),
				int(duration.Seconds())%60)
		}

		values = append(values, []interface{}{
			result.Chip,
			timeStr,
		})
	}

	// Om inga giltiga resultat finns
	if len(values) == 0 {
		return fmt.Errorf("inga giltiga resultat att exportera")
	}

	// Hitta eller skapa fliken
	_, err := s.findOrCreateSheet(spreadsheetId, sheetName)
	if err != nil {
		return fmt.Errorf("kunde inte hitta/skapa flik: %v", err)
	}

	// Rensa befintligt innehåll
	clearRange := fmt.Sprintf("%s!A1:Z1000", sheetName)
	_, err = s.service.Spreadsheets.Values.Clear(spreadsheetId, clearRange, &sheets.ClearValuesRequest{}).Do()
	if err != nil {
		return fmt.Errorf("kunde inte rensa flik: %v", err)
	}

	// Uppdatera med nya värden
	valueRange := &sheets.ValueRange{
		Values: values,
	}

	updateRange := fmt.Sprintf("%s!A1:B%d", sheetName, len(values))
	_, err = s.service.Spreadsheets.Values.Update(spreadsheetId, updateRange, valueRange).
		ValueInputOption("RAW").Do()

	if err != nil {
		return fmt.Errorf("kunde inte uppdatera värden: %v", err)
	}

	return nil
}

// findOrCreateSheet hittar eller skapar en flik med det angivna namnet
func (s *SheetsService) findOrCreateSheet(spreadsheetId, sheetName string) (*sheets.Sheet, error) {
	spreadsheet, err := s.service.Spreadsheets.Get(spreadsheetId).Do()
	if err != nil {
		return nil, fmt.Errorf("kunde inte hämta kalkylark: %v", err)
	}

	// Sök efter befintlig flik
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return sheet, nil
		}
	}

	// Skapa ny flik om den inte finns
	addSheetRequest := &sheets.AddSheetRequest{
		Properties: &sheets.SheetProperties{
			Title: sheetName,
		},
	}

	batchUpdateRequest := &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{{
			AddSheet: addSheetRequest,
		}},
	}

	_, err = s.service.Spreadsheets.BatchUpdate(spreadsheetId, batchUpdateRequest).Do()
	if err != nil {
		return nil, fmt.Errorf("kunde inte skapa ny flik: %v", err)
	}

	// Hämta den nya fliken
	spreadsheet, err = s.service.Spreadsheets.Get(spreadsheetId).Do()
	if err != nil {
		return nil, fmt.Errorf("kunde inte hämta uppdaterat kalkylark: %v", err)
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return sheet, nil
		}
	}

	return nil, fmt.Errorf("kunde inte hitta den skapade fliken")
}

// Result representerar ett tävlingsresultat
type Result struct {
	Chip     string
	Time     time.Time
	Duration time.Duration
	Invalid  bool
	Manual   bool
}
