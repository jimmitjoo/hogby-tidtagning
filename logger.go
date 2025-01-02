package main

import (
	"fmt"
	"log"
	"os"
	"sync"
)

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
