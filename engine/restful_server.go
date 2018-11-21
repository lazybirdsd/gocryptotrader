package engine

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/thrasher-/gocryptotrader/config"
	"github.com/thrasher-/gocryptotrader/exchanges/assets"
	log "github.com/thrasher-/gocryptotrader/logger"
)

// RESTfulJSONResponse outputs a JSON response of the response interface
func RESTfulJSONResponse(w http.ResponseWriter, response interface{}) error {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	return json.NewEncoder(w).Encode(response)
}

// RESTfulError prints the REST method and error
func RESTfulError(method string, err error) {
	log.Errorf("RESTful %s: server failed to send JSON response. Error %s",
		method, err)
}

// RESTGetAllSettings replies to a request with an encoded JSON response about the
// trading Bots configuration.
func RESTGetAllSettings(w http.ResponseWriter, r *http.Request) {
	err := RESTfulJSONResponse(w, Bot.Config)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// RESTSaveAllSettings saves all current settings from request body as a JSON
// document then reloads state and returns the settings
func RESTSaveAllSettings(w http.ResponseWriter, r *http.Request) {
	// Get the data from the request
	decoder := json.NewDecoder(r.Body)
	var responseData config.Post
	err := decoder.Decode(&responseData)
	if err != nil {
		RESTfulError(r.Method, err)
	}
	//Save change the settings
	err = Bot.Config.UpdateConfig(Bot.Settings.ConfigFile, &responseData.Data)
	if err != nil {
		RESTfulError(r.Method, err)
	}

	err = RESTfulJSONResponse(w, Bot.Config)
	if err != nil {
		RESTfulError(r.Method, err)
	}

	SetupExchanges()
}

// RESTGetOrderbook returns orderbook info for a given currency, exchange and
// asset type
func RESTGetOrderbook(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	currency := vars["currency"]
	exchangeName := vars["exchangeName"]
	assetType := vars["assetType"]

	if assetType == "" {
		assetType = assets.AssetTypeSpot.String()
	}

	response, err := GetSpecificOrderbook(currency, exchangeName, assets.AssetType(assetType))
	if err != nil {
		log.Errorf("Failed to fetch orderbook for %s currency: %s\n", exchangeName,
			currency)
		return
	}

	err = RESTfulJSONResponse(w, response)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// GetAllActiveOrderbooks returns all enabled exchanges orderbooks
func GetAllActiveOrderbooks() []EnabledExchangeOrderbooks {
	var orderbookData []EnabledExchangeOrderbooks

	for _, exch := range Bot.Exchanges {
		if !exch.IsEnabled() {
			continue
		}

		assets := exch.GetAssetTypes()
		exchName := exch.GetName()
		var exchangeOB EnabledExchangeOrderbooks
		exchangeOB.ExchangeName = exchName

		for y := range assets {
			currencies := exch.GetEnabledPairs(assets[y])
			for z := range currencies {
				ob, err := exch.FetchOrderbook(currencies[z], assets[y])
				if err != nil {
					log.Printf("Exchange %s failed to retrieve %s orderbook. Err: %s", exchName,
						currencies[z].String(),
						err)
					continue
				}
				exchangeOB.ExchangeValues = append(exchangeOB.ExchangeValues, ob)
			}
			orderbookData = append(orderbookData, exchangeOB)
		}
		orderbookData = append(orderbookData, exchangeOB)
	}
	return orderbookData
}

// RESTGetAllActiveOrderbooks returns all enabled exchange orderbooks
func RESTGetAllActiveOrderbooks(w http.ResponseWriter, r *http.Request) {
	var response AllEnabledExchangeOrderbooks
	response.Data = GetAllActiveOrderbooks()

	err := RESTfulJSONResponse(w, response)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// RESTGetPortfolio returns the Bot portfolio
func RESTGetPortfolio(w http.ResponseWriter, r *http.Request) {
	result := Bot.Portfolio.GetPortfolioSummary()
	err := RESTfulJSONResponse(w, result)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// RESTGetTicker returns ticker info for a given currency, exchange and
// asset type
func RESTGetTicker(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	currency := vars["currency"]
	exchName := vars["exchangeName"]
	assetType := vars["assetType"]

	if assetType == "" {
		assetType = assets.AssetTypeSpot.String()
	}
	response, err := GetSpecificTicker(currency, exchName, assets.AssetType(assetType))
	if err != nil {
		log.Printf("Failed to fetch ticker for %s currency: %s\n", exchName,
			currency)
		return
	}
	err = RESTfulJSONResponse(w, response)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// GetAllActiveTickers returns all enabled exchange tickers
func GetAllActiveTickers() []EnabledExchangeCurrencies {
	var tickerData []EnabledExchangeCurrencies

	for _, exch := range Bot.Exchanges {
		if !exch.IsEnabled() {
			continue
		}

		assets := exch.GetAssetTypes()
		exchName := exch.GetName()
		var exchangeTicker EnabledExchangeCurrencies
		exchangeTicker.ExchangeName = exchName

		for y := range assets {
			currencies := exch.GetEnabledPairs(assets[y])
			for z := range currencies {
				tp, err := exch.FetchTicker(currencies[z], assets[y])
				if err != nil {
					log.Printf("Exchange %s failed to retrieve %s ticker. Err: %s", exchName,
						currencies[z].String(),
						err)
					continue
				}
				exchangeTicker.ExchangeValues = append(exchangeTicker.ExchangeValues, tp)
			}
			tickerData = append(tickerData, exchangeTicker)
		}
		tickerData = append(tickerData, exchangeTicker)
	}
	return tickerData
}

// RESTGetAllActiveTickers returns all active tickers
func RESTGetAllActiveTickers(w http.ResponseWriter, r *http.Request) {
	var response AllEnabledExchangeCurrencies
	response.Data = GetAllActiveTickers()

	err := RESTfulJSONResponse(w, response)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}

// GetAllEnabledExchangeAccountInfo returns all the current enabled exchanges
func GetAllEnabledExchangeAccountInfo() AllEnabledExchangeAccounts {
	var response AllEnabledExchangeAccounts
	for _, individualBot := range Bot.Exchanges {
		if individualBot != nil && individualBot.IsEnabled() {
			if !individualBot.GetAuthenticatedAPISupport() {
				if Bot.Settings.Verbose {
					log.Debugf("GetAllEnabledExchangeAccountInfo: Skippping %s due to disabled authenticated API support.", individualBot.GetName())
				}
				continue
			}
			individualExchange, err := individualBot.GetAccountInfo()
			if err != nil {
				log.Errorf("Error encountered retrieving exchange account info for %s. Error %s",
					individualBot.GetName(), err)
				continue
			}
			response.Data = append(response.Data, individualExchange)
		}
	}
	return response
}

// RESTGetAllEnabledAccountInfo via get request returns JSON response of account
// info
func RESTGetAllEnabledAccountInfo(w http.ResponseWriter, r *http.Request) {
	response := GetAllEnabledExchangeAccountInfo()
	err := RESTfulJSONResponse(w, response)
	if err != nil {
		RESTfulError(r.Method, err)
	}
}
