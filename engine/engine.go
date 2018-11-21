package engine

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/communications"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/currency"
	"github.com/thrasher-/gocryptotrader/currency/coinmarketcap"
	"github.com/thrasher-/gocryptotrader/engine/events"
	exchange "github.com/thrasher-/gocryptotrader/exchanges"
	"github.com/thrasher-/gocryptotrader/exchanges/request"
	log "github.com/thrasher-/gocryptotrader/logger"
	"github.com/thrasher-/gocryptotrader/portfolio"
	"github.com/thrasher-/gocryptotrader/utils"
)

// Engine contains configuration, portfolio, exchange & ticker data and is the
// overarching type across this code base.
type Engine struct {
	Config                         *config.Config
	Portfolio                      *portfolio.Base
	Exchanges                      []exchange.IBotExchange
	CommsRelayer                   *communications.Communications
	Shutdown                       chan bool
	Settings                       Settings
	CryptocurrencyDepositAddresses map[string]map[string]string
}

// Vars for engine
var (
	Bot *Engine
)

func init() {
	if Bot == nil {
		return
	}
}

// New starts a new engine
func New() (*Engine, error) {
	var b Engine
	b.Config = &config.Cfg

	err := b.Config.LoadConfig("")
	if err != nil {
		return nil, fmt.Errorf("failed to load config. Err: %s", err)
	}

	b.CryptocurrencyDepositAddresses = make(map[string]map[string]string)

	return &b, nil
}

// NewFromSettings starts a new engine based on supplied settings
func NewFromSettings(settings *Settings) (*Engine, error) {
	if settings == nil {
		return nil, errors.New("engine: settings is nil")
	}

	var b Engine
	b.Config = &config.Cfg
	log.Debugf("Loading config file %s..\n", settings.ConfigFile)
	err := b.Config.LoadConfig(settings.ConfigFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load config. Err: %s", err)
	}

	err = common.CheckDir(settings.DataDir, true)
	if err != nil {
		return nil, fmt.Errorf("failed to open/create data directory: %s. Err: %s", settings.DataDir, err)
	}

	b.Settings.ConfigFile = settings.ConfigFile
	b.Settings.DataDir = settings.DataDir
	b.Settings.LogFile = utils.GetLogFile(settings.DataDir)
	b.CryptocurrencyDepositAddresses = make(map[string]map[string]string)

	err = utils.AdjustGoMaxProcs(settings.GoMaxProcs)
	if err != nil {
		return nil, fmt.Errorf("unable to adjust runtime GOMAXPROCS value. Err: %s", err)
	}

	err = utils.InitLogFile(b.Settings.LogFile)
	if err != nil {
		log.Debugf("failed to create log file writer. Err: %s", err)
	} else {
		log.Debugf("Using log file: %s.\n", b.Settings.LogFile)
	}

	b.handleInterrupt()

	ValidateSettings(&b, settings)

	return &b, nil
}

