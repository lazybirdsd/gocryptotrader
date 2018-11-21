package engine

import (
	"fmt"

	"github.com/thrasher-/gocryptotrader/common"
	"github.com/thrasher-/gocryptotrader/currency"
	exchange "github.com/thrasher-/gocryptotrader/exchanges"
	"github.com/thrasher-/gocryptotrader/exchanges/assets"
	"github.com/thrasher-/gocryptotrader/exchanges/orderbook"
	"github.com/thrasher-/gocryptotrader/exchanges/stats"
	"github.com/thrasher-/gocryptotrader/exchanges/ticker"
	log "github.com/thrasher-/gocryptotrader/logger"
	"github.com/thrasher-/gocryptotrader/portfolio"
)

// GetAllAvailablePairs returns a list of all available pairs on either enabled
// or disabled exchanges
func GetAllAvailablePairs(enabledExchangesOnly bool, assetType assets.AssetType) currency.Pairs {
	var pairList currency.Pairs
	for x := range Bot.Config.Exchanges {
		if enabledExchangesOnly && !Bot.Config.Exchanges[x].Enabled {
			continue
		}

		exchName := Bot.Config.Exchanges[x].Name
		pairs, err := Bot.Config.GetAvailablePairs(exchName, assetType)
		if err != nil {
			continue
		}

		for y := range pairs {
			if pairList.Contains(pairs[y], false) {
				continue
			}
			pairList = append(pairList, pairs[y])
		}
	}
	return pairList
}

// GetSpecificAvailablePairs returns a list of supported pairs based on specific
// parameters
func GetSpecificAvailablePairs(enabledExchangesOnly, fiatPairs, includeUSDT, cryptoPairs bool, assetType assets.AssetType) currency.Pairs {
	var pairList currency.Pairs
	supportedPairs := GetAllAvailablePairs(enabledExchangesOnly, assetType)

	for x := range supportedPairs {
		if fiatPairs {
			if supportedPairs[x].IsCryptoFiatPair() &&
				!supportedPairs[x].ContainsCurrency(currency.USDT) ||
				(includeUSDT &&
					supportedPairs[x].ContainsCurrency(currency.USDT) &&
					supportedPairs[x].IsCryptoPair()) {
				if pairList.Contains(supportedPairs[x], false) {
					continue
				}
				pairList = append(pairList, supportedPairs[x])
			}
		}
		if cryptoPairs {
			if supportedPairs[x].IsCryptoPair() {
				if pairList.Contains(supportedPairs[x], false) {
					continue
				}
				pairList = append(pairList, supportedPairs[x])
			}
		}
	}
	return pairList
}

// IsRelatablePairs checks to see if the two pairs are relatable
func IsRelatablePairs(p1, p2 currency.Pair, includeUSDT bool) bool {
	if p1.EqualIncludeReciprocal(p2) {
		return true
	}

	var relatablePairs = GetRelatableCurrencies(p1, true, includeUSDT)
	if p1.IsCryptoFiatPair() {
		for x := range relatablePairs {
			relatablePairs = append(relatablePairs,
				GetRelatableFiatCurrencies(relatablePairs[x])...)
		}
	}
	return relatablePairs.Contains(p2, false)
}

// MapCurrenciesByExchange returns a list of currency pairs mapped to an
// exchange
func MapCurrenciesByExchange(p currency.Pairs, enabledExchangesOnly bool, assetType assets.AssetType) map[string]currency.Pairs {
	currencyExchange := make(map[string]currency.Pairs)
	for x := range p {
		for y := range Bot.Config.Exchanges {
			if enabledExchangesOnly && !Bot.Config.Exchanges[y].Enabled {
				continue
			}
			exchName := Bot.Config.Exchanges[y].Name
			success, err := Bot.Config.SupportsPair(exchName, p[x], assetType)
			if err != nil || !success {
				continue
			}

			result, ok := currencyExchange[exchName]
			if !ok {
				var pairs []currency.Pair
				pairs = append(pairs, p[x])
				currencyExchange[exchName] = pairs
			} else {
				if result.Contains(p[x], false) {
					continue
				}
				result = append(result, p[x])
				currencyExchange[exchName] = result
			}
		}
	}
	return currencyExchange
}

