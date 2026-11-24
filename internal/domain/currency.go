package domain

import "math/big"

// Currency describes one ISO 4217 currency the service accepts.
// MinorUnits is the number of decimal places (JPY 0, INR 2, KWD 3).
// PerUSD is the indicative number of units of this currency for 1 USD.
type Currency struct {
	Code       string `json:"code"`
	Name       string `json:"name"`
	MinorUnits int    `json:"minorUnits"`
	PerUSD     string `json:"-"`
}

// currency holds the public Currency plus its parsed rate for Convert.
type currency struct {
	Currency
	rate *big.Rat
}

// currencyTable is sorted by code. Rates are static and indicative — swap them for a
// live feed in production. Every transfer stores the rate it used, so history is stable.
var currencyTable = []Currency{
	{"AED", "UAE Dirham", 2, "3.6725"},
	{"AFN", "Afghan Afghani", 2, "70"},
	{"ALL", "Albanian Lek", 2, "92"},
	{"AMD", "Armenian Dram", 2, "385"},
	{"AOA", "Angolan Kwanza", 2, "912"},
	{"ARS", "Argentine Peso", 2, "1450"},
	{"AUD", "Australian Dollar", 2, "1.52"},
	{"AWG", "Aruban Florin", 2, "1.79"},
	{"AZN", "Azerbaijani Manat", 2, "1.70"},
	{"BAM", "Bosnia and Herzegovina Convertible Mark", 2, "1.68"},
	{"BBD", "Barbadian Dollar", 2, "2"},
	{"BDT", "Bangladeshi Taka", 2, "122"},
	{"BHD", "Bahraini Dinar", 3, "0.376"},
	{"BIF", "Burundian Franc", 0, "2950"},
	{"BMD", "Bermudian Dollar", 2, "1"},
	{"BND", "Brunei Dollar", 2, "1.30"},
	{"BOB", "Bolivian Boliviano", 2, "6.91"},
	{"BRL", "Brazilian Real", 2, "5.35"},
	{"BSD", "Bahamian Dollar", 2, "1"},
	{"BTN", "Bhutanese Ngultrum", 2, "88"},
	{"BWP", "Botswana Pula", 2, "13.5"},
	{"BYN", "Belarusian Ruble", 2, "3.0"},
	{"BZD", "Belize Dollar", 2, "2"},
	{"CAD", "Canadian Dollar", 2, "1.40"},
	{"CDF", "Congolese Franc", 2, "2250"},
	{"CHF", "Swiss Franc", 2, "0.80"},
	{"CLP", "Chilean Peso", 0, "940"},
	{"CNY", "Chinese Yuan", 2, "7.10"},
	{"COP", "Colombian Peso", 2, "3900"},
	{"CRC", "Costa Rican Colon", 2, "500"},
	{"CUP", "Cuban Peso", 2, "24"},
	{"CVE", "Cape Verdean Escudo", 2, "95"},
	{"CZK", "Czech Koruna", 2, "21"},
	{"DJF", "Djiboutian Franc", 0, "177.72"},
	{"DKK", "Danish Krone", 2, "6.45"},
	{"DOP", "Dominican Peso", 2, "63"},
	{"DZD", "Algerian Dinar", 2, "130"},
	{"EGP", "Egyptian Pound", 2, "47.5"},
	{"ERN", "Eritrean Nakfa", 2, "15"},
	{"ETB", "Ethiopian Birr", 2, "150"},
	{"EUR", "Euro", 2, "0.86"},
	{"FJD", "Fijian Dollar", 2, "2.28"},
	{"FKP", "Falkland Islands Pound", 2, "0.76"},
	{"GBP", "Pound Sterling", 2, "0.76"},
	{"GEL", "Georgian Lari", 2, "2.70"},
	{"GHS", "Ghanaian Cedi", 2, "11"},
	{"GIP", "Gibraltar Pound", 2, "0.76"},
	{"GMD", "Gambian Dalasi", 2, "72"},
	{"GNF", "Guinean Franc", 0, "8700"},
	{"GTQ", "Guatemalan Quetzal", 2, "7.65"},
	{"GYD", "Guyanese Dollar", 2, "209"},
	{"HKD", "Hong Kong Dollar", 2, "7.78"},
	{"HNL", "Honduran Lempira", 2, "26.3"},
	{"HTG", "Haitian Gourde", 2, "131"},
	{"HUF", "Hungarian Forint", 2, "335"},
	{"IDR", "Indonesian Rupiah", 2, "16600"},
	{"ILS", "Israeli New Shekel", 2, "3.25"},
	{"INR", "Indian Rupee", 2, "88.5"},
	{"IQD", "Iraqi Dinar", 3, "1310"},
	{"IRR", "Iranian Rial", 2, "1000000"},
	{"ISK", "Icelandic Krona", 0, "127"},
	{"JMD", "Jamaican Dollar", 2, "160"},
	{"JOD", "Jordanian Dinar", 3, "0.709"},
	{"JPY", "Japanese Yen", 0, "152"},
	{"KES", "Kenyan Shilling", 2, "129"},
	{"KGS", "Kyrgyzstani Som", 2, "87.5"},
	{"KHR", "Cambodian Riel", 2, "4020"},
	{"KMF", "Comorian Franc", 0, "425"},
	{"KPW", "North Korean Won", 2, "900"},
	{"KRW", "South Korean Won", 0, "1450"},
	{"KWD", "Kuwaiti Dinar", 3, "0.307"},
	{"KYD", "Cayman Islands Dollar", 2, "0.833"},
	{"KZT", "Kazakhstani Tenge", 2, "520"},
	{"LAK", "Lao Kip", 2, "21700"},
	{"LBP", "Lebanese Pound", 2, "89500"},
	{"LKR", "Sri Lankan Rupee", 2, "303"},
	{"LRD", "Liberian Dollar", 2, "183"},
	{"LSL", "Lesotho Loti", 2, "17.3"},
	{"LYD", "Libyan Dinar", 3, "5.45"},
	{"MAD", "Moroccan Dirham", 2, "9.2"},
	{"MDL", "Moldovan Leu", 2, "17"},
	{"MGA", "Malagasy Ariary", 2, "4500"},
	{"MKD", "Macedonian Denar", 2, "53"},
	{"MMK", "Myanmar Kyat", 2, "2100"},
	{"MNT", "Mongolian Tugrik", 2, "3600"},
	{"MOP", "Macanese Pataca", 2, "8.0"},
	{"MRU", "Mauritanian Ouguiya", 2, "40"},
	{"MUR", "Mauritian Rupee", 2, "45.5"},
	{"MVR", "Maldivian Rufiyaa", 2, "15.4"},
	{"MWK", "Malawian Kwacha", 2, "1735"},
	{"MXN", "Mexican Peso", 2, "18.5"},
	{"MYR", "Malaysian Ringgit", 2, "4.2"},
	{"MZN", "Mozambican Metical", 2, "63.9"},
	{"NAD", "Namibian Dollar", 2, "17.3"},
	{"NGN", "Nigerian Naira", 2, "1450"},
	{"NIO", "Nicaraguan Cordoba", 2, "36.8"},
	{"NOK", "Norwegian Krone", 2, "10.1"},
	{"NPR", "Nepalese Rupee", 2, "141"},
	{"NZD", "New Zealand Dollar", 2, "1.75"},
	{"OMR", "Omani Rial", 3, "0.385"},
	{"PAB", "Panamanian Balboa", 2, "1"},
	{"PEN", "Peruvian Sol", 2, "3.4"},
	{"PGK", "Papua New Guinean Kina", 2, "4.2"},
	{"PHP", "Philippine Peso", 2, "58.5"},
	{"PKR", "Pakistani Rupee", 2, "281"},
	{"PLN", "Polish Zloty", 2, "3.65"},
	{"PYG", "Paraguayan Guarani", 0, "7100"},
	{"QAR", "Qatari Riyal", 2, "3.64"},
	{"RON", "Romanian Leu", 2, "4.37"},
	{"RSD", "Serbian Dinar", 2, "100.5"},
	{"RUB", "Russian Ruble", 2, "80"},
	{"RWF", "Rwandan Franc", 0, "1450"},
	{"SAR", "Saudi Riyal", 2, "3.75"},
	{"SBD", "Solomon Islands Dollar", 2, "8.3"},
	{"SCR", "Seychellois Rupee", 2, "14.5"},
	{"SDG", "Sudanese Pound", 2, "600"},
	{"SEK", "Swedish Krona", 2, "9.5"},
	{"SGD", "Singapore Dollar", 2, "1.30"},
	{"SHP", "Saint Helena Pound", 2, "0.76"},
	{"SLE", "Sierra Leonean Leone", 2, "23"},
	{"SOS", "Somali Shilling", 2, "571"},
	{"SRD", "Surinamese Dollar", 2, "38"},
	{"SSP", "South Sudanese Pound", 2, "4500"},
	{"STN", "Sao Tome and Principe Dobra", 2, "21.1"},
	{"SVC", "Salvadoran Colon", 2, "8.75"},
	{"SZL", "Eswatini Lilangeni", 2, "17.3"},
	{"THB", "Thai Baht", 2, "32.5"},
	{"TJS", "Tajikistani Somoni", 2, "9.3"},
	{"TMT", "Turkmenistani Manat", 2, "3.5"},
	{"TND", "Tunisian Dinar", 3, "2.95"},
	{"TOP", "Tongan Paanga", 2, "2.35"},
	{"TRY", "Turkish Lira", 2, "42"},
	{"TTD", "Trinidad and Tobago Dollar", 2, "6.78"},
	{"TWD", "New Taiwan Dollar", 2, "31"},
	{"TZS", "Tanzanian Shilling", 2, "2450"},
	{"UAH", "Ukrainian Hryvnia", 2, "41.5"},
	{"UGX", "Ugandan Shilling", 0, "3500"},
	{"USD", "United States Dollar", 2, "1"},
	{"UYU", "Uruguayan Peso", 2, "40"},
	{"UZS", "Uzbekistani Som", 2, "12000"},
	{"VES", "Venezuelan Bolivar", 2, "250"},
	{"VND", "Vietnamese Dong", 0, "26300"},
	{"VUV", "Vanuatu Vatu", 0, "120"},
	{"WST", "Samoan Tala", 2, "2.75"},
	{"XAF", "Central African CFA Franc", 0, "565"},
	{"XCD", "East Caribbean Dollar", 2, "2.7"},
	{"XCG", "Caribbean Guilder", 2, "1.79"},
	{"XOF", "West African CFA Franc", 0, "565"},
	{"XPF", "CFP Franc", 0, "103"},
	{"YER", "Yemeni Rial", 2, "250"},
	{"ZAR", "South African Rand", 2, "17.3"},
	{"ZMW", "Zambian Kwacha", 2, "23"},
	{"ZWG", "Zimbabwe Gold", 2, "26.5"},
}

// currencies is built once: parse rates, reject duplicates, ready for Convert.
var currencies = func() map[string]currency {
	byCode := make(map[string]currency, len(currencyTable))
	for _, c := range currencyTable {
		if _, dup := byCode[c.Code]; dup {
			panic("duplicate currency " + c.Code)
		}
		r, ok := new(big.Rat).SetString(c.PerUSD)
		if !ok || r.Sign() <= 0 {
			panic("invalid exchange rate for " + c.Code)
		}
		if c.MinorUnits < 0 || c.MinorUnits > 3 {
			panic("invalid minor units for " + c.Code)
		}
		byCode[c.Code] = currency{Currency: c, rate: r}
	}
	return byCode
}()

// LookupCurrency returns the currency for an ISO 4217 code.
func LookupCurrency(code string) (Currency, bool) {
	c, ok := currencies[code]
	return c.Currency, ok
}

func lookupCurrency(code string) (currency, bool) {
	c, ok := currencies[code]
	return c, ok
}

// Currencies lists every supported currency (table order is already sorted by code).
func Currencies() []Currency {
	out := make([]Currency, len(currencyTable))
	copy(out, currencyTable)
	return out
}