// ValidateSettings validates and sets all bot settings
func ValidateSettings(b *Engine, s *Settings) {
	b.Settings.Verbose = s.Verbose
	b.Settings.EnableDryRun = s.EnableDryRun
	b.Settings.EnableAllExchanges = s.EnableAllExchanges
	b.Settings.EnableAllPairs = s.EnableAllPairs
	b.Settings.EnablePortfolioWatcher = s.EnablePortfolioWatcher
	b.Settings.EnableCoinmarketcapAnalysis = s.EnableCoinmarketcapAnalysis

	// TO-DO: FIXME
	if flag.Lookup("websocketserver") != nil {
		b.Settings.EnableWebsocketServer = s.EnableWebsocketServer
	} else {
		b.Settings.EnableWebsocketServer = b.Config.WebsocketServer.Enabled
	}

	if flag.Lookup("restserver") != nil {
		b.Settings.EnableRESTServer = s.EnableRESTServer
	} else {
		b.Settings.EnableRESTServer = b.Config.RESTServer.Enabled
	}

	b.Settings.EnableCommsRelayer = s.EnableCommsRelayer
	b.Settings.EnableEventManager = s.EnableEventManager

	if b.Settings.EnableEventManager {
		events.Verbose = b.Settings.Verbose
		if b.Settings.EventManagerDelay != time.Duration(0) && s.EventManagerDelay > 0 {
			b.Settings.EventManagerDelay = s.EventManagerDelay
		} else {
			b.Settings.EventManagerDelay = events.SleepDelay
		}
	}

	b.Settings.EnableTickerRoutine = s.EnableTickerRoutine
	b.Settings.EnableOrderbookRoutine = s.EnableOrderbookRoutine
	b.Settings.EnableWebsocketRoutine = s.EnableWebsocketRoutine
	b.Settings.EnableExchangeAutoPairUpdates = s.EnableExchangeAutoPairUpdates
	b.Settings.EnableExchangeWebsocketSupport = s.EnableExchangeWebsocketSupport
	b.Settings.EnableExchangeRESTSupport = s.EnableExchangeRESTSupport
	b.Settings.EnableExchangeVerbose = s.EnableExchangeVerbose
	b.Settings.EnableHTTPRateLimiter = s.EnableHTTPRateLimiter
	b.Settings.DisableExchangeAutoPairUpdates = s.DisableExchangeAutoPairUpdates
	b.Settings.ExchangePurgeCredentials = s.ExchangePurgeCredentials

	if !b.Settings.EnableHTTPRateLimiter {
		request.DisableRateLimiter = true
	}

	// Checks if the flag values are different from the defaults
	b.Settings.MaxHTTPRequestJobsLimit = s.MaxHTTPRequestJobsLimit
	if b.Settings.MaxHTTPRequestJobsLimit != request.DefaultMaxRequestJobs && s.MaxHTTPRequestJobsLimit > 0 {
		request.MaxRequestJobs = b.Settings.MaxHTTPRequestJobsLimit
	}

	b.Settings.RequestTimeoutRetryAttempts = s.RequestTimeoutRetryAttempts
	if b.Settings.RequestTimeoutRetryAttempts != request.DefaultTimeoutRetryAttempts && s.RequestTimeoutRetryAttempts > 0 {
		request.TimeoutRetryAttempts = b.Settings.RequestTimeoutRetryAttempts
	}

	b.Settings.ExchangeHTTPTimeout = s.ExchangeHTTPTimeout
	if s.ExchangeHTTPTimeout != time.Duration(0) && s.ExchangeHTTPTimeout > 0 {
		b.Settings.ExchangeHTTPTimeout = s.ExchangeHTTPTimeout
	} else {
		b.Settings.ExchangeHTTPTimeout = b.Config.GlobalHTTPTimeout
	}

	b.Settings.ExchangeHTTPUserAgent = s.ExchangeHTTPUserAgent
	b.Settings.ExchangeHTTPProxy = s.ExchangeHTTPProxy

	if s.GlobalHTTPTimeout != time.Duration(0) && s.GlobalHTTPTimeout > 0 {
		b.Settings.GlobalHTTPTimeout = s.GlobalHTTPTimeout
	} else {
		b.Settings.GlobalHTTPTimeout = b.Config.GlobalHTTPTimeout
	}
	common.HTTPClient = common.NewHTTPClientWithTimeout(b.Settings.GlobalHTTPTimeout)

	b.Settings.GlobalHTTPUserAgent = s.GlobalHTTPUserAgent
	if b.Settings.GlobalHTTPUserAgent != "" {
		common.HTTPUserAgent = b.Settings.GlobalHTTPUserAgent
	}

	b.Settings.GlobalHTTPProxy = s.GlobalHTTPProxy
}

// PrintSettings returns the engine settings
func PrintSettings(s Settings) {
	log.Debugln()
	log.Debugln("ENGINE SETTINGS")
	log.Debugf("- CORE SETTINGS:")
	log.Debugf("\t Verbose mode: %v", s.Verbose)
	log.Debugf("\t Enable dry run mode: %v", s.EnableDryRun)
	log.Debugf("\t Enable all exchanges: %v", s.EnableAllExchanges)
	log.Debugf("\t Enable all pairs: %v", s.EnableAllPairs)
	log.Debugf("\t Enable portfolio watcher: %v", s.EnablePortfolioWatcher)
	log.Debugf("\t Enable websocket server: %v", s.EnableWebsocketServer)
	log.Debugf("\t Enable REST server: %v", s.EnableRESTServer)
	log.Debugf("\t Enable comms relayer: %v", s.EnableCommsRelayer)
	log.Debugf("\t Enable event manager: %v", s.EnableEventManager)
	log.Debugf("\t Event manager sleep delay: %v", s.EventManagerDelay)
	log.Debugf("\t Enable ticker routine: %v", s.EnableTickerRoutine)
	log.Debugf("\t Enable orderbook routine: %v", s.EnableOrderbookRoutine)
	log.Debugf("\t Enable websocket routine: %v\n", s.EnableWebsocketRoutine)
	log.Debugf("- EXCHANGE SETTINGS:")
	log.Debugf("\t Enable exchange auto pair updates: %v", s.EnableExchangeAutoPairUpdates)
	log.Debugf("\t Disable all exchange auto pair updates: %v", s.DisableExchangeAutoPairUpdates)
	log.Debugf("\t Enable exchange websocket support: %v", s.EnableExchangeWebsocketSupport)
	log.Debugf("\t Enable exchange verbose mode: %v", s.EnableExchangeVerbose)
	log.Debugf("\t Enable exchange HTTP rate limiter: %v", s.EnableHTTPRateLimiter)
	log.Debugf("\t Exchange max HTTP request jobs: %v", s.MaxHTTPRequestJobsLimit)
	log.Debugf("\t Exchange HTTP request timeout retry amount: %v", s.RequestTimeoutRetryAttempts)
	log.Debugf("\t Exchange HTTP timeout: %v", s.ExchangeHTTPTimeout)
	log.Debugf("\t Exchange HTTP user agent: %v", s.ExchangeHTTPUserAgent)
	log.Debugf("\t Exchange HTTP proxy: %v\n", s.ExchangeHTTPProxy)
	log.Debugf("- COMMON SETTINGS:")
	log.Debugf("\t Global HTTP timeout: %v", s.GlobalHTTPTimeout)
	log.Debugf("\t Global HTTP user agent: %v", s.GlobalHTTPUserAgent)
	log.Debugf("\t Global HTTP proxy: %v", s.ExchangeHTTPProxy)
	log.Debugln()
}