// GetExchangeNamesByCurrency returns a list of exchanges supporting
// a currency pair based on whether the exchange is enabled or not
func GetExchangeNamesByCurrency(p currency.Pair, enabled bool, assetType assets.AssetType) []string {
	var exchanges []string
	for x := range Bot.Config.Exchanges {
		if enabled != Bot.Config.Exchanges[x].Enabled {
			continue
		}

		exchName := Bot.Config.Exchanges[x].Name
		success, err := Bot.Config.SupportsPair(exchName, p, assetType)
		if err != nil {
			continue
		}

		if success {
			exchanges = append(exchanges, exchName)
		}
	}
	return exchanges
}

// GetRelatableCryptocurrencies returns a list of currency pairs if it can find
// any relatable currencies (e.g ETHBTC -> ETHLTC -> ETHUSDT -> ETHREP)
func GetRelatableCryptocurrencies(p currency.Pair) currency.Pairs {
	var pairs currency.Pairs
	cryptocurrencies := currency.GetCryptocurrencies()

	for x := range cryptocurrencies {
		newPair := currency.NewPair(p.Base, cryptocurrencies[x])
		if newPair.IsInvalid() {
			continue
		}

		if newPair.Base.Upper() == p.Base.Upper() &&
			newPair.Quote.Upper() == p.Quote.Upper() {
			continue
		}

		if pairs.Contains(newPair, false) {
			continue
		}
		pairs = append(pairs, newPair)
	}
	return pairs
}

// GetRelatableFiatCurrencies returns a list of currency pairs if it can find
// any relatable currencies (e.g ETHUSD -> ETHAUD -> ETHGBP -> ETHJPY)
func GetRelatableFiatCurrencies(p currency.Pair) currency.Pairs {
	var pairs currency.Pairs
	fiatCurrencies := currency.GetFiatCurrencies()

	for x := range fiatCurrencies {
		newPair := currency.NewPair(p.Base, fiatCurrencies[x])
		if newPair.Base.Upper() == newPair.Quote.Upper() {
			continue
		}

		if newPair.Base.Upper() == p.Base.Upper() &&
			newPair.Quote.Upper() == p.Quote.Upper() {
			continue
		}

		if pairs.Contains(newPair, false) {
			continue
		}
		pairs = append(pairs, newPair)
	}
	return pairs
}

// GetRelatableCurrencies returns a list of currency pairs if it can find
// any relatable currencies (e.g BTCUSD -> BTC USDT -> XBT USDT -> XBT USD)
// incOrig includes the supplied pair if desired
func GetRelatableCurrencies(p currency.Pair, incOrig, incUSDT bool) currency.Pairs {
	var pairs currency.Pairs

	addPair := func(p currency.Pair) {
		if pairs.Contains(p, true) {
			return
		}
		pairs = append(pairs, p)
	}

	buildPairs := func(p currency.Pair, incOrig bool) {
		if incOrig {
			addPair(p)
		}

		first, ok := currency.GetTranslation(p.Base)
		if ok {
			addPair(currency.NewPair(first, p.Quote))

			var second currency.Code
			second, ok = currency.GetTranslation(p.Quote)
			if ok {
				addPair(currency.NewPair(first, second))
			}
		}

		second, ok := currency.GetTranslation(p.Quote)
		if ok {
			addPair(currency.NewPair(p.Base, second))
		}
	}

	buildPairs(p, incOrig)
	buildPairs(p.Swap(), incOrig)

	if !incUSDT {
		pairs = pairs.RemovePairsByFilter(currency.USDT)
	}

	return pairs
}

