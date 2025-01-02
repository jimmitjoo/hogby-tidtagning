package main

import "sync"

type AppState struct {
	mu             sync.RWMutex
	stopWatchers   map[string]func()
	activeSearches map[string]string
	resultWindows  map[string]*ResultWindow
	logger         *Logger
}

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
