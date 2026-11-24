package domain

import (
	"fmt"
	"math/big"
	"strings"
)

// rateScale is how many decimal places the stored exchange rate keeps.
const rateScale = 12

// pow10Rat[n] is 10^n. MinorUnits is always 0..3, so these cover every currency.
var pow10Rat = [4]*big.Rat{
	big.NewRat(1, 1),
	big.NewRat(10, 1),
	big.NewRat(100, 1),
	big.NewRat(1000, 1),
}

// Conversion is the result of moving an amount from one currency into another.
// Rate is major units: 1 FromCurrency = Rate ToCurrency (rounded for display/audit).
type Conversion struct {
	FromCurrency string `json:"fromCurrency"`
	ToCurrency   string `json:"toCurrency"`
	FromAmount   int64  `json:"fromAmount"`
	ToAmount     int64  `json:"toAmount"`
	Rate         string `json:"exchangeRate"`
}

// Convert turns an amount in minor units of one currency into minor units of another.
// Math is exact (big.Rat) and rounded down, so conversion never creates money.
// Same-currency transfers keep rate "1" and the same amount.
func Convert(fromCode, toCode string, amount int64) (Conversion, error) {
	from, fromOK := lookupCurrency(fromCode)
	to, toOK := lookupCurrency(toCode)
	if !fromOK || !toOK {
		unknown := fromCode
		if fromOK {
			unknown = toCode
		}
		return Conversion{}, ErrCurrencyNotSupported.WithMessage(
			"Currency " + unknown + " is not supported, so this payment cannot be converted. Use GET /currencies to see supported currencies")
	}
	if fromCode == toCode {
		return Conversion{fromCode, toCode, amount, amount, "1"}, nil
	}

	// 1 from = (perUSD(to) / perUSD(from)) to, in major units.
	cross := new(big.Rat).Quo(to.rate, from.rate)

	// amount_minor_from → major_from → major_to → minor_to, then floor.
	value := new(big.Rat).SetInt64(amount)
	value.Mul(value, cross)
	value.Mul(value, pow10Rat[to.MinorUnits])
	value.Quo(value, pow10Rat[from.MinorUnits])
	toAmount := new(big.Int).Quo(value.Num(), value.Denom())

	switch {
	case toAmount.Sign() <= 0:
		return Conversion{}, ErrAmountTooSmall.WithMessage(fmt.Sprintf(
			"This amount is too small to convert from %s to %s. It would be worth less than the smallest %s unit",
			from.Code, to.Code, to.Code))
	case !toAmount.IsInt64():
		return Conversion{}, ErrAmountOverflow.WithMessage(
			"The converted amount is too large to process")
	}
	return Conversion{from.Code, to.Code, amount, toAmount.Int64(), trimRate(cross.FloatString(rateScale))}, nil
}

func trimRate(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	return strings.TrimRight(strings.TrimRight(s, "0"), ".")
}