// GetSpecificOrderbook returns a specific orderbook given the currency,
// exchangeName and assetType
func GetSpecificOrderbook(currencyPair, exchangeName string, assetType assets.AssetType) (orderbook.Base, error) {
	var specificOrderbook orderbook.Base
	var err error
	for x := range Bot.Exchanges {
		if Bot.Exchanges[x] != nil {
			if Bot.Exchanges[x].GetName() == exchangeName {
				specificOrderbook, err = Bot.Exchanges[x].FetchOrderbook(
					currency.NewPairFromString(currencyPair),
					assetType,
				)
				break
			}
		}
	}
	return specificOrderbook, err
}

// GetSpecificTicker returns a specific ticker given the currency,
// exchangeName and assetType
func GetSpecificTicker(currencyPair, exchangeName string, assetType assets.AssetType) (ticker.Price, error) {
	var specificTicker ticker.Price
	var err error
	for x := range Bot.Exchanges {
		if Bot.Exchanges[x] != nil {
			if Bot.Exchanges[x].GetName() == exchangeName {
				specificTicker, err = Bot.Exchanges[x].FetchTicker(
					currency.NewPairFromString(currencyPair),
					assetType,
				)
				break
			}
		}
	}
	return specificTicker, err
}

// GetCollatedExchangeAccountInfoByCoin collates individual exchange account
// information and turns into into a map string of
// exchange.AccountCurrencyInfo
func GetCollatedExchangeAccountInfoByCoin(exchAccounts []exchange.AccountInfo) map[currency.Code]exchange.AccountCurrencyInfo {
	result := make(map[currency.Code]exchange.AccountCurrencyInfo)
	for _, accounts := range exchAccounts {
		for _, account := range accounts.Accounts {
			for _, accountCurrencyInfo := range account.Currencies {
				currencyName := accountCurrencyInfo.CurrencyName
				avail := accountCurrencyInfo.TotalValue
				onHold := accountCurrencyInfo.Hold

				info, ok := result[currencyName]
				if !ok {
					accountInfo := exchange.AccountCurrencyInfo{CurrencyName: currencyName, Hold: onHold, TotalValue: avail}
					result[currencyName] = accountInfo
				} else {
					info.Hold += onHold
					info.TotalValue += avail
					result[currencyName] = info
				}
			}
		}
	}
	return result
}

// GetAccountCurrencyInfoByExchangeName returns info for an exchange
func GetAccountCurrencyInfoByExchangeName(accounts []exchange.AccountInfo, exchangeName string) (exchange.AccountInfo, error) {
	for i := 0; i < len(accounts); i++ {
		if accounts[i].Exchange == exchangeName {
			return accounts[i], nil
		}
	}
	return exchange.AccountInfo{}, ErrExchangeNotFound
}

// GetExchangeHighestPriceByCurrencyPair returns the exchange with the highest
// price for a given currency pair and asset type
func GetExchangeHighestPriceByCurrencyPair(p currency.Pair, assetType assets.AssetType) (string, error) {
	result := stats.SortExchangesByPrice(p, assetType, true)
	if len(result) == 0 {
		return "", fmt.Errorf("no stats for supplied currency pair and asset type")
	}

	return result[0].Exchange, nil
}

// GetExchangeLowestPriceByCurrencyPair returns the exchange with the lowest
// price for a given currency pair and asset type
func GetExchangeLowestPriceByCurrencyPair(p currency.Pair, assetType assets.AssetType) (string, error) {
	result := stats.SortExchangesByPrice(p, assetType, false)
	if len(result) == 0 {
		return "", fmt.Errorf("no stats for supplied currency pair and asset type")
	}

	return result[0].Exchange, nil
}

