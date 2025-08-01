package logger

import (
	"fmt"
	"log"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/lumberjack.v3"
)

var once sync.Once
var appLogger *zap.Logger
var stateLogger *zap.Logger
var arbitrageLogger *zap.Logger
var scrapingLogger *zap.Logger

type Config struct {
	Filename   string
	MaxSize    int // megabytes
	MaxBackups int
	MaxAge     int // days
	Compress   bool
}

// Get returns the main application logger
func Get() *zap.Logger {
	once.Do(initLoggers)
	return appLogger
}

// GetStateLogger returns the internal state logger
func GetStateLogger() *zap.Logger {
	once.Do(initLoggers)
	return stateLogger
}

func GetArbitrageLogger() *zap.Logger {
	once.Do(initLoggers)
	return arbitrageLogger
}

// GetScrapingLogger returns the scraping logger
func GetScrapingLogger() *zap.Logger {
	once.Do(initLoggers)
	return scrapingLogger
}

func newLogger(config Config, useConsole bool) (*zap.Logger, error) {
	fileHandler, err := lumberjack.New(
		lumberjack.WithFileName(config.Filename),
		lumberjack.WithMaxBytes(int64(config.MaxSize*1024*1024)),
		lumberjack.WithMaxBackups(config.MaxBackups),
		lumberjack.WithMaxDays(config.MaxAge),
		lumberjack.WithCompress(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create file handler: %w", err)
	}

	level := zap.InfoLevel
	if levelEnv := os.Getenv("LOG_LEVEL"); levelEnv != "" {
		if parsedLevel, err := zapcore.ParseLevel(levelEnv); err == nil {
			level = parsedLevel
		}
	}
	logLevel := zap.NewAtomicLevelAt(level)

	productionCfg := zap.NewProductionEncoderConfig()
	productionCfg.TimeKey = "timestamp"
	productionCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	productionCfg.EncodeCaller = zapcore.ShortCallerEncoder

	developmentCfg := zap.NewDevelopmentEncoderConfig()
	developmentCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	developmentCfg.EncodeCaller = zapcore.ShortCallerEncoder

	consoleEncoder := zapcore.NewConsoleEncoder(developmentCfg)
	fileEncoder := zapcore.NewJSONEncoder(productionCfg)

	var cores []zapcore.Core
	if useConsole {
		cores = append(cores, zapcore.NewCore(consoleEncoder, zapcore.AddSync(os.Stdout), logLevel))
	}
	cores = append(cores, zapcore.NewCore(fileEncoder, zapcore.AddSync(fileHandler), logLevel))

	return zap.New(zapcore.NewTee(cores...), zap.AddCaller()), nil
}

func initLoggers() {
	appConfig := Config{
		Filename:   "logs/app.log",
		MaxSize:    5,
		MaxBackups: 10,
		MaxAge:     14,
		Compress:   true,
	}

	stateConfig := Config{
		Filename:   "logs/internal_state.log",
		MaxSize:    5,
		MaxBackups: 10,
		MaxAge:     14,
		Compress:   true,
	}

	arbitrageConfig := Config{
		Filename:   "logs/arbitrage.log",
		MaxSize:    5,
		MaxBackups: 10,
		MaxAge:     14,
		Compress:   true,
	}

	scrapingConfig := Config{
		Filename:   "logs/scraping.log",
		MaxSize:    5,
		MaxBackups: 10,
		MaxAge:     14,
		Compress:   true,
	}

	var err error
	appLogger, err = newLogger(appConfig, true) // with console output
	if err != nil {
		log.Fatalf("failed to create app logger: %v", err)
	}

	stateLogger, err = newLogger(stateConfig, false) // without console output
	if err != nil {
		log.Fatalf("failed to create state logger: %v", err)
	}

	arbitrageLogger, err = newLogger(arbitrageConfig, true) // with console output
	if err != nil {
		log.Fatalf("failed to create arbitrage logger: %v", err)
	}

	scrapingLogger, err = newLogger(scrapingConfig, false) // with console output
	if err != nil {
		log.Fatalf("failed to create scraping logger: %v", err)
	}
}