// Start starts the engine
func (e *Engine) Start() {
	if e == nil {
		log.Fatal("Engine instance is nil")
	}

	log.Debugf("Bot '%s' started.\n", e.Config.Name)

	enabledExchanges := e.Config.CountEnabledExchanges()
	if e.Settings.EnableAllExchanges {
		enabledExchanges = len(e.Config.Exchanges)
	}

	log.Debugln()
	log.Debugln("EXCHANGE COVERAGE")
	log.Debugf("\t Available Exchanges: %d. Enabled Exchanges: %d.\n",
		len(e.Config.Exchanges), enabledExchanges)

	if e.Settings.ExchangePurgeCredentials {
		log.Debugln("Purging exchange API credentials.")
		e.Config.PurgeExchangeAPICredentials()
	}

	log.Debugln("Setting up exchanges..")
	SetupExchanges()
	if len(e.Exchanges) == 0 {
		log.Fatalf("No exchanges were able to be loaded. Exiting")
	}

	if e.Settings.EnableCommsRelayer {
		log.Debugln("Starting communication mediums..")
		commsCfg := e.Config.GetCommunicationsConfig()
		e.CommsRelayer = communications.NewComm(&commsCfg)
		e.CommsRelayer.GetEnabledCommunicationMediums()
	}

	var newFxSettings []currency.FXSettings
	for _, d := range e.Config.Currency.ForexProviders {
		newFxSettings = append(newFxSettings, currency.FXSettings(d))
	}

	err := currency.RunStorageUpdater(currency.BotOverrides{
		Coinmarketcap:       e.Settings.EnableCoinmarketcapAnalysis,
		FxCurrencyConverter: e.Settings.EnableCurrencyConverter,
		FxCurrencyLayer:     e.Settings.EnableCurrencyLayer,
		FxFixer:             e.Settings.EnableFixer,
		FxOpenExchangeRates: e.Settings.EnableOpenExchangeRates,
	},
		currency.MainConfiguration{
			ForexProviders:         newFxSettings,
			CryptocurrencyProvider: coinmarketcap.Settings(e.Config.Currency.CryptocurrencyProvider),
			Cryptocurrencies:       e.Config.Currency.Cryptocurrencies,
			FiatDisplayCurrency:    e.Config.Currency.FiatDisplayCurrency,
			CurrencyDelay:          e.Config.Currency.CurrencyFileUpdateDuration,
			FxRateDelay:            e.Config.Currency.ForeignExchangeUpdateDuration,
		},
		e.Settings.DataDir,
		e.Settings.Verbose)
	if err != nil {
		log.Warn("currency updater system failed to start", err)
	}

	e.Portfolio = &portfolio.Portfolio
	e.Portfolio.Seed(e.Config.Portfolio)
	SeedExchangeAccountInfo(GetAllEnabledExchangeAccountInfo().Data)

	e.CryptocurrencyDepositAddresses = GetExchangeCryptocurrencyDepositAddresses()

	if e.Settings.EnableRESTServer {
		go StartRESTServer()
	}

	if e.Settings.EnableWebsocketServer {
		go StartWebsocketServer()
		StartWebsocketHandler()
	}

	if e.Settings.EnablePortfolioWatcher {
		go portfolio.StartPortfolioWatcher()
	}

	if e.Settings.EnableTickerRoutine {
		go TickerUpdaterRoutine()
	}

	if e.Settings.EnableOrderbookRoutine {
		go OrderbookUpdaterRoutine()
	}

	if e.Settings.EnableWebsocketRoutine {
		go WebsocketRoutine()
	}

	if e.Settings.EnableEventManager {
		go events.EventManger()
	}

	<-e.Shutdown
	e.Stop()
}

// Stop correctly shuts down engine saving configuration files
func (e *Engine) Stop() {
	log.Debugln("Engine shutting down..")

	if len(portfolio.Portfolio.Addresses) != 0 {
		e.Config.Portfolio = portfolio.Portfolio
	}

	if !e.Settings.EnableDryRun {
		err := e.Config.SaveConfig(e.Settings.ConfigFile)

		if err != nil {
			log.Debugln("Unable to save config.")
		} else {
			log.Debugln("Config file saved successfully.")
		}
	}

	log.Debugln("Exiting.")

	if utils.LogFileHandle != nil {
		utils.LogFileHandle.Close()
	}
	os.Exit(0)
}

// handleInterrupt monitors and captures the SIGTERM in a new goroutine then
// shuts down the engine instance
func (e *Engine) handleInterrupt() {
	c := make(chan os.Signal, 1)
	e.Shutdown = make(chan bool)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-c
		log.Debugf("Captured %v, shutdown requested.", sig)
		e.Shutdown <- true
	}()
}