// SeedExchangeAccountInfo seeds account info
func SeedExchangeAccountInfo(data []exchange.AccountInfo) {
	if len(data) == 0 {
		return
	}

	port := portfolio.GetPortfolio()

	for _, exchangeData := range data {
		exchangeName := exchangeData.Exchange

		var currencies []exchange.AccountCurrencyInfo
		for _, account := range exchangeData.Accounts {
			for _, info := range account.Currencies {

				var update bool
				for i := range currencies {
					if info.CurrencyName == currencies[i].CurrencyName {
						currencies[i].Hold += info.Hold
						currencies[i].TotalValue += info.TotalValue
						update = true
					}
				}

				if update {
					continue
				}

				currencies = append(currencies, exchange.AccountCurrencyInfo{
					CurrencyName: info.CurrencyName,
					TotalValue:   info.TotalValue,
					Hold:         info.Hold,
				})
			}
		}

		for _, total := range currencies {
			currencyName := total.CurrencyName
			total := total.TotalValue

			if !port.ExchangeAddressExists(exchangeName, currencyName) {
				if total <= 0 {
					continue
				}

				log.Debugf("Portfolio: Adding new exchange address: %s, %s, %f, %s\n",
					exchangeName,
					currencyName,
					total,
					portfolio.PortfolioAddressExchange)

				port.Addresses = append(
					port.Addresses,
					portfolio.Address{Address: exchangeName,
						CoinType:    currencyName,
						Balance:     total,
						Description: portfolio.PortfolioAddressExchange})

			} else {
				if total <= 0 {
					log.Debugf("Portfolio: Removing %s %s entry.\n",
						exchangeName,
						currencyName)

					port.RemoveExchangeAddress(exchangeName, currencyName)
				} else {
					balance, ok := port.GetAddressBalance(exchangeName,
						portfolio.PortfolioAddressExchange,
						currencyName)

					if !ok {
						continue
					}

					if balance != total {
						log.Debugf("Portfolio: Updating %s %s entry with balance %f.\n",
							exchangeName,
							currencyName,
							total)

						port.UpdateExchangeAddressBalance(exchangeName,
							currencyName,
							total)
					}
				}
			}
		}
	}
}

// GetCryptocurrenciesByExchange returns a list of cryptocurrencies the exchange supports
func GetCryptocurrenciesByExchange(exchangeName string, enabledExchangesOnly, enabledPairs bool, assetType assets.AssetType) ([]string, error) {
	var cryptocurrencies []string
	for x := range Bot.Config.Exchanges {
		if Bot.Config.Exchanges[x].Name == exchangeName {
			if enabledExchangesOnly && !Bot.Config.Exchanges[x].Enabled {
				continue
			}

			exchName := Bot.Config.Exchanges[x].Name
			var pairs currency.Pairs
			var err error

			if enabledPairs {
				pairs, err = Bot.Config.GetEnabledPairs(exchName, assetType)
				if err != nil {
					return nil, err
				}
			} else {
				pairs, err = Bot.Config.GetAvailablePairs(exchName, assetType)
				if err != nil {
					return nil, err
				}
			}

			for y := range pairs {
				if pairs[y].Base.IsCryptocurrency() &&
					!common.StringDataCompareUpper(cryptocurrencies, pairs[y].Base.String()) {
					cryptocurrencies = append(cryptocurrencies, pairs[y].Base.String())
				}

				if pairs[y].Quote.IsCryptocurrency() &&
					!common.StringDataCompareUpper(cryptocurrencies, pairs[y].Quote.String()) {
					cryptocurrencies = append(cryptocurrencies, pairs[y].Quote.String())
				}
			}
		}
	}
	return cryptocurrencies, nil
}

// GetExchangeCryptocurrencyDepositAddress returns the cryptocurrency deposit address for a particular
// exchange
func GetExchangeCryptocurrencyDepositAddress(exchName string, item currency.Code) (string, error) {
	exch := GetExchangeByName(exchName)
	if exch == nil {
		return "", ErrExchangeNotFound
	}

	return exch.GetDepositAddress(item, "")
}

// GetExchangeCryptocurrencyDepositAddresses obtains an exchanges deposit cryptocurrency list
func GetExchangeCryptocurrencyDepositAddresses() map[string]map[string]string {
	result := make(map[string]map[string]string)

	for x := range Bot.Exchanges {
		if !Bot.Exchanges[x].IsEnabled() {
			continue
		}
		exchName := Bot.Exchanges[x].GetName()

		if !Bot.Exchanges[x].GetAuthenticatedAPISupport() {
			if Bot.Settings.Verbose {
				log.Printf("GetExchangeCryptocurrencyDepositAddresses: Skippping %s due to disabled authenticated API support.", exchName)
			}
			continue
		}

		cryptoCurrencies, err := GetCryptocurrenciesByExchange(exchName, true, true, assets.AssetTypeSpot)
		if err != nil {
			log.Printf("%s failed to get cryptocurrency deposit addresses. Err: %s", exchName, err)
			continue
		}

		cryptoAddr := make(map[string]string)
		for y := range cryptoCurrencies {
			cryptocurrency := cryptoCurrencies[y]
			depositAddr, err := Bot.Exchanges[x].GetDepositAddress(currency.NewCode(cryptocurrency), "")
			if err != nil {
				log.Printf("%s failed to get cryptocurrency deposit addresses. Err: %s", exchName, err)
				continue
			}
			cryptoAddr[cryptocurrency] = depositAddr
		}
		result[exchName] = cryptoAddr
	}

	return result
}

// GetDepositAddressByExchange returns a deposit address for the specified exchange and cryptocurrency
// if it exists
func GetDepositAddressByExchange(exchName string, currencyItem currency.Code) string {
	for x, y := range Bot.CryptocurrencyDepositAddresses {
		if exchName == x {
			addr, ok := y[currencyItem.String()]
			if ok {
				return addr
			}
		}
	}
	return ""
}

// GetOrderByExchange returns the order info for the desired exchange
func GetOrderByExchange(exchName string, orderID string) (exchange.OrderDetail, error) {
	exch := GetExchangeByName(exchName)
	if exch == nil {
		return exchange.OrderDetail{}, ErrExchangeNotFound
	}

	return exch.GetOrderInfo(orderID)
}

// GetDepositAddressesByExchange returns a list of cryptocurrency addresses for the specified
// exchange if they exist
func GetDepositAddressesByExchange(exchName string) map[string]string {
	for x, y := range Bot.CryptocurrencyDepositAddresses {
		if exchName == x {
			return y
		}
	}
	return nil
}

// CancelAllOrdersByExchange sends a cancel all orders request to the desired exchange
func CancelAllOrdersByExchange(exchName string) error {
	exch := GetExchangeByName(exchName)
	if exch == nil {
		return ErrExchangeNotFound
	}

	// to-do
	return nil
}

// CancelOrderByExchange sends a cancel order request based on the desired exchange
// and order ID
func CancelOrderByExchange(exchName string, order *exchange.OrderCancellation) error {
	exch := GetExchangeByName(exchName)
	if exch == nil {
		return ErrExchangeNotFound
	}

	return exch.CancelOrder(order)
}

// WithdrawCryptocurrencyFundsByExchange withdraws the desired cryptocurrency and amount to a desired cryptocurrency address
func WithdrawCryptocurrencyFundsByExchange(exchName string, cryptocurrency currency.Code, address string, amount float64) (string, error) {
	exch := GetExchangeByName(exchName)
	if exch == nil {
		return "", ErrExchangeNotFound
	}

	// TO-DO: FILL
	return exch.WithdrawCryptocurrencyFunds(&exchange.WithdrawRequest{})
}

// FormatCurrency is a method that formats and returns a currency pair
// based on the user currency display preferences
func FormatCurrency(p currency.Pair) currency.Pair {
	return p.Format(Bot.Config.Currency.CurrencyPairFormat.Delimiter,
		Bot.Config.Currency.CurrencyPairFormat.Uppercase)
}
